package grpc

import (
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
	"time"

	"github.com/limyedb/limyedb/pkg/collection"
	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/storage/snapshot"
	pb "github.com/limyedb/limyedb/api/grpc/proto"
)

// Server represents the gRPC server
type Server struct {
	grpcServer  *grpc.Server
	config      *config.ServerConfig
	collections *collection.Manager
	snapshots   *snapshot.Manager
	listener    net.Listener
}

// NewServer creates a new gRPC server
func NewServer(cfg *config.ServerConfig, collections *collection.Manager, snapshots *snapshot.Manager) *Server {
	// Create gRPC server with options
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(int(cfg.MaxRequestSize)),
		grpc.MaxSendMsgSize(int(cfg.MaxRequestSize)),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     15 * time.Minute, // Evict idle connections to prevent memory starvation
			MaxConnectionAge:      30 * time.Minute, // Aggressively cycle connections checking load balancers scaling
			MaxConnectionAgeGrace: 5 * time.Minute,
			Time:                  5 * time.Minute,  // Ping the client if idle 
			Timeout:               1 * time.Second,  // Wait 1 second for ping back
		}),
	}

	grpcServer := grpc.NewServer(opts...)

	s := &Server{
		grpcServer:  grpcServer,
		config:      cfg,
		collections: collections,
		snapshots:   snapshots,
	}

	// Register service
	service := NewLimyeDBService(collections, snapshots)
	pb.RegisterLimyeDBServer(grpcServer, service)

	// Enable reflection for debugging
	reflection.Register(grpcServer)

	return s
}

// Start starts the gRPC server
func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.config.GRPCAddress)
	if err != nil {
		return err
	}
	s.listener = listener

	return s.grpcServer.Serve(listener)
}

// Stop gracefully stops the gRPC server
func (s *Server) Stop() {
	s.grpcServer.GracefulStop()
	if s.listener != nil {
		_ = s.listener.Close() // Error intentionally ignored during shutdown
	}
}
