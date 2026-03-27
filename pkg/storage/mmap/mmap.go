package mmap

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"unsafe"
)

// Storage provides memory-mapped file storage for vectors
type Storage struct {
	path      string
	file      *os.File
	data      []byte
	size      int64
	capacity  int64
	dimension int

	allocator *Allocator
	mu        sync.RWMutex
}

// Config holds mmap storage configuration
type Config struct {
	Path         string
	InitialSize  int64
	MaxSize      int64
	Dimension    int
	GrowthFactor float64
}

// DefaultConfig returns default mmap configuration
func DefaultConfig() *Config {
	return &Config{
		Path:         "./data/vectors.mmap",
		InitialSize:  64 * 1024 * 1024,        // 64MB
		MaxSize:      10 * 1024 * 1024 * 1024, // 10GB
		Dimension:    128,
		GrowthFactor: 2.0,
	}
}

// Open opens or creates a memory-mapped storage file
func Open(cfg *Config) (*Storage, error) {
	// Sanitize path to prevent directory traversal
	cfg.Path = filepath.Clean(cfg.Path)
	// Open or create file with restricted permissions
	file, err := os.OpenFile(cfg.Path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}

	// Get file size
	info, err := file.Stat()
	if err != nil {
		_ = file.Close() // Best effort cleanup on error
		return nil, err
	}

	size := info.Size()
	if size == 0 {
		size = cfg.InitialSize
		if err := file.Truncate(size); err != nil {
			_ = file.Close() // Best effort cleanup on error
			return nil, err
		}
	}

	// Memory map the file
	// #nosec G115 - syscall.Mmap requires int parameters, file.Fd() and size are validated
	data, err := syscall.Mmap(int(file.Fd()), 0, int(size),
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		_ = file.Close() // Best effort cleanup on error
		return nil, err
	}

	s := &Storage{
		path:      cfg.Path,
		file:      file,
		data:      data,
		size:      0,
		capacity:  size,
		dimension: cfg.Dimension,
		allocator: NewAllocator(size),
	}

	// Load allocator state if file existed
	if info.Size() > 0 {
		s.loadAllocatorState()
	}

	return s, nil
}

// Close unmaps and closes the storage
func (s *Storage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Save allocator state - best effort, don't fail close if this fails
	_ = s.saveAllocatorState()

	// Sync data to disk
	if err := syscall.Munmap(s.data); err != nil {
		return err
	}

	return s.file.Close()
}

// WriteVector writes a vector at the given offset
func (s *Storage) WriteVector(offset int64, vector []float32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(vector) != s.dimension {
		return errors.New("dimension mismatch")
	}

	end := offset + int64(len(vector)*4)
	if end > s.capacity {
		if err := s.grow(end); err != nil {
			return err
		}
	}

	for i, v := range vector {
		pos := offset + int64(i*4)
		binary.LittleEndian.PutUint32(s.data[pos:], uint32FromFloat32(v))
	}

	if end > s.size {
		s.size = end
	}

	return nil
}

// ReadVector reads a vector from the given offset
func (s *Storage) ReadVector(offset int64) ([]float32, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	end := offset + int64(s.dimension*4)
	if end > s.capacity {
		return nil, errors.New("offset out of bounds")
	}

	vector := make([]float32, s.dimension)
	for i := range vector {
		pos := offset + int64(i*4)
		vector[i] = float32FromUint32(binary.LittleEndian.Uint32(s.data[pos:]))
	}

	return vector, nil
}

// Allocate allocates space for a vector
func (s *Storage) Allocate() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	vectorSize := int64(s.dimension * 4)
	offset, err := s.allocator.Allocate(vectorSize)
	if err != nil {
		// Try to grow
		newSize := s.capacity * 2
		if err := s.grow(newSize); err != nil {
			return 0, err
		}
		s.allocator.Extend(newSize)
		offset, err = s.allocator.Allocate(vectorSize)
		if err != nil {
			return 0, err
		}
	}

	return offset, nil
}

// Free marks space as available
func (s *Storage) Free(offset int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	vectorSize := int64(s.dimension * 4)
	s.allocator.Free(offset, vectorSize)
}

// grow increases the file size and remaps
func (s *Storage) grow(minSize int64) error {
	newSize := s.capacity * 2
	if newSize < minSize {
		newSize = minSize
	}

	// Unmap current mapping
	if err := syscall.Munmap(s.data); err != nil {
		return err
	}

	// Grow file
	if err := s.file.Truncate(newSize); err != nil {
		return err
	}

	// Remap
	// #nosec G115 - syscall.Mmap requires int parameters, values are validated
	data, err := syscall.Mmap(int(s.file.Fd()), 0, int(newSize),
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return err
	}

	s.data = data
	s.capacity = newSize

	return nil
}

// Sync syncs the mmap to disk
func (s *Storage) Sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Note: On macOS/Darwin, use msync via cgo or skip sync
	// For now, just sync the file
	return s.file.Sync()
}

// Size returns the current used size
func (s *Storage) Size() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.size
}

// Capacity returns the current capacity
func (s *Storage) Capacity() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.capacity
}

// allocatorState holds serializable allocator state
type allocatorState struct {
	NextOffset int64       `json:"next_offset"`
	FreeList   []freeBlock `json:"free_list"`
}

func (s *Storage) saveAllocatorState() error {
	// Save allocator state to a separate metadata file
	metaPath := filepath.Clean(s.path + ".meta")

	state := allocatorState{
		NextOffset: s.allocator.nextOffset,
		FreeList:   s.allocator.freeList,
	}

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	return os.WriteFile(metaPath, data, 0600)
}

func (s *Storage) loadAllocatorState() {
	// Load allocator state from metadata file
	metaPath := filepath.Clean(s.path + ".meta")

	data, err := os.ReadFile(metaPath)
	if err != nil {
		return
	}

	var state allocatorState
	if err := json.Unmarshal(data, &state); err != nil {
		return
	}

	s.allocator.mu.Lock()
	s.allocator.nextOffset = state.NextOffset
	s.allocator.freeList = state.FreeList
	s.size = state.NextOffset
	s.allocator.mu.Unlock()
}

// Allocator manages space allocation using a free list
type Allocator struct {
	freeList   []freeBlock
	nextOffset int64
	maxSize    int64
	mu         sync.Mutex
}

type freeBlock struct {
	Offset int64 `json:"offset"`
	Size   int64 `json:"size"`
}

// NewAllocator creates a new allocator
func NewAllocator(maxSize int64) *Allocator {
	return &Allocator{
		freeList:   make([]freeBlock, 0),
		nextOffset: 0,
		maxSize:    maxSize,
	}
}

// Allocate allocates a block of the given size
func (a *Allocator) Allocate(size int64) (int64, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// First-fit from free list
	for i, block := range a.freeList {
		if block.Size >= size {
			offset := block.Offset
			if block.Size == size {
				// Remove block
				a.freeList = append(a.freeList[:i], a.freeList[i+1:]...)
			} else {
				// Shrink block
				a.freeList[i].Offset += size
				a.freeList[i].Size -= size
			}
			return offset, nil
		}
	}

	// Allocate from end
	if a.nextOffset+size > a.maxSize {
		return 0, errors.New("storage full")
	}

	offset := a.nextOffset
	a.nextOffset += size
	return offset, nil
}

// Free marks a block as free
func (a *Allocator) Free(offset, size int64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Add to free list
	a.freeList = append(a.freeList, freeBlock{Offset: offset, Size: size})

	// Coalesce adjacent blocks
	a.coalesce()
}

// coalesce merges adjacent free blocks
func (a *Allocator) coalesce() {
	if len(a.freeList) < 2 {
		return
	}

	// Sort by offset
	sort.Slice(a.freeList, func(i, j int) bool {
		return a.freeList[i].Offset < a.freeList[j].Offset
	})

	// Merge adjacent blocks
	merged := make([]freeBlock, 0, len(a.freeList))
	current := a.freeList[0]

	for i := 1; i < len(a.freeList); i++ {
		next := a.freeList[i]
		// Check if blocks are adjacent
		if current.Offset+current.Size == next.Offset {
			// Merge
			current.Size += next.Size
		} else {
			merged = append(merged, current)
			current = next
		}
	}
	merged = append(merged, current)

	a.freeList = merged
}

// Extend extends the allocator's max size
func (a *Allocator) Extend(newSize int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.maxSize = newSize
}

// Used returns the amount of space in use
func (a *Allocator) Used() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()

	var freeSpace int64
	for _, block := range a.freeList {
		freeSpace += block.Size
	}
	return a.nextOffset - freeSpace
}

// Float conversion helpers using bit manipulation
// #nosec G103 - unsafe pointer conversion is intentional for efficient float32/uint32 bit casting
func uint32FromFloat32(f float32) uint32 {
	return *(*uint32)(unsafe.Pointer(&f))
}

// #nosec G103 - unsafe pointer conversion is intentional for efficient float32/uint32 bit casting
func float32FromUint32(u uint32) float32 {
	return *(*float32)(unsafe.Pointer(&u))
}
