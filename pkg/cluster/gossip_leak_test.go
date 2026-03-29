package cluster

import (
	"context"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// mockTransport implements Transport for testing without network I/O.
type mockTransport struct{}

func (m *mockTransport) Start() error { return nil }
func (m *mockTransport) Stop() error  { return nil }
func (m *mockTransport) Send(_ context.Context, _ string, msg *Message) (*Message, error) {
	// Return an ack so callers don't block.
	return &Message{Type: "ack"}, nil
}
func (m *mockTransport) Stream(_ context.Context, _ string) (Stream, error) { return nil, nil }
func (m *mockTransport) OnMessage(_ MessageHandler)                         {}

func TestGossiperNoGoroutineLeak(t *testing.T) {
	defer goleak.VerifyNone(t,
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
		goleak.IgnoreTopFunction("time.Sleep"),
	)

	cfg := &GossipConfig{
		GossipInterval: 50 * time.Millisecond,
		GossipFanout:   2,
		SuspicionMult:  2,
		ProbeInterval:  50 * time.Millisecond,
		ProbeTimeout:   25 * time.Millisecond,
		RetransmitMult: 2,
	}

	g := NewGossiper("node-1", "127.0.0.1:9000", &mockTransport{}, cfg)

	if err := g.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Let the gossip/probe/suspicion loops tick a few times.
	time.Sleep(200 * time.Millisecond)

	members := g.GetMembers()
	if len(members) != 1 {
		t.Errorf("expected 1 member (self), got %d", len(members))
	}

	// Stop should cleanly shut down all three goroutines.
	if err := g.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestGossiperStartStopCycle(t *testing.T) {
	defer goleak.VerifyNone(t,
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
		goleak.IgnoreTopFunction("time.Sleep"),
	)

	// Rapidly create, start, and stop gossipers to stress goroutine cleanup.
	for i := 0; i < 5; i++ {
		cfg := DefaultGossipConfig()
		cfg.GossipInterval = 20 * time.Millisecond
		cfg.ProbeInterval = 20 * time.Millisecond

		g := NewGossiper("node-cycle", "127.0.0.1:9001", &mockTransport{}, cfg)
		if err := g.Start(); err != nil {
			t.Fatalf("iteration %d: Start failed: %v", i, err)
		}
		time.Sleep(50 * time.Millisecond)
		if err := g.Stop(); err != nil {
			t.Fatalf("iteration %d: Stop failed: %v", i, err)
		}
	}
}
