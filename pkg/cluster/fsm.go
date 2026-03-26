package cluster

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	"github.com/limyedb/limyedb/pkg/collection"
	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/point"
	"github.com/limyedb/limyedb/pkg/storage/snapshot"
)

type OpType string

const (
	OpCreateCollection OpType = "create_collection"
	OpDeleteCollection OpType = "delete_collection"
	OpUpsertPoints     OpType = "upsert_points"
	OpDeletePoints     OpType = "delete_points"
	OpSetLeaderRest    OpType = "set_leader_rest"
)

type Command struct {
	Op   OpType          `json:"op"`
	Data json.RawMessage `json:"data"`
}

type SetLeaderRestData struct {
	RestAddr string `json:"rest_addr"`
}

type CreateCollectionData struct {
	Config *config.CollectionConfig `json:"config"`
}

type DeleteCollectionData struct {
	Name string `json:"name"`
}

type UpsertPointsData struct {
	CollectionName string         `json:"collection_name"`
	Points         []*point.Point `json:"points"`
}

type DeletePointsData struct {
	CollectionName string   `json:"collection_name"`
	IDs            []string `json:"ids"`
}

// FSM implements the raft.FSM interface.
type FSM struct {
	manager *collection.Manager
	snapMgr *snapshot.Manager
	rn      *RaftNode
}

// NewFSM creates a new FSM wrapping the LimyeDB collection manager.
func NewFSM(manager *collection.Manager, snapMgr *snapshot.Manager) *FSM {
	return &FSM{manager: manager, snapMgr: snapMgr}
}

// SetRaftNode links the FSM closely with the wrapper to manipulate atomic network caches.
func (f *FSM) SetRaftNode(rn *RaftNode) {
	f.rn = rn
}

// Apply is invoked by Raft once a log entry is committed.
func (f *FSM) Apply(logEntry *raft.Log) interface{} {
	var cmd Command
	if err := json.Unmarshal(logEntry.Data, &cmd); err != nil {
		slog.Error("FSM Apply failed to unmarshal command", "err", err)
		return nil
	}

	switch cmd.Op {
	case OpSetLeaderRest:
		var data SetLeaderRestData
		if err := json.Unmarshal(cmd.Data, &data); err != nil {
			return err
		}
		if f.rn != nil {
			f.rn.SetLeaderRestAddr(data.RestAddr)
		}
		return nil

	case OpCreateCollection:
		var data CreateCollectionData
		if err := json.Unmarshal(cmd.Data, &data); err != nil {
			return err
		}
		if !f.manager.Exists(data.Config.Name) {
			if _, err := f.manager.Create(data.Config); err != nil {
				return err
			}
		}
		return nil

	case OpDeleteCollection:
		var data DeleteCollectionData
		if err := json.Unmarshal(cmd.Data, &data); err != nil {
			return err
		}
		if f.manager.Exists(data.Name) {
			if err := f.manager.Delete(data.Name); err != nil {
				return err
			}
		}
		return nil

	case OpUpsertPoints:
		var data UpsertPointsData
		if err := json.Unmarshal(cmd.Data, &data); err != nil {
			return err
		}
		coll, err := f.manager.Get(data.CollectionName)
		if err != nil {
			return err
		}
		_, err = coll.InsertBatch(data.Points)
		return err

	case OpDeletePoints:
		var data DeletePointsData
		if err := json.Unmarshal(cmd.Data, &data); err != nil {
			return err
		}
		coll, err := f.manager.Get(data.CollectionName)
		if err != nil {
			return err
		}
		for _, id := range data.IDs {
			_ = coll.Delete(id)
		}
		return nil

	default:
		return fmt.Errorf("unknown FSM command op: %s", cmd.Op)
	}
}

// Snapshot is used to support log compaction.
func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	// LimyeDB's Snapshot manager handles writing the entire Vector Graph to disk natively.
	// We dynamically map this to generate an immutable point-in-time `.snap`.
	snap, err := f.manager.CreateSnapshot(f.snapMgr)
	if err != nil {
		return nil, fmt.Errorf("failed creating internal db snapshot: %w", err)
	}

	return &fsmSnapshot{
		snap:    snap,
		snapMgr: f.snapMgr,
	}, nil
}

// Restore is used to load an FSM from a snapshot.
func (f *FSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()

	// Raft provides a stream containing the exact bytes of a `.snap` file.
	// Let's write this stream to a local physical file to hook into the snapshot un-marshaler natively
	tempID := fmt.Sprintf("raft_restore_%d", time.Now().UnixNano())
	tempPath := filepath.Join(f.snapMgr.Dir(), tempID+".snap")

	file, err := os.Create(tempPath)
	if err != nil {
		return err
	}
	defer os.Remove(tempPath) // Always clean up temporary unmarshaling files
	defer os.Remove(filepath.Join(f.snapMgr.Dir(), tempID+".snap.meta"))

	if _, err := io.Copy(file, rc); err != nil {
		_ = file.Close() // Best effort close on copy error
		return err
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close snapshot file: %w", err)
	}

	// Rehydrate LimyeDB structures securely
	if err := f.manager.RestoreSnapshot(f.snapMgr, tempID); err != nil {
		return fmt.Errorf("raft restore snapshot failed: %w", err)
	}

	return nil
}

type fsmSnapshot struct {
	snap    *snapshot.Snapshot
	snapMgr *snapshot.Manager
}

func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	file, err := os.Open(s.snap.Path)
	if err != nil {
		_ = sink.Cancel() // Best effort cancel on open error
		return err
	}
	defer file.Close()

	// Stream internal representation cleanly out to the underlying TCP Raft clusters
	if _, err := io.Copy(sink, file); err != nil {
		_ = sink.Cancel() // Best effort cancel on copy error
		return err
	}

	return sink.Close()
}

func (s *fsmSnapshot) Release() {
	// Allow LimyeDB's internal `Prune()` logic or background jobs to silently sweep disk
}
