package wal

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRecordTypes(t *testing.T) {
	tests := []struct {
		name       string
		recordType RecordType
		expected   RecordType
	}{
		{"Insert", RecordTypeInsert, RecordType(1)},
		{"Update", RecordTypeUpdate, RecordType(2)},
		{"Delete", RecordTypeDelete, RecordType(3)},
		{"Checkpoint", RecordTypeCheckpoint, RecordType(4)},
		{"Batch", RecordTypeBatch, RecordType(5)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.recordType != tt.expected {
				t.Errorf("RecordType %s = %d, want %d", tt.name, tt.recordType, tt.expected)
			}
		})
	}
}

func TestEncodeDecodeRecord(t *testing.T) {
	tests := []struct {
		name   string
		record *Record
	}{
		{
			name: "Insert record",
			record: &Record{
				SeqNum:     1,
				Type:       RecordTypeInsert,
				Collection: "test_collection",
				Data:       []byte(`{"id":"1","vector":[0.1,0.2,0.3]}`),
				Timestamp:  time.Now().UnixNano(),
			},
		},
		{
			name: "Update record",
			record: &Record{
				SeqNum:     2,
				Type:       RecordTypeUpdate,
				Collection: "another_collection",
				Data:       []byte(`{"id":"2","vector":[0.4,0.5,0.6]}`),
				Timestamp:  time.Now().UnixNano(),
			},
		},
		{
			name: "Delete record",
			record: &Record{
				SeqNum:     3,
				Type:       RecordTypeDelete,
				Collection: "test",
				Data:       []byte(`{"id":"3"}`),
				Timestamp:  time.Now().UnixNano(),
			},
		},
		{
			name: "Checkpoint record",
			record: &Record{
				SeqNum:     4,
				Type:       RecordTypeCheckpoint,
				Collection: "",
				Data:       nil,
				Timestamp:  time.Now().UnixNano(),
			},
		},
		{
			name: "Empty data",
			record: &Record{
				SeqNum:     5,
				Type:       RecordTypeInsert,
				Collection: "empty",
				Data:       []byte{},
				Timestamp:  time.Now().UnixNano(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := encodeRecord(tt.record)
			if err != nil {
				t.Fatalf("encodeRecord() error = %v", err)
			}

			reader := bytes.NewReader(encoded)
			decoded, err := decodeRecord(reader)
			if err != nil {
				t.Fatalf("decodeRecord() error = %v", err)
			}

			if decoded.SeqNum != tt.record.SeqNum {
				t.Errorf("SeqNum = %d, want %d", decoded.SeqNum, tt.record.SeqNum)
			}
			if decoded.Type != tt.record.Type {
				t.Errorf("Type = %d, want %d", decoded.Type, tt.record.Type)
			}
			if decoded.Collection != tt.record.Collection {
				t.Errorf("Collection = %s, want %s", decoded.Collection, tt.record.Collection)
			}
			if !bytes.Equal(decoded.Data, tt.record.Data) {
				t.Errorf("Data = %v, want %v", decoded.Data, tt.record.Data)
			}
		})
	}
}

func TestCRC32Validation(t *testing.T) {
	record := &Record{
		SeqNum:     1,
		Type:       RecordTypeInsert,
		Collection: "test",
		Data:       []byte("test data"),
		Timestamp:  time.Now().UnixNano(),
	}

	encoded, err := encodeRecord(record)
	if err != nil {
		t.Fatalf("encodeRecord() error = %v", err)
	}

	// Corrupt the data (not the CRC)
	encoded[10] ^= 0xFF

	reader := bytes.NewReader(encoded)
	_, err = decodeRecord(reader)
	if err == nil {
		t.Error("decodeRecord() should have failed with corrupted data")
	}
	if err.Error() != "CRC mismatch" {
		t.Errorf("Expected CRC mismatch error, got: %v", err)
	}
}

func TestCRC32CorruptedCRC(t *testing.T) {
	record := &Record{
		SeqNum:     1,
		Type:       RecordTypeInsert,
		Collection: "test",
		Data:       []byte("test data"),
		Timestamp:  time.Now().UnixNano(),
	}

	encoded, err := encodeRecord(record)
	if err != nil {
		t.Fatalf("encodeRecord() error = %v", err)
	}

	// Corrupt the CRC at the end
	encoded[len(encoded)-1] ^= 0xFF

	reader := bytes.NewReader(encoded)
	_, err = decodeRecord(reader)
	if err == nil {
		t.Error("decodeRecord() should have failed with corrupted CRC")
	}
}

func TestWALOpenAndClose(t *testing.T) {
	dir := t.TempDir()

	cfg := &Config{
		Dir:         dir,
		SegmentSize: 1024 * 1024,
		SyncOnWrite: true,
	}

	wal, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if err := wal.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestWALWrite(t *testing.T) {
	dir := t.TempDir()

	cfg := &Config{
		Dir:         dir,
		SegmentSize: 1024 * 1024,
		SyncOnWrite: true,
	}

	wal, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer wal.Close()

	record := &Record{
		Type:       RecordTypeInsert,
		Collection: "test",
		Data:       []byte("test data"),
	}

	if err := wal.Write(record); err != nil {
		t.Errorf("Write() error = %v", err)
	}

	if record.SeqNum != 1 {
		t.Errorf("SeqNum = %d, want 1", record.SeqNum)
	}

	if wal.LastSeqNum() != 1 {
		t.Errorf("LastSeqNum() = %d, want 1", wal.LastSeqNum())
	}
}

func TestWALWriteBatch(t *testing.T) {
	dir := t.TempDir()

	cfg := &Config{
		Dir:         dir,
		SegmentSize: 1024 * 1024,
		SyncOnWrite: true,
	}

	wal, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer wal.Close()

	records := []*Record{
		{Type: RecordTypeInsert, Collection: "test", Data: []byte("data1")},
		{Type: RecordTypeInsert, Collection: "test", Data: []byte("data2")},
		{Type: RecordTypeInsert, Collection: "test", Data: []byte("data3")},
	}

	if err := wal.WriteBatch(records); err != nil {
		t.Errorf("WriteBatch() error = %v", err)
	}

	if wal.LastSeqNum() != 3 {
		t.Errorf("LastSeqNum() = %d, want 3", wal.LastSeqNum())
	}

	for i, r := range records {
		expectedSeq := uint64(i + 1)
		if r.SeqNum != expectedSeq {
			t.Errorf("Record[%d].SeqNum = %d, want %d", i, r.SeqNum, expectedSeq)
		}
	}
}

func TestWALSegmentRotation(t *testing.T) {
	dir := t.TempDir()

	// Very small segment size to force rotation
	cfg := &Config{
		Dir:         dir,
		SegmentSize: 200, // Small enough to trigger rotation
		SyncOnWrite: true,
	}

	wal, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer wal.Close()

	// Write enough records to trigger rotation
	for i := 0; i < 10; i++ {
		record := &Record{
			Type:       RecordTypeInsert,
			Collection: "test_collection",
			Data:       []byte("some test data that should trigger rotation"),
		}
		if err := wal.Write(record); err != nil {
			t.Errorf("Write() error = %v", err)
		}
	}

	// Check that segments were created
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}

	walFiles := 0
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".wal" {
			walFiles++
		}
	}

	if walFiles < 2 {
		t.Errorf("Expected at least 2 WAL segments, got %d", walFiles)
	}
}

func TestWALReplay(t *testing.T) {
	dir := t.TempDir()

	cfg := &Config{
		Dir:         dir,
		SegmentSize: 1024 * 1024,
		SyncOnWrite: true,
	}

	// Write records
	wal, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	expectedRecords := []*Record{
		{Type: RecordTypeInsert, Collection: "test1", Data: []byte("data1")},
		{Type: RecordTypeUpdate, Collection: "test2", Data: []byte("data2")},
		{Type: RecordTypeDelete, Collection: "test3", Data: []byte("data3")},
	}

	for _, r := range expectedRecords {
		if err := wal.Write(r); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	if err := wal.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Reopen and replay
	wal2, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer wal2.Close()

	var replayedRecords []*Record
	err = wal2.Replay(func(r *Record) error {
		replayedRecords = append(replayedRecords, r)
		return nil
	})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}

	if len(replayedRecords) != len(expectedRecords) {
		t.Errorf("Replayed %d records, want %d", len(replayedRecords), len(expectedRecords))
	}

	for i, r := range replayedRecords {
		if r.Type != expectedRecords[i].Type {
			t.Errorf("Record[%d].Type = %d, want %d", i, r.Type, expectedRecords[i].Type)
		}
		if r.Collection != expectedRecords[i].Collection {
			t.Errorf("Record[%d].Collection = %s, want %s", i, r.Collection, expectedRecords[i].Collection)
		}
		if !bytes.Equal(r.Data, expectedRecords[i].Data) {
			t.Errorf("Record[%d].Data mismatch", i)
		}
	}
}

func TestWALReplayAfterCrash(t *testing.T) {
	dir := t.TempDir()

	cfg := &Config{
		Dir:         dir,
		SegmentSize: 1024 * 1024,
		SyncOnWrite: true,
	}

	// Write records and simulate crash (close without proper shutdown)
	wal, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	for i := 0; i < 5; i++ {
		record := &Record{
			Type:       RecordTypeInsert,
			Collection: "test",
			Data:       []byte("crash test data"),
		}
		if err := wal.Write(record); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	// Force sync before "crash"
	if err := wal.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	// Close properly (in real crash scenario this wouldn't happen)
	if err := wal.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Reopen and verify replay works
	wal2, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer wal2.Close()

	count := 0
	err = wal2.Replay(func(r *Record) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}

	if count != 5 {
		t.Errorf("Replayed %d records, want 5", count)
	}
}

func TestWALConcurrentWrites(t *testing.T) {
	dir := t.TempDir()

	cfg := &Config{
		Dir:         dir,
		SegmentSize: 1024 * 1024,
		SyncOnWrite: false, // Disable sync for faster test
	}

	wal, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer wal.Close()

	const numGoroutines = 10
	const recordsPerGoroutine = 100

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*recordsPerGoroutine)

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < recordsPerGoroutine; i++ {
				record := &Record{
					Type:       RecordTypeInsert,
					Collection: "concurrent_test",
					Data:       []byte("concurrent data"),
				}
				if err := wal.Write(record); err != nil {
					errors <- err
				}
			}
		}(g)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent write error: %v", err)
	}

	expectedSeqNum := uint64(numGoroutines * recordsPerGoroutine)
	if wal.LastSeqNum() != expectedSeqNum {
		t.Errorf("LastSeqNum() = %d, want %d", wal.LastSeqNum(), expectedSeqNum)
	}
}

func TestWALTruncate(t *testing.T) {
	dir := t.TempDir()

	// Small segment size to create multiple segments
	cfg := &Config{
		Dir:         dir,
		SegmentSize: 200,
		SyncOnWrite: true,
	}

	wal, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	// Write enough to create multiple segments
	for i := 0; i < 20; i++ {
		record := &Record{
			Type:       RecordTypeInsert,
			Collection: "truncate_test",
			Data:       []byte("truncate test data payload"),
		}
		if err := wal.Write(record); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	if err := wal.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Reopen
	wal2, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer wal2.Close()

	// Count segments before truncate
	entriesBefore, _ := os.ReadDir(dir)
	walFilesBefore := 0
	for _, e := range entriesBefore {
		if filepath.Ext(e.Name()) == ".wal" {
			walFilesBefore++
		}
	}

	// Truncate old segments
	err = wal2.Truncate(10)
	if err != nil {
		t.Fatalf("Truncate() error = %v", err)
	}

	// Count segments after truncate
	entriesAfter, _ := os.ReadDir(dir)
	walFilesAfter := 0
	for _, e := range entriesAfter {
		if filepath.Ext(e.Name()) == ".wal" {
			walFilesAfter++
		}
	}

	// We should have fewer or equal segments after truncation
	if walFilesAfter > walFilesBefore {
		t.Errorf("Expected fewer segments after truncate: before=%d, after=%d", walFilesBefore, walFilesAfter)
	}
}

func TestSafeIntConversions(t *testing.T) {
	t.Run("safeIntToUint32", func(t *testing.T) {
		tests := []struct {
			input   int
			wantVal uint32
			wantErr bool
		}{
			{0, 0, false},
			{100, 100, false},
			{math.MaxUint32, math.MaxUint32, false},
			{-1, 0, true},
			{-100, 0, true},
		}

		for _, tt := range tests {
			val, err := safeIntToUint32(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("safeIntToUint32(%d) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && val != tt.wantVal {
				t.Errorf("safeIntToUint32(%d) = %d, want %d", tt.input, val, tt.wantVal)
			}
		}
	})

	t.Run("safeIntToUint16", func(t *testing.T) {
		tests := []struct {
			input   int
			wantVal uint16
			wantErr bool
		}{
			{0, 0, false},
			{100, 100, false},
			{math.MaxUint16, math.MaxUint16, false},
			{math.MaxUint16 + 1, 0, true},
			{-1, 0, true},
		}

		for _, tt := range tests {
			val, err := safeIntToUint16(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("safeIntToUint16(%d) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && val != tt.wantVal {
				t.Errorf("safeIntToUint16(%d) = %d, want %d", tt.input, val, tt.wantVal)
			}
		}
	})

	t.Run("safeInt64ToUint64", func(t *testing.T) {
		tests := []struct {
			input int64
			want  uint64
		}{
			{0, 0},
			{100, 100},
			{math.MaxInt64, math.MaxInt64},
			{-1, 0},
			{-100, 0},
		}

		for _, tt := range tests {
			got := safeInt64ToUint64(tt.input)
			if got != tt.want {
				t.Errorf("safeInt64ToUint64(%d) = %d, want %d", tt.input, got, tt.want)
			}
		}
	})

	t.Run("safeUint64ToInt64", func(t *testing.T) {
		tests := []struct {
			input uint64
			want  int64
		}{
			{0, 0},
			{100, 100},
			{uint64(math.MaxInt64), math.MaxInt64},
			{math.MaxUint64, math.MaxInt64}, // Overflow capped
		}

		for _, tt := range tests {
			got := safeUint64ToInt64(tt.input)
			if got != tt.want {
				t.Errorf("safeUint64ToInt64(%d) = %d, want %d", tt.input, got, tt.want)
			}
		}
	})
}

func TestEncodeRecordLargeCollection(t *testing.T) {
	// Test collection name that exceeds uint16 max
	largeCollection := make([]byte, math.MaxUint16+1)
	for i := range largeCollection {
		largeCollection[i] = 'a'
	}

	record := &Record{
		SeqNum:     1,
		Type:       RecordTypeInsert,
		Collection: string(largeCollection),
		Data:       []byte("test"),
	}

	_, err := encodeRecord(record)
	if err == nil {
		t.Error("encodeRecord() should fail with collection name too long")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Dir != "./data/wal" {
		t.Errorf("DefaultConfig().Dir = %s, want ./data/wal", cfg.Dir)
	}
	if cfg.SegmentSize != 64*1024*1024 {
		t.Errorf("DefaultConfig().SegmentSize = %d, want %d", cfg.SegmentSize, 64*1024*1024)
	}
	if !cfg.SyncOnWrite {
		t.Error("DefaultConfig().SyncOnWrite = false, want true")
	}
}

func TestWALSync(t *testing.T) {
	dir := t.TempDir()

	cfg := &Config{
		Dir:         dir,
		SegmentSize: 1024 * 1024,
		SyncOnWrite: false, // Disable auto-sync
	}

	wal, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer wal.Close()

	record := &Record{
		Type:       RecordTypeInsert,
		Collection: "test",
		Data:       []byte("sync test"),
	}

	if err := wal.Write(record); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// Manual sync
	if err := wal.Sync(); err != nil {
		t.Errorf("Sync() error = %v", err)
	}
}

func TestDecodeRecordTruncatedInput(t *testing.T) {
	record := &Record{
		SeqNum:     1,
		Type:       RecordTypeInsert,
		Collection: "test",
		Data:       []byte("test data"),
		Timestamp:  time.Now().UnixNano(),
	}

	encoded, err := encodeRecord(record)
	if err != nil {
		t.Fatalf("encodeRecord() error = %v", err)
	}

	// Test with truncated length header
	reader := bytes.NewReader(encoded[:2])
	_, err = decodeRecord(reader)
	if err != io.ErrUnexpectedEOF && err != io.EOF {
		t.Errorf("Expected EOF error for truncated header, got: %v", err)
	}

	// Test with truncated payload
	reader = bytes.NewReader(encoded[:10])
	_, err = decodeRecord(reader)
	if err != io.ErrUnexpectedEOF && err != io.EOF {
		t.Errorf("Expected EOF error for truncated payload, got: %v", err)
	}
}

func TestRecordCRCCalculation(t *testing.T) {
	record := &Record{
		SeqNum:     1,
		Type:       RecordTypeInsert,
		Collection: "test",
		Data:       []byte("crc test"),
		Timestamp:  12345678,
	}

	encoded, err := encodeRecord(record)
	if err != nil {
		t.Fatalf("encodeRecord() error = %v", err)
	}

	// Extract the stored CRC
	storedCRC := binary.LittleEndian.Uint32(encoded[len(encoded)-4:])

	// Calculate expected CRC (everything between length field and CRC)
	length := binary.LittleEndian.Uint32(encoded[:4])
	payload := encoded[4 : 4+length]
	expectedCRC := crc32.ChecksumIEEE(payload)

	if storedCRC != expectedCRC {
		t.Errorf("Stored CRC = %d, expected %d", storedCRC, expectedCRC)
	}
}

func BenchmarkWALWrite(b *testing.B) {
	dir := b.TempDir()

	cfg := &Config{
		Dir:         dir,
		SegmentSize: 64 * 1024 * 1024,
		SyncOnWrite: false,
	}

	wal, err := Open(cfg)
	if err != nil {
		b.Fatalf("Open() error = %v", err)
	}
	defer wal.Close()

	record := &Record{
		Type:       RecordTypeInsert,
		Collection: "benchmark",
		Data:       []byte("benchmark data payload for performance testing"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := wal.Write(record); err != nil {
			b.Fatalf("Write() error = %v", err)
		}
	}
}

func BenchmarkEncodeRecord(b *testing.B) {
	record := &Record{
		SeqNum:     1,
		Type:       RecordTypeInsert,
		Collection: "benchmark",
		Data:       []byte("benchmark data payload"),
		Timestamp:  time.Now().UnixNano(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := encodeRecord(record)
		if err != nil {
			b.Fatalf("encodeRecord() error = %v", err)
		}
	}
}

func BenchmarkDecodeRecord(b *testing.B) {
	record := &Record{
		SeqNum:     1,
		Type:       RecordTypeInsert,
		Collection: "benchmark",
		Data:       []byte("benchmark data payload"),
		Timestamp:  time.Now().UnixNano(),
	}

	encoded, err := encodeRecord(record)
	if err != nil {
		b.Fatalf("encodeRecord() error = %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(encoded)
		_, err := decodeRecord(reader)
		if err != nil {
			b.Fatalf("decodeRecord() error = %v", err)
		}
	}
}
