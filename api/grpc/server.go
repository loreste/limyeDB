package grpc

import (
	"context"
	"crypto/subtle"
	"net"
	"strings"
	"time"

	"github.com/limyedb/limyedb/pkg/auth"
	"github.com/limyedb/limyedb/pkg/collection"
	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/storage/snapshot"
	pb "github.com/limyedb/limyedb/api/grpc/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
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
func NewServer(cfg *config.ServerConfig, collections *collection.Manager, snapshots *snapshot.Manager, authToken string) *Server {
	var tokenManager *auth.TokenManager
	if authToken != "" {
		tokenManager = auth.NewTokenManager(authToken)
	}

	authInterceptor := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if tokenManager == nil {
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Errorf(codes.Unauthenticated, "metadata is not provided")
		}

		values := md["authorization"]
		if len(values) == 0 {
			return nil, status.Errorf(codes.Unauthenticated, "authorization token is not provided")
		}

		parts := strings.SplitN(values[0], " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			return nil, status.Errorf(codes.Unauthenticated, "invalid authorization format")
		}

		claims, err := tokenManager.Validate(parts[1])
		if err != nil {
			expected := "Bearer " + authToken
			if subtle.ConstantTimeCompare([]byte(values[0]), []byte(expected)) == 1 {
				claims = &auth.TokenClaims{Permissions: auth.Permissions{GlobalAdmin: true}}
			} else {
				return nil, status.Errorf(codes.Unauthenticated, "invalid token claims")
			}
		}

		ctx = context.WithValue(ctx, "token_claims", claims)
		return handler(ctx, req)
	}

	// Create gRPC server with options
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(int(cfg.MaxRequestSize)),
		grpc.MaxSendMsgSize(int(cfg.MaxRequestSize)),
		grpc.UnaryInterceptor(authInterceptor),
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
