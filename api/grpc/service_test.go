package grpc

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	pb "github.com/limyedb/limyedb/api/grpc/proto"
	"github.com/limyedb/limyedb/pkg/collection"
	"github.com/limyedb/limyedb/pkg/storage/snapshot"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// testSetup creates a fresh collection manager and service for testing
func testSetup(t *testing.T) (*LimyeDBService, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	collDir := filepath.Join(tmpDir, "collections")
	snapDir := filepath.Join(tmpDir, "snapshots")

	mgr, err := collection.NewManager(&collection.ManagerConfig{
		DataDir:        collDir,
		MaxCollections: 100,
	})
	if err != nil {
		t.Fatalf("Failed to create collection manager: %v", err)
	}

	snapMgr, err := snapshot.NewManager(&snapshot.Config{
		Dir:         snapDir,
		RetainCount: 5,
	})
	if err != nil {
		t.Fatalf("Failed to create snapshot manager: %v", err)
	}

	svc := NewLimyeDBService(mgr, snapMgr)

	cleanup := func() {
		_ = mgr.Close()
		_ = os.RemoveAll(tmpDir)
	}

	return svc, cleanup
}

// testSetupWithoutSnapshots creates a service without snapshot manager
func testSetupWithoutSnapshots(t *testing.T) (*LimyeDBService, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	collDir := filepath.Join(tmpDir, "collections")

	mgr, err := collection.NewManager(&collection.ManagerConfig{
		DataDir:        collDir,
		MaxCollections: 100,
	})
	if err != nil {
		t.Fatalf("Failed to create collection manager: %v", err)
	}

	svc := NewLimyeDBService(mgr, nil)

	cleanup := func() {
		_ = mgr.Close()
		_ = os.RemoveAll(tmpDir)
	}

	return svc, cleanup
}

// ================== Collection Operations Tests ==================

func TestCreateCollection(t *testing.T) {
	t.Parallel()

	t.Run("valid collection", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()
		req := &pb.CreateCollectionRequest{
			Config: &pb.CollectionConfig{
				Name:      "test_collection",
				Dimension: 128,
				Metric:    "cosine",
			},
		}

		resp, err := svc.CreateCollection(ctx, req)
		if err != nil {
			t.Fatalf("CreateCollection failed: %v", err)
		}

		if !resp.Success {
			t.Error("Expected success=true")
		}
		if resp.Info == nil {
			t.Fatal("Expected non-nil info")
		}
		if resp.Info.Name != "test_collection" {
			t.Errorf("Expected name=test_collection, got %s", resp.Info.Name)
		}
		if resp.Info.Dimension != 128 {
			t.Errorf("Expected dimension=128, got %d", resp.Info.Dimension)
		}
	})

	t.Run("with HNSW config", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()
		req := &pb.CreateCollectionRequest{
			Config: &pb.CollectionConfig{
				Name:      "hnsw_collection",
				Dimension: 64,
				Metric:    "euclidean",
				Hnsw: &pb.HNSWConfig{
					M:              32,
					EfConstruction: 400,
					EfSearch:       200,
				},
			},
		}

		resp, err := svc.CreateCollection(ctx, req)
		if err != nil {
			t.Fatalf("CreateCollection failed: %v", err)
		}

		if !resp.Success {
			t.Error("Expected success=true")
		}
	})

	t.Run("duplicate collection", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()
		req := &pb.CreateCollectionRequest{
			Config: &pb.CollectionConfig{
				Name:      "duplicate_test",
				Dimension: 64,
				Metric:    "cosine",
			},
		}

		// First creation should succeed
		_, err := svc.CreateCollection(ctx, req)
		if err != nil {
			t.Fatalf("First CreateCollection failed: %v", err)
		}

		// Second creation should fail
		_, err = svc.CreateCollection(ctx, req)
		if err == nil {
			t.Error("Expected error for duplicate collection")
		}
	})
}

func TestGetCollection(t *testing.T) {
	t.Parallel()

	t.Run("existing collection", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()

		// Create collection first
		_, err := svc.CreateCollection(ctx, &pb.CreateCollectionRequest{
			Config: &pb.CollectionConfig{
				Name:      "get_test",
				Dimension: 64,
				Metric:    "cosine",
			},
		})
		if err != nil {
			t.Fatalf("CreateCollection failed: %v", err)
		}

		// Get collection
		resp, err := svc.GetCollection(ctx, &pb.GetCollectionRequest{Name: "get_test"})
		if err != nil {
			t.Fatalf("GetCollection failed: %v", err)
		}

		if resp.Info == nil {
			t.Fatal("Expected non-nil info")
		}
		if resp.Info.Name != "get_test" {
			t.Errorf("Expected name=get_test, got %s", resp.Info.Name)
		}
	})

	t.Run("non-existent collection", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()
		_, err := svc.GetCollection(ctx, &pb.GetCollectionRequest{Name: "nonexistent"})
		if err == nil {
			t.Error("Expected error for non-existent collection")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("Expected gRPC status error")
		}
		if st.Code() != codes.NotFound {
			t.Errorf("Expected NotFound code, got %v", st.Code())
		}
	})
}

func TestListCollections(t *testing.T) {
	t.Parallel()

	t.Run("empty list", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()
		resp, err := svc.ListCollections(ctx, &pb.ListCollectionsRequest{})
		if err != nil {
			t.Fatalf("ListCollections failed: %v", err)
		}

		if len(resp.Collections) != 0 {
			t.Errorf("Expected 0 collections, got %d", len(resp.Collections))
		}
	})

	t.Run("multiple collections", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()

		// Create multiple collections
		for i := 0; i < 3; i++ {
			_, err := svc.CreateCollection(ctx, &pb.CreateCollectionRequest{
				Config: &pb.CollectionConfig{
					Name:      "list_test_" + string(rune('a'+i)),
					Dimension: 64,
					Metric:    "cosine",
				},
			})
			if err != nil {
				t.Fatalf("CreateCollection failed: %v", err)
			}
		}

		resp, err := svc.ListCollections(ctx, &pb.ListCollectionsRequest{})
		if err != nil {
			t.Fatalf("ListCollections failed: %v", err)
		}

		if len(resp.Collections) != 3 {
			t.Errorf("Expected 3 collections, got %d", len(resp.Collections))
		}
	})
}

func TestDeleteCollection(t *testing.T) {
	t.Parallel()

	t.Run("existing collection", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()

		// Create collection
		_, err := svc.CreateCollection(ctx, &pb.CreateCollectionRequest{
			Config: &pb.CollectionConfig{
				Name:      "delete_test",
				Dimension: 64,
				Metric:    "cosine",
			},
		})
		if err != nil {
			t.Fatalf("CreateCollection failed: %v", err)
		}

		// Delete collection
		resp, err := svc.DeleteCollection(ctx, &pb.DeleteCollectionRequest{Name: "delete_test"})
		if err != nil {
			t.Fatalf("DeleteCollection failed: %v", err)
		}

		if !resp.Success {
			t.Error("Expected success=true")
		}

		// Verify deletion
		_, err = svc.GetCollection(ctx, &pb.GetCollectionRequest{Name: "delete_test"})
		if err == nil {
			t.Error("Expected error for deleted collection")
		}
	})

	t.Run("non-existent collection", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()
		_, err := svc.DeleteCollection(ctx, &pb.DeleteCollectionRequest{Name: "nonexistent"})
		if err == nil {
			t.Error("Expected error for non-existent collection")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("Expected gRPC status error")
		}
		if st.Code() != codes.NotFound {
			t.Errorf("Expected NotFound code, got %v", st.Code())
		}
	})
}

// ================== Point Operations Tests ==================

func TestUpsertPoints(t *testing.T) {
	t.Parallel()

	t.Run("single point", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()

		// Create collection
		_, err := svc.CreateCollection(ctx, &pb.CreateCollectionRequest{
			Config: &pb.CollectionConfig{
				Name:      "upsert_test",
				Dimension: 4,
				Metric:    "cosine",
			},
		})
		if err != nil {
			t.Fatalf("CreateCollection failed: %v", err)
		}

		// Upsert point
		resp, err := svc.UpsertPoints(ctx, &pb.UpsertPointsRequest{
			Collection: "upsert_test",
			Points: []*pb.Point{
				{
					Id:     "point-1",
					Vector: &pb.Vector{Data: []float32{1, 0, 0, 0}},
				},
			},
		})
		if err != nil {
			t.Fatalf("UpsertPoints failed: %v", err)
		}

		if !resp.Success {
			t.Error("Expected success=true")
		}
		if resp.Upserted != 1 {
			t.Errorf("Expected upserted=1, got %d", resp.Upserted)
		}
	})

	t.Run("batch upsert", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()

		// Create collection
		_, err := svc.CreateCollection(ctx, &pb.CreateCollectionRequest{
			Config: &pb.CollectionConfig{
				Name:      "batch_upsert_test",
				Dimension: 4,
				Metric:    "cosine",
			},
		})
		if err != nil {
			t.Fatalf("CreateCollection failed: %v", err)
		}

		// Upsert multiple points
		points := make([]*pb.Point, 10)
		for i := 0; i < 10; i++ {
			points[i] = &pb.Point{
				Id:     "point-" + string(rune('0'+i)),
				Vector: &pb.Vector{Data: []float32{float32(i), 0, 0, 0}},
			}
		}

		resp, err := svc.UpsertPoints(ctx, &pb.UpsertPointsRequest{
			Collection: "batch_upsert_test",
			Points:     points,
		})
		if err != nil {
			t.Fatalf("UpsertPoints failed: %v", err)
		}

		if resp.Upserted != 10 {
			t.Errorf("Expected upserted=10, got %d", resp.Upserted)
		}
	})

	t.Run("collection not found", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()
		_, err := svc.UpsertPoints(ctx, &pb.UpsertPointsRequest{
			Collection: "nonexistent",
			Points: []*pb.Point{
				{
					Id:     "point-1",
					Vector: &pb.Vector{Data: []float32{1, 0, 0, 0}},
				},
			},
		})
		if err == nil {
			t.Error("Expected error for non-existent collection")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("Expected gRPC status error")
		}
		if st.Code() != codes.NotFound {
			t.Errorf("Expected NotFound code, got %v", st.Code())
		}
	})
}

func TestGetPoints(t *testing.T) {
	t.Parallel()

	t.Run("with vector and payload", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()

		// Create collection
		_, err := svc.CreateCollection(ctx, &pb.CreateCollectionRequest{
			Config: &pb.CollectionConfig{
				Name:      "get_points_test",
				Dimension: 4,
				Metric:    "cosine",
			},
		})
		if err != nil {
			t.Fatalf("CreateCollection failed: %v", err)
		}

		// Upsert point with payload
		_, err = svc.UpsertPoints(ctx, &pb.UpsertPointsRequest{
			Collection: "get_points_test",
			Points: []*pb.Point{
				{
					Id:     "point-1",
					Vector: &pb.Vector{Data: []float32{1, 0, 0, 0}},
					Payload: map[string]*pb.Value{
						"name": {Kind: &pb.Value_StringValue{StringValue: "test"}},
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("UpsertPoints failed: %v", err)
		}

		// Get points
		resp, err := svc.GetPoints(ctx, &pb.GetPointsRequest{
			Collection:  "get_points_test",
			Ids:         []string{"point-1"},
			WithVector:  true,
			WithPayload: true,
		})
		if err != nil {
			t.Fatalf("GetPoints failed: %v", err)
		}

		if len(resp.Points) != 1 {
			t.Fatalf("Expected 1 point, got %d", len(resp.Points))
		}

		point := resp.Points[0]
		if point.Id != "point-1" {
			t.Errorf("Expected id=point-1, got %s", point.Id)
		}
		if point.Vector == nil {
			t.Error("Expected vector to be present")
		}
		if point.Payload == nil {
			t.Error("Expected payload to be present")
		}
	})

	t.Run("without vector", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()

		// Create collection
		_, err := svc.CreateCollection(ctx, &pb.CreateCollectionRequest{
			Config: &pb.CollectionConfig{
				Name:      "get_points_no_vec",
				Dimension: 4,
				Metric:    "cosine",
			},
		})
		if err != nil {
			t.Fatalf("CreateCollection failed: %v", err)
		}

		// Upsert point
		_, err = svc.UpsertPoints(ctx, &pb.UpsertPointsRequest{
			Collection: "get_points_no_vec",
			Points: []*pb.Point{
				{
					Id:     "point-1",
					Vector: &pb.Vector{Data: []float32{1, 0, 0, 0}},
				},
			},
		})
		if err != nil {
			t.Fatalf("UpsertPoints failed: %v", err)
		}

		// Get points without vector
		resp, err := svc.GetPoints(ctx, &pb.GetPointsRequest{
			Collection:  "get_points_no_vec",
			Ids:         []string{"point-1"},
			WithVector:  false,
			WithPayload: false,
		})
		if err != nil {
			t.Fatalf("GetPoints failed: %v", err)
		}

		if len(resp.Points) != 1 {
			t.Fatalf("Expected 1 point, got %d", len(resp.Points))
		}

		point := resp.Points[0]
		if point.Vector != nil {
			t.Error("Expected vector to be nil")
		}
	})

	t.Run("collection not found", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()
		_, err := svc.GetPoints(ctx, &pb.GetPointsRequest{
			Collection: "nonexistent",
			Ids:        []string{"point-1"},
		})
		if err == nil {
			t.Error("Expected error for non-existent collection")
		}
	})
}

func TestDeletePoints(t *testing.T) {
	t.Parallel()

	t.Run("existing points", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()

		// Create collection
		_, err := svc.CreateCollection(ctx, &pb.CreateCollectionRequest{
			Config: &pb.CollectionConfig{
				Name:      "delete_points_test",
				Dimension: 4,
				Metric:    "cosine",
			},
		})
		if err != nil {
			t.Fatalf("CreateCollection failed: %v", err)
		}

		// Upsert points
		_, err = svc.UpsertPoints(ctx, &pb.UpsertPointsRequest{
			Collection: "delete_points_test",
			Points: []*pb.Point{
				{Id: "point-1", Vector: &pb.Vector{Data: []float32{1, 0, 0, 0}}},
				{Id: "point-2", Vector: &pb.Vector{Data: []float32{0, 1, 0, 0}}},
			},
		})
		if err != nil {
			t.Fatalf("UpsertPoints failed: %v", err)
		}

		// Delete one point
		resp, err := svc.DeletePoints(ctx, &pb.DeletePointsRequest{
			Collection: "delete_points_test",
			Ids:        []string{"point-1"},
		})
		if err != nil {
			t.Fatalf("DeletePoints failed: %v", err)
		}

		if !resp.Success {
			t.Error("Expected success=true")
		}
		if resp.Deleted != 1 {
			t.Errorf("Expected deleted=1, got %d", resp.Deleted)
		}
	})

	t.Run("non-existent points", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()

		// Create collection
		_, err := svc.CreateCollection(ctx, &pb.CreateCollectionRequest{
			Config: &pb.CollectionConfig{
				Name:      "delete_nonexistent",
				Dimension: 4,
				Metric:    "cosine",
			},
		})
		if err != nil {
			t.Fatalf("CreateCollection failed: %v", err)
		}

		// Try to delete non-existent points
		resp, err := svc.DeletePoints(ctx, &pb.DeletePointsRequest{
			Collection: "delete_nonexistent",
			Ids:        []string{"nonexistent-1", "nonexistent-2"},
		})
		if err != nil {
			t.Fatalf("DeletePoints failed: %v", err)
		}

		if resp.Deleted != 0 {
			t.Errorf("Expected deleted=0, got %d", resp.Deleted)
		}
	})
}

// mockStreamUpsertServer implements the streaming interface for testing
type mockStreamUpsertServer struct {
	requests  []*pb.UpsertPointsRequest
	idx       int
	response  *pb.UpsertPointsResponse
	sendErr   error
	recvError error
}

func (m *mockStreamUpsertServer) Recv() (*pb.UpsertPointsRequest, error) {
	if m.recvError != nil {
		return nil, m.recvError
	}
	if m.idx >= len(m.requests) {
		return nil, io.EOF
	}
	req := m.requests[m.idx]
	m.idx++
	return req, nil
}

func (m *mockStreamUpsertServer) SendAndClose(resp *pb.UpsertPointsResponse) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.response = resp
	return nil
}

func (m *mockStreamUpsertServer) SetHeader(_ metadata.MD) error  { return nil }
func (m *mockStreamUpsertServer) SendHeader(_ metadata.MD) error { return nil }
func (m *mockStreamUpsertServer) SetTrailer(_ metadata.MD)       {}
func (m *mockStreamUpsertServer) Context() context.Context       { return context.Background() }
func (m *mockStreamUpsertServer) SendMsg(_ interface{}) error    { return nil }
func (m *mockStreamUpsertServer) RecvMsg(_ interface{}) error    { return nil }

func TestStreamUpsertPoints(t *testing.T) {
	t.Parallel()

	t.Run("streaming upsert", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()

		// Create collection
		_, err := svc.CreateCollection(ctx, &pb.CreateCollectionRequest{
			Config: &pb.CollectionConfig{
				Name:      "stream_test",
				Dimension: 4,
				Metric:    "cosine",
			},
		})
		if err != nil {
			t.Fatalf("CreateCollection failed: %v", err)
		}

		// Create mock stream
		stream := &mockStreamUpsertServer{
			requests: []*pb.UpsertPointsRequest{
				{
					Collection: "stream_test",
					Points: []*pb.Point{
						{Id: "p1", Vector: &pb.Vector{Data: []float32{1, 0, 0, 0}}},
					},
				},
				{
					Collection: "stream_test",
					Points: []*pb.Point{
						{Id: "p2", Vector: &pb.Vector{Data: []float32{0, 1, 0, 0}}},
						{Id: "p3", Vector: &pb.Vector{Data: []float32{0, 0, 1, 0}}},
					},
				},
			},
		}

		err = svc.StreamUpsertPoints(stream)
		if err != nil {
			t.Fatalf("StreamUpsertPoints failed: %v", err)
		}

		if stream.response == nil {
			t.Fatal("Expected response")
		}
		if stream.response.Upserted != 3 {
			t.Errorf("Expected upserted=3, got %d", stream.response.Upserted)
		}
	})
}

// ================== Search Operations Tests ==================

func TestSearch(t *testing.T) {
	t.Parallel()

	t.Run("basic search", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()

		// Create collection
		_, err := svc.CreateCollection(ctx, &pb.CreateCollectionRequest{
			Config: &pb.CollectionConfig{
				Name:      "search_test",
				Dimension: 4,
				Metric:    "euclidean",
			},
		})
		if err != nil {
			t.Fatalf("CreateCollection failed: %v", err)
		}

		// Upsert points
		_, err = svc.UpsertPoints(ctx, &pb.UpsertPointsRequest{
			Collection: "search_test",
			Points: []*pb.Point{
				{Id: "p1", Vector: &pb.Vector{Data: []float32{1, 0, 0, 0}}},
				{Id: "p2", Vector: &pb.Vector{Data: []float32{0.9, 0.1, 0, 0}}},
				{Id: "p3", Vector: &pb.Vector{Data: []float32{0, 1, 0, 0}}},
				{Id: "p4", Vector: &pb.Vector{Data: []float32{0, 0, 1, 0}}},
			},
		})
		if err != nil {
			t.Fatalf("UpsertPoints failed: %v", err)
		}

		// Search
		resp, err := svc.Search(ctx, &pb.SearchRequest{
			Collection: "search_test",
			Vector:     &pb.Vector{Data: []float32{1, 0, 0, 0}},
			Limit:      2,
		})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(resp.Results) == 0 {
			t.Error("Expected results")
		}
	})

	t.Run("search with filter", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()

		// Create collection
		_, err := svc.CreateCollection(ctx, &pb.CreateCollectionRequest{
			Config: &pb.CollectionConfig{
				Name:      "search_filter_test",
				Dimension: 4,
				Metric:    "euclidean",
			},
		})
		if err != nil {
			t.Fatalf("CreateCollection failed: %v", err)
		}

		// Upsert points with payload
		_, err = svc.UpsertPoints(ctx, &pb.UpsertPointsRequest{
			Collection: "search_filter_test",
			Points: []*pb.Point{
				{
					Id:     "p1",
					Vector: &pb.Vector{Data: []float32{1, 0, 0, 0}},
					Payload: map[string]*pb.Value{
						"category": {Kind: &pb.Value_StringValue{StringValue: "A"}},
					},
				},
				{
					Id:     "p2",
					Vector: &pb.Vector{Data: []float32{0.9, 0.1, 0, 0}},
					Payload: map[string]*pb.Value{
						"category": {Kind: &pb.Value_StringValue{StringValue: "B"}},
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("UpsertPoints failed: %v", err)
		}

		// Search with filter
		resp, err := svc.Search(ctx, &pb.SearchRequest{
			Collection: "search_filter_test",
			Vector:     &pb.Vector{Data: []float32{1, 0, 0, 0}},
			Limit:      10,
			Filter: &pb.Filter{
				Filter: &pb.Filter_Field{
					Field: &pb.FieldCondition{
						Field: "category",
						Condition: &pb.FieldCondition_Match{
							Match: &pb.MatchCondition{
								Value: &pb.Value{Kind: &pb.Value_StringValue{StringValue: "A"}},
							},
						},
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		// Should only return points with category=A
		for _, result := range resp.Results {
			if result.Id == "p2" {
				t.Error("Filter should have excluded p2 (category B)")
			}
		}
	})

	t.Run("collection not found", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()
		_, err := svc.Search(ctx, &pb.SearchRequest{
			Collection: "nonexistent",
			Vector:     &pb.Vector{Data: []float32{1, 0, 0, 0}},
			Limit:      10,
		})
		if err == nil {
			t.Error("Expected error for non-existent collection")
		}
	})
}

func TestSearchBatch(t *testing.T) {
	t.Parallel()

	t.Run("multiple queries", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()

		// Create collection
		_, err := svc.CreateCollection(ctx, &pb.CreateCollectionRequest{
			Config: &pb.CollectionConfig{
				Name:      "batch_search_test",
				Dimension: 4,
				Metric:    "euclidean",
			},
		})
		if err != nil {
			t.Fatalf("CreateCollection failed: %v", err)
		}

		// Upsert points
		_, err = svc.UpsertPoints(ctx, &pb.UpsertPointsRequest{
			Collection: "batch_search_test",
			Points: []*pb.Point{
				{Id: "p1", Vector: &pb.Vector{Data: []float32{1, 0, 0, 0}}},
				{Id: "p2", Vector: &pb.Vector{Data: []float32{0, 1, 0, 0}}},
				{Id: "p3", Vector: &pb.Vector{Data: []float32{0, 0, 1, 0}}},
			},
		})
		if err != nil {
			t.Fatalf("UpsertPoints failed: %v", err)
		}

		// Batch search
		resp, err := svc.SearchBatch(ctx, &pb.SearchBatchRequest{
			Collection: "batch_search_test",
			Vectors: []*pb.Vector{
				{Data: []float32{1, 0, 0, 0}},
				{Data: []float32{0, 1, 0, 0}},
			},
			Limit: 2,
		})
		if err != nil {
			t.Fatalf("SearchBatch failed: %v", err)
		}

		if len(resp.Results) != 2 {
			t.Errorf("Expected 2 result sets, got %d", len(resp.Results))
		}
	})

	t.Run("collection not found", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()
		_, err := svc.SearchBatch(ctx, &pb.SearchBatchRequest{
			Collection: "nonexistent",
			Vectors: []*pb.Vector{
				{Data: []float32{1, 0, 0, 0}},
			},
			Limit: 10,
		})
		if err == nil {
			t.Error("Expected error for non-existent collection")
		}
	})
}

func TestRecommend(t *testing.T) {
	t.Parallel()

	t.Run("with positive IDs", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()

		// Create collection
		_, err := svc.CreateCollection(ctx, &pb.CreateCollectionRequest{
			Config: &pb.CollectionConfig{
				Name:      "recommend_test",
				Dimension: 4,
				Metric:    "euclidean",
			},
		})
		if err != nil {
			t.Fatalf("CreateCollection failed: %v", err)
		}

		// Upsert points
		_, err = svc.UpsertPoints(ctx, &pb.UpsertPointsRequest{
			Collection: "recommend_test",
			Points: []*pb.Point{
				{Id: "p1", Vector: &pb.Vector{Data: []float32{1, 0, 0, 0}}},
				{Id: "p2", Vector: &pb.Vector{Data: []float32{0.9, 0.1, 0, 0}}},
				{Id: "p3", Vector: &pb.Vector{Data: []float32{0, 1, 0, 0}}},
			},
		})
		if err != nil {
			t.Fatalf("UpsertPoints failed: %v", err)
		}

		// Recommend
		resp, err := svc.Recommend(ctx, &pb.RecommendRequest{
			Collection:  "recommend_test",
			PositiveIds: []string{"p1"},
			Limit:       2,
		})
		if err != nil {
			t.Fatalf("Recommend failed: %v", err)
		}

		if len(resp.Results) == 0 {
			t.Error("Expected results")
		}

		// Should not include the query point
		for _, r := range resp.Results {
			if r.Id == "p1" {
				t.Error("Recommend should not include query point")
			}
		}
	})

	t.Run("no positive IDs", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()

		// Create collection
		_, err := svc.CreateCollection(ctx, &pb.CreateCollectionRequest{
			Config: &pb.CollectionConfig{
				Name:      "recommend_no_pos",
				Dimension: 4,
				Metric:    "euclidean",
			},
		})
		if err != nil {
			t.Fatalf("CreateCollection failed: %v", err)
		}

		// Recommend without positive IDs
		_, err = svc.Recommend(ctx, &pb.RecommendRequest{
			Collection:  "recommend_no_pos",
			PositiveIds: []string{},
			Limit:       2,
		})
		if err == nil {
			t.Error("Expected error for missing positive IDs")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("Expected gRPC status error")
		}
		if st.Code() != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument code, got %v", st.Code())
		}
	})
}

func TestDiscover(t *testing.T) {
	t.Parallel()

	t.Run("with target and context", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()

		// Create collection
		_, err := svc.CreateCollection(ctx, &pb.CreateCollectionRequest{
			Config: &pb.CollectionConfig{
				Name:      "discover_test",
				Dimension: 4,
				Metric:    "euclidean",
			},
		})
		if err != nil {
			t.Fatalf("CreateCollection failed: %v", err)
		}

		// Upsert points
		_, err = svc.UpsertPoints(ctx, &pb.UpsertPointsRequest{
			Collection: "discover_test",
			Points: []*pb.Point{
				{Id: "p1", Vector: &pb.Vector{Data: []float32{1, 0, 0, 0}}},
				{Id: "p2", Vector: &pb.Vector{Data: []float32{0.8, 0.2, 0, 0}}},
				{Id: "p3", Vector: &pb.Vector{Data: []float32{0, 1, 0, 0}}},
				{Id: "p4", Vector: &pb.Vector{Data: []float32{0, 0, 1, 0}}},
			},
		})
		if err != nil {
			t.Fatalf("UpsertPoints failed: %v", err)
		}

		// Discover with context
		resp, err := svc.Discover(ctx, &pb.DiscoverRequest{
			Collection: "discover_test",
			Target:     &pb.Vector{Data: []float32{1, 0, 0, 0}},
			Context: []*pb.ContextPair{
				{PositiveId: "p1", NegativeId: "p3"},
			},
			Limit: 2,
		})
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}

		if len(resp.Results) == 0 {
			t.Error("Expected results")
		}
	})

	t.Run("collection not found", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()
		_, err := svc.Discover(ctx, &pb.DiscoverRequest{
			Collection: "nonexistent",
			Target:     &pb.Vector{Data: []float32{1, 0, 0, 0}},
			Limit:      10,
		})
		if err == nil {
			t.Error("Expected error for non-existent collection")
		}
	})
}

// ================== Snapshot Operations Tests ==================

func TestCreateSnapshot(t *testing.T) {
	t.Parallel()

	t.Run("create snapshot", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()

		// Create collection
		_, err := svc.CreateCollection(ctx, &pb.CreateCollectionRequest{
			Config: &pb.CollectionConfig{
				Name:      "snapshot_test",
				Dimension: 4,
				Metric:    "cosine",
			},
		})
		if err != nil {
			t.Fatalf("CreateCollection failed: %v", err)
		}

		// Upsert points
		_, err = svc.UpsertPoints(ctx, &pb.UpsertPointsRequest{
			Collection: "snapshot_test",
			Points: []*pb.Point{
				{Id: "p1", Vector: &pb.Vector{Data: []float32{1, 0, 0, 0}}},
			},
		})
		if err != nil {
			t.Fatalf("UpsertPoints failed: %v", err)
		}

		// Create snapshot
		resp, err := svc.CreateSnapshot(ctx, &pb.CreateSnapshotRequest{})
		if err != nil {
			t.Fatalf("CreateSnapshot failed: %v", err)
		}

		if resp.Id == "" {
			t.Error("Expected snapshot ID")
		}
		if resp.Timestamp == 0 {
			t.Error("Expected timestamp")
		}
	})

	t.Run("snapshots not configured", func(t *testing.T) {
		svc, cleanup := testSetupWithoutSnapshots(t)
		defer cleanup()

		ctx := context.Background()
		_, err := svc.CreateSnapshot(ctx, &pb.CreateSnapshotRequest{})
		if err == nil {
			t.Error("Expected error when snapshots not configured")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("Expected gRPC status error")
		}
		if st.Code() != codes.Unavailable {
			t.Errorf("Expected Unavailable code, got %v", st.Code())
		}
	})
}

func TestListSnapshots(t *testing.T) {
	t.Parallel()

	t.Run("list snapshots", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()

		// Create collection and snapshot
		_, err := svc.CreateCollection(ctx, &pb.CreateCollectionRequest{
			Config: &pb.CollectionConfig{
				Name:      "list_snap_test",
				Dimension: 4,
				Metric:    "cosine",
			},
		})
		if err != nil {
			t.Fatalf("CreateCollection failed: %v", err)
		}

		_, err = svc.CreateSnapshot(ctx, &pb.CreateSnapshotRequest{})
		if err != nil {
			t.Fatalf("CreateSnapshot failed: %v", err)
		}

		// List snapshots
		resp, err := svc.ListSnapshots(ctx, &pb.ListSnapshotsRequest{})
		if err != nil {
			t.Fatalf("ListSnapshots failed: %v", err)
		}

		if len(resp.Snapshots) < 1 {
			t.Error("Expected at least 1 snapshot")
		}
	})

	t.Run("snapshots not configured", func(t *testing.T) {
		svc, cleanup := testSetupWithoutSnapshots(t)
		defer cleanup()

		ctx := context.Background()
		_, err := svc.ListSnapshots(ctx, &pb.ListSnapshotsRequest{})
		if err == nil {
			t.Error("Expected error when snapshots not configured")
		}
	})
}

func TestRestoreSnapshot(t *testing.T) {
	t.Parallel()

	t.Run("snapshots not configured", func(t *testing.T) {
		svc, cleanup := testSetupWithoutSnapshots(t)
		defer cleanup()

		ctx := context.Background()
		_, err := svc.RestoreSnapshot(ctx, &pb.RestoreSnapshotRequest{Id: "snap_123"})
		if err == nil {
			t.Error("Expected error when snapshots not configured")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("Expected gRPC status error")
		}
		if st.Code() != codes.Unavailable {
			t.Errorf("Expected Unavailable code, got %v", st.Code())
		}
	})
}

// ================== Health Tests ==================

func TestHealth(t *testing.T) {
	t.Parallel()

	t.Run("health check", func(t *testing.T) {
		svc, cleanup := testSetup(t)
		defer cleanup()

		ctx := context.Background()
		resp, err := svc.Health(ctx, &pb.HealthRequest{})
		if err != nil {
			t.Fatalf("Health failed: %v", err)
		}

		if resp.Status != "healthy" {
			t.Errorf("Expected status=healthy, got %s", resp.Status)
		}
		if resp.Version == "" {
			t.Error("Expected non-empty version")
		}
	})
}

// ================== Helper Function Tests ==================

func TestSafeIntToInt32(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    int
		expected int32
	}{
		{"zero", 0, 0},
		{"positive", 100, 100},
		{"negative", -100, -100},
		{"max int32", 2147483647, 2147483647},
		{"min int32", -2147483648, -2147483648},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := safeIntToInt32(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %d, got %d", tc.expected, result)
			}
		})
	}
}

func TestPayloadConversion(t *testing.T) {
	t.Parallel()

	t.Run("nil payload", func(t *testing.T) {
		result := payloadToProto(nil)
		if result != nil {
			t.Error("Expected nil result for nil payload")
		}
	})

	t.Run("string value", func(t *testing.T) {
		payload := map[string]interface{}{
			"key": "value",
		}
		result := payloadToProto(payload)
		if result == nil {
			t.Fatal("Expected non-nil result")
		}
		if result["key"] == nil {
			t.Fatal("Expected key to exist")
		}
		sv, ok := result["key"].Kind.(*pb.Value_StringValue)
		if !ok {
			t.Fatal("Expected string value")
		}
		if sv.StringValue != "value" {
			t.Errorf("Expected value, got %s", sv.StringValue)
		}
	})

	t.Run("number value", func(t *testing.T) {
		payload := map[string]interface{}{
			"num": 42.5,
		}
		result := payloadToProto(payload)
		if result == nil {
			t.Fatal("Expected non-nil result")
		}
		nv, ok := result["num"].Kind.(*pb.Value_NumberValue)
		if !ok {
			t.Fatal("Expected number value")
		}
		if nv.NumberValue != 42.5 {
			t.Errorf("Expected 42.5, got %f", nv.NumberValue)
		}
	})

	t.Run("bool value", func(t *testing.T) {
		payload := map[string]interface{}{
			"flag": true,
		}
		result := payloadToProto(payload)
		if result == nil {
			t.Fatal("Expected non-nil result")
		}
		bv, ok := result["flag"].Kind.(*pb.Value_BoolValue)
		if !ok {
			t.Fatal("Expected bool value")
		}
		if !bv.BoolValue {
			t.Error("Expected true")
		}
	})
}

func TestProtoPayloadToMap(t *testing.T) {
	t.Parallel()

	t.Run("nil payload", func(t *testing.T) {
		result := protoPayloadToMap(nil)
		if result != nil {
			t.Error("Expected nil result for nil payload")
		}
	})

	t.Run("mixed values", func(t *testing.T) {
		payload := map[string]*pb.Value{
			"str":  {Kind: &pb.Value_StringValue{StringValue: "hello"}},
			"num":  {Kind: &pb.Value_NumberValue{NumberValue: 123}},
			"bool": {Kind: &pb.Value_BoolValue{BoolValue: true}},
		}
		result := protoPayloadToMap(payload)
		if result == nil {
			t.Fatal("Expected non-nil result")
		}
		if result["str"] != "hello" {
			t.Errorf("Expected str=hello, got %v", result["str"])
		}
		if result["num"] != float64(123) {
			t.Errorf("Expected num=123, got %v", result["num"])
		}
		if result["bool"] != true {
			t.Errorf("Expected bool=true, got %v", result["bool"])
		}
	})
}

func TestCheckPermission(t *testing.T) {
	t.Parallel()

	t.Run("no auth context", func(t *testing.T) {
		ctx := context.Background()
		if !checkPermission(ctx, "test", "read") {
			t.Error("Expected true when no auth configured")
		}
	})
}
