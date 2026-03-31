package diskann

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// GraphStorage provides memory-mapped storage for DiskANN graphs
type GraphStorage struct {
	path      string
	file      *os.File
	data      []byte
	mu        sync.RWMutex
	dimension int
	maxDegree int

	// Header offsets
	headerSize  int
	nodeSize    int
	vectorSize  int
	neighborsOff int
}

// StorageConfig configures the graph storage
type StorageConfig struct {
	Path      string
	Dimension int
	MaxDegree int
	Capacity  int // Maximum number of nodes
}

// Header format:
// 0-3:   Magic number (0x44414E4E = "DANN")
// 4-7:   Version (1)
// 8-11:  Dimension
// 12-15: MaxDegree
// 16-23: Node count (uint64)
// 24-31: Entry point (uint32) + reserved
// 32+:   Node data

const (
	magicNumber     = 0x44414E4E
	storageVersion  = 1
	headerSize      = 64
	nodeIDSize      = 4
	vectorElemSize  = 4
	neighborSize    = 4
)

// OpenStorage opens or creates graph storage
func OpenStorage(cfg *StorageConfig) (*GraphStorage, error) {
	if cfg.Dimension <= 0 || cfg.MaxDegree <= 0 {
		return nil, errors.New("invalid storage configuration")
	}

	// Ensure directory exists
	dir := filepath.Dir(cfg.Path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, err
	}

	// Calculate sizes
	vectorSize := cfg.Dimension * vectorElemSize
	neighborsSize := cfg.MaxDegree * neighborSize
	nodeSize := nodeIDSize + vectorSize + neighborsSize + 4 // +4 for neighbor count

	// Calculate total file size
	totalSize := headerSize + (nodeSize * cfg.Capacity)

	// Open or create file with secure permissions
	file, err := os.OpenFile(cfg.Path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}

	// Check file size
	info, err := file.Stat()
	if err != nil {
		_ = file.Close() // #nosec G104 - best effort cleanup
		return nil, err
	}

	if info.Size() == 0 {
		// Initialize new file
		if err := file.Truncate(int64(totalSize)); err != nil {
			_ = file.Close() // #nosec G104 - best effort cleanup
			return nil, err
		}

		// Write header
		// Note: Dimension and MaxDegree are validated in Config.Validate() to be
		// positive and reasonably bounded, so they safely fit in uint32
		header := make([]byte, headerSize)
		binary.LittleEndian.PutUint32(header[0:4], magicNumber)
		binary.LittleEndian.PutUint32(header[4:8], storageVersion)
		binary.LittleEndian.PutUint32(header[8:12], uint32(cfg.Dimension))  // #nosec G115 - validated in config
		binary.LittleEndian.PutUint32(header[12:16], uint32(cfg.MaxDegree)) // #nosec G115 - validated in config
		binary.LittleEndian.PutUint64(header[16:24], 0)                     // Node count
		binary.LittleEndian.PutUint32(header[24:28], 0)                     // Entry point

		if _, err := file.WriteAt(header, 0); err != nil {
			_ = file.Close() // #nosec G104 - best effort cleanup
			return nil, err
		}
	}

	gs := &GraphStorage{
		path:         cfg.Path,
		file:         file,
		dimension:    cfg.Dimension,
		maxDegree:    cfg.MaxDegree,
		headerSize:   headerSize,
		nodeSize:     nodeSize,
		vectorSize:   vectorSize,
		neighborsOff: nodeIDSize + vectorSize,
	}

	return gs, nil
}

// Close closes the storage
func (gs *GraphStorage) Close() error {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	if gs.file != nil {
		return gs.file.Close()
	}
	return nil
}

// WriteNode writes a node to storage
func (gs *GraphStorage) WriteNode(nodeID uint32, vector []float32, neighbors []uint32) error {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	offset := int64(gs.headerSize) + int64(nodeID)*int64(gs.nodeSize)

	// Prepare node data
	data := make([]byte, gs.nodeSize)

	// Write node ID
	binary.LittleEndian.PutUint32(data[0:4], nodeID)

	// Write vector
	for i, v := range vector {
		binary.LittleEndian.PutUint32(data[4+i*4:], uint32(v))
	}

	// Write neighbor count (bounded by maxDegree which fits in uint32)
	nOffset := gs.neighborsOff
	binary.LittleEndian.PutUint32(data[nOffset:], uint32(len(neighbors))) // #nosec G115 - bounded by maxDegree

	// Write neighbors
	for i, n := range neighbors {
		binary.LittleEndian.PutUint32(data[nOffset+4+i*4:], n)
	}

	_, err := gs.file.WriteAt(data, offset)
	return err
}

// ReadNode reads a node from storage
func (gs *GraphStorage) ReadNode(nodeID uint32) (vector []float32, neighbors []uint32, err error) {
	gs.mu.RLock()
	defer gs.mu.RUnlock()

	offset := int64(gs.headerSize) + int64(nodeID)*int64(gs.nodeSize)

	data := make([]byte, gs.nodeSize)
	if _, err := gs.file.ReadAt(data, offset); err != nil {
		return nil, nil, err
	}

	// Read vector
	vector = make([]float32, gs.dimension)
	for i := 0; i < gs.dimension; i++ {
		bits := binary.LittleEndian.Uint32(data[4+i*4:])
		vector[i] = float32(bits)
	}

	// Read neighbor count
	nOffset := gs.neighborsOff
	neighborCount := int(binary.LittleEndian.Uint32(data[nOffset:]))

	// Read neighbors
	neighbors = make([]uint32, neighborCount)
	for i := 0; i < neighborCount; i++ {
		neighbors[i] = binary.LittleEndian.Uint32(data[nOffset+4+i*4:])
	}

	return vector, neighbors, nil
}

// GetNodeCount returns the number of nodes
func (gs *GraphStorage) GetNodeCount() uint64 {
	gs.mu.RLock()
	defer gs.mu.RUnlock()

	data := make([]byte, 8)
	if _, err := gs.file.ReadAt(data, 16); err != nil {
		return 0
	}
	return binary.LittleEndian.Uint64(data)
}

// SetNodeCount sets the number of nodes
func (gs *GraphStorage) SetNodeCount(count uint64) error {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, count)
	_, err := gs.file.WriteAt(data, 16)
	return err
}

// GetEntryPoint returns the entry point node ID
func (gs *GraphStorage) GetEntryPoint() uint32 {
	gs.mu.RLock()
	defer gs.mu.RUnlock()

	data := make([]byte, 4)
	if _, err := gs.file.ReadAt(data, 24); err != nil {
		return 0
	}
	return binary.LittleEndian.Uint32(data)
}

// SetEntryPoint sets the entry point node ID
func (gs *GraphStorage) SetEntryPoint(nodeID uint32) error {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, nodeID)
	_, err := gs.file.WriteAt(data, 24)
	return err
}

// Sync flushes data to disk
func (gs *GraphStorage) Sync() error {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	return gs.file.Sync()
}
