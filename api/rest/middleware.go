package rest

import (
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/limyedb/limyedb/pkg/metrics"
)

// RateLimiter implements a token bucket rate limiter
type RateLimiter struct {
	tokens        map[string]float64
	lastAccess    map[string]time.Time
	rate          float64 // tokens per second
	capacity      float64 // max tokens
	mu            sync.Mutex
	stopCleanup   chan struct{}
	cleanupTicker *time.Ticker
}

const (
	rateLimiterMaxEntries      = 100000
	rateLimiterCleanupInterval = 5 * time.Minute
	rateLimiterEntryTTL        = 1 * time.Hour
)

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rate, capacity float64) *RateLimiter {
	rl := &RateLimiter{
		tokens:        make(map[string]float64),
		lastAccess:    make(map[string]time.Time),
		rate:          rate,
		capacity:      capacity,
		stopCleanup:   make(chan struct{}),
		cleanupTicker: time.NewTicker(rateLimiterCleanupInterval),
	}
	go rl.cleanupExpiredEntries()
	return rl
}

// Stop stops the rate limiter cleanup goroutine
func (rl *RateLimiter) Stop() {
	close(rl.stopCleanup)
	rl.cleanupTicker.Stop()
}

// cleanupExpiredEntries removes old entries to prevent memory leaks
func (rl *RateLimiter) cleanupExpiredEntries() {
	for {
		select {
		case <-rl.stopCleanup:
			return
		case <-rl.cleanupTicker.C:
			rl.mu.Lock()
			now := time.Now()
			for key, lastAccess := range rl.lastAccess {
				if now.Sub(lastAccess) > rateLimiterEntryTTL {
					delete(rl.tokens, key)
					delete(rl.lastAccess, key)
				}
			}
			// If still too many entries, remove oldest
			if len(rl.lastAccess) > rateLimiterMaxEntries {
				rl.removeOldestEntries(rateLimiterMaxEntries / 2)
			}
			rl.mu.Unlock()
		}
	}
}

// removeOldestEntries removes the oldest entries to stay under limit
func (rl *RateLimiter) removeOldestEntries(keepCount int) {
	if len(rl.lastAccess) <= keepCount {
		return
	}

	// Find entries to remove (already holding lock)
	type entry struct {
		key  string
		time time.Time
	}
	entries := make([]entry, 0, len(rl.lastAccess))
	for k, t := range rl.lastAccess {
		entries = append(entries, entry{k, t})
	}

	// Sort by time (oldest first) - simple bubble sort for small lists
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].time.Before(entries[i].time) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	// Remove oldest entries
	removeCount := len(entries) - keepCount
	for i := 0; i < removeCount; i++ {
		delete(rl.tokens, entries[i].key)
		delete(rl.lastAccess, entries[i].key)
	}
}

// Allow checks if a request should be allowed
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Get current tokens
	tokens, exists := rl.tokens[key]
	if !exists {
		tokens = rl.capacity
	}

	// Add tokens based on elapsed time
	if lastAccess, ok := rl.lastAccess[key]; ok {
		elapsed := now.Sub(lastAccess).Seconds()
		tokens = min(tokens+elapsed*rl.rate, rl.capacity)
	}

	// Check if we have a token
	if tokens < 1 {
		return false
	}

	// Consume a token
	rl.tokens[key] = tokens - 1
	rl.lastAccess[key] = now

	return true
}

// RateLimitMiddleware returns a rate limiting middleware
func RateLimitMiddleware(limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Use client IP as key
		key := c.ClientIP()

		if !limiter.Allow(key) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, ErrorResponse{
				Error: "rate limit exceeded",
				Code:  "RATE_LIMITED",
			})
			return
		}

		c.Next()
	}
}

// AuthMiddleware provides API key authentication
func AuthMiddleware(apiKeys map[string]bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip auth for health endpoints
		if c.Request.URL.Path == "/health" || c.Request.URL.Path == "/readiness" {
			c.Next()
			return
		}

		// Check Authorization header
		auth := c.GetHeader("Authorization")
		if auth == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{
				Error: "missing authorization header",
				Code:  "UNAUTHORIZED",
			})
			return
		}

		// Extract API key
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{
				Error: "invalid authorization format",
				Code:  "UNAUTHORIZED",
			})
			return
		}

		apiKey := parts[1]
		valid := false
		for key := range apiKeys {
			if subtle.ConstantTimeCompare([]byte(key), []byte(apiKey)) == 1 {
				valid = true
				break
			}
		}
		if !valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{
				Error: "invalid API key",
				Code:  "UNAUTHORIZED",
			})
			return
		}

		c.Next()
	}
}

// generateUUID produces a version-4 UUID using crypto/rand.
func generateUUID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	// Set version 4 and variant bits per RFC 4122.
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16]), nil
}

// RequestIDMiddleware adds a request ID to each request.
// If the incoming request carries an X-Request-Id header the value is reused;
// otherwise a new UUID v4 is generated via crypto/rand.
// The resolved ID is stored on the gin.Context under "request_id" and echoed
// back in the X-Request-Id response header.
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-Id")
		if id == "" {
			generated, err := generateUUID()
			if err != nil {
				// Fallback: let the request continue without an ID rather than
				// failing the whole request because of entropy exhaustion.
				c.Next()
				return
			}
			id = generated
		}

		c.Set("request_id", id)
		c.Header("X-Request-Id", id)

		c.Next()
	}
}

// TimeoutMiddleware adds a timeout to requests
func TimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Note: This is a simplified implementation
		// A full implementation would use context cancellation
		c.Next()
	}
}

// RecoveryMiddleware recovers from panics
func RecoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				reqID, _ := c.Get("request_id")
				rid, _ := reqID.(string)
				c.AbortWithStatusJSON(http.StatusInternalServerError, StructuredErrorResponse{
					Error: StructuredError{
						Code:      "INTERNAL_ERROR",
						Message:   "internal server error",
						RequestID: rid,
					},
				})
			}
		}()

		c.Next()
	}
}

// CompressionMiddleware adds gzip compression
func CompressionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if client accepts gzip
		if !strings.Contains(c.GetHeader("Accept-Encoding"), "gzip") {
			c.Next()
			return
		}

		// Note: In production, use gin-contrib/gzip
		c.Next()
	}
}

// ContentTypeMiddleware ensures proper content type
func ContentTypeMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// For non-GET requests, require JSON content type
		if c.Request.Method != "GET" && c.Request.Method != "OPTIONS" {
			contentType := c.GetHeader("Content-Type")
			if !strings.Contains(contentType, "application/json") {
				c.AbortWithStatusJSON(http.StatusUnsupportedMediaType, ErrorResponse{
					Error: "content type must be application/json",
					Code:  "INVALID_CONTENT_TYPE",
				})
				return
			}
		}

		c.Next()
	}
}

// MetricsMiddleware collects request metrics
type MetricsMiddleware struct {
	requestCount    map[string]int64
	requestDuration map[string]time.Duration
	mu              sync.RWMutex
}

// NewMetricsMiddleware creates a new metrics middleware
func NewMetricsMiddleware() *MetricsMiddleware {
	return &MetricsMiddleware{
		requestCount:    make(map[string]int64),
		requestDuration: make(map[string]time.Duration),
	}
}

// Handler returns the middleware handler
func (m *MetricsMiddleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start)
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		key := c.Request.Method + " " + path

		m.mu.Lock()
		m.requestCount[key]++
		m.requestDuration[key] += duration
		m.mu.Unlock()
	}
}

// GetMetrics returns collected metrics
func (m *MetricsMiddleware) GetMetrics() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	counts := make(map[string]int64)
	durations := make(map[string]float64)

	for k, v := range m.requestCount {
		counts[k] = v
		if v > 0 {
			durations[k] = m.requestDuration[k].Seconds() / float64(v)
		}
	}

	return map[string]interface{}{
		"request_count":        counts,
		"avg_duration_seconds": durations,
	}
}

// PrometheusMetricsMiddleware returns a gin middleware that records HTTP
// request duration and total count in Prometheus histograms/counters.
func PrometheusMetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		method := c.Request.Method

		metrics.RequestDuration.WithLabelValues(method, path, status).Observe(duration)
		metrics.RequestTotal.WithLabelValues(method, path, status).Inc()
	}
}

// EndpointRateLimitConfig holds per-endpoint rate limiting settings.
// When set on ServerOptions.RateLimits, the middleware is activated.
type EndpointRateLimitConfig struct {
	// DefaultRate is the default requests-per-second for endpoints without a
	// specific override.
	DefaultRate float64
	// DefaultBurst is the default burst size.
	DefaultBurst float64
	// Overrides maps a route pattern (e.g. "POST /collections/:name/search")
	// to a dedicated RateLimiter with its own rate/burst settings.
	Overrides map[string]*RateLimiter
}

// DefaultEndpointRateLimitConfig returns a sensible default configuration:
// 100 req/s for search endpoints, 1000 req/s for read endpoints, 500 req/s
// default for everything else.
func DefaultEndpointRateLimitConfig() *EndpointRateLimitConfig {
	return &EndpointRateLimitConfig{
		DefaultRate:  500,
		DefaultBurst: 1000,
		Overrides: map[string]*RateLimiter{
			"POST /collections/:name/search":    NewRateLimiter(100, 200),
			"POST /collections/:name/search/v2": NewRateLimiter(100, 200),
			"POST /collections/:name/recommend": NewRateLimiter(100, 200),
			"POST /collections/:name/discover":  NewRateLimiter(100, 200),
			"GET /collections/:name":            NewRateLimiter(1000, 2000),
			"GET /collections":                  NewRateLimiter(1000, 2000),
			"GET /collections/:name/points/:id": NewRateLimiter(1000, 2000),
		},
	}
}

// EndpointRateLimitMiddleware returns middleware that applies per-endpoint rate
// limits based on the provided configuration. Clients are keyed by IP address.
func EndpointRateLimitMiddleware(cfg *EndpointRateLimitConfig) gin.HandlerFunc {
	defaultLimiter := NewRateLimiter(cfg.DefaultRate, cfg.DefaultBurst)

	return func(c *gin.Context) {
		// Skip rate limiting for health/metrics endpoints
		p := c.Request.URL.Path
		if p == "/health" || p == "/readiness" || p == "/metrics" {
			c.Next()
			return
		}

		routeKey := c.Request.Method + " " + c.FullPath()
		limiter := defaultLimiter
		if override, ok := cfg.Overrides[routeKey]; ok {
			limiter = override
		}

		clientKey := c.ClientIP()
		if !limiter.Allow(clientKey) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, ErrorResponse{
				Error: "rate limit exceeded",
				Code:  "RATE_LIMITED",
			})
			return
		}
		c.Next()
	}
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
