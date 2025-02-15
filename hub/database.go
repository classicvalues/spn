package hub

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/safing/portbase/database"
	"github.com/safing/portbase/database/iterator"
	"github.com/safing/portbase/database/query"
	"github.com/safing/portbase/database/record"
)

var (
	db = database.NewInterface(&database.Options{
		Local:    true,
		Internal: true,
	})

	getFromNavigator func(mapName, hubID string) *Hub
)

func MakeHubDBKey(mapName, hubID string) string {
	return fmt.Sprintf("cache:spn/hubs/%s/%s", mapName, hubID)
}

func MakeHubMsgDBKey(mapName string, msgType MsgType, hubID string) string {
	return fmt.Sprintf("cache:spn/msgs/%s/%s/%s", mapName, msgType, hubID)
}

// SetNavigatorAccess sets a shortcut function to access hubs from the navigator instead of having go through the database.
// This also reduces the number of object in RAM and better caches parsed attributes.
func SetNavigatorAccess(fn func(mapName, hubID string) *Hub) {
	if getFromNavigator == nil {
		getFromNavigator = fn
	}
}

// GetHub get a Hub from the database - or the navigator, if configured.
func GetHub(mapName string, hubID string) (*Hub, error) {
	if getFromNavigator != nil {
		hub := getFromNavigator(mapName, hubID)
		if hub != nil {
			return hub, nil
		}
	}

	return GetHubByKey(MakeHubDBKey(mapName, hubID))
}

func GetHubByKey(key string) (*Hub, error) {
	r, err := db.Get(key)
	if err != nil {
		return nil, err
	}

	hub, err := EnsureHub(r)
	if err != nil {
		return nil, err
	}

	// Check Formatting
	// This should also be checked on records loaded from disk in order to update validation criteria retroactively.
	if err = hub.Info.validateFormatting(); err != nil {
		return nil, fmt.Errorf("announcement failed format validation: %w", err)
	}
	if err = hub.Status.validateFormatting(); err != nil {
		return nil, fmt.Errorf("status failed format validation: %w", err)
	}

	return hub, nil
}

// EnsureHub makes sure a database record is a Hub.
func EnsureHub(r record.Record) (*Hub, error) {
	// unwrap
	if r.IsWrapped() {
		// only allocate a new struct, if we need it
		new := &Hub{}
		err := record.Unwrap(r, new)
		if err != nil {
			return nil, err
		}
		return checkAndReturn(new), nil
	}

	// or adjust type
	new, ok := r.(*Hub)
	if !ok {
		return nil, fmt.Errorf("record not of type *Hub, but %T", r)
	}

	// ensure status
	return checkAndReturn(new), nil
}

func checkAndReturn(h *Hub) *Hub {
	if h.Status == nil {
		h.Status = &Status{}
	}
	h.Measurements = getSharedMeasurements(h.ID, h.Measurements)
	return h
}

// Save saves to Hub to the correct scope in the database.
func (hub *Hub) Save() error {
	if !hub.KeyIsSet() {
		hub.SetKey(MakeHubDBKey(hub.Map, hub.ID))
	}

	return db.Put(hub)
}

// RemoveHubAndMsgs deletes a Hub and it's saved messages from the database.
func RemoveHubAndMsgs(mapName string, hubID string) (err error) {
	err = db.Delete(MakeHubDBKey(mapName, hubID))
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return fmt.Errorf("failed to delete main hub entry: %w", err)
	}

	err = db.Delete(MakeHubMsgDBKey(mapName, MsgTypeAnnouncement, hubID))
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return fmt.Errorf("failed to delete hub announcement data: %w", err)
	}

	err = db.Delete(MakeHubMsgDBKey(mapName, MsgTypeStatus, hubID))
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return fmt.Errorf("failed to delete hub status data: %w", err)
	}

	return nil
}

// HubMsg stores raw Hub messages.
type HubMsg struct {
	record.Base
	sync.Mutex

	ID   string
	Map  string
	Type MsgType
	Data []byte

	Received int64
}

// SaveHubMsg saves a raw (and signed) message received by another Hub.
func SaveHubMsg(id string, mapName string, msgType MsgType, data []byte) error {
	// create wrapper record
	msg := &HubMsg{
		ID:       id,
		Map:      mapName,
		Type:     msgType,
		Data:     data,
		Received: time.Now().Unix(),
	}
	// set key
	msg.SetKey(MakeHubMsgDBKey(msg.Map, msg.Type, msg.ID))
	// save
	return db.PutNew(msg)
}

func QueryRawGossipMsgs(mapName string, msgType MsgType) (it *iterator.Iterator, err error) {
	it, err = db.Query(query.New(MakeHubMsgDBKey(mapName, msgType, "")))
	return
}

// EnsureHubMsg makes sure a database record is a HubMsg.
func EnsureHubMsg(r record.Record) (*HubMsg, error) {
	// unwrap
	if r.IsWrapped() {
		// only allocate a new struct, if we need it
		new := &HubMsg{}
		err := record.Unwrap(r, new)
		if err != nil {
			return nil, err
		}
		return new, nil
	}

	// or adjust type
	new, ok := r.(*HubMsg)
	if !ok {
		return nil, fmt.Errorf("record not of type *Hub, but %T", r)
	}
	return new, nil
}
