package security

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestAPIKeyManager(t *testing.T) {
	config := &APIKeyConfig{
		Enabled:    true,
		HeaderName: "X-API-Key",
		Keys: []APIKey{
			{
				Key:         "test-key-123",
				Name:        "test-key",
				Permissions: []string{"read", "write"},
			},
		},
	}

	manager := NewAPIKeyManager(config)

	// Test valid key
	key, err := manager.Authenticate("test-key-123")
	if err != nil {
		t.Fatalf("Expected successful auth, got error: %v", err)
	}
	if key.Name != "test-key" {
		t.Errorf("Expected key name 'test-key', got '%s'", key.Name)
	}

	// Test invalid key
	_, err = manager.Authenticate("invalid-key")
	if err == nil {
		t.Error("Expected error for invalid key")
	}

	// Test permissions
	if !manager.HasPermission(key, "read") {
		t.Error("Expected 'read' permission")
	}
	if !manager.HasPermission(key, "write") {
		t.Error("Expected 'write' permission")
	}
	if manager.HasPermission(key, "admin") {
		t.Error("Should not have 'admin' permission")
	}
}

func TestAPIKeyExpiration(t *testing.T) {
	config := &APIKeyConfig{
		Enabled: true,
		Keys: []APIKey{
			{
				Key:         "expired-key",
				Name:        "expired",
				Permissions: []string{"read"},
				ExpiresAt:   time.Now().Add(-1 * time.Hour), // Expired
			},
		},
	}

	manager := NewAPIKeyManager(config)

	_, err := manager.Authenticate("expired-key")
	if err == nil {
		t.Error("Expected error for expired key")
	}
	if err.Error() != "API key expired" {
		t.Errorf("Expected 'API key expired' error, got: %v", err)
	}
}

func TestAPIKeyRateLimit(t *testing.T) {
	config := &APIKeyConfig{
		Enabled: true,
		Keys: []APIKey{
			{
				Key:         "rate-limited-key",
				Name:        "rate-limited",
				Permissions: []string{"read"},
				RateLimit:   5, // 5 requests per minute
			},
		},
	}

	manager := NewAPIKeyManager(config)

	// First 5 should succeed
	for i := 0; i < 5; i++ {
		_, err := manager.Authenticate("rate-limited-key")
		if err != nil {
			t.Fatalf("Request %d should succeed: %v", i, err)
		}
	}

	// 6th should fail
	_, err := manager.Authenticate("rate-limited-key")
	if err == nil {
		t.Error("6th request should be rate limited")
	}
}

func TestGenerateAPIKey(t *testing.T) {
	key1 := GenerateAPIKey()
	key2 := GenerateAPIKey()

	if len(key1) < 70 { // "limye_" + 64 hex chars
		t.Errorf("Key too short: %s", key1)
	}

	if key1 == key2 {
		t.Error("Generated keys should be unique")
	}

	if key1[:6] != "limye_" {
		t.Errorf("Key should start with 'limye_': %s", key1)
	}
}

func TestHTTPMiddleware(t *testing.T) {
	config := &APIKeyConfig{
		Enabled:    true,
		HeaderName: "X-API-Key",
		Keys: []APIKey{
			{
				Key:         "valid-key",
				Name:        "valid",
				Permissions: []string{"read"},
			},
		},
	}

	manager := NewAPIKeyManager(config)

	handler := manager.HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Test without key
	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", rr.Code)
	}

	// Test with valid key
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "valid-key")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rr.Code)
	}

	// Test with invalid key
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "invalid-key")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", rr.Code)
	}
}

func TestEncryptor(t *testing.T) {
	// Create temp key file
	keyFile, err := os.CreateTemp("", "test-key-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(keyFile.Name())

	// Write key
	key := GenerateEncryptionKey()
	keyFile.WriteString(key)
	keyFile.Close()

	config := &EncryptionConfig{
		Enabled: true,
		KeyFile: keyFile.Name(),
	}

	encryptor, err := NewEncryptor(config)
	if err != nil {
		t.Fatalf("NewEncryptor failed: %v", err)
	}

	// Test encrypt/decrypt
	plaintext := []byte("Hello, World!")
	ciphertext, err := encryptor.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Ciphertext should be different
	if string(ciphertext) == string(plaintext) {
		t.Error("Ciphertext should differ from plaintext")
	}

	decrypted, err := encryptor.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("Decryption mismatch: expected %s, got %s", plaintext, decrypted)
	}
}

func TestEncryptPayload(t *testing.T) {
	keyFile, err := os.CreateTemp("", "test-key-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(keyFile.Name())

	key := GenerateEncryptionKey()
	keyFile.WriteString(key)
	keyFile.Close()

	encryptor, _ := NewEncryptor(&EncryptionConfig{
		Enabled: true,
		KeyFile: keyFile.Name(),
	})

	payload := map[string]interface{}{
		"name": "test",
		"value": 123,
	}

	encrypted, err := encryptor.EncryptPayload(payload)
	if err != nil {
		t.Fatalf("EncryptPayload failed: %v", err)
	}

	decrypted, err := encryptor.DecryptPayload(encrypted)
	if err != nil {
		t.Fatalf("DecryptPayload failed: %v", err)
	}

	if decrypted["name"] != "test" {
		t.Errorf("Payload mismatch: expected 'test', got '%v'", decrypted["name"])
	}
}

func TestAuditLogger(t *testing.T) {
	// Create temp log file
	logFile, err := os.CreateTemp("", "audit-log-*")
	if err != nil {
		t.Fatal(err)
	}
	logFile.Close()
	defer os.Remove(logFile.Name())

	config := &AuditConfig{
		Enabled:  true,
		LogFile:  logFile.Name(),
		LogLevel: "all",
	}

	logger, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("NewAuditLogger failed: %v", err)
	}
	defer logger.Close()

	// Log some events
	logger.LogAuthentication("test-key", "127.0.0.1", true, nil)
	logger.LogCollectionAccess("search", "test_collection", "test-key", "127.0.0.1", true)
	logger.LogWrite("insert", "test_collection", "test-key", "127.0.0.1", 100, true)

	// Verify file has content
	data, err := os.ReadFile(logFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	if len(data) == 0 {
		t.Error("Audit log should have content")
	}
}

func TestSecureCompare(t *testing.T) {
	if !SecureCompare("test", "test") {
		t.Error("Equal strings should match")
	}

	if SecureCompare("test", "other") {
		t.Error("Different strings should not match")
	}

	if SecureCompare("test", "tes") {
		t.Error("Different length strings should not match")
	}
}

func TestAPIKeyContext(t *testing.T) {
	key := &APIKey{
		Key:  "test",
		Name: "test-key",
	}

	ctx := context.Background()
	ctx = SetAPIKeyContext(ctx, key)

	retrieved := GetAPIKeyFromContext(ctx)
	if retrieved == nil {
		t.Fatal("Expected to retrieve API key from context")
	}

	if retrieved.Name != "test-key" {
		t.Errorf("Expected name 'test-key', got '%s'", retrieved.Name)
	}
}

func TestAddRevokeKey(t *testing.T) {
	manager := NewAPIKeyManager(&APIKeyConfig{
		Enabled: true,
	})

	// Add key
	err := manager.AddKey(APIKey{
		Key:         "new-key",
		Name:        "new",
		Permissions: []string{"read"},
	})
	if err != nil {
		t.Fatalf("AddKey failed: %v", err)
	}

	// Verify key works
	_, err = manager.Authenticate("new-key")
	if err != nil {
		t.Errorf("New key should authenticate: %v", err)
	}

	// Revoke key
	err = manager.RevokeKey("new-key")
	if err != nil {
		t.Fatalf("RevokeKey failed: %v", err)
	}

	// Verify key no longer works
	_, err = manager.Authenticate("new-key")
	if err == nil {
		t.Error("Revoked key should not authenticate")
	}
}
