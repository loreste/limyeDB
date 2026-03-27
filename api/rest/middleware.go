package rest

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
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

// RequestIDMiddleware adds a request ID to each request
func RequestIDMiddleware() gin.HandlerFunc {
	var counter uint64
	var mu sync.Mutex

	return func(c *gin.Context) {
		mu.Lock()
		counter++
		id := counter
		mu.Unlock()

		c.Set("request_id", id)
		c.Header("X-Request-ID", strconv.FormatUint(id, 10))

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
				c.AbortWithStatusJSON(http.StatusInternalServerError, ErrorResponse{
					Error: "internal server error",
					Code:  "INTERNAL_ERROR",
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

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
