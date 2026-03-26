package auth

import (
	"testing"
	"time"
)

func TestNewTokenManager(t *testing.T) {
	tm := NewTokenManager("test-secret-key")
	if tm == nil {
		t.Fatal("NewTokenManager returned nil")
	}
	if len(tm.secretKey) == 0 {
		t.Error("secretKey should not be empty")
	}
}

func TestGenerateToken(t *testing.T) {
	tm := NewTokenManager("test-secret-key-256-bits-long!!")

	perms := Permissions{
		GlobalAdmin: false,
		Collections: map[string][]string{
			"test_collection": {"READ_ONLY"},
		},
	}

	token, err := tm.GenerateToken("test-user", perms, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	if token == "" {
		t.Error("Generated token should not be empty")
	}

	// Token should have 3 parts (header.payload.signature)
	parts := 0
	for _, c := range token {
		if c == '.' {
			parts++
		}
	}
	if parts != 2 {
		t.Errorf("JWT token should have 3 parts separated by 2 dots, got %d dots", parts)
	}
}

func TestValidateToken(t *testing.T) {
	tm := NewTokenManager("test-secret-key-256-bits-long!!")

	perms := Permissions{
		GlobalAdmin: false,
		Collections: map[string][]string{
			"collection1": {"READ_ONLY", "READ_WRITE"},
			"collection2": {"COLLECTION_ADMIN"},
		},
	}

	token, err := tm.GenerateToken("test-user", perms, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	claims, err := tm.Validate(token)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if claims.Subject != "test-user" {
		t.Errorf("Subject = %s, want test-user", claims.Subject)
	}

	if claims.Permissions.GlobalAdmin {
		t.Error("GlobalAdmin should be false")
	}

	if len(claims.Permissions.Collections) != 2 {
		t.Errorf("Collections count = %d, want 2", len(claims.Permissions.Collections))
	}
}

func TestValidateTokenExpired(t *testing.T) {
	tm := NewTokenManager("test-secret-key-256-bits-long!!")

	perms := Permissions{
		GlobalAdmin: true,
	}

	// Generate token that expires immediately
	token, err := tm.GenerateToken("test-user", perms, -time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	_, err = tm.Validate(token)
	if err == nil {
		t.Error("Validate() should fail for expired token")
	}
}

func TestValidateTokenInvalid(t *testing.T) {
	tm := NewTokenManager("test-secret-key-256-bits-long!!")

	tests := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"garbage", "not-a-valid-token"},
		{"partial", "header.payload"},
		{"wrong-signature", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.wrong-signature"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tm.Validate(tt.token)
			if err == nil {
				t.Error("Validate() should fail for invalid token")
			}
		})
	}
}

func TestValidateTokenWrongSecret(t *testing.T) {
	tm1 := NewTokenManager("secret-key-one-256-bits-long!!")
	tm2 := NewTokenManager("secret-key-two-256-bits-long!!")

	perms := Permissions{GlobalAdmin: true}
	token, err := tm1.GenerateToken("test-user", perms, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	// Validate with different secret should fail
	_, err = tm2.Validate(token)
	if err == nil {
		t.Error("Validate() should fail with wrong secret")
	}
}

func TestCanRead(t *testing.T) {
	tests := []struct {
		name       string
		claims     *TokenClaims
		collection string
		want       bool
	}{
		{
			name: "global_admin_can_read_any",
			claims: &TokenClaims{
				Permissions: Permissions{GlobalAdmin: true},
			},
			collection: "any_collection",
			want:       true,
		},
		{
			name: "read_only_permission",
			claims: &TokenClaims{
				Permissions: Permissions{
					Collections: map[string][]string{
						"test": {"READ_ONLY"},
					},
				},
			},
			collection: "test",
			want:       true,
		},
		{
			name: "read_write_permission",
			claims: &TokenClaims{
				Permissions: Permissions{
					Collections: map[string][]string{
						"test": {"READ_WRITE"},
					},
				},
			},
			collection: "test",
			want:       true,
		},
		{
			name: "collection_admin_permission",
			claims: &TokenClaims{
				Permissions: Permissions{
					Collections: map[string][]string{
						"test": {"COLLECTION_ADMIN"},
					},
				},
			},
			collection: "test",
			want:       true,
		},
		{
			name: "no_permission_for_collection",
			claims: &TokenClaims{
				Permissions: Permissions{
					Collections: map[string][]string{
						"other": {"READ_ONLY"},
					},
				},
			},
			collection: "test",
			want:       false,
		},
		{
			name: "empty_permissions",
			claims: &TokenClaims{
				Permissions: Permissions{
					Collections: map[string][]string{},
				},
			},
			collection: "test",
			want:       false,
		},
		{
			name: "unknown_permission_type",
			claims: &TokenClaims{
				Permissions: Permissions{
					Collections: map[string][]string{
						"test": {"UNKNOWN_PERM"},
					},
				},
			},
			collection: "test",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.claims.CanRead(tt.collection)
			if got != tt.want {
				t.Errorf("CanRead(%s) = %v, want %v", tt.collection, got, tt.want)
			}
		})
	}
}

func TestCanWrite(t *testing.T) {
	tests := []struct {
		name       string
		claims     *TokenClaims
		collection string
		want       bool
	}{
		{
			name: "global_admin_can_write_any",
			claims: &TokenClaims{
				Permissions: Permissions{GlobalAdmin: true},
			},
			collection: "any_collection",
			want:       true,
		},
		{
			name: "read_only_cannot_write",
			claims: &TokenClaims{
				Permissions: Permissions{
					Collections: map[string][]string{
						"test": {"READ_ONLY"},
					},
				},
			},
			collection: "test",
			want:       false,
		},
		{
			name: "read_write_can_write",
			claims: &TokenClaims{
				Permissions: Permissions{
					Collections: map[string][]string{
						"test": {"READ_WRITE"},
					},
				},
			},
			collection: "test",
			want:       true,
		},
		{
			name: "collection_admin_can_write",
			claims: &TokenClaims{
				Permissions: Permissions{
					Collections: map[string][]string{
						"test": {"COLLECTION_ADMIN"},
					},
				},
			},
			collection: "test",
			want:       true,
		},
		{
			name: "case_insensitive_read_write",
			claims: &TokenClaims{
				Permissions: Permissions{
					Collections: map[string][]string{
						"test": {"read_write"},
					},
				},
			},
			collection: "test",
			want:       true,
		},
		{
			name: "case_insensitive_collection_admin",
			claims: &TokenClaims{
				Permissions: Permissions{
					Collections: map[string][]string{
						"test": {"collection_admin"},
					},
				},
			},
			collection: "test",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.claims.CanWrite(tt.collection)
			if got != tt.want {
				t.Errorf("CanWrite(%s) = %v, want %v", tt.collection, got, tt.want)
			}
		})
	}
}

func TestCanAdmin(t *testing.T) {
	tests := []struct {
		name       string
		claims     *TokenClaims
		collection string
		want       bool
	}{
		{
			name: "global_admin_can_admin_any",
			claims: &TokenClaims{
				Permissions: Permissions{GlobalAdmin: true},
			},
			collection: "any_collection",
			want:       true,
		},
		{
			name: "read_only_cannot_admin",
			claims: &TokenClaims{
				Permissions: Permissions{
					Collections: map[string][]string{
						"test": {"READ_ONLY"},
					},
				},
			},
			collection: "test",
			want:       false,
		},
		{
			name: "read_write_cannot_admin",
			claims: &TokenClaims{
				Permissions: Permissions{
					Collections: map[string][]string{
						"test": {"READ_WRITE"},
					},
				},
			},
			collection: "test",
			want:       false,
		},
		{
			name: "collection_admin_can_admin",
			claims: &TokenClaims{
				Permissions: Permissions{
					Collections: map[string][]string{
						"test": {"COLLECTION_ADMIN"},
					},
				},
			},
			collection: "test",
			want:       true,
		},
		{
			name: "case_insensitive_collection_admin",
			claims: &TokenClaims{
				Permissions: Permissions{
					Collections: map[string][]string{
						"test": {"Collection_Admin"},
					},
				},
			},
			collection: "test",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.claims.CanAdmin(tt.collection)
			if got != tt.want {
				t.Errorf("CanAdmin(%s) = %v, want %v", tt.collection, got, tt.want)
			}
		})
	}
}

func TestPermissionsMultipleCollections(t *testing.T) {
	claims := &TokenClaims{
		Permissions: Permissions{
			GlobalAdmin: false,
			Collections: map[string][]string{
				"readonly_coll":  {"READ_ONLY"},
				"readwrite_coll": {"READ_WRITE"},
				"admin_coll":     {"COLLECTION_ADMIN"},
				"mixed_coll":     {"READ_ONLY", "READ_WRITE"},
			},
		},
	}

	// readonly_coll
	if !claims.CanRead("readonly_coll") {
		t.Error("Should be able to read readonly_coll")
	}
	if claims.CanWrite("readonly_coll") {
		t.Error("Should not be able to write readonly_coll")
	}
	if claims.CanAdmin("readonly_coll") {
		t.Error("Should not be able to admin readonly_coll")
	}

	// readwrite_coll
	if !claims.CanRead("readwrite_coll") {
		t.Error("Should be able to read readwrite_coll")
	}
	if !claims.CanWrite("readwrite_coll") {
		t.Error("Should be able to write readwrite_coll")
	}
	if claims.CanAdmin("readwrite_coll") {
		t.Error("Should not be able to admin readwrite_coll")
	}

	// admin_coll
	if !claims.CanRead("admin_coll") {
		t.Error("Should be able to read admin_coll")
	}
	if !claims.CanWrite("admin_coll") {
		t.Error("Should be able to write admin_coll")
	}
	if !claims.CanAdmin("admin_coll") {
		t.Error("Should be able to admin admin_coll")
	}

	// mixed_coll
	if !claims.CanRead("mixed_coll") {
		t.Error("Should be able to read mixed_coll")
	}
	if !claims.CanWrite("mixed_coll") {
		t.Error("Should be able to write mixed_coll")
	}

	// unknown_coll
	if claims.CanRead("unknown_coll") {
		t.Error("Should not be able to read unknown_coll")
	}
	if claims.CanWrite("unknown_coll") {
		t.Error("Should not be able to write unknown_coll")
	}
	if claims.CanAdmin("unknown_coll") {
		t.Error("Should not be able to admin unknown_coll")
	}
}

func TestTokenRoundTrip(t *testing.T) {
	tm := NewTokenManager("a-very-secure-secret-key-here!!")

	originalPerms := Permissions{
		GlobalAdmin: false,
		Collections: map[string][]string{
			"products":  {"READ_ONLY"},
			"users":     {"READ_WRITE"},
			"analytics": {"COLLECTION_ADMIN"},
		},
	}

	token, err := tm.GenerateToken("user@example.com", originalPerms, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	claims, err := tm.Validate(token)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	// Verify all permissions survived the round trip
	if claims.Subject != "user@example.com" {
		t.Errorf("Subject mismatch: got %s", claims.Subject)
	}

	if claims.Permissions.GlobalAdmin != originalPerms.GlobalAdmin {
		t.Error("GlobalAdmin mismatch")
	}

	for coll, expectedPerms := range originalPerms.Collections {
		actualPerms, exists := claims.Permissions.Collections[coll]
		if !exists {
			t.Errorf("Collection %s missing from claims", coll)
			continue
		}
		if len(actualPerms) != len(expectedPerms) {
			t.Errorf("Collection %s permissions count mismatch", coll)
		}
	}
}

func BenchmarkGenerateToken(b *testing.B) {
	tm := NewTokenManager("benchmark-secret-key-256-bits!!")
	perms := Permissions{
		GlobalAdmin: false,
		Collections: map[string][]string{
			"test": {"READ_WRITE"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := tm.GenerateToken("test-user", perms, time.Hour)
		if err != nil {
			b.Fatalf("GenerateToken() error = %v", err)
		}
	}
}

func BenchmarkValidateToken(b *testing.B) {
	tm := NewTokenManager("benchmark-secret-key-256-bits!!")
	perms := Permissions{
		GlobalAdmin: false,
		Collections: map[string][]string{
			"test": {"READ_WRITE"},
		},
	}

	token, _ := tm.GenerateToken("test-user", perms, time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := tm.Validate(token)
		if err != nil {
			b.Fatalf("Validate() error = %v", err)
		}
	}
}

func BenchmarkCanRead(b *testing.B) {
	claims := &TokenClaims{
		Permissions: Permissions{
			Collections: map[string][]string{
				"test1": {"READ_ONLY"},
				"test2": {"READ_WRITE"},
				"test3": {"COLLECTION_ADMIN"},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = claims.CanRead("test2")
	}
}
