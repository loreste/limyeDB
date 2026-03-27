package test

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"testing"
	"time"

	"github.com/limyedb/limyedb/api/rest"
	"github.com/limyedb/limyedb/pkg/collection"
	"github.com/limyedb/limyedb/pkg/config"
)

func TestGoroutineLeaks(t *testing.T) {
	// Let the test environment stabilize
	time.Sleep(100 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	t.Logf("Initial Goroutines: %d", initialGoroutines)

	// Spin up LimyeDB core manager
	managerCfg := collection.DefaultManagerConfig()
	managerCfg.DataDir = t.TempDir()
	cm, err := collection.NewManager(managerCfg)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Spin up REST server
	cfg := config.DefaultConfig()
	cfg.Server.RESTAddress = ":18080"
	server := rest.NewServer(&cfg.Server, cm, nil)

	errCh := make(chan error, 1)

	go func() {
		errCh <- server.Start()
	}()

	// Wait for server to boot
	time.Sleep(200 * time.Millisecond)

	// Simulate 100 concurrent transient clients connecting and disconnecting
	for i := 0; i < 100; i++ {
		go func() {
			client := &http.Client{Timeout: 1 * time.Second}
			client.Get("http://localhost:18080/health")
		}()
	}

	// Wait for clients to finish
	time.Sleep(1 * time.Second)

	// Stop the server
	t.Log("Shutting down the server to trace leak releases...")
	err = server.Stop(context.Background())
	if err != nil {
		t.Logf("Server shutdown with error (expected usually): %v", err)
	}

	// GC and let lingering routines die
	runtime.GC()
	time.Sleep(500 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	t.Logf("Final Goroutines after teardown: %d", finalGoroutines)

	// Allow a tiny margin for Go testing internal routines
	if finalGoroutines > initialGoroutines+5 {
		t.Errorf("Goroutine leak detected! Started with %d, ended with %d", initialGoroutines, finalGoroutines)

		buf := make([]byte, 1<<16)
		n := runtime.Stack(buf, true)
		fmt.Printf("%s", string(buf[:n]))
	}
}
