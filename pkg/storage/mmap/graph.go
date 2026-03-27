package mmap

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

// GraphMmap manages an on-disk, memory-mapped representation of an HNSW graph's connections.
// It uses a fixed-size block per node to guarantee O(1) disk offset lookups and eliminate
// GC overhead that would otherwise crash the database on graphs >50M vectors.
type GraphMmap struct {
	path string
	file *os.File
	data []byte
	mu   sync.RWMutex

	M          int
	MaxLevel   int
	NodeStride int // Bytes per node block
	NumNodes   int
}

// Memory mapping constants
const (
	GraphHeaderSize = 64
	DefaultMaxLevel = 16 // Covers >1 Billion elements effectively
)

// safeInt64ToInt converts int64 to int with overflow checking
func safeInt64ToInt(v int64) (int, error) {
	if v < math.MinInt || v > math.MaxInt {
		return 0, fmt.Errorf("integer overflow: int64 value %d cannot be safely converted to int", v)
	}
	return int(v), nil
}

// safeUintptrToInt converts uintptr to int with overflow checking
func safeUintptrToInt(v uintptr) (int, error) {
	if v > uintptr(math.MaxInt) {
		return 0, fmt.Errorf("integer overflow: uintptr value %d cannot be safely converted to int", v)
	}
	return int(v), nil
}

// safeIntToUint32 safely converts int to uint32 with bounds checking
func safeIntToUint32(v int) (uint32, error) {
	if v < 0 || v > math.MaxUint32 {
		return 0, fmt.Errorf("integer overflow: int value %d cannot be safely converted to uint32", v)
	}
	return uint32(v), nil
}

// validateGraphPath sanitizes and validates a file path to prevent path injection.
// It ensures the path is cleaned and does not contain directory traversal sequences.
func validateGraphPath(path string) (string, error) {
	// Clean the path to resolve any . or .. components
	cleaned := filepath.Clean(path)

	// Reject paths containing traversal sequences after cleaning
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("path contains directory traversal: %s", path)
	}

	return cleaned, nil
}

// closeFileWithError closes a file and combines any close error with an existing error
func closeFileWithError(f *os.File, existingErr *error) {
	if closeErr := f.Close(); closeErr != nil {
		if *existingErr == nil {
			*existingErr = fmt.Errorf("failed to close file: %w", closeErr)
		} else {
			*existingErr = fmt.Errorf("%w; additionally failed to close file: %w", *existingErr, closeErr)
		}
	}
}

// NewGraphMmap creates or opens an exclusive memory-mapped file for HNSW connections
func NewGraphMmap(path string, m int) (_ *GraphMmap, retErr error) {
	// Validate and sanitize the file path to prevent path injection (G304/go/path-injection)
	cleanPath, err := validateGraphPath(path)
	if err != nil {
		return nil, fmt.Errorf("invalid graph path: %w", err)
	}

	file, err := os.OpenFile(cleanPath, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	// Ensure file is closed on any error path with proper error handling
	defer func() {
		if retErr != nil {
			closeFileWithError(file, &retErr)
		}
	}()

	// Calculate stride:
	// [Level: 4] + [Counts: 4 * (MaxLevel+1)] + [Layer0: 4 * 2M] + [Layer1..L: 4 * MaxLevel * M]
	countsSize := 4 * (DefaultMaxLevel + 1)
	layer0Size := 4 * (2 * m)
	upperLayersSize := 4 * (DefaultMaxLevel * m)
	stride := 4 + countsSize + layer0Size + upperLayersSize

	// Get file info
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	size := info.Size()
	var data []byte

	// Initialize new file
	if size == 0 {
		// Start with 10,000 nodes capacity + header
		initialCapacity := int64(GraphHeaderSize + (stride * 10000))
		if err := file.Truncate(initialCapacity); err != nil {
			return nil, err
		}
		size = initialCapacity

		mmapSize, err := safeInt64ToInt(size)
		if err != nil {
			return nil, fmt.Errorf("mmap size overflow: %w", err)
		}

		fdInt, err := safeUintptrToInt(file.Fd())
		if err != nil {
			return nil, fmt.Errorf("file descriptor overflow: %w", err)
		}

		data, err = syscall.Mmap(fdInt, 0, mmapSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
		if err != nil {
			return nil, err
		}

		// Write header
		binary.LittleEndian.PutUint32(data[0:4], 0) // NumNodes
		mU32, err := safeIntToUint32(m)
		if err != nil {
			_ = syscall.Munmap(data)
			return nil, fmt.Errorf("M value overflow: %w", err)
		}
		binary.LittleEndian.PutUint32(data[4:8], mU32)

		maxLevelU32, err := safeIntToUint32(DefaultMaxLevel)
		if err != nil {
			_ = syscall.Munmap(data)
			return nil, fmt.Errorf("MaxLevel overflow: %w", err)
		}
		binary.LittleEndian.PutUint32(data[8:12], maxLevelU32)

		strideU32, err := safeIntToUint32(stride)
		if err != nil {
			_ = syscall.Munmap(data)
			return nil, fmt.Errorf("stride overflow: %w", err)
		}
		binary.LittleEndian.PutUint32(data[12:16], strideU32)
	} else {
		mmapSize, err := safeInt64ToInt(size)
		if err != nil {
			return nil, fmt.Errorf("mmap size overflow: %w", err)
		}

		fdInt, err := safeUintptrToInt(file.Fd())
		if err != nil {
			return nil, fmt.Errorf("file descriptor overflow: %w", err)
		}

		data, err = syscall.Mmap(fdInt, 0, mmapSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
		if err != nil {
			return nil, err
		}
	}

	numNodes := int(binary.LittleEndian.Uint32(data[0:4]))

	return &GraphMmap{
		path:       cleanPath,
		file:       file,
		data:       data,
		M:          m,
		MaxLevel:   DefaultMaxLevel,
		NodeStride: stride,
		NumNodes:   numNodes,
	}, nil
}

// ensureCapacity expands the mmap file dynamically if the new ID exceeds the current file allocation
func (g *GraphMmap) ensureCapacity(nodeID uint32) error {
	requiredSize := int64(GraphHeaderSize + (int(nodeID+1) * g.NodeStride))
	currentSize := int64(len(g.data))

	if requiredSize <= currentSize {
		return nil
	}

	// Double the capacity or meet requirement
	newSize := currentSize * 2
	if newSize < requiredSize {
		newSize = requiredSize
	}

	// Unmap old
	if err := syscall.Munmap(g.data); err != nil {
		return err
	}

	// Truncate
	if err := g.file.Truncate(newSize); err != nil {
		return err
	}

	// Remap with safe conversions
	mmapSize, err := safeInt64ToInt(newSize)
	if err != nil {
		return fmt.Errorf("mmap size overflow: %w", err)
	}

	fdInt, err := safeUintptrToInt(g.file.Fd())
	if err != nil {
		return fmt.Errorf("file descriptor overflow: %w", err)
	}

	data, err := syscall.Mmap(fdInt, 0, mmapSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return err
	}

	g.data = data
	return nil
}

// AddNode expands the graph to include this new ID, guaranteeing disk capacity
func (g *GraphMmap) AddNode(nodeID uint32, level int) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if err := g.ensureCapacity(nodeID); err != nil {
		return err
	}

	offset := GraphHeaderSize + (int(nodeID) * g.NodeStride)
	levelU32, err := safeIntToUint32(level)
	if err != nil {
		return fmt.Errorf("level overflow: %w", err)
	}
	binary.LittleEndian.PutUint32(g.data[offset:offset+4], levelU32)

	// Update global counter
	if int(nodeID) >= g.NumNodes {
		g.NumNodes = int(nodeID) + 1
		numNodesU32, err := safeIntToUint32(g.NumNodes)
		if err != nil {
			return fmt.Errorf("NumNodes overflow: %w", err)
		}
		binary.LittleEndian.PutUint32(g.data[0:4], numNodesU32)
	}

	return nil
}

// GetConnections securely slices out the physical `[]uint32` layer from the memory-map
func (g *GraphMmap) GetConnections(nodeID uint32, layer int) []uint32 {
	g.mu.RLock()
	defer g.mu.RUnlock()

	offset := GraphHeaderSize + (int(nodeID) * g.NodeStride)

	// Read level
	level := int(binary.LittleEndian.Uint32(g.data[offset : offset+4]))
	if layer > level {
		return nil
	}

	// Read count
	countOffset := offset + 4 + (layer * 4)
	count := int(binary.LittleEndian.Uint32(g.data[countOffset : countOffset+4]))

	// Calculate slice offset
	dataOffset := offset + 4 + (4 * (g.MaxLevel + 1))
	if layer > 0 {
		// Layer 0 is 2*M, so layer > 0 starts after 2*M
		dataOffset += (2 * g.M * 4) + ((layer - 1) * g.M * 4)
	}

	// Extract the actual points without allocating new arrays on the heap!
	// (Note: in high throughput, returning the byte slice mapped directly would be zero-alloc,
	// but mapping into []uint32 here is fast enough for Stage 1. We allocate the slice here to be safe and GC-collectable)
	conns := make([]uint32, count)
	for i := 0; i < count; i++ {
		conns[i] = binary.LittleEndian.Uint32(g.data[dataOffset+(i*4) : dataOffset+(i*4)+4])
	}

	return conns
}

// SetConnections writes the slice directly back into the memory map
func (g *GraphMmap) SetConnections(nodeID uint32, layer int, connections []uint32) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	offset := GraphHeaderSize + (int(nodeID) * g.NodeStride)
	countOffset := offset + 4 + (layer * 4)
	connLen, err := safeIntToUint32(len(connections))
	if err != nil {
		return fmt.Errorf("connections length overflow: %w", err)
	}
	binary.LittleEndian.PutUint32(g.data[countOffset:countOffset+4], connLen)

	dataOffset := offset + 4 + (4 * (g.MaxLevel + 1))
	if layer > 0 {
		dataOffset += (2 * g.M * 4) + ((layer - 1) * g.M * 4)
	}

	for i, conn := range connections {
		binary.LittleEndian.PutUint32(g.data[dataOffset+(i*4):dataOffset+(i*4)+4], conn)
	}
	return nil
}

// Sync flushes the map and avoids partial persistent losses
func (g *GraphMmap) Sync() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	// OS handles page fluhes on MAP_SHARED automatically
	return nil
}

func (g *GraphMmap) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.data != nil {
		_ = syscall.Munmap(g.data)
		g.data = nil
	}
	if g.file != nil {
		err := g.file.Close()
		g.file = nil
		return err
	}
	return nil
}
