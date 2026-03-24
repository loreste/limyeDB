package auth

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Permissions map a collection name to a list of allowed actions (e.g., "READ_ONLY", "COLLECTION_ADMIN")
type Permissions struct {
	GlobalAdmin bool                `json:"global_admin"`
	Collections map[string][]string `json:"collections"`
}

// TokenClaims represents the custom JWT claims used by LimyeDB
type TokenClaims struct {
	Permissions Permissions `json:"limyedb_permissions"`
	jwt.RegisteredClaims
}

// TokenManager handles JWT validation and parsing
type TokenManager struct {
	secretKey []byte
}

// NewTokenManager creates a new JWT token manager
func NewTokenManager(secret string) *TokenManager {
	return &TokenManager{
		secretKey: []byte(secret),
	}
}

// GenerateToken creates a new JWT with specific permissions
func (m *TokenManager) GenerateToken(subject string, permissions Permissions, ttl time.Duration) (string, error) {
	claims := TokenClaims{
		Permissions: permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secretKey)
}

// Validate parses and validates a JWT string natively
func (m *TokenManager) Validate(tokenString string) (*TokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return m.secretKey, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*TokenClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token claims")
}

// CanRead checks if the claims allow reading a specific collection
func (c *TokenClaims) CanRead(collection string) bool {
	if c.Permissions.GlobalAdmin {
		return true
	}
	perms, exists := c.Permissions.Collections[collection]
	if !exists {
		return false
	}
	for _, p := range perms {
		if p == "READ_ONLY" || p == "COLLECTION_ADMIN" || p == "READ_WRITE" {
			return true
		}
	}
	return false
}

// CanWrite checks if the claims allow modifying a specific collection
func (c *TokenClaims) CanWrite(collection string) bool {
	if c.Permissions.GlobalAdmin {
		return true
	}
	perms, exists := c.Permissions.Collections[collection]
	if !exists {
		return false
	}
	for _, p := range perms {
		if strings.EqualFold(p, "COLLECTION_ADMIN") || strings.EqualFold(p, "READ_WRITE") {
			return true
		}
	}
	return false
}

// CanAdmin checks if the claims give administrative rights over a specific collection
func (c *TokenClaims) CanAdmin(collection string) bool {
	if c.Permissions.GlobalAdmin {
		return true
	}
	perms, exists := c.Permissions.Collections[collection]
	if !exists {
		return false
	}
	for _, p := range perms {
		if strings.EqualFold(p, "COLLECTION_ADMIN") {
			return true
		}
	}
	return false
}
