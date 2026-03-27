package snapshot

import (
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// safeIntToUint32 safely converts int to uint32 with bounds checking
func safeIntToUint32(v int) (uint32, error) {
	if v < 0 || v > math.MaxUint32 {
		return 0, errors.New("integer overflow: value out of uint32 range")
	}
	return uint32(v), nil
}

// safeIntToUint16 safely converts int to uint16 with bounds checking
func safeIntToUint16(v int) (uint16, error) {
	if v < 0 || v > math.MaxUint16 {
		return 0, errors.New("integer overflow: value out of uint16 range")
	}
	return uint16(v), nil
}

// validSnapshotIDPattern defines allowed characters for snapshot IDs
var validSnapshotIDPattern = regexp.MustCompile(`^snap_[0-9]+$`)

// validateSnapshotID checks if a snapshot ID is safe (no path traversal)
func validateSnapshotID(id string) error {
	if id == "" {
		return errors.New("snapshot ID cannot be empty")
	}
	if len(id) > 64 {
		return errors.New("snapshot ID too long")
	}
	if !validSnapshotIDPattern.MatchString(id) {
		return errors.New("invalid snapshot ID format")
	}
	if strings.Contains(id, "..") || strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return errors.New("snapshot ID contains invalid path characters")
	}
	return nil
}

// Manager handles snapshot creation and restoration
type Manager struct {
	dir              string
	retainCount      int
	compressionLevel int

	mu sync.Mutex
}

// Config holds snapshot configuration
type Config struct {
	Dir              string
	RetainCount      int
	CompressionLevel int
}

// DefaultConfig returns default snapshot configuration
func DefaultConfig() *Config {
	return &Config{
		Dir:              "./data/snapshots",
		RetainCount:      5,
		CompressionLevel: gzip.DefaultCompression,
	}
}

// NewManager creates a new snapshot manager
func NewManager(cfg *Config) (*Manager, error) {
	if err := os.MkdirAll(cfg.Dir, 0750); err != nil {
		return nil, err
	}

	return &Manager{
		dir:              cfg.Dir,
		retainCount:      cfg.RetainCount,
		compressionLevel: cfg.CompressionLevel,
	}, nil
}

// Dir returns the directory path where snapshots are stored
func (m *Manager) Dir() string {
	return m.dir
}

// Snapshot represents a point-in-time snapshot
type Snapshot struct {
	ID          string    `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	Version     uint64    `json:"version"`
	Collections []string  `json:"collections"`
	Size        int64     `json:"size"`
	Path        string    `json:"path"`
}

// SnapshotWriter writes snapshot data
type SnapshotWriter struct {
	file          *os.File
	gzWriter      *gzip.Writer
	meta          *Snapshot
	headerWritten bool
}

// CreateSnapshot creates a new snapshot
func (m *Manager) CreateSnapshot(collections []string) (*SnapshotWriter, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := fmt.Sprintf("snap_%d", time.Now().UnixNano())
	path := filepath.Join(m.dir, id+".snap")

	// #nosec G304 - path is constructed internally from m.dir and generated ID
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	gzWriter, err := gzip.NewWriterLevel(file, m.compressionLevel)
	if err != nil {
		_ = file.Close() // Best effort cleanup
		return nil, err
	}

	meta := &Snapshot{
		ID:          id,
		Timestamp:   time.Now(),
		Collections: collections,
		Path:        path,
	}

	return &SnapshotWriter{
		file:     file,
		gzWriter: gzWriter,
		meta:     meta,
	}, nil
}

// WriteHeader writes the snapshot header
func (sw *SnapshotWriter) WriteHeader(version uint64, totalCollections int) error {
	sw.meta.Version = version

	// Write magic number
	if err := binary.Write(sw.gzWriter, binary.LittleEndian, uint32(0x564558DB)); err != nil {
		return err
	}

	// Write version
	if err := binary.Write(sw.gzWriter, binary.LittleEndian, version); err != nil {
		return err
	}

	// Write timestamp
	if err := binary.Write(sw.gzWriter, binary.LittleEndian, sw.meta.Timestamp.UnixNano()); err != nil {
		return err
	}

	// Write number of collections with bounds checking
	collCount, err := safeIntToUint32(totalCollections)
	if err != nil {
		return fmt.Errorf("invalid collection count: %w", err)
	}
	if err := binary.Write(sw.gzWriter, binary.LittleEndian, collCount); err != nil {
		return err
	}

	sw.headerWritten = true
	return nil
}

// WriteCollection writes a collection to the snapshot
func (sw *SnapshotWriter) WriteCollection(name string, data CollectionData) error {
	if !sw.headerWritten {
		return errors.New("header not written")
	}

	// Write collection name with bounds checking
	nameBytes := []byte(name)
	nameLen, err := safeIntToUint16(len(nameBytes))
	if err != nil {
		return fmt.Errorf("collection name too long: %w", err)
	}
	if err := binary.Write(sw.gzWriter, binary.LittleEndian, nameLen); err != nil {
		return err
	}
	if _, err := sw.gzWriter.Write(nameBytes); err != nil {
		return err
	}

	// Write config as JSON with bounds checking
	configBytes, err := json.Marshal(data.Config)
	if err != nil {
		return err
	}
	configLen, err := safeIntToUint32(len(configBytes))
	if err != nil {
		return fmt.Errorf("config too large: %w", err)
	}
	if err := binary.Write(sw.gzWriter, binary.LittleEndian, configLen); err != nil {
		return err
	}
	if _, err := sw.gzWriter.Write(configBytes); err != nil {
		return err
	}

	// Write number of points (uint64 can hold any Go slice length)
	if err := binary.Write(sw.gzWriter, binary.LittleEndian, uint64(len(data.Points))); err != nil {
		return err
	}

	// Write each point
	for _, p := range data.Points {
		if err := sw.writePoint(p); err != nil {
			return err
		}
	}

	return nil
}

func (sw *SnapshotWriter) writePoint(p PointData) error {
	// Write ID with bounds checking
	idBytes := []byte(p.ID)
	idLen, err := safeIntToUint16(len(idBytes))
	if err != nil {
		return fmt.Errorf("point ID too long: %w", err)
	}
	if err := binary.Write(sw.gzWriter, binary.LittleEndian, idLen); err != nil {
		return err
	}
	if _, err := sw.gzWriter.Write(idBytes); err != nil {
		return err
	}

	// Write vector with bounds checking
	vecLen, err := safeIntToUint32(len(p.Vector))
	if err != nil {
		return fmt.Errorf("vector too large: %w", err)
	}
	if err := binary.Write(sw.gzWriter, binary.LittleEndian, vecLen); err != nil {
		return err
	}
	for _, v := range p.Vector {
		if err := binary.Write(sw.gzWriter, binary.LittleEndian, v); err != nil {
			return err
		}
	}

	// Write payload with bounds checking
	payloadBytes, err := json.Marshal(p.Payload)
	if err != nil {
		return err
	}
	payloadLen, err := safeIntToUint32(len(payloadBytes))
	if err != nil {
		return fmt.Errorf("payload too large: %w", err)
	}
	if err := binary.Write(sw.gzWriter, binary.LittleEndian, payloadLen); err != nil {
		return err
	}
	if _, err := sw.gzWriter.Write(payloadBytes); err != nil {
		return err
	}

	return nil
}

// Finish completes the snapshot
func (sw *SnapshotWriter) Finish() (*Snapshot, error) {
	if err := sw.gzWriter.Close(); err != nil {
		return nil, err
	}

	// Get final size
	info, err := sw.file.Stat()
	if err != nil {
		_ = sw.file.Close() // Best effort cleanup
		return nil, err
	}
	sw.meta.Size = info.Size()

	if err := sw.file.Close(); err != nil {
		return nil, err
	}

	// Write metadata file
	metaPath := sw.meta.Path + ".meta"
	metaBytes, err := json.MarshalIndent(sw.meta, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(metaPath, metaBytes, 0600); err != nil {
		return nil, err
	}

	return sw.meta, nil
}

// Cancel cancels snapshot creation
func (sw *SnapshotWriter) Cancel() error {
	var errs []error
	if err := sw.gzWriter.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close gzip writer: %w", err))
	}
	fileName := sw.file.Name()
	if err := sw.file.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close file: %w", err))
	}
	if err := os.Remove(fileName); err != nil {
		errs = append(errs, fmt.Errorf("remove file: %w", err))
	}
	if len(errs) > 0 {
		return errs[0] // Return first error
	}
	return nil
}

// CollectionData holds collection snapshot data
type CollectionData struct {
	Config interface{} `json:"config"`
	Points []PointData `json:"points"`
}

// PointData holds point snapshot data
type PointData struct {
	ID      string                 `json:"id"`
	Vector  []float32              `json:"vector"`
	Payload map[string]interface{} `json:"payload"`
}

// SnapshotReader reads snapshot data
type SnapshotReader struct {
	file           *os.File
	gzReader       *gzip.Reader
	Version        uint64
	Timestamp      time.Time
	NumCollections uint32
}

// OpenSnapshot opens a snapshot for reading
func (m *Manager) OpenSnapshot(id string) (*SnapshotReader, error) {
	// Validate snapshot ID to prevent path traversal
	if err := validateSnapshotID(id); err != nil {
		return nil, err
	}
	path := filepath.Join(m.dir, id+".snap")
	return m.OpenSnapshotFile(path)
}

// OpenSnapshotFile opens a snapshot file
func (m *Manager) OpenSnapshotFile(path string) (*SnapshotReader, error) {
	file, err := os.Open(path) // #nosec G304 - path is validated by caller
	if err != nil {
		return nil, err
	}

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		_ = file.Close() // Best effort cleanup
		return nil, err
	}

	sr := &SnapshotReader{
		file:     file,
		gzReader: gzReader,
	}

	// Read header
	var magic uint32
	if err := binary.Read(gzReader, binary.LittleEndian, &magic); err != nil {
		_ = sr.Close() // Best effort cleanup
		return nil, err
	}
	if magic != 0x564558DB {
		_ = sr.Close() // Best effort cleanup
		return nil, errors.New("invalid snapshot magic number")
	}

	if err := binary.Read(gzReader, binary.LittleEndian, &sr.Version); err != nil {
		_ = sr.Close() // Best effort cleanup
		return nil, err
	}

	var tsNano int64
	if err := binary.Read(gzReader, binary.LittleEndian, &tsNano); err != nil {
		_ = sr.Close() // Best effort cleanup
		return nil, err
	}
	sr.Timestamp = time.Unix(0, tsNano)

	if err := binary.Read(gzReader, binary.LittleEndian, &sr.NumCollections); err != nil {
		_ = sr.Close() // Best effort cleanup
		return nil, err
	}

	return sr, nil
}

// ReadCollection reads the next collection from the snapshot
func (sr *SnapshotReader) ReadCollection() (string, *CollectionData, error) {
	// Read collection name
	var nameLen uint16
	if err := binary.Read(sr.gzReader, binary.LittleEndian, &nameLen); err != nil {
		if errors.Is(err, io.EOF) {
			return "", nil, err
		}
		return "", nil, err
	}
	nameBytes := make([]byte, nameLen)
	if _, err := io.ReadFull(sr.gzReader, nameBytes); err != nil {
		return "", nil, err
	}
	name := string(nameBytes)

	// Read config
	var configLen uint32
	if err := binary.Read(sr.gzReader, binary.LittleEndian, &configLen); err != nil {
		return "", nil, err
	}
	configBytes := make([]byte, configLen)
	if _, err := io.ReadFull(sr.gzReader, configBytes); err != nil {
		return "", nil, err
	}
	var config interface{}
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return "", nil, err
	}

	// Read number of points
	var numPoints uint64
	if err := binary.Read(sr.gzReader, binary.LittleEndian, &numPoints); err != nil {
		return "", nil, err
	}

	// Read points
	points := make([]PointData, numPoints)
	for i := uint64(0); i < numPoints; i++ {
		p, err := sr.readPoint()
		if err != nil {
			return "", nil, err
		}
		points[i] = *p
	}

	return name, &CollectionData{Config: config, Points: points}, nil
}

func (sr *SnapshotReader) readPoint() (*PointData, error) {
	p := &PointData{}

	// Read ID
	var idLen uint16
	if err := binary.Read(sr.gzReader, binary.LittleEndian, &idLen); err != nil {
		return nil, err
	}
	idBytes := make([]byte, idLen)
	if _, err := io.ReadFull(sr.gzReader, idBytes); err != nil {
		return nil, err
	}
	p.ID = string(idBytes)

	// Read vector
	var vecLen uint32
	if err := binary.Read(sr.gzReader, binary.LittleEndian, &vecLen); err != nil {
		return nil, err
	}
	p.Vector = make([]float32, vecLen)
	for i := range p.Vector {
		if err := binary.Read(sr.gzReader, binary.LittleEndian, &p.Vector[i]); err != nil {
			return nil, err
		}
	}

	// Read payload
	var payloadLen uint32
	if err := binary.Read(sr.gzReader, binary.LittleEndian, &payloadLen); err != nil {
		return nil, err
	}
	if payloadLen > 0 {
		payloadBytes := make([]byte, payloadLen)
		if _, err := io.ReadFull(sr.gzReader, payloadBytes); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(payloadBytes, &p.Payload); err != nil {
			return nil, err
		}
	}

	return p, nil
}

// Close closes the snapshot reader
func (sr *SnapshotReader) Close() error {
	gzErr := sr.gzReader.Close()
	fileErr := sr.file.Close()
	if gzErr != nil {
		return gzErr
	}
	return fileErr
}

// List lists all available snapshots
func (m *Manager) List() ([]*Snapshot, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil, err
	}

	var snapshots []*Snapshot
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".meta" {
			metaPath := filepath.Join(m.dir, entry.Name())
			// #nosec G304 - metaPath is constructed from internal m.dir
			data, err := os.ReadFile(metaPath)
			if err != nil {
				continue
			}
			var snap Snapshot
			if err := json.Unmarshal(data, &snap); err != nil {
				continue
			}
			snapshots = append(snapshots, &snap)
		}
	}

	// Sort by timestamp descending
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Timestamp.After(snapshots[j].Timestamp)
	})

	return snapshots, nil
}

// Delete deletes a snapshot
func (m *Manager) Delete(id string) error {
	// Validate snapshot ID to prevent path traversal
	if err := validateSnapshotID(id); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	snapPath := filepath.Join(m.dir, id+".snap")
	metaPath := filepath.Join(m.dir, id+".snap.meta")

	// Verify paths are within snapshot directory
	absSnapPath, err := filepath.Abs(snapPath)
	if err != nil {
		return err
	}
	absDir, err := filepath.Abs(m.dir)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(absSnapPath, absDir) {
		return errors.New("invalid snapshot path")
	}

	_ = os.Remove(snapPath) // Best effort removal
	_ = os.Remove(metaPath) // Best effort removal

	return nil
}

// Prune removes old snapshots keeping only retainCount newest
func (m *Manager) Prune() error {
	snapshots, err := m.List()
	if err != nil {
		return err
	}

	if len(snapshots) <= m.retainCount {
		return nil
	}

	// Delete oldest snapshots
	for _, snap := range snapshots[m.retainCount:] {
		if err := m.Delete(snap.ID); err != nil {
			return err
		}
	}

	return nil
}
