// Package ratelimit provides rate limiting middleware for LimyeDB.
package ratelimit

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Limiter implements a token bucket rate limiter.
type Limiter struct {
	mu           sync.Mutex
	tokens       float64
	maxTokens    float64
	refillRate   float64 // tokens per second
	lastRefill   time.Time
}

// NewLimiter creates a new rate limiter.
func NewLimiter(maxTokens float64, refillRate float64) *Limiter {
	return &Limiter{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed.
func (l *Limiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(l.lastRefill).Seconds()
	l.tokens += elapsed * l.refillRate
	if l.tokens > l.maxTokens {
		l.tokens = l.maxTokens
	}
	l.lastRefill = now

	if l.tokens >= 1 {
		l.tokens--
		return true
	}
	return false
}

// Tokens returns the current number of tokens.
func (l *Limiter) Tokens() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.tokens
}

// RateLimiterStore manages rate limiters per key (e.g., IP, API key).
type RateLimiterStore struct {
	mu        sync.RWMutex
	limiters  map[string]*Limiter
	maxTokens float64
	refillRate float64
	cleanup   time.Duration
}

// NewRateLimiterStore creates a new store for rate limiters.
func NewRateLimiterStore(maxTokens, refillRate float64, cleanup time.Duration) *RateLimiterStore {
	store := &RateLimiterStore{
		limiters:   make(map[string]*Limiter),
		maxTokens:  maxTokens,
		refillRate: refillRate,
		cleanup:    cleanup,
	}

	// Start cleanup goroutine
	go store.cleanupLoop()

	return store
}

// GetLimiter gets or creates a limiter for the given key.
func (s *RateLimiterStore) GetLimiter(key string) *Limiter {
	s.mu.RLock()
	limiter, exists := s.limiters[key]
	s.mu.RUnlock()

	if exists {
		return limiter
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, exists = s.limiters[key]; exists {
		return limiter
	}

	limiter = NewLimiter(s.maxTokens, s.refillRate)
	s.limiters[key] = limiter
	return limiter
}

func (s *RateLimiterStore) cleanupLoop() {
	ticker := time.NewTicker(s.cleanup)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		// Remove limiters that are at full capacity (inactive)
		for key, limiter := range s.limiters {
			if limiter.Tokens() >= s.maxTokens {
				delete(s.limiters, key)
			}
		}
		s.mu.Unlock()
	}
}

// Config holds rate limiter configuration.
type Config struct {
	// Requests per second
	RequestsPerSecond float64
	// Burst size (max tokens)
	BurstSize float64
	// Key function to identify clients
	KeyFunc func(*gin.Context) string
	// Skip function to bypass rate limiting
	SkipFunc func(*gin.Context) bool
	// Custom error handler
	ErrorHandler func(*gin.Context)
}

// DefaultConfig returns default rate limiter configuration.
func DefaultConfig() Config {
	return Config{
		RequestsPerSecond: 100,
		BurstSize:         200,
		KeyFunc:           DefaultKeyFunc,
		SkipFunc:          nil,
		ErrorHandler:      DefaultErrorHandler,
	}
}

// DefaultKeyFunc extracts client IP as the rate limit key.
func DefaultKeyFunc(c *gin.Context) string {
	// Check X-Forwarded-For header first
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		return xff
	}
	// Check X-Real-IP header
	if xri := c.GetHeader("X-Real-IP"); xri != "" {
		return xri
	}
	// Fall back to remote address
	return c.ClientIP()
}

// APIKeyFunc extracts API key as the rate limit key.
func APIKeyFunc(c *gin.Context) string {
	// Check Authorization header
	if auth := c.GetHeader("Authorization"); auth != "" {
		return auth
	}
	// Fall back to IP
	return DefaultKeyFunc(c)
}

// DefaultErrorHandler handles rate limit exceeded errors.
func DefaultErrorHandler(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
		"error":   "rate limit exceeded",
		"message": "Too many requests, please try again later",
	})
}

// Middleware creates a Gin middleware for rate limiting.
func Middleware(config Config) gin.HandlerFunc {
	store := NewRateLimiterStore(config.BurstSize, config.RequestsPerSecond, time.Minute*5)

	return func(c *gin.Context) {
		// Check if should skip
		if config.SkipFunc != nil && config.SkipFunc(c) {
			c.Next()
			return
		}

		key := config.KeyFunc(c)
		limiter := store.GetLimiter(key)

		if !limiter.Allow() {
			config.ErrorHandler(c)
			return
		}

		// Add rate limit headers
		c.Header("X-RateLimit-Remaining", formatFloat(limiter.Tokens()))
		c.Header("X-RateLimit-Limit", formatFloat(config.BurstSize))

		c.Next()
	}
}

// New creates rate limiting middleware with default config.
func New() gin.HandlerFunc {
	return Middleware(DefaultConfig())
}

// WithConfig creates rate limiting middleware with custom config.
func WithConfig(requestsPerSecond, burstSize float64) gin.HandlerFunc {
	config := DefaultConfig()
	config.RequestsPerSecond = requestsPerSecond
	config.BurstSize = burstSize
	return Middleware(config)
}

func formatFloat(f float64) string {
	return string(rune(int(f)))
}

// SlidingWindowLimiter implements sliding window rate limiting.
type SlidingWindowLimiter struct {
	mu          sync.Mutex
	requests    []time.Time
	windowSize  time.Duration
	maxRequests int
}

// NewSlidingWindowLimiter creates a sliding window limiter.
func NewSlidingWindowLimiter(windowSize time.Duration, maxRequests int) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		requests:    make([]time.Time, 0, maxRequests),
		windowSize:  windowSize,
		maxRequests: maxRequests,
	}
}

// Allow checks if a request is allowed within the sliding window.
func (l *SlidingWindowLimiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-l.windowSize)

	// Remove old requests outside the window
	valid := l.requests[:0]
	for _, t := range l.requests {
		if t.After(windowStart) {
			valid = append(valid, t)
		}
	}
	l.requests = valid

	// Check if we can allow this request
	if len(l.requests) < l.maxRequests {
		l.requests = append(l.requests, now)
		return true
	}
	return false
}

// TenantRateLimiter provides per-tenant rate limiting.
type TenantRateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*RateLimiterStore
	configs  map[string]Config
	default_ Config
}

// NewTenantRateLimiter creates a tenant-aware rate limiter.
func NewTenantRateLimiter(defaultConfig Config) *TenantRateLimiter {
	return &TenantRateLimiter{
		limiters: make(map[string]*RateLimiterStore),
		configs:  make(map[string]Config),
		default_: defaultConfig,
	}
}

// SetTenantConfig sets rate limit configuration for a tenant.
func (t *TenantRateLimiter) SetTenantConfig(tenantID string, config Config) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.configs[tenantID] = config
	t.limiters[tenantID] = NewRateLimiterStore(
		config.BurstSize,
		config.RequestsPerSecond,
		time.Minute*5,
	)
}

// Allow checks if a request from a tenant is allowed.
func (t *TenantRateLimiter) Allow(tenantID, key string) bool {
	t.mu.RLock()
	store, exists := t.limiters[tenantID]
	t.mu.RUnlock()

	if !exists {
		// Use default limiter
		t.mu.Lock()
		if store, exists = t.limiters[tenantID]; !exists {
			store = NewRateLimiterStore(
				t.default_.BurstSize,
				t.default_.RequestsPerSecond,
				time.Minute*5,
			)
			t.limiters[tenantID] = store
		}
		t.mu.Unlock()
	}

	return store.GetLimiter(key).Allow()
}
