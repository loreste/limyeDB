package snapshot

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	m, err := NewManager(&Config{
		Dir:              t.TempDir(),
		RetainCount:      5,
		CompressionLevel: gzip.NoCompression,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m
}

// TestFinishLeavesNoTmpFiles verifies that after a successful Finish,
// the .snap.tmp and .meta.tmp staging files are gone and the published
// .snap + .meta pair is in place.
func TestFinishLeavesNoTmpFiles(t *testing.T) {
	m := newTestManager(t)
	sw, err := m.CreateSnapshot([]string{"docs"})
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if err := sw.WriteHeader(1, 0); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}
	snap, err := sw.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// Final files exist.
	if _, err := os.Stat(snap.Path); err != nil {
		t.Errorf(".snap missing: %v", err)
	}
	metaPath := snap.Path + ".meta"
	if _, err := os.Stat(metaPath); err != nil {
		t.Errorf(".meta missing: %v", err)
	}

	// No tmp files lingered.
	tmps, _ := filepath.Glob(filepath.Join(m.Dir(), "*.tmp"))
	if len(tmps) > 0 {
		t.Errorf("tmp files lingered after Finish: %v", tmps)
	}
}

// TestCancelRemovesTmpFile verifies Cancel cleans up the in-flight
// .snap.tmp and produces no .snap or .meta in the snapshots directory.
func TestCancelRemovesTmpFile(t *testing.T) {
	m := newTestManager(t)
	sw, err := m.CreateSnapshot([]string{"docs"})
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	tmpPath := sw.tmpPath
	if _, err := os.Stat(tmpPath); err != nil {
		t.Fatalf(".snap.tmp must exist while writing: %v", err)
	}

	if err := sw.Cancel(); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf(".snap.tmp must be removed by Cancel (err=%v)", err)
	}
	snaps, _ := filepath.Glob(filepath.Join(m.Dir(), "*.snap"))
	for _, p := range snaps {
		if !strings.HasSuffix(p, ".tmp") {
			t.Errorf("Cancel must not leave a published .snap: %v", p)
		}
	}
	metas, _ := filepath.Glob(filepath.Join(m.Dir(), "*.meta"))
	if len(metas) != 0 {
		t.Errorf("Cancel must not leave a .meta: %v", metas)
	}
}

// TestListIgnoresTmpFiles simulates a partial-write crash by dropping a
// stray .meta.tmp into the snapshot directory and verifies List returns
// only the fully-published snapshot. The .meta.tmp would cause silent
// listing of an incomplete snapshot if List filtered loosely.
func TestListIgnoresTmpFiles(t *testing.T) {
	m := newTestManager(t)

	// Publish one real snapshot.
	sw, err := m.CreateSnapshot([]string{"a"})
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if err := sw.WriteHeader(1, 0); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}
	if _, err := sw.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// Plant a stray .meta.tmp that would have been left behind by a
	// crashed save. List must NOT pick it up as a snapshot.
	stray := filepath.Join(m.Dir(), "snap_99999.snap.meta.tmp")
	if err := os.WriteFile(stray, []byte(`{"id":"snap_99999"}`), 0600); err != nil {
		t.Fatalf("plant stray: %v", err)
	}

	snaps, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(snaps) != 1 {
		t.Errorf("List returned %d snapshots, want 1 (stray .meta.tmp must be ignored)", len(snaps))
	}
}
