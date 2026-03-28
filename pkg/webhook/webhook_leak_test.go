package webhook

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestManagerNoGoroutineLeak(t *testing.T) {
	defer goleak.VerifyNone(t,
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
		goleak.IgnoreTopFunction("net/http.(*persistConn).writeLoop"),
		goleak.IgnoreTopFunction("net/http.(*persistConn).readLoop"),
	)

	// Set up an HTTP test server to receive webhook events.
	var received atomic.Int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Create a Manager with fast retry settings for the test.
	policy := RetryPolicy{
		MaxRetries:    1,
		InitialDelay:  10 * time.Millisecond,
		MaxDelay:      50 * time.Millisecond,
		BackoffFactor: 1.5,
	}
	mgr := NewManager(2, policy)
	mgr.allowLocalURLs = true

	// Subscribe to point.insert events.
	err := mgr.Subscribe(&Subscription{
		ID:     "sub-1",
		URL:    ts.URL,
		Events: []EventType{EventPointInsert},
		Active: true,
	})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Emit several events.
	for i := 0; i < 5; i++ {
		mgr.EmitPointInsert("test-collection", "point-1", map[string]interface{}{"i": i})
	}

	// Give workers time to deliver.
	deadline := time.After(3 * time.Second)
	for received.Load() < 5 {
		select {
		case <-deadline:
			t.Logf("received %d/5 deliveries before timeout", received.Load())
			goto done
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}
done:

	// Close the manager -- this cancels the context so workers exit.
	mgr.Close()

	// Drain any remaining results after close to unblock workers.
	drainDone := make(chan struct{})
	go func() {
		for {
			select {
			case _, ok := <-mgr.Results():
				if !ok {
					close(drainDone)
					return
				}
			case <-time.After(200 * time.Millisecond):
				// No more results coming, exit the drain loop.
				close(drainDone)
				return
			}
		}
	}()
	<-drainDone

	if received.Load() == 0 {
		t.Error("expected at least one delivery, got none")
	}
}
