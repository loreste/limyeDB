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
	version = "0.1.0"
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
	authToken := flag.String("auth-token", "", "Master bearer token for zero-trust API authentication")
	tlsCert := flag.String("tls-cert", "", "Path to HTTPS TLS certificate file")
	tlsKey := flag.String("tls-key", "", "Path to HTTPS TLS private key file")
	
	showVersion := flag.Bool("version", false, "Show version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("LimyeDB version %s\n", version)
		return
	}

	// Initialize Enterprise JSON Logger natively
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	slog.Info(fmt.Sprintf("\n%s\nVersion: %s", banner, version))

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

	collMgr, err := collection.NewManager(&collection.ManagerConfig{
		DataDir:        cfg.Storage.DataDir + "/collections",
		MaxCollections: cfg.Storage.MaxCollections,
	})
	if err != nil {
		slog.Error("Failed to initialize collection manager", "error", err)
		os.Exit(1)
	}

	snapMgr, err := snapshot.NewManager(&snapshot.Config{
		Dir:             cfg.Snapshot.Dir,
		RetainCount:     cfg.Snapshot.RetainCount,
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
		Addr:        cfg.Server.RESTAddress,
		Snapshots:   snapMgr,
		Raft:        raftNode,
		AuthToken:   *authToken,
		TLSCert:     *tlsCert,
		TLSKey:      *tlsKey,
	})
	go func() {
		slog.Info("Starting REST API server", "address", cfg.Server.RESTAddress)
		if err := restServer.Start(); err != nil {
			slog.Error("REST server error", "error", err)
		}
	}()

	// Start gRPC server
	grpcServer := grpc.NewServer(&cfg.Server, collMgr, snapMgr)
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

	// Stop servers
	if err := restServer.Stop(ctx); err != nil {
		slog.Error("REST server shutdown error", "error", err)
	}
	grpcServer.Stop()

	// Flush and close collection manager
	if err := collMgr.Flush(); err != nil {
		slog.Error("Failed to flush collections", "error", err)
	}
	if err := collMgr.Close(); err != nil {
		slog.Error("Failed to close collection manager", "error", err)
	}

	slog.Info("LimyeDB shutdown complete")
}
