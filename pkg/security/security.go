package security

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ErrAuthDisabled is returned when authentication is disabled.
var ErrAuthDisabled = errors.New("authentication disabled")

// =============================================================================
// API Key Authentication
// =============================================================================

// APIKeyConfig holds API key configuration
type APIKeyConfig struct {
	Enabled    bool     `json:"enabled"`
	Keys       []APIKey `json:"keys"`
	HeaderName string   `json:"header_name"` // Default: "X-API-Key"
	QueryParam string   `json:"query_param"` // Default: "api_key"
}

// APIKey represents an API key with permissions
type APIKey struct {
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Permissions []string  `json:"permissions"` // "read", "write", "admin"
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
	RateLimit   int       `json:"rate_limit"` // Requests per minute, 0 = unlimited
}

// DefaultAPIKeyConfig returns default API key configuration
func DefaultAPIKeyConfig() *APIKeyConfig {
	return &APIKeyConfig{
		Enabled:    false,
		Keys:       []APIKey{},
		HeaderName: "X-API-Key",
		QueryParam: "api_key",
	}
}

// APIKeyManager manages API key authentication
type APIKeyManager struct {
	config *APIKeyConfig
	keys   map[string]*APIKey // Hashed key -> APIKey
	mu     sync.RWMutex

	// Rate limiting
	rateLimits    map[string]*rateLimitEntry
	rateMu        sync.RWMutex
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
}

type rateLimitEntry struct {
	count     int
	resetTime time.Time
}

const (
	// maxRateLimitEntries prevents unbounded memory growth
	maxRateLimitEntries = 100000
	// rateLimitCleanupInterval is how often expired entries are cleaned
	rateLimitCleanupInterval = 5 * time.Minute
)

// NewAPIKeyManager creates a new API key manager
func NewAPIKeyManager(config *APIKeyConfig) *APIKeyManager {
	if config == nil {
		config = DefaultAPIKeyConfig()
	}

	m := &APIKeyManager{
		config:        config,
		keys:          make(map[string]*APIKey),
		rateLimits:    make(map[string]*rateLimitEntry),
		cleanupTicker: time.NewTicker(rateLimitCleanupInterval),
		stopCleanup:   make(chan struct{}),
	}

	// Hash and store keys
	for i := range config.Keys {
		hashedKey := hashKey(config.Keys[i].Key)
		m.keys[hashedKey] = &config.Keys[i]
	}

	// Start background cleanup goroutine to prevent memory leaks
	go m.cleanupExpiredRateLimits()

	return m
}

// Stop stops the API key manager and cleans up resources
func (m *APIKeyManager) Stop() {
	if m.cleanupTicker != nil {
		m.cleanupTicker.Stop()
	}
	close(m.stopCleanup)
}

// cleanupExpiredRateLimits periodically removes expired rate limit entries
func (m *APIKeyManager) cleanupExpiredRateLimits() {
	for {
		select {
		case <-m.stopCleanup:
			return
		case <-m.cleanupTicker.C:
			m.rateMu.Lock()
			now := time.Now()
			for key, entry := range m.rateLimits {
				if now.After(entry.resetTime) {
					delete(m.rateLimits, key)
				}
			}
			m.rateMu.Unlock()
		}
	}
}

// Authenticate validates an API key and returns the key info
func (m *APIKeyManager) Authenticate(key string) (*APIKey, error) {
	if !m.config.Enabled {
		return nil, ErrAuthDisabled
	}

	if key == "" {
		return nil, errors.New("API key required")
	}

	m.mu.RLock()
	hashedKey := hashKey(key)
	apiKey, exists := m.keys[hashedKey]
	m.mu.RUnlock()

	if !exists {
		return nil, errors.New("invalid API key")
	}

	// Check expiration
	if !apiKey.ExpiresAt.IsZero() && time.Now().After(apiKey.ExpiresAt) {
		return nil, errors.New("API key expired")
	}

	// Check rate limit
	if apiKey.RateLimit > 0 {
		if err := m.checkRateLimit(hashedKey, apiKey.RateLimit); err != nil {
			return nil, err
		}
	}

	return apiKey, nil
}

// checkRateLimit checks and updates rate limit for a key
func (m *APIKeyManager) checkRateLimit(hashedKey string, limit int) error {
	m.rateMu.Lock()
	defer m.rateMu.Unlock()

	now := time.Now()
	entry, exists := m.rateLimits[hashedKey]

	if !exists || now.After(entry.resetTime) {
		// Enforce max entries to prevent unbounded memory growth
		if len(m.rateLimits) >= maxRateLimitEntries {
			// Evict oldest expired entries first
			for key, e := range m.rateLimits {
				if now.After(e.resetTime) {
					delete(m.rateLimits, key)
				}
				if len(m.rateLimits) < maxRateLimitEntries {
					break
				}
			}
			// If still at limit, reject new entries
			if len(m.rateLimits) >= maxRateLimitEntries {
				return errors.New("rate limit service temporarily unavailable")
			}
		}
		m.rateLimits[hashedKey] = &rateLimitEntry{
			count:     1,
			resetTime: now.Add(time.Minute),
		}
		return nil
	}

	if entry.count >= limit {
		return fmt.Errorf("rate limit exceeded: %d requests per minute", limit)
	}

	entry.count++
	return nil
}

// HasPermission checks if a key has a specific permission
func (m *APIKeyManager) HasPermission(apiKey *APIKey, permission string) bool {
	if apiKey == nil {
		return !m.config.Enabled // Allow all if auth disabled
	}

	for _, p := range apiKey.Permissions {
		if p == permission || p == "admin" {
			return true
		}
	}
	return false
}

// AddKey adds a new API key
func (m *APIKeyManager) AddKey(key APIKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	hashedKey := hashKey(key.Key)
	if _, exists := m.keys[hashedKey]; exists {
		return errors.New("key already exists")
	}

	key.CreatedAt = time.Now()
	m.keys[hashedKey] = &key
	m.config.Keys = append(m.config.Keys, key)

	return nil
}

// RevokeKey revokes an API key
func (m *APIKeyManager) RevokeKey(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	hashedKey := hashKey(key)
	if _, exists := m.keys[hashedKey]; !exists {
		return errors.New("key not found")
	}

	delete(m.keys, hashedKey)

	// Remove from config
	newKeys := m.config.Keys[:0]
	for _, k := range m.config.Keys {
		if hashKey(k.Key) != hashedKey {
			newKeys = append(newKeys, k)
		}
	}
	m.config.Keys = newKeys

	return nil
}

// GenerateAPIKey generates a new random API key
func GenerateAPIKey() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return "limye_" + hex.EncodeToString(bytes)
}

// hashKey creates a SHA-256 hash of an API key
func hashKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// HTTPMiddleware returns an HTTP middleware for API key authentication
func (m *APIKeyManager) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.config.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Try header first, then query param
		key := r.Header.Get(m.config.HeaderName)
		if key == "" {
			key = r.URL.Query().Get(m.config.QueryParam)
		}

		apiKey, err := m.Authenticate(key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		// Store key in context for downstream handlers
		r = r.WithContext(SetAPIKeyContext(r.Context(), apiKey))

		next.ServeHTTP(w, r)
	})
}

// =============================================================================
// Payload Encryption
// =============================================================================

// EncryptionConfig holds encryption configuration
type EncryptionConfig struct {
	Enabled bool   `json:"enabled"`
	KeyFile string `json:"key_file"` // Path to encryption key file
}

// Encryptor handles payload encryption/decryption
type Encryptor struct {
	config  *EncryptionConfig
	gcm     cipher.AEAD
	enabled bool
}

// NewEncryptor creates a new encryptor
func NewEncryptor(config *EncryptionConfig) (*Encryptor, error) {
	if config == nil || !config.Enabled {
		return &Encryptor{enabled: false}, nil
	}

	// Load key from file
	keyData, err := os.ReadFile(config.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read encryption key: %w", err)
	}

	key := strings.TrimSpace(string(keyData))
	keyBytes, err := hex.DecodeString(key)
	if err != nil {
		// Try base64
		keyBytes, err = base64.StdEncoding.DecodeString(key)
		if err != nil {
			return nil, errors.New("invalid encryption key format")
		}
	}

	if len(keyBytes) != 32 {
		return nil, errors.New("encryption key must be 32 bytes")
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return &Encryptor{
		config:  config,
		gcm:     gcm,
		enabled: true,
	}, nil
}

// Encrypt encrypts data
func (e *Encryptor) Encrypt(plaintext []byte) ([]byte, error) {
	if !e.enabled {
		return plaintext, nil
	}

	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return e.gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt decrypts data
func (e *Encryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	if !e.enabled {
		return ciphertext, nil
	}

	if len(ciphertext) < e.gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:e.gcm.NonceSize()], ciphertext[e.gcm.NonceSize():]
	return e.gcm.Open(nil, nonce, ciphertext, nil)
}

// EncryptPayload encrypts a payload map
func (e *Encryptor) EncryptPayload(payload map[string]interface{}) ([]byte, error) {
	if !e.enabled || payload == nil {
		data, _ := json.Marshal(payload)
		return data, nil
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return e.Encrypt(data)
}

// DecryptPayload decrypts a payload
func (e *Encryptor) DecryptPayload(encrypted []byte) (map[string]interface{}, error) {
	if !e.enabled || len(encrypted) == 0 {
		var payload map[string]interface{}
		_ = json.Unmarshal(encrypted, &payload) // Error intentionally ignored - empty/invalid data returns nil map
		return payload, nil
	}

	decrypted, err := e.Decrypt(encrypted)
	if err != nil {
		return nil, err
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(decrypted, &payload); err != nil {
		return nil, err
	}

	return payload, nil
}

// GenerateEncryptionKey generates a new 256-bit encryption key
func GenerateEncryptionKey() string {
	key := make([]byte, 32)
	rand.Read(key)
	return hex.EncodeToString(key)
}

// =============================================================================
// Audit Logging
// =============================================================================

// AuditConfig holds audit logging configuration
type AuditConfig struct {
	Enabled  bool   `json:"enabled"`
	LogFile  string `json:"log_file"`
	LogLevel string `json:"log_level"` // "all", "write", "admin"
}

// AuditLogger logs security-relevant events
type AuditLogger struct {
	config *AuditConfig
	file   *os.File
	mu     sync.Mutex
}

// AuditEvent represents an auditable event
type AuditEvent struct {
	Timestamp  time.Time              `json:"timestamp"`
	EventType  string                 `json:"event_type"`
	Action     string                 `json:"action"`
	Collection string                 `json:"collection,omitempty"`
	APIKeyName string                 `json:"api_key_name,omitempty"`
	ClientIP   string                 `json:"client_ip,omitempty"`
	Success    bool                   `json:"success"`
	Error      string                 `json:"error,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(config *AuditConfig) (*AuditLogger, error) {
	if config == nil || !config.Enabled {
		return &AuditLogger{config: &AuditConfig{Enabled: false}}, nil
	}

	file, err := os.OpenFile(config.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open audit log: %w", err)
	}

	return &AuditLogger{
		config: config,
		file:   file,
	}, nil
}

// Log logs an audit event
func (l *AuditLogger) Log(event AuditEvent) error {
	if l.config == nil || !l.config.Enabled {
		return nil
	}

	// Check log level
	if !l.shouldLog(event.EventType) {
		return nil
	}

	event.Timestamp = time.Now()

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	_, err = l.file.Write(append(data, '\n'))
	return err
}

// shouldLog checks if an event should be logged based on log level
func (l *AuditLogger) shouldLog(eventType string) bool {
	switch l.config.LogLevel {
	case "all":
		return true
	case "write":
		return eventType == "write" || eventType == "delete" || eventType == "admin"
	case "admin":
		return eventType == "admin"
	default:
		return true
	}
}

// Close closes the audit log
func (l *AuditLogger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// LogAuthentication logs an authentication event
func (l *AuditLogger) LogAuthentication(keyName, clientIP string, success bool, err error) {
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	_ = l.Log(AuditEvent{ // Error intentionally ignored - audit logging is best-effort
		EventType:  "auth",
		Action:     "authenticate",
		APIKeyName: keyName,
		ClientIP:   clientIP,
		Success:    success,
		Error:      errStr,
	})
}

// LogCollectionAccess logs collection access
func (l *AuditLogger) LogCollectionAccess(action, collection, keyName, clientIP string, success bool) {
	_ = l.Log(AuditEvent{ // Error intentionally ignored - audit logging is best-effort
		EventType:  "read",
		Action:     action,
		Collection: collection,
		APIKeyName: keyName,
		ClientIP:   clientIP,
		Success:    success,
	})
}

// LogWrite logs write operations
func (l *AuditLogger) LogWrite(action, collection, keyName, clientIP string, pointCount int, success bool) {
	_ = l.Log(AuditEvent{ // Error intentionally ignored - audit logging is best-effort
		EventType:  "write",
		Action:     action,
		Collection: collection,
		APIKeyName: keyName,
		ClientIP:   clientIP,
		Success:    success,
		Details: map[string]interface{}{
			"point_count": pointCount,
		},
	})
}

// LogAdmin logs admin operations
func (l *AuditLogger) LogAdmin(action, keyName, clientIP string, details map[string]interface{}, success bool) {
	_ = l.Log(AuditEvent{ // Error intentionally ignored - audit logging is best-effort
		EventType:  "admin",
		Action:     action,
		APIKeyName: keyName,
		ClientIP:   clientIP,
		Success:    success,
		Details:    details,
	})
}

// =============================================================================
// Secure Compare
// =============================================================================

// SecureCompare compares two strings in constant time
func SecureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// =============================================================================
// Context helpers
// =============================================================================

type contextKey string

const apiKeyContextKey contextKey = "api_key"

// SetAPIKeyContext sets the API key in context
func SetAPIKeyContext(ctx context.Context, key *APIKey) context.Context {
	return context.WithValue(ctx, apiKeyContextKey, key)
}

// GetAPIKeyFromContext gets the API key from context
func GetAPIKeyFromContext(ctx context.Context) *APIKey {
	key, _ := ctx.Value(apiKeyContextKey).(*APIKey)
	return key
}
