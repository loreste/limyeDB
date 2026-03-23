package limyedb_test

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

func setupServer(t *testing.T) (*exec.Cmd, string) {
	// Start the main LimyeDB binary for testing from the root directory
	cmd := exec.Command("go", "-C", "../../..", "run", "cmd/limyedb/main.go", "--rest", ":8181", "--data", "/tmp/limyedb_gotest")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start limyedb: %v", err)
	}
	
	host := "http://localhost:8181"
	if !waitForServer(host, 10*time.Second) {
		cmd.Process.Kill()
		t.Fatalf("Server failed to respond within timeout")
	}

	return cmd, host
}

func TestClientIntegration(t *testing.T) {
	cmd, host := setupServer(t)
	defer func() {
		cmd.Process.Kill()
		os.RemoveAll("/tmp/limyedb_gotest")
	}()

	client := limyedb.NewClient(host)

	t.Run("Create and Delete Collection", func(t *testing.T) {
		_ = client.DeleteCollection("go_test_coll")

		err := client.CreateCollection(limyedb.CollectionConfig{
			Name:      "go_test_coll",
			Dimension: 3,
			Metric:    "euclidean",
		})
		if err != nil {
			t.Fatalf("Failed to create collection: %v", err)
		}
	})

	t.Run("Upsert and Search", func(t *testing.T) {
		points := []limyedb.Point{
			{ID: "1", Vector: []float32{1.0, 0.0, 0.0}, Payload: map[string]interface{}{"color": "red"}},
			{ID: "2", Vector: []float32{0.0, 1.0, 0.0}, Payload: map[string]interface{}{"color": "green"}},
			{ID: "3", Vector: []float32{0.0, 0.0, 1.0}, Payload: map[string]interface{}{"color": "blue"}},
		}

		if err := client.Upsert("go_test_coll", points); err != nil {
			t.Fatalf("Failed to upsert points: %v", err)
		}

		matches, err := client.Search("go_test_coll", []float32{1.0, 0.1, 0.0}, 2)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(matches) != 2 {
			t.Errorf("Expected 2 matches, got %d", len(matches))
		}

		if matches[0].ID != "1" {
			t.Errorf("Expected top match ID '1', got %s", matches[0].ID)
		}
	})

	t.Run("Discover", func(t *testing.T) {
		_ = client.DeleteCollection("go_disc_coll")
		client.CreateCollection(limyedb.CollectionConfig{
			Name:      "go_disc_coll",
			Dimension: 2,
		})

		points := []limyedb.Point{
			{ID: "a", Vector: []float32{1.0, 1.0}},
			{ID: "b", Vector: []float32{-1.0, -1.0}},
			{ID: "c", Vector: []float32{1.0, -1.0}},
		}
		client.Upsert("go_disc_coll", points)

		params := limyedb.DiscoverParams{
			Context: &limyedb.ContextPair{
				Positive: []limyedb.ContextExample{{ID: "a"}},
				Negative: []limyedb.ContextExample{{ID: "b"}},
			},
			Limit: 2,
		}

		matches, err := client.Discover("go_disc_coll", params)
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}

		if len(matches) == 0 {
			t.Fatalf("Discover returned 0 matches")
		}

		if matches[0].ID != "a" {
			t.Errorf("Expected top discover match ID 'a', got %s", matches[0].ID)
		}
	})
}
