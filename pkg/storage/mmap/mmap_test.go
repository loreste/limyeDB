package mmap

import (
	"os"
	"path/filepath"
	"testing"
)

func createTestStorage(t *testing.T, dimension int) (*Storage, string) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "limyedb-mmap-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	cfg := &Config{
		Path:        filepath.Join(tmpDir, "vectors.mmap"),
		InitialSize: 1024 * 1024, // 1MB
		MaxSize:     10 * 1024 * 1024,
		Dimension:   dimension,
	}

	s, err := Open(cfg)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to open storage: %v", err)
	}
	return s, tmpDir
}

func TestOpenClose(t *testing.T) {
	s, tmpDir := createTestStorage(t, 4)
	defer os.RemoveAll(tmpDir)

	if s.Capacity() == 0 {
		t.Error("Expected non-zero capacity")
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestWriteReadVector(t *testing.T) {
	s, tmpDir := createTestStorage(t, 4)
	defer os.RemoveAll(tmpDir)
	defer s.Close()

	vec := []float32{1.0, 2.5, -3.0, 0.0}
	if err := s.WriteVector(0, vec); err != nil {
		t.Fatalf("WriteVector() error = %v", err)
	}

	got, err := s.ReadVector(0)
	if err != nil {
		t.Fatalf("ReadVector() error = %v", err)
	}

	for i := range vec {
		if got[i] != vec[i] {
			t.Errorf("Vector[%d]: got %f, want %f", i, got[i], vec[i])
		}
	}
}

func TestWriteMultipleVectors(t *testing.T) {
	dim := 8
	s, tmpDir := createTestStorage(t, dim)
	defer os.RemoveAll(tmpDir)
	defer s.Close()

	vectorSize := int64(dim * 4)
	vecs := [][]float32{
		{1, 2, 3, 4, 5, 6, 7, 8},
		{9, 10, 11, 12, 13, 14, 15, 16},
		{17, 18, 19, 20, 21, 22, 23, 24},
	}

	for i, vec := range vecs {
		offset := int64(i) * vectorSize
		if err := s.WriteVector(offset, vec); err != nil {
			t.Fatalf("WriteVector(%d) error = %v", i, err)
		}
	}

	// Read them back in reverse order
	for i := len(vecs) - 1; i >= 0; i-- {
		offset := int64(i) * vectorSize
		got, err := s.ReadVector(offset)
		if err != nil {
			t.Fatalf("ReadVector(%d) error = %v", i, err)
		}
		for j := range vecs[i] {
			if got[j] != vecs[i][j] {
				t.Errorf("Vec[%d][%d]: got %f, want %f", i, j, got[j], vecs[i][j])
			}
		}
	}
}

func TestWriteVectorDimensionMismatch(t *testing.T) {
	s, tmpDir := createTestStorage(t, 4)
	defer os.RemoveAll(tmpDir)
	defer s.Close()

	wrong := []float32{1.0, 2.0} // dimension 2, expected 4
	err := s.WriteVector(0, wrong)
	if err == nil {
		t.Error("Expected error for dimension mismatch")
	}
}

func TestReadVectorOutOfBounds(t *testing.T) {
	s, tmpDir := createTestStorage(t, 4)
	defer os.RemoveAll(tmpDir)
	defer s.Close()

	_, err := s.ReadVector(s.Capacity() + 100)
	if err == nil {
		t.Error("Expected error for out-of-bounds read")
	}
}

func TestAllocateAndFree(t *testing.T) {
	s, tmpDir := createTestStorage(t, 4)
	defer os.RemoveAll(tmpDir)
	defer s.Close()

	off1, err := s.Allocate()
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}

	off2, err := s.Allocate()
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}

	if off1 == off2 {
		t.Error("Two allocations returned same offset")
	}

	// Free the first, allocate again -- should reuse
	s.Free(off1)
	off3, err := s.Allocate()
	if err != nil {
		t.Fatalf("Allocate() after Free error = %v", err)
	}
	if off3 != off1 {
		t.Errorf("Expected reuse of offset %d, got %d", off1, off3)
	}
}

func TestSizeAndCapacity(t *testing.T) {
	s, tmpDir := createTestStorage(t, 4)
	defer os.RemoveAll(tmpDir)
	defer s.Close()

	if s.Size() != 0 {
		t.Errorf("Expected initial size 0, got %d", s.Size())
	}

	vec := []float32{1.0, 2.0, 3.0, 4.0}
	s.WriteVector(0, vec)

	if s.Size() == 0 {
		t.Error("Expected non-zero size after write")
	}
}

func TestSync(t *testing.T) {
	s, tmpDir := createTestStorage(t, 4)
	defer os.RemoveAll(tmpDir)
	defer s.Close()

	if err := s.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
}

func TestPersistence(t *testing.T) {
	dim := 4
	tmpDir, err := os.MkdirTemp("", "limyedb-mmap-persist")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	path := filepath.Join(tmpDir, "vectors.mmap")
	cfg := &Config{
		Path:        path,
		InitialSize: 1024 * 1024,
		MaxSize:     10 * 1024 * 1024,
		Dimension:   dim,
	}

	// Write data
	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	vec := []float32{3.14, 2.71, 1.41, 1.73}
	if err := s.WriteVector(0, vec); err != nil {
		t.Fatalf("WriteVector() error = %v", err)
	}
	s.Close()

	// Reopen and read
	s2, err := Open(cfg)
	if err != nil {
		t.Fatalf("Reopen error = %v", err)
	}
	defer s2.Close()

	got, err := s2.ReadVector(0)
	if err != nil {
		t.Fatalf("ReadVector() after reopen error = %v", err)
	}
	for i := range vec {
		if got[i] != vec[i] {
			t.Errorf("After reopen Vector[%d]: got %f, want %f", i, got[i], vec[i])
		}
	}
}

func TestAllocatorCoalesce(t *testing.T) {
	a := NewAllocator(1024)

	// Allocate three blocks of 16 bytes each
	off1, _ := a.Allocate(16)
	off2, _ := a.Allocate(16)
	off3, _ := a.Allocate(16)

	// Free all three -- they should coalesce into one block
	a.Free(off1, 16)
	a.Free(off2, 16)
	a.Free(off3, 16)

	// Should be able to allocate a 48-byte block from the coalesced free space
	off, err := a.Allocate(48)
	if err != nil {
		t.Fatalf("Allocate(48) after coalesce error = %v", err)
	}
	if off != off1 {
		t.Errorf("Expected coalesced block at offset %d, got %d", off1, off)
	}
}

func TestAllocatorUsed(t *testing.T) {
	a := NewAllocator(1024)

	a.Allocate(64)
	a.Allocate(64)

	if a.Used() != 128 {
		t.Errorf("Expected 128 bytes used, got %d", a.Used())
	}

	a.Free(0, 64)
	if a.Used() != 64 {
		t.Errorf("Expected 64 bytes used after free, got %d", a.Used())
	}
}

func TestAllocatorFull(t *testing.T) {
	a := NewAllocator(32)

	_, err := a.Allocate(32)
	if err != nil {
		t.Fatalf("First allocate should succeed: %v", err)
	}

	_, err = a.Allocate(1)
	if err == nil {
		t.Error("Expected error when allocator is full")
	}
}
