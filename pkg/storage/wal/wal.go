package wal

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
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

// safeInt64ToUint64 safely converts int64 to uint64
func safeInt64ToUint64(v int64) uint64 {
	if v < 0 {
		return 0
	}
	return uint64(v)
}

// safeUint64ToInt64 safely converts uint64 to int64
func safeUint64ToInt64(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}

// WAL represents a write-ahead log
type WAL struct {
	dir         string
	segmentSize int64
	syncOnWrite bool

	currentSegment *segment
	segments       []*segmentInfo
	lastSeqNum     uint64

	mu sync.Mutex
}

// Config holds WAL configuration
type Config struct {
	Dir         string
	SegmentSize int64 // In bytes
	SyncOnWrite bool
}

// DefaultConfig returns default WAL configuration
func DefaultConfig() *Config {
	return &Config{
		Dir:         "./data/wal",
		SegmentSize: 64 * 1024 * 1024, // 64MB
		SyncOnWrite: true,
	}
}

// RecordType indicates the type of WAL record
type RecordType uint8

const (
	RecordTypeInsert RecordType = iota + 1
	RecordTypeUpdate
	RecordTypeDelete
	RecordTypeCheckpoint
	RecordTypeBatch
)

// Record represents a WAL record
type Record struct {
	SeqNum     uint64
	Type       RecordType
	Collection string
	Data       []byte
	Timestamp  int64
}

// segment represents a single WAL segment file
type segment struct {
	file   *os.File
	writer *bufio.Writer
	path   string
	size   int64
	mu     sync.Mutex
}

type segmentInfo struct {
	path       string
	lastSeqNum uint64
	size       int64
}

// Open opens or creates a WAL
func Open(cfg *Config) (*WAL, error) {
	if err := os.MkdirAll(cfg.Dir, 0750); err != nil {
		return nil, err
	}

	wal := &WAL{
		dir:         cfg.Dir,
		segmentSize: cfg.SegmentSize,
		syncOnWrite: cfg.SyncOnWrite,
		segments:    make([]*segmentInfo, 0),
	}

	// Find existing segments
	if err := wal.loadSegments(); err != nil {
		return nil, err
	}

	// Open or create current segment
	if err := wal.openCurrentSegment(); err != nil {
		return nil, err
	}

	return wal, nil
}

func (w *WAL) loadSegments() error {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return err
	}

	var paths []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".wal" {
			paths = append(paths, filepath.Join(w.dir, entry.Name()))
		}
	}

	sort.Strings(paths)

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		w.segments = append(w.segments, &segmentInfo{
			path: path,
			size: info.Size(),
		})
	}

	return nil
}

func (w *WAL) openCurrentSegment() error {
	// Create new segment file
	name := fmt.Sprintf("%020d.wal", time.Now().UnixNano())
	path := filepath.Join(w.dir, name)

	// #nosec G304 - path is constructed internally from w.dir and timestamp
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return err
	}

	w.currentSegment = &segment{
		file:   file,
		writer: bufio.NewWriter(file),
		path:   path,
		size:   0,
	}

	return nil
}

// Write writes a record to the WAL
func (w *WAL) Write(record *Record) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Assign sequence number
	w.lastSeqNum++
	record.SeqNum = w.lastSeqNum
	record.Timestamp = time.Now().UnixNano()

	// Encode record
	data, err := encodeRecord(record)
	if err != nil {
		return err
	}

	// Check if we need to rotate
	if w.currentSegment.size+int64(len(data)) > w.segmentSize {
		if err := w.rotate(); err != nil {
			return err
		}
	}

	// Write to segment
	w.currentSegment.mu.Lock()
	defer w.currentSegment.mu.Unlock()

	if _, err := w.currentSegment.writer.Write(data); err != nil {
		return err
	}

	w.currentSegment.size += int64(len(data))

	// Sync if required
	if w.syncOnWrite {
		if err := w.currentSegment.writer.Flush(); err != nil {
			return err
		}
		if err := w.currentSegment.file.Sync(); err != nil {
			return err
		}
	}

	return nil
}

// WriteBatch writes multiple records atomically
func (w *WAL) WriteBatch(records []*Record) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, record := range records {
		w.lastSeqNum++
		record.SeqNum = w.lastSeqNum
		record.Timestamp = time.Now().UnixNano()

		data, err := encodeRecord(record)
		if err != nil {
			return err
		}

		// Check if we need to rotate
		if w.currentSegment.size+int64(len(data)) > w.segmentSize {
			if err := w.rotate(); err != nil {
				return err
			}
		}

		if _, err := w.currentSegment.writer.Write(data); err != nil {
			return err
		}
		w.currentSegment.size += int64(len(data))
	}

	// Sync after batch
	if err := w.currentSegment.writer.Flush(); err != nil {
		return err
	}
	if w.syncOnWrite {
		if err := w.currentSegment.file.Sync(); err != nil {
			return err
		}
	}

	return nil
}

func (w *WAL) rotate() error {
	// Flush and close current segment
	if w.currentSegment != nil {
		w.currentSegment.mu.Lock()
		if err := w.currentSegment.writer.Flush(); err != nil {
			w.currentSegment.mu.Unlock()
			return err
		}
		if err := w.currentSegment.file.Sync(); err != nil {
			w.currentSegment.mu.Unlock()
			return err
		}
		if err := w.currentSegment.file.Close(); err != nil {
			w.currentSegment.mu.Unlock()
			return err
		}
		w.currentSegment.mu.Unlock()

		// Add to segment list
		w.segments = append(w.segments, &segmentInfo{
			path: w.currentSegment.path,
			size: w.currentSegment.size,
		})
	}

	// Open new segment
	return w.openCurrentSegment()
}

// Sync forces a sync to disk
func (w *WAL) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.currentSegment == nil {
		return nil
	}

	w.currentSegment.mu.Lock()
	defer w.currentSegment.mu.Unlock()

	if err := w.currentSegment.writer.Flush(); err != nil {
		return err
	}
	return w.currentSegment.file.Sync()
}

// Close closes the WAL
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.currentSegment == nil {
		return nil
	}

	w.currentSegment.mu.Lock()
	defer w.currentSegment.mu.Unlock()

	if err := w.currentSegment.writer.Flush(); err != nil {
		return err
	}
	if err := w.currentSegment.file.Sync(); err != nil {
		return err
	}
	return w.currentSegment.file.Close()
}

// Replay replays all records from the WAL
func (w *WAL) Replay(fn func(*Record) error) error {
	// Replay old segments
	for _, seg := range w.segments {
		if err := w.replaySegment(seg.path, fn); err != nil {
			return err
		}
	}

	// Replay current segment
	if w.currentSegment != nil {
		// Flush first to ensure all data is on disk
		w.currentSegment.mu.Lock()
		if err := w.currentSegment.writer.Flush(); err != nil {
			w.currentSegment.mu.Unlock()
			return err
		}
		w.currentSegment.mu.Unlock()

		if err := w.replaySegment(w.currentSegment.path, fn); err != nil {
			return err
		}
	}

	return nil
}

func (w *WAL) replaySegment(path string, fn func(*Record) error) error {
	// #nosec G304 - path is from internal directory listing, not user input
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	for {
		record, err := decodeRecord(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		if err := fn(record); err != nil {
			return err
		}
	}

	return nil
}

// Truncate removes old segments up to the given sequence number
func (w *WAL) Truncate(beforeSeqNum uint64) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	var remaining []*segmentInfo
	for _, seg := range w.segments {
		if seg.lastSeqNum < beforeSeqNum {
			if err := os.Remove(seg.path); err != nil {
				return err
			}
		} else {
			remaining = append(remaining, seg)
		}
	}

	w.segments = remaining
	return nil
}

// LastSeqNum returns the last sequence number
func (w *WAL) LastSeqNum() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastSeqNum
}

// Record encoding/decoding
// Format: [length:4][type:1][seqnum:8][timestamp:8][collection_len:2][collection][data_len:4][data][crc:4]

func encodeRecord(r *Record) ([]byte, error) {
	collBytes := []byte(r.Collection)

	// Validate lengths with bounds checking
	collLen, err := safeIntToUint16(len(collBytes))
	if err != nil {
		return nil, fmt.Errorf("collection name too long: %w", err)
	}
	dataLen, err := safeIntToUint32(len(r.Data))
	if err != nil {
		return nil, fmt.Errorf("data too large: %w", err)
	}
	totalLen, err := safeIntToUint32(1 + 8 + 8 + 2 + len(collBytes) + 4 + len(r.Data))
	if err != nil {
		return nil, fmt.Errorf("record too large: %w", err)
	}

	buf := make([]byte, 4+int(totalLen)+4) // length + payload + crc

	// Length (excluding length field and CRC)
	binary.LittleEndian.PutUint32(buf[0:4], totalLen)

	offset := 4
	buf[offset] = byte(r.Type)
	offset++

	binary.LittleEndian.PutUint64(buf[offset:offset+8], r.SeqNum)
	offset += 8

	binary.LittleEndian.PutUint64(buf[offset:offset+8], safeInt64ToUint64(r.Timestamp))
	offset += 8

	binary.LittleEndian.PutUint16(buf[offset:offset+2], collLen)
	offset += 2

	copy(buf[offset:], collBytes)
	offset += len(collBytes)

	binary.LittleEndian.PutUint32(buf[offset:offset+4], dataLen)
	offset += 4

	copy(buf[offset:], r.Data)
	offset += len(r.Data)

	// CRC32
	crc := crc32.ChecksumIEEE(buf[4:offset])
	binary.LittleEndian.PutUint32(buf[offset:], crc)

	return buf, nil
}

func decodeRecord(r io.Reader) (*Record, error) {
	// Read length
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, err
	}
	length := binary.LittleEndian.Uint32(lenBuf[:])

	// Read payload + CRC
	buf := make([]byte, length+4)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	// Verify CRC
	expectedCRC := binary.LittleEndian.Uint32(buf[length:])
	actualCRC := crc32.ChecksumIEEE(buf[:length])
	if expectedCRC != actualCRC {
		return nil, errors.New("CRC mismatch")
	}

	record := &Record{}
	offset := 0

	record.Type = RecordType(buf[offset])
	offset++

	record.SeqNum = binary.LittleEndian.Uint64(buf[offset : offset+8])
	offset += 8

	record.Timestamp = safeUint64ToInt64(binary.LittleEndian.Uint64(buf[offset : offset+8]))
	offset += 8

	collLen := binary.LittleEndian.Uint16(buf[offset : offset+2])
	offset += 2

	record.Collection = string(buf[offset : offset+int(collLen)])
	offset += int(collLen)

	dataLen := binary.LittleEndian.Uint32(buf[offset : offset+4])
	offset += 4

	record.Data = make([]byte, dataLen)
	copy(record.Data, buf[offset:offset+int(dataLen)])

	return record, nil
}
