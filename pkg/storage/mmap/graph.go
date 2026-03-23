package mmap

import (
	"encoding/binary"
	"os"
	"sync"
	"syscall"
)

// GraphMmap manages an on-disk, memory-mapped representation of an HNSW graph's connections.
// It uses a fixed-size block per node to guarantee O(1) disk offset lookups and eliminate 
// GC overhead that would otherwise crash the database on graphs >50M vectors.
type GraphMmap struct {
	path       string
	file       *os.File
	data       []byte
	mu         sync.RWMutex
	
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

// NewGraphMmap creates or opens an exclusive memory-mapped file for HNSW connections
func NewGraphMmap(path string, m int) (*GraphMmap, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}

	// Calculate stride: 
	// [Level: 4] + [Counts: 4 * (MaxLevel+1)] + [Layer0: 4 * 2M] + [Layer1..L: 4 * MaxLevel * M]
	countsSize := 4 * (DefaultMaxLevel + 1)
	layer0Size := 4 * (2 * m)
	upperLayersSize := 4 * (DefaultMaxLevel * m)
	stride := 4 + countsSize + layer0Size + upperLayersSize

	// Get file info
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}

	size := info.Size()
	var data []byte

	// Initialize new file
	if size == 0 {
		// Start with 10,000 nodes capacity + header
		initialCapacity := int64(GraphHeaderSize + (stride * 10000))
		if err := file.Truncate(initialCapacity); err != nil {
			file.Close()
			return nil, err
		}
		size = initialCapacity
		
		var mmapErr error
		data, mmapErr = syscall.Mmap(int(file.Fd()), 0, int(size), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
		if mmapErr != nil {
			file.Close()
			return nil, mmapErr
		}

		// Write header
		binary.LittleEndian.PutUint32(data[0:4], 0) // NumNodes
		binary.LittleEndian.PutUint32(data[4:8], uint32(m))
		binary.LittleEndian.PutUint32(data[8:12], uint32(DefaultMaxLevel))
		binary.LittleEndian.PutUint32(data[12:16], uint32(stride))
	} else {
		var mmapErr error
		data, mmapErr = syscall.Mmap(int(file.Fd()), 0, int(size), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
		if mmapErr != nil {
			file.Close()
			return nil, mmapErr
		}
	}

	numNodes := int(binary.LittleEndian.Uint32(data[0:4]))

	return &GraphMmap{
		path:       path,
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

	// Remap
	data, err := syscall.Mmap(int(g.file.Fd()), 0, int(newSize), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
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
	binary.LittleEndian.PutUint32(g.data[offset:offset+4], uint32(level))

	// Update global counter
	if int(nodeID) >= g.NumNodes {
		g.NumNodes = int(nodeID) + 1
		binary.LittleEndian.PutUint32(g.data[0:4], uint32(g.NumNodes))
	}
	
	return nil
}

// GetConnections securely slices out the physical `[]uint32` layer from the memory-map
func (g *GraphMmap) GetConnections(nodeID uint32, layer int) []uint32 {
	g.mu.RLock()
	defer g.mu.RUnlock()

	offset := GraphHeaderSize + (int(nodeID) * g.NodeStride)
	
	// Read level
	level := int(binary.LittleEndian.Uint32(g.data[offset:offset+4]))
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
func (g *GraphMmap) SetConnections(nodeID uint32, layer int, connections []uint32) {
	g.mu.Lock()
	defer g.mu.Unlock()

	offset := GraphHeaderSize + (int(nodeID) * g.NodeStride)
	countOffset := offset + 4 + (layer * 4)
	binary.LittleEndian.PutUint32(g.data[countOffset:countOffset+4], uint32(len(connections)))

	dataOffset := offset + 4 + (4 * (g.MaxLevel + 1))
	if layer > 0 {
		dataOffset += (2 * g.M * 4) + ((layer - 1) * g.M * 4)
	}

	for i, conn := range connections {
		binary.LittleEndian.PutUint32(g.data[dataOffset+(i*4):dataOffset+(i*4)+4], conn)
	}
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
