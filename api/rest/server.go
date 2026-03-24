package rest

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/limyedb/limyedb/pkg/auth"
	"github.com/limyedb/limyedb/pkg/cluster"
	"github.com/limyedb/limyedb/pkg/collection"
	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/realtime"
	"github.com/limyedb/limyedb/pkg/storage/snapshot"
)

// Server represents the REST API server
type Server struct {
	router      *gin.Engine
	httpServer  *http.Server
	config      *config.ServerConfig
	collections *collection.Manager
	snapshots   *snapshot.Manager
	aliases     *collection.AliasManager
	raft        *cluster.RaftNode
	opts        *ServerOptions // Added to store ServerOptions
	realtimeHub *realtime.Hub
}

// ServerOptions configures the REST server
type ServerOptions struct {
	Addr        string
	Snapshots   *snapshot.Manager // Kept Snapshots here for NewServer compatibility
	Aliases     *collection.AliasManager
	Raft        *cluster.RaftNode
	AuthToken   string
	TLSCert     string
	TLSKey      string
}

// NewServer creates a new REST API server
func NewServer(cfg *config.ServerConfig, collections *collection.Manager, snapshots *snapshot.Manager) *Server {
	return NewServerWithOptions(cfg, collections, &ServerOptions{Snapshots: snapshots})
}

// NewServerWithOptions creates a new REST API server with optional dependencies
func NewServerWithOptions(cfg *config.ServerConfig, collections *collection.Manager, opts *ServerOptions) *Server {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	// gin.Recovery() is moved to setupMiddleware to allow custom middleware order

	s := &Server{
		router:      router,
		config:      cfg,
		collections: collections,
		opts:        opts, // Store opts
		realtimeHub: realtime.NewHub(),
	}

	if opts != nil {
		s.snapshots = opts.Snapshots
		s.aliases = opts.Aliases
		s.raft = opts.Raft
	}

	// Start Real-Time Event Hub
	go s.realtimeHub.Run(context.Background())

	s.setupRoutes()
	s.setupMiddleware()

	return s
}

func (s *Server) setupRoutes() {
	// Health check
	s.router.GET("/health", s.handleHealth)
	s.router.GET("/readiness", s.handleReadiness)

	// Collections - Legacy (single vector)
	s.router.POST("/collections", s.handleCreateCollection)
	s.router.GET("/collections", s.handleListCollections)
	s.router.GET("/collections/:name", s.requirePermission("read"), s.handleGetCollection)
	s.router.DELETE("/collections/:name", s.requirePermission("admin"), s.handleDeleteCollection)
	s.router.PATCH("/collections/:name", s.requirePermission("admin"), s.handleUpdateCollection)

	// Collections V2 - Named vectors support
	s.router.POST("/collections/v2", s.handleCreateCollectionV2)

	// Points - Legacy (single vector)
	s.router.PUT("/collections/:name/points", s.requirePermission("write"), s.handleUpsertPoints)
	s.router.GET("/collections/:name/points/:id", s.requirePermission("read"), s.handleGetPoint)
	s.router.DELETE("/collections/:name/points/:id", s.requirePermission("write"), s.handleDeletePoint)
	s.router.POST("/collections/:name/points/batch", s.requirePermission("write"), s.handleBatchUpsert)
	s.router.POST("/collections/:name/points/delete", s.requirePermission("write"), s.handleBatchDelete)

	// Points V2 - Named vectors support
	s.router.PUT("/collections/:name/points/v2", s.requirePermission("write"), s.handleUpsertPointsV2)

	// Auto-Embedding V2 Pipeline
	s.router.POST("/collections/:name/auto-embed", s.requirePermission("write"), s.handleAutoEmbed)

	// Scroll/Pagination API
	s.router.POST("/collections/:name/points/scroll", s.requirePermission("read"), s.handleScroll)

	// Search - Legacy (single vector)
	s.router.POST("/collections/:name/search", s.requirePermission("read"), s.handleSearch)
	s.router.POST("/collections/:name/recommend", s.requirePermission("read"), s.handleRecommend)

	// Search V2 - Named vectors support
	s.router.POST("/collections/:name/search/v2", s.requirePermission("read"), s.handleSearchV2)
	s.router.POST("/collections/:name/recommend/v2", s.requirePermission("read"), s.handleRecommendV2)

	// Discovery/Context Search API
	s.router.POST("/collections/:name/discover", s.requirePermission("read"), s.handleDiscover)

	// Group Search API
	s.router.POST("/collections/:name/search/groups", s.requirePermission("read"), s.handleGroupSearch)

	// Cluster Operations
	s.router.POST("/cluster/join", s.handleJoinCluster)

	// Faceted Search API
	s.router.POST("/collections/:name/facet", s.requirePermission("read"), s.handleFacet)
	s.router.POST("/collections/:name/facets", s.requirePermission("read"), s.handleMultiFacet)

	// Query Explain/Planning API
	s.router.POST("/collections/:name/explain", s.requirePermission("read"), s.handleExplain)

	// Payload Index Configuration API
	s.router.POST("/collections/:name/payload-indexes", s.requirePermission("admin"), s.handleCreatePayloadIndex)
	s.router.GET("/collections/:name/payload-indexes", s.requirePermission("read"), s.handleListPayloadIndexes)
	s.router.DELETE("/collections/:name/payload-indexes/:field", s.requirePermission("admin"), s.handleDeletePayloadIndex)

	// Collection Aliases API
	s.router.POST("/aliases", s.requirePermission("admin"), s.handleCreateAlias)
	s.router.GET("/aliases", s.requirePermission("read"), s.handleListAliases)
	s.router.DELETE("/aliases/:alias", s.requirePermission("admin"), s.handleDeleteAlias)
	s.router.PUT("/aliases/:alias", s.requirePermission("admin"), s.handleSwitchAlias)

	// Snapshots
	s.router.POST("/snapshots", s.requirePermission("admin"), s.handleCreateSnapshot)
	s.router.GET("/snapshots", s.requirePermission("admin"), s.handleListSnapshots)
	s.router.POST("/snapshots/:id/restore", s.requirePermission("admin"), s.handleRestoreSnapshot)
	s.router.DELETE("/snapshots/:id", s.requirePermission("admin"), s.handleDeleteSnapshot)

	// Cluster HTTP Streaming / WebSockets
	s.router.GET("/stream", s.handleWebSocket)

	// Metrics
	s.router.GET("/metrics", s.handleMetrics)
}

func (s *Server) setupMiddleware() {
	// Standard Recovery middleware
	s.router.Use(gin.Recovery())

	// Enterprise Zero-Trust Token Bearer Interceptor
	if s.opts.AuthToken != "" {
		tokenManager := auth.NewTokenManager(s.opts.AuthToken)
		s.router.Use(func(c *gin.Context) {
			if c.Request.URL.Path == "/health" || c.Request.URL.Path == "/readiness" {
				c.Next()
				return
			}
			
			authHeader := c.GetHeader("Authorization")
			parts := strings.SplitN(authHeader, " ", 2)
			
			if len(parts) != 2 || parts[0] != "Bearer" {
				respondError(c, http.StatusUnauthorized, errors.New("unauthorized: missing or invalid bearer format"))
				c.Abort()
				return
			}

			// Validate granular JWT natively
			claims, err := tokenManager.Validate(parts[1])
			if err != nil {
				// Fallback to static global API key for backward-compatibility or explicit Root overlays
				if authHeader == "Bearer "+s.opts.AuthToken {
					claims = &auth.TokenClaims{
						Permissions: auth.Permissions{GlobalAdmin: true},
					}
				} else {
					respondError(c, http.StatusUnauthorized, errors.New("unauthorized: invalid token claims"))
					c.Abort()
					return
				}
			}

			// Scope request context dynamically
			c.Set("token_claims", claims)
			c.Next()
		})
	}
	s.router.Use(s.requestLogger())
	s.router.Use(s.corsMiddleware())
	s.router.Use(s.requestSizeLimit())
}

// Start starts the HTTP server
func (s *Server) Start() error {
	s.httpServer = &http.Server{
		Addr:         s.opts.Addr,
		Handler:      s.router,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
	}

	if s.opts != nil && s.opts.TLSCert != "" && s.opts.TLSKey != "" {
		return s.httpServer.ListenAndServeTLS(s.opts.TLSCert, s.opts.TLSKey)
	}

	return s.httpServer.ListenAndServe()
}

// Stop gracefully stops the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

// checkPermission verifies if the current request context has adequate JWT roles.
func (s *Server) checkPermission(c *gin.Context, collection string, action string) bool {
	claimsRaw, exists := c.Get("token_claims")
	if !exists {
		// No auth token configured / passed successfully (meaning auth is disabled)
		return true 
	}
	
	claims, ok := claimsRaw.(*auth.TokenClaims)
	if !ok {
		return false
	}
	
	switch action {
	case "read": return claims.CanRead(collection)
	case "write": return claims.CanWrite(collection)
	case "admin": return claims.CanAdmin(collection)
	case "global_admin": return claims.Permissions.GlobalAdmin
	}
	return false
}

// requirePermission dynamically halts Gin routers checking the active JWT
func (s *Server) requirePermission(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		
		// Unnamed global routes map entirely to global admins unconditionally!
		if name == "" {
			if !s.checkPermission(c, "", "global_admin") {
				respondError(c, http.StatusForbidden, errors.New("forbidden: Global Admin required"))
				c.Abort()
				return
			}
			c.Next()
			return
		}
		
		if !s.checkPermission(c, name, action) {
			respondError(c, http.StatusForbidden, errors.New("forbidden: insufficient scoped privileges against collection"))
			c.Abort()
			return
		}
		c.Next()
	}
}

// Middleware

func (s *Server) requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		// Output JSON telemetry for Kubernetes metrics extraction
		slog.Info("HTTP Request Segment",
			slog.Int("status", status),
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.Duration("latency", latency),
		)
	}
}

func (s *Server) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func (s *Server) requestSizeLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, s.config.MaxRequestSize)
		c.Next()
	}
}

// Response types

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

// SuccessResponse represents a success response
type SuccessResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
}

func respondError(c *gin.Context, status int, err error) {
	c.JSON(status, ErrorResponse{
		Error: err.Error(),
	})
}

func respondSuccess(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, SuccessResponse{
		Success: true,
		Data:    data,
	})
}

func respondCreated(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, SuccessResponse{
		Success: true,
		Data:    data,
	})
}

// handleWebSocket transparently passes the Gin HTTP structures into the pure Real-Time WebSocket hub upgrader.
func (s *Server) handleWebSocket(c *gin.Context) {
	s.realtimeHub.ServeWS(c.Writer, c.Request)
}
