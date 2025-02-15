package docks

import (
	"testing"
	"time"

	"github.com/safing/spn/terminal"
)

func TestLatencyOp(t *testing.T) {
	var (
		latTestDelay            = 10 * time.Millisecond
		latTestQueueSize uint16 = 10
	)

	// Create test terminal pair.
	a, b, err := terminal.NewSimpleTestTerminalPair(
		latTestDelay,
		&terminal.TerminalOpts{
			QueueSize: latTestQueueSize,
		},
	)
	if err != nil {
		t.Fatalf("failed to create test terminal pair: %s", err)
	}

	// Grant permission for op on remote terminal and start op.
	b.GrantPermission(terminal.IsCraneController)
	op, tErr := NewLatencyTestOp(a)
	if tErr != nil {
		t.Fatalf("failed to start op: %s", err)
	}

	// Wait for result and check error.
	tErr = <-op.Result()
	if tErr.IsError() {
		t.Fatalf("op failed: %s", tErr)
	}
	t.Logf("measured latency: %f ms", float64(op.testResult)/float64(time.Millisecond))

	// Calculate expected latency.
	expectedLatency := float64(latTestDelay * 2)
	t.Logf("expected latency: %f ms", expectedLatency/float64(time.Millisecond))

	// Check if measured latency is within parameters.
	if float64(op.testResult) > expectedLatency*1.2 {
		t.Fatal("measured latency too high")
	}
	if float64(op.testResult) < expectedLatency*0.9 {
		t.Fatal("measured latency too low")
	}
}
