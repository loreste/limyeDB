package mmap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGraphMmap(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "limyedb-graph-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	path := filepath.Join(tmpDir, "graph.mmap")
	
	// Create with M=16
	m := 16
	graph, err := NewGraphMmap(path, m)
	if err != nil {
		t.Fatalf("Failed to create GraphMmap: %v", err)
	}

	// Calculate expected stride
	// Header(64) + Stride per node: 4 (level) + 4*(MaxLevel+1) + 4*(2M) + 4*(MaxLevel*M)
	// MaxLevel = 16
	// 4 + 68 + 128 + 1024 = 1224 bytes per node
	expectedStride := 1224
	if graph.NodeStride != expectedStride {
		t.Errorf("Expected stride %d, got %d", expectedStride, graph.NodeStride)
	}

	// Add node 0, level 2
	err = graph.AddNode(0, 2)
	if err != nil {
		t.Fatalf("Failed to add node 0: %v", err)
	}

	// Set connections for node 0, layer 0
	conns0 := []uint32{10, 20, 30}
	if err := graph.SetConnections(0, 0, conns0); err != nil {
		t.Fatalf("Failed to set connections for layer 0: %v", err)
	}

	// Set connections for node 0, layer 1
	conns1 := []uint32{40, 50}
	if err := graph.SetConnections(0, 1, conns1); err != nil {
		t.Fatalf("Failed to set connections for layer 1: %v", err)
	}

	// Verify connections layer 0
	retrieved0 := graph.GetConnections(0, 0)
	if len(retrieved0) != len(conns0) {
		t.Errorf("Layer 0: expected %d connections, got %d", len(conns0), len(retrieved0))
	}
	for i, c := range retrieved0 {
		if c != conns0[i] {
			t.Errorf("Layer 0: expected connection %d, got %d", conns0[i], c)
		}
	}

	// Verify connections layer 1
	retrieved1 := graph.GetConnections(0, 1)
	if len(retrieved1) != len(conns1) {
		t.Errorf("Layer 1: expected %d connections, got %d", len(conns1), len(retrieved1))
	}
	for i, c := range retrieved1 {
		if c != conns1[i] {
			t.Errorf("Layer 1: expected connection %d, got %d", conns1[i], c)
		}
	}

	// Verify layer > Level returns nil
	retrieved3 := graph.GetConnections(0, 3)
	if len(retrieved3) != 0 {
		t.Errorf("Layer 3: expected 0 connections, got %d", len(retrieved3))
	}

	// Test persistence
	graph.Sync()
	graph.Close()

	// Reopen
	graph2, err := NewGraphMmap(path, m)
	if err != nil {
		t.Fatalf("Failed to reopen GraphMmap: %v", err)
	}
	defer graph2.Close()

	if graph2.NumNodes != 1 {
		t.Errorf("Expected 1 node, got %d", graph2.NumNodes)
	}

	reRetrieved1 := graph2.GetConnections(0, 1)
	if len(reRetrieved1) != len(conns1) {
		t.Errorf("Reopened Layer 1: expected %d connections, got %d", len(conns1), len(reRetrieved1))
	}
	for i, c := range reRetrieved1 {
		if c != conns1[i] {
			t.Errorf("Reopened Layer 1: expected connection %d, got %d", conns1[i], c)
		}
	}
}
