package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackup_CreateAndRestore(t *testing.T) {
	// Create temp directories
	dataDir, err := os.MkdirTemp("", "backup-test-data")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dataDir)

	backupDir, err := os.MkdirTemp("", "backup-test-output")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(backupDir)

	// Create test data
	collDir := filepath.Join(dataDir, "test_collection")
	os.MkdirAll(collDir, 0755)
	os.WriteFile(filepath.Join(collDir, "data.json"), []byte(`{"test": "data"}`), 0644)

	// Create backup
	b := NewBackup(dataDir)
	backupPath := filepath.Join(backupDir, "backup.tar.gz")

	meta, err := b.Create(backupPath, DefaultBackupOptions())
	if err != nil {
		t.Errorf("failed to create backup: %v", err)
	}

	if meta == nil {
		t.Fatal("expected metadata")
	}

	if meta.ID == "" {
		t.Error("expected backup ID")
	}

	// Verify backup file exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("backup file not created")
	}

	// Create new restore directory
	restoreDir, err := os.MkdirTemp("", "backup-test-restore")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(restoreDir)

	// Restore backup
	restoreBackup := NewBackup(restoreDir)
	restoredMeta, err := restoreBackup.Restore(backupPath, RestoreOptions{OverwriteExisting: true})
	if err != nil {
		t.Errorf("failed to restore backup: %v", err)
	}

	if restoredMeta == nil {
		t.Fatal("expected restored metadata")
	}

	// Verify restored file
	restoredFile := filepath.Join(restoreDir, "test_collection", "data.json")
	if _, err := os.Stat(restoredFile); os.IsNotExist(err) {
		t.Error("restored file not found")
	}
}

func TestBackup_CreateWithoutCompression(t *testing.T) {
	dataDir, err := os.MkdirTemp("", "backup-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dataDir)

	// Create test data
	os.WriteFile(filepath.Join(dataDir, "data.json"), []byte(`{"test": "data"}`), 0644)

	backupDir, err := os.MkdirTemp("", "backup-output")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(backupDir)

	b := NewBackup(dataDir)
	backupPath := filepath.Join(backupDir, "backup.tar")

	opts := DefaultBackupOptions()
	opts.Compress = false

	meta, err := b.Create(backupPath, opts)
	if err != nil {
		t.Errorf("failed to create uncompressed backup: %v", err)
	}

	if meta == nil {
		t.Fatal("expected metadata")
	}
}

func TestBackup_CollectionFilter(t *testing.T) {
	dataDir, err := os.MkdirTemp("", "backup-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dataDir)

	// Create multiple collections
	for _, coll := range []string{"coll1", "coll2", "coll3"} {
		collDir := filepath.Join(dataDir, coll)
		os.MkdirAll(collDir, 0755)
		os.WriteFile(filepath.Join(collDir, "data.json"), []byte(`{}`), 0644)
	}

	backupDir, err := os.MkdirTemp("", "backup-output")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(backupDir)

	b := NewBackup(dataDir)
	backupPath := filepath.Join(backupDir, "backup.tar.gz")

	opts := DefaultBackupOptions()
	opts.Collections = []string{"coll1", "coll2"} // Only backup coll1 and coll2

	_, err = b.Create(backupPath, opts)
	if err != nil {
		t.Errorf("failed to create filtered backup: %v", err)
	}
}

func TestBackup_ReadMetadata(t *testing.T) {
	dataDir, err := os.MkdirTemp("", "backup-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dataDir)

	os.WriteFile(filepath.Join(dataDir, "data.json"), []byte(`{}`), 0644)

	backupDir, err := os.MkdirTemp("", "backup-output")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(backupDir)

	b := NewBackup(dataDir)
	backupPath := filepath.Join(backupDir, "backup.tar.gz")

	created, err := b.Create(backupPath, DefaultBackupOptions())
	if err != nil {
		t.Fatal(err)
	}

	// Read metadata without full restore
	read, err := b.ReadMetadata(backupPath)
	if err != nil {
		t.Errorf("failed to read metadata: %v", err)
	}

	if read.ID != created.ID {
		t.Errorf("metadata ID mismatch: expected %s, got %s", created.ID, read.ID)
	}
}

func TestBackup_RestoreWithoutOverwrite(t *testing.T) {
	dataDir, err := os.MkdirTemp("", "backup-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dataDir)

	os.WriteFile(filepath.Join(dataDir, "data.json"), []byte(`{"original": true}`), 0644)

	backupDir, err := os.MkdirTemp("", "backup-output")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(backupDir)

	b := NewBackup(dataDir)
	backupPath := filepath.Join(backupDir, "backup.tar.gz")

	b.Create(backupPath, DefaultBackupOptions())

	// Modify original file
	os.WriteFile(filepath.Join(dataDir, "data.json"), []byte(`{"modified": true}`), 0644)

	// Restore without overwrite
	b.Restore(backupPath, RestoreOptions{OverwriteExisting: false})

	// File should still have modified content
	content, _ := os.ReadFile(filepath.Join(dataDir, "data.json"))
	if string(content) != `{"modified": true}` {
		t.Error("file should not be overwritten")
	}
}

func TestExportJSON(t *testing.T) {
	dataDir, err := os.MkdirTemp("", "export-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dataDir)

	outputPath := filepath.Join(dataDir, "export.json")
	err = ExportJSON(dataDir, "test_collection", outputPath)
	if err != nil {
		t.Errorf("failed to export JSON: %v", err)
	}

	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("export file not created")
	}
}

func TestImportJSON(t *testing.T) {
	dataDir, err := os.MkdirTemp("", "import-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dataDir)

	// Create import file
	importPath := filepath.Join(dataDir, "import.json")
	content := `{"points": [{"id": "1"}, {"id": "2"}, {"id": "3"}]}`
	os.WriteFile(importPath, []byte(content), 0644)

	count, err := ImportJSON(dataDir, "test_collection", importPath)
	if err != nil {
		t.Errorf("failed to import JSON: %v", err)
	}

	if count != 3 {
		t.Errorf("expected 3 points imported, got %d", count)
	}
}

func TestDefaultBackupOptions(t *testing.T) {
	opts := DefaultBackupOptions()

	if !opts.IncludeIndexes {
		t.Error("expected IncludeIndexes to be true by default")
	}
	if !opts.Compress {
		t.Error("expected Compress to be true by default")
	}
	if opts.Collections != nil {
		t.Error("expected Collections to be nil by default")
	}
}

func TestIsIndexFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"data.hnsw", true},
		{"data.idx", true},
		{"data.json", false},
		{"collection/vectors.hnsw", true},
	}

	for _, tc := range tests {
		result := isIndexFile(tc.path)
		if result != tc.expected {
			t.Errorf("isIndexFile(%s) = %v, expected %v", tc.path, result, tc.expected)
		}
	}
}

func TestContains(t *testing.T) {
	slice := []string{"a", "b", "c"}

	if !contains(slice, "a") {
		t.Error("expected to find 'a'")
	}
	if !contains(slice, "c") {
		t.Error("expected to find 'c'")
	}
	if contains(slice, "d") {
		t.Error("should not find 'd'")
	}
}

// TestSanitizeTarPath tests the Zip Slip protection
func TestSanitizeTarPath(t *testing.T) {
	baseDir := "/data/backup"

	tests := []struct {
		name      string
		tarPath   string
		expectErr bool
	}{
		{"normal path", "collection/data.json", false},
		{"nested path", "a/b/c/file.txt", false},
		{"simple traversal", "../etc/passwd", true},
		{"hidden traversal", "foo/../../../etc/passwd", true},
		{"absolute path escape", "/etc/passwd", true},
		{"traversal in middle", "foo/../../etc/passwd", true},
		{"double dot filename ok", "collection/file..name.txt", false},
		{"parent dir literal", "collection/../other/file.txt", true},
		{"trailing parent", "collection/..", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := sanitizeTarPath(baseDir, tc.tarPath)
			if tc.expectErr && err == nil {
				t.Errorf("expected error for path %q", tc.tarPath)
			}
			if !tc.expectErr && err != nil {
				t.Errorf("unexpected error for path %q: %v", tc.tarPath, err)
			}
		})
	}
}

// TestSanitizeTarPathResult verifies the sanitized path stays within baseDir
func TestSanitizeTarPathResult(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "sanitize-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)

	result, err := sanitizeTarPath(baseDir, "collection/data.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := filepath.Join(baseDir, "collection/data.json")
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func BenchmarkBackup_Create(b *testing.B) {
	dataDir, _ := os.MkdirTemp("", "bench-data")
	defer os.RemoveAll(dataDir)

	// Create test data
	for i := 0; i < 100; i++ {
		collDir := filepath.Join(dataDir, "collection")
		os.MkdirAll(collDir, 0755)
		content := make([]byte, 1024)
		os.WriteFile(filepath.Join(collDir, "data.json"), content, 0644)
	}

	backupDir, _ := os.MkdirTemp("", "bench-output")
	defer os.RemoveAll(backupDir)

	bk := NewBackup(dataDir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backupPath := filepath.Join(backupDir, "backup.tar.gz")
		bk.Create(backupPath, DefaultBackupOptions())
		os.Remove(backupPath)
	}
}
