package cluster_test

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/limyedb/limyedb/clients/go/limyedb"
)

func waitForServer(url string, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url+"/health", nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return true
		}
		if ctx.Err() != nil {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func startNode(t *testing.T, id, restPort, raftPort, dataDir string, bootstrap bool, joinAddr string) *exec.Cmd {
	args := []string{
		"--rest", restPort,
		"--grpc", ":0", // disable static grpc collision
		"--data", dataDir,
		"--raft-node-id", id,
		"--raft-bind", "127.0.0.1" + raftPort,
	}

	if bootstrap {
		args = append(args, "--raft-bootstrap=true")
	}
	if joinAddr != "" {
		args = append(args, "--raft-join", joinAddr)
	}

	cmd := exec.Command("/tmp/limyedb_raft_test_bin", args...)
	// Decouple stdout/stderr to prevent go test I/O wait timeout panics on Linux
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start node %s: %v", id, err)
	}

	// Asynchronously reap the zombie process safely avoiding locking the primary test runner loop
	go func() {
		_ = cmd.Wait()
	}()

	if !waitForServer("http://localhost"+restPort, 15*time.Second) {
		cmd.Process.Kill()
		t.Fatalf("Node %s failed to start HTTP server within timeout", id)
	}

	return cmd
}

func TestRaftClusterIntegration(t *testing.T) {
	// Pre-compile the binary to avert orphaned processes from 'go run'
	buildCmd := exec.Command("go", "build", "-o", "/tmp/limyedb_raft_test_bin", "../../cmd/limyedb/main.go")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to pre-compile test binary: %v", err)
	}
	defer os.Remove("/tmp/limyedb_raft_test_bin")

	// Clean up any remaining previous state
	_ = os.RemoveAll("/tmp/limyedb_raft1")
	_ = os.RemoveAll("/tmp/limyedb_raft2")
	_ = os.RemoveAll("/tmp/limyedb_raft3")
	defer func() {
		_ = os.RemoveAll("/tmp/limyedb_raft1")
		_ = os.RemoveAll("/tmp/limyedb_raft2")
		_ = os.RemoveAll("/tmp/limyedb_raft3")
	}()

	// Node 1 (Bootstrap = True)
	cmd1 := startNode(t, "node1", ":8281", ":7201", "/tmp/limyedb_raft1", true, "")
	defer cmd1.Process.Kill()

	time.Sleep(2 * time.Second) // Let Node 1 elect itself

	// Node 2 (Join = Node 1)
	cmd2 := startNode(t, "node2", ":8282", ":7202", "/tmp/limyedb_raft2", false, "http://localhost:8281")
	defer cmd2.Process.Kill()

	// Node 3 (Join = Node 1)
	cmd3 := startNode(t, "node3", ":8283", ":7203", "/tmp/limyedb_raft3", false, "http://localhost:8281")
	defer cmd3.Process.Kill()

	time.Sleep(5 * time.Second) // Allow Raft replication log propagation & heartbeat alignment

	// Execute Writes strictly against the Leader (Node 1)
	client1 := limyedb.NewClient("http://localhost:8281")
	err := client1.CreateCollection(limyedb.CollectionConfig{
		Name:      "raft_test",
		Dimension: 4,
	})
	if err != nil {
		t.Fatalf("Leader failed to route CreateCollection consensus: %v", err)
	}

	points := []limyedb.Point{
		{ID: "point_a", Vector: []float32{1.0, 0.0, 0.0, 0.0}, Payload: map[string]interface{}{"val": 1}},
		{ID: "point_b", Vector: []float32{0.0, 1.0, 0.0, 0.0}, Payload: map[string]interface{}{"val": 2}},
	}
	if err := client1.Upsert("raft_test", points); err != nil {
		t.Fatalf("Leader failed to route Upsert consensus: %v", err)
	}

	time.Sleep(3 * time.Second) // Await FSM commit applied across nodes

	// Search against a FOLLOWER (Node 2)
	client2 := limyedb.NewClient("http://localhost:8282")
	results, err := client2.Search("raft_test", []float32{0.9, 0.0, 0.0, 0.0}, 1)
	if err != nil {
		t.Fatalf("Follower Node 2 failed to execute localized search sequence: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("Follower Node 2 indices are empty! Replicated State Machine Failed!")
	}
	if results[0].ID != "point_a" {
		t.Errorf("Expected 'point_a' on Node 2, got %s", results[0].ID)
	}

	// Search against another FOLLOWER (Node 3)
	client3 := limyedb.NewClient("http://localhost:8283")
	results3, err := client3.Search("raft_test", []float32{0.0, 0.9, 0.0, 0.0}, 1)
	if err != nil {
		t.Fatalf("Follower Node 3 failed to execute localized search sequence: %v", err)
	}
	if len(results3) == 0 {
		t.Fatalf("Follower Node 3 indices are empty! Replicated State Machine Failed!")
	}
	if results3[0].ID != "point_b" {
		t.Errorf("Expected 'point_b' on Node 3, got %s", results3[0].ID)
	}
}
