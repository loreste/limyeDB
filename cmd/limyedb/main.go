package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/limyedb/limyedb/api/grpc"
	"github.com/limyedb/limyedb/api/rest"
	"github.com/limyedb/limyedb/pkg/cluster"
	"github.com/limyedb/limyedb/pkg/collection"
	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/storage/snapshot"
	"github.com/limyedb/limyedb/pkg/storage/wal"
	"github.com/limyedb/limyedb/pkg/version"
)

const (
	banner = `
 _     _                     ____  ____
| |   (_)_ __ ___  _   _  __|  _ \| __ )
| |   | | '_ ` + "`" + ` _ \| | | |/ _ \ | | |  _ \
| |___| | | | | | | |_| |  __/ |_| | |_) |
|_____|_|_| |_| |_|\__, |\___|____/|____/
                   |___/
High-Performance Vector Database
`
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "", "Path to configuration file")
	dataDir := flag.String("data", "./data", "Data directory")
	restAddr := flag.String("rest", ":8080", "REST API address")
	grpcAddr := flag.String("grpc", ":50051", "gRPC API address")

	raftBind := flag.String("raft-bind", "", "Raft TCP Bind Address (e.g. 127.0.0.1:7000)")
	raftData := flag.String("raft-data", "", "Raft Data Directory")
	raftNodeID := flag.String("raft-node-id", "node0", "Raft Node ID")
	raftBootstrap := flag.Bool("raft-bootstrap", false, "Bootstrap this node as the first leader of the cluster")
	raftJoinAddr := flag.String("raft-join", "", "Address of an existing raft node to join (e.g., http://127.0.0.1:8080)")

	// Security Configuration
	authToken := flag.String("auth-token", "", "Master bearer token for API authentication and JWT signing. Required unless -allow-anonymous is passed.")
	allowAnonymous := flag.Bool("allow-anonymous", false, "Run without authentication. WARNING: every REST and gRPC endpoint becomes publicly mutable. Required to launch when -auth-token is empty.")
	tlsCert := flag.String("tls-cert", "", "Path to HTTPS TLS certificate file")
	tlsKey := flag.String("tls-key", "", "Path to HTTPS TLS private key file")

	showVersion := flag.Bool("version", false, "Show version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("LimyeDB version %s\n", version.String())
		return
	}

	// Initialize Enterprise JSON Logger natively
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	slog.Info(fmt.Sprintf("\n%s\nVersion: %s", banner, version.Version))

	// Refuse to start without an auth decision. Without this guard, the
	// REST and gRPC servers would run wide open whenever -auth-token was
	// not supplied (the historical behavior) and any caller on the
	// network could create, mutate, or delete collections. Operators who
	// genuinely want an unauthenticated server must opt in explicitly.
	if *authToken == "" && !*allowAnonymous {
		slog.Error("authentication is required: pass -auth-token <secret> to enable JWT/Bearer auth, or pass -allow-anonymous to run without authentication (NOT recommended)")
		os.Exit(1)
	}
	if *allowAnonymous && *authToken == "" {
		slog.Warn("running with -allow-anonymous: REST and gRPC are unauthenticated; do not expose this process to an untrusted network")
	}

	// Load configuration
	var cfg *config.Config
	var err error

	if *configPath != "" {
		cfg, err = config.Load(*configPath)
		if err != nil {
			slog.Error("Failed to load config", "error", err)
			os.Exit(1)
		}
	} else {
		cfg = config.DefaultConfig()
	}

	// Override with command line flags
	if *dataDir != "" {
		cfg.Storage.DataDir = *dataDir
	}
	if *restAddr != "" {
		cfg.Server.RESTAddress = *restAddr
	}
	if *grpcAddr != "" {
		cfg.Server.GRPCAddress = *grpcAddr
	}

	// Initialize storage directories with restricted permissions
	if err := os.MkdirAll(cfg.Storage.DataDir, 0750); err != nil {
		slog.Error("Failed to create data directory", "error", err, "path", cfg.Storage.DataDir)
		os.Exit(1)
	}

	// Initialize WAL if enabled
	var walInstance *wal.WAL
	if cfg.WAL.Enabled {
		walDir := cfg.WAL.Dir
		if walDir == "" {
			walDir = cfg.Storage.DataDir + "/wal"
		}
		walInstance, err = wal.Open(&wal.Config{
			Dir:            walDir,
			SegmentSize:    int64(cfg.WAL.SegmentSizeMB) * 1024 * 1024,
			SyncOnWrite:    cfg.WAL.SyncOnWrite,
			AsyncEnabled:   cfg.WAL.AsyncEnabled,
			AsyncBatchSize: cfg.WAL.AsyncBatchSize,
			AsyncInterval:  time.Duration(cfg.WAL.AsyncIntervalMs) * time.Millisecond,
		})
		if err != nil {
			slog.Error("Failed to initialize WAL", "error", err)
			os.Exit(1)
		}
		slog.Info("WAL initialized", "dir", walDir, "syncOnWrite", cfg.WAL.SyncOnWrite, "asyncEnabled", cfg.WAL.AsyncEnabled)
	}

	collMgr, err := collection.NewManager(&collection.ManagerConfig{
		DataDir:        cfg.Storage.DataDir + "/collections",
		MaxCollections: cfg.Storage.MaxCollections,
		WAL:            walInstance,
	})
	if err != nil {
		slog.Error("Failed to initialize collection manager", "error", err)
		os.Exit(1)
	}

	// Run recovery BEFORE starting servers
	if err := collMgr.Recover(); err != nil {
		slog.Error("Recovery failed", "error", err)
		os.Exit(1)
	}

	snapMgr, err := snapshot.NewManager(&snapshot.Config{
		Dir:              cfg.Snapshot.Dir,
		RetainCount:      cfg.Snapshot.RetainCount,
		CompressionLevel: cfg.Snapshot.CompressionLvl,
	})
	if err != nil {
		slog.Error("Failed to initialize snapshot manager", "error", err)
		os.Exit(1)
	}

	// Initialize Raft if configured
	var raftNode *cluster.RaftNode
	if *raftBind != "" {
		rcfg := &cluster.RaftConfig{
			NodeID:   *raftNodeID,
			BindAddr: *raftBind,
			DataDir:  *raftData,
			IsLeader: *raftBootstrap,
		}
		if *raftData == "" {
			rcfg.DataDir = cfg.Storage.DataDir + "/raft" // Default to native data dir subdirectory
		}

		rn, err := cluster.NewRaftNode(rcfg, collMgr, snapMgr)
		if err != nil {
			slog.Error("Failed to initialize Raft subsystem", "error", err)
			os.Exit(1)
		}
		raftNode = rn
		slog.Info("Raft subsystem bound", "node", *raftNodeID, "bind", *raftBind)

		if *raftJoinAddr != "" {
			slog.Info("Configured to join cluster", "node", *raftNodeID, "joinAddr", *raftJoinAddr)

			// Dial asynchronously once the local HTTP server has booted
			go func() {
				time.Sleep(3 * time.Second)
				payload := map[string]string{
					"node_id":   *raftNodeID,
					"raft_addr": *raftBind,
				}
				body, _ := json.Marshal(payload)
				resp, err := http.Post(*raftJoinAddr+"/cluster/join", "application/json", bytes.NewReader(body))
				if err != nil {
					slog.Error("Failed to join cluster", "joinAddr", *raftJoinAddr, "error", err)
					return
				}
				defer resp.Body.Close()
				if resp.StatusCode != 200 {
					slog.Error("Cluster join rejected", "joinAddr", *raftJoinAddr, "status", resp.StatusCode)
				} else {
					slog.Info("Successfully joined the distributed LimyeDB cluster", "joinAddr", *raftJoinAddr)
				}
			}()
		}
	}

	// Start REST server
	restServer := rest.NewServerWithOptions(&cfg.Server, collMgr, &rest.ServerOptions{
		Addr:      cfg.Server.RESTAddress,
		Snapshots: snapMgr,
		Raft:      raftNode,
		AuthToken: *authToken,
		TLSCert:   *tlsCert,
		TLSKey:    *tlsKey,
	})
	go func() {
		slog.Info("Starting REST API server", "address", cfg.Server.RESTAddress)
		if err := restServer.Start(); err != nil {
			slog.Error("REST server error", "error", err)
		}
	}()

	// Start gRPC server
	grpcServer := grpc.NewServer(&cfg.Server, collMgr, snapMgr, *authToken)
	go func() {
		slog.Info("Starting gRPC API server", "address", cfg.Server.GRPCAddress)
		if err := grpcServer.Start(); err != nil {
			slog.Error("gRPC server error", "error", err)
		}
	}()

	slog.Info("LimyeDB Ready", "rest", "http://localhost"+cfg.Server.RESTAddress, "grpc", "localhost"+cfg.Server.GRPCAddress)

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down LimyeDB...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.WriteTimeout)
	defer cancel()

	// Step 1: Stop accepting new requests
	if err := restServer.Stop(ctx); err != nil {
		slog.Error("REST server shutdown error", "error", err)
	}
	grpcServer.Stop()

	// Step 2: Sync WAL to disk
	if walInstance != nil {
		slog.Info("Syncing WAL to disk")
		if err := walInstance.Sync(); err != nil {
			slog.Error("Failed to sync WAL", "error", err)
		}
	}

	// Step 3: Save index metadata for all collections
	slog.Info("Saving index metadata")
	if err := collMgr.SaveAllIndexMetadata(); err != nil {
		slog.Error("Failed to save index metadata", "error", err)
	}

	// Step 4: Flush and close collection manager
	if err := collMgr.Flush(); err != nil {
		slog.Error("Failed to flush collections", "error", err)
	}
	if err := collMgr.Close(); err != nil {
		slog.Error("Failed to close collection manager", "error", err)
	}

	// Step 5: Close WAL
	if walInstance != nil {
		slog.Info("Closing WAL")
		if err := walInstance.Close(); err != nil {
			slog.Error("Failed to close WAL", "error", err)
		}
	}

	slog.Info("LimyeDB shutdown complete")
}
