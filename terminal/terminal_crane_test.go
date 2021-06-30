package terminal

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/safing/portbase/container"
)

func init() {
	addPaddingTo = 0
}

func TestCraneTerminal(t *testing.T) {
	testQueueSize := defaultQueueSize
	countToQueueSize := uint64(testQueueSize)

	var term1 *CraneTerminal
	term1Submit := func(c *container.Container) {
		// Fast track nil containers.
		if c == nil {
			term1.DuplexFlowQueue.Deliver(c)
			return
		}

		// Log message.
		// t.Logf("2>1: %v\n", c.CompileData())

		// Strip terminal ID, as we are not multiplexing in this test.
		_, err := c.GetNextN32()
		if err != nil {
			t.Errorf("2>1: failed to strip Terminal ID: %s", err)
		}

		// Deliver to other terminal.
		dErr := term1.DuplexFlowQueue.Deliver(c)
		if dErr != ErrNil {
			t.Errorf("2>1: failed to deliver to terminal: %s", dErr)
			term1.End("delivery failed", ErrInternalError)
		}
	}

	var term2 *CraneTerminal
	term2Submit := func(c *container.Container) {
		// Fast track nil containers.
		if c == nil {
			term2.DuplexFlowQueue.Deliver(c)
			return
		}

		// Log message.
		// t.Logf("1>2: %v\n", c.CompileData())

		// Strip terminal ID, as we are not multiplexing in this test.
		_, err := c.GetNextN32()
		if err != nil {
			t.Errorf("1>2: failed to strip Terminal ID: %s", err)
		}

		// Deliver to other terminal.
		dErr := term2.DuplexFlowQueue.Deliver(c)
		if dErr != ErrNil {
			t.Errorf("1>2: failed to deliver to terminal: %s", dErr)
			term2.End("delivery failed", ErrInternalError)
		}
	}

	term1 = NewCraneTerminal(module.Ctx, "c1", 127, nil, term2Submit)
	atomic.StoreUint32(term1.nextOpID, 0)
	term2 = NewCraneTerminal(module.Ctx, "c2", 127, nil, term1Submit)
	atomic.StoreUint32(term2.nextOpID, 1)

	// Start testing with counters.

	testTerminalWithCounters(t, term1, term2, &testWithCounterOpts{
		testName:        "oneway-flushing-waiting",
		oneWay:          true,
		flush:           true,
		countTo:         countToQueueSize * 2,
		waitBetweenMsgs: sendThresholdMaxWait * 2,
	})

	testTerminalWithCounters(t, term1, term2, &testWithCounterOpts{
		testName:        "oneway-waiting",
		oneWay:          true,
		countTo:         10,
		waitBetweenMsgs: sendThresholdMaxWait * 2,
	})

	testTerminalWithCounters(t, term1, term2, &testWithCounterOpts{
		testName:        "oneway-flushing",
		oneWay:          true,
		flush:           true,
		countTo:         countToQueueSize * 2,
		waitBetweenMsgs: time.Millisecond,
	})

	testTerminalWithCounters(t, term1, term2, &testWithCounterOpts{
		testName:        "oneway",
		oneWay:          true,
		countTo:         countToQueueSize * 2,
		waitBetweenMsgs: time.Millisecond,
	})

	testTerminalWithCounters(t, term1, term2, &testWithCounterOpts{
		testName:        "twoway-flushing-waiting",
		flush:           true,
		countTo:         defaultQueueSize * 2,
		waitBetweenMsgs: sendThresholdMaxWait * 2,
	})

	testTerminalWithCounters(t, term1, term2, &testWithCounterOpts{
		testName:        "twoway-waiting",
		flush:           true,
		countTo:         10,
		waitBetweenMsgs: sendThresholdMaxWait * 2,
	})

	testTerminalWithCounters(t, term1, term2, &testWithCounterOpts{
		testName:        "twoway-flushing",
		flush:           true,
		countTo:         countToQueueSize * 2,
		waitBetweenMsgs: time.Millisecond,
	})

	testTerminalWithCounters(t, term1, term2, &testWithCounterOpts{
		testName:        "twoway",
		countTo:         countToQueueSize * 2,
		waitBetweenMsgs: time.Millisecond,
	})

	testTerminalWithCounters(t, term1, term2, &testWithCounterOpts{
		testName:        "stresstest",
		countTo:         1000000,
		waitBetweenMsgs: time.Microsecond,
	})

}

type testWithCounterOpts struct {
	testName        string
	oneWay          bool
	flush           bool
	countTo         uint64
	waitBetweenMsgs time.Duration
}

func testTerminalWithCounters(t *testing.T, term1, term2 *CraneTerminal, opts *testWithCounterOpts) {
	var counter1, counter2 *CounterOp

	t.Logf("starting terminal counter test %s", opts.testName)
	defer t.Logf("stopping terminal counter test %s", opts.testName)

	// Start counters.
	counter1 = runTerminalCounter(t, term1, opts)
	if !opts.oneWay {
		counter2 = runTerminalCounter(t, term2, opts)
	}

	// Wait until counters are done.
	counter1.Wait.Wait()
	if !opts.oneWay {
		counter2.Wait.Wait()
	}

	// Log stats.
	t.Logf("%s: counter1: counter=%d countTo=%d", opts.testName, counter1.Counter, counter1.CountTo)
	if !opts.oneWay {
		t.Logf("%s: counter2: counter=%d countTo=%d", opts.testName, counter2.Counter, counter2.CountTo)
	}
	printCTStats(t, opts.testName, "term1", term1)
	printCTStats(t, opts.testName, "term2", term2)
}

func runTerminalCounter(t *testing.T, term *CraneTerminal, opts *testWithCounterOpts) *CounterOp {
	counter, err := NewCounterOp(term, opts.countTo)
	if err != ErrNil {
		t.Fatalf("%s: %s: failed to start counter op: %s", opts.testName, term.parentID, err)
		return nil
	}

	go func() {
		var round uint64
		for {
			// Send counter msg.
			err = counter.SendCounter()
			switch err {
			case ErrNil:
				// All good.
			case ErrOpEnded:
				return // Done!
			default:
				// Something went wrong.
				t.Errorf("%s: %s: failed to send counter: %s", opts.testName, term.parentID, err)
				return
			}

			if opts.flush {
				// Force sending message.
				term.Flush()
			}

			if opts.waitBetweenMsgs > 0 {
				// Wait shortly.
				// In order for the test to succeed, this should be roughly the same as
				// the sendThresholdMaxWait.
				time.Sleep(opts.waitBetweenMsgs)
			}

			// Endless loop check
			round++
			if round > counter.CountTo*2 {
				t.Errorf("%s: %s: looping more than it should", opts.testName, term.parentID)
				return
			}

			// Log progress
			if round%100000 == 0 {
				t.Logf("%s: %s: sent %d messages", opts.testName, term.parentID, round)
			}
		}
	}()

	return counter
}

func printCTStats(t *testing.T, testName, name string, term *CraneTerminal) {
	t.Logf(
		"%s: %s: sq=%d rq=%d sends=%d reps=%d opq=%d",
		testName,
		name,
		len(term.DuplexFlowQueue.sendQueue),
		len(term.DuplexFlowQueue.recvQueue),
		atomic.LoadInt32(term.DuplexFlowQueue.sendSpace),
		atomic.LoadInt32(term.DuplexFlowQueue.reportedSpace),
		len(term.opMsgQueue),
	)
}
