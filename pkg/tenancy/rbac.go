package tenancy

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Permission represents a specific permission
type Permission string

const (
	// Collection permissions
	PermCollectionCreate Permission = "collection:create"
	PermCollectionRead   Permission = "collection:read"
	PermCollectionUpdate Permission = "collection:update"
	PermCollectionDelete Permission = "collection:delete"

	// Point permissions
	PermPointCreate Permission = "point:create"
	PermPointRead   Permission = "point:read"
	PermPointUpdate Permission = "point:update"
	PermPointDelete Permission = "point:delete"

	// Search permissions
	PermSearch    Permission = "search:query"
	PermRecommend Permission = "search:recommend"

	// Admin permissions
	PermAdminUsers       Permission = "admin:users"
	PermAdminRoles       Permission = "admin:roles"
	PermAdminTenants     Permission = "admin:tenants"
	PermAdminSnapshots   Permission = "admin:snapshots"
	PermAdminCluster     Permission = "admin:cluster"
	PermAdminAll         Permission = "admin:*"
)

// Role represents a role with a set of permissions
type Role struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Permissions []Permission `json:"permissions"`
	TenantID    string       `json:"tenant_id,omitempty"` // Empty for global roles
	IsSystem    bool         `json:"is_system"`           // System roles can't be deleted
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// User represents a user in the system
type User struct {
	ID           string            `json:"id"`
	Username     string            `json:"username"`
	Email        string            `json:"email"`
	PasswordHash string            `json:"password_hash"`
	TenantID     string            `json:"tenant_id"`
	Roles        []string          `json:"roles"` // Role IDs
	APIKeys      []APIKey          `json:"api_keys"`
	Status       UserStatus        `json:"status"`
	Metadata     map[string]string `json:"metadata"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	LastLoginAt  time.Time         `json:"last_login_at"`
}

// UserStatus represents the status of a user
type UserStatus string

const (
	UserStatusActive    UserStatus = "active"
	UserStatusInactive  UserStatus = "inactive"
	UserStatusSuspended UserStatus = "suspended"
)

// APIKey represents an API key for authentication
type APIKey struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	KeyHash     string    `json:"key_hash"` // Store hash, not actual key
	Prefix      string    `json:"prefix"`   // First 8 chars for identification
	Permissions []Permission `json:"permissions,omitempty"` // Optional key-specific permissions
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
	LastUsedAt  time.Time `json:"last_used_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// RBACManager manages roles and permissions
type RBACManager struct {
	roles   map[string]*Role
	users   map[string]*User
	dataDir string
	mu      sync.RWMutex
}

// NewRBACManager creates a new RBAC manager
func NewRBACManager(dataDir string) (*RBACManager, error) {
	rm := &RBACManager{
		roles:   make(map[string]*Role),
		users:   make(map[string]*User),
		dataDir: dataDir,
	}

	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return nil, err
	}

	// Create system roles
	rm.createSystemRoles()

	// Load existing data
	if err := rm.loadRoles(); err != nil {
		return nil, err
	}
	if err := rm.loadUsers(); err != nil {
		return nil, err
	}

	return rm, nil
}

// createSystemRoles creates default system roles
func (rm *RBACManager) createSystemRoles() {
	// Super Admin - full access
	rm.roles["super_admin"] = &Role{
		ID:          "super_admin",
		Name:        "Super Admin",
		Description: "Full system access",
		Permissions: []Permission{PermAdminAll},
		IsSystem:    true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Admin - manage tenant resources
	rm.roles["admin"] = &Role{
		ID:          "admin",
		Name:        "Admin",
		Description: "Tenant administrator",
		Permissions: []Permission{
			PermCollectionCreate, PermCollectionRead, PermCollectionUpdate, PermCollectionDelete,
			PermPointCreate, PermPointRead, PermPointUpdate, PermPointDelete,
			PermSearch, PermRecommend,
			PermAdminUsers, PermAdminRoles,
		},
		IsSystem:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Editor - read and write access
	rm.roles["editor"] = &Role{
		ID:          "editor",
		Name:        "Editor",
		Description: "Read and write access",
		Permissions: []Permission{
			PermCollectionRead,
			PermPointCreate, PermPointRead, PermPointUpdate, PermPointDelete,
			PermSearch, PermRecommend,
		},
		IsSystem:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Viewer - read-only access
	rm.roles["viewer"] = &Role{
		ID:          "viewer",
		Name:        "Viewer",
		Description: "Read-only access",
		Permissions: []Permission{
			PermCollectionRead,
			PermPointRead,
			PermSearch, PermRecommend,
		},
		IsSystem:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// loadRoles loads roles from disk
func (rm *RBACManager) loadRoles() error {
	path := filepath.Join(rm.dataDir, "roles.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var roles map[string]*Role
	if err := json.Unmarshal(data, &roles); err != nil {
		return err
	}

	// Merge with system roles
	for id, role := range roles {
		if _, isSystem := rm.roles[id]; !isSystem {
			rm.roles[id] = role
		}
	}

	return nil
}

// loadUsers loads users from disk
func (rm *RBACManager) loadUsers() error {
	path := filepath.Join(rm.dataDir, "users.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &rm.users)
}

// saveRoles saves roles to disk
func (rm *RBACManager) saveRoles() error {
	path := filepath.Join(rm.dataDir, "roles.json")
	data, err := json.MarshalIndent(rm.roles, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// saveUsers saves users to disk
func (rm *RBACManager) saveUsers() error {
	path := filepath.Join(rm.dataDir, "users.json")
	data, err := json.MarshalIndent(rm.users, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// CreateRole creates a new role
func (rm *RBACManager) CreateRole(role *Role) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.roles[role.ID]; exists {
		return errors.New("role already exists")
	}

	role.CreatedAt = time.Now()
	role.UpdatedAt = time.Now()
	rm.roles[role.ID] = role

	return rm.saveRoles()
}

// GetRole retrieves a role by ID
func (rm *RBACManager) GetRole(id string) (*Role, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	role, exists := rm.roles[id]
	if !exists {
		return nil, errors.New("role not found")
	}
	return role, nil
}

// UpdateRole updates a role
func (rm *RBACManager) UpdateRole(id string, permissions []Permission) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	role, exists := rm.roles[id]
	if !exists {
		return errors.New("role not found")
	}

	if role.IsSystem {
		return errors.New("cannot modify system role")
	}

	role.Permissions = permissions
	role.UpdatedAt = time.Now()

	return rm.saveRoles()
}

// DeleteRole deletes a role
func (rm *RBACManager) DeleteRole(id string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	role, exists := rm.roles[id]
	if !exists {
		return errors.New("role not found")
	}

	if role.IsSystem {
		return errors.New("cannot delete system role")
	}

	delete(rm.roles, id)
	return rm.saveRoles()
}

// ListRoles returns all roles
func (rm *RBACManager) ListRoles() []*Role {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	roles := make([]*Role, 0, len(rm.roles))
	for _, role := range rm.roles {
		roles = append(roles, role)
	}
	return roles
}

// CreateUser creates a new user
func (rm *RBACManager) CreateUser(user *User, password string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.users[user.ID]; exists {
		return errors.New("user already exists")
	}

	// Hash password
	user.PasswordHash = hashPassword(password)
	user.Status = UserStatusActive
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()
	user.APIKeys = []APIKey{}

	rm.users[user.ID] = user

	return rm.saveUsers()
}

// GetUser retrieves a user by ID
func (rm *RBACManager) GetUser(id string) (*User, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	user, exists := rm.users[id]
	if !exists {
		return nil, errors.New("user not found")
	}
	return user, nil
}

// GetUserByUsername retrieves a user by username
func (rm *RBACManager) GetUserByUsername(username string) (*User, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	for _, user := range rm.users {
		if user.Username == username {
			return user, nil
		}
	}
	return nil, errors.New("user not found")
}

// UpdateUser updates a user
func (rm *RBACManager) UpdateUser(id string, updates map[string]interface{}) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	user, exists := rm.users[id]
	if !exists {
		return errors.New("user not found")
	}

	if email, ok := updates["email"].(string); ok {
		user.Email = email
	}
	if roles, ok := updates["roles"].([]string); ok {
		user.Roles = roles
	}
	if status, ok := updates["status"].(UserStatus); ok {
		user.Status = status
	}

	user.UpdatedAt = time.Now()
	return rm.saveUsers()
}

// DeleteUser deletes a user
func (rm *RBACManager) DeleteUser(id string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.users[id]; !exists {
		return errors.New("user not found")
	}

	delete(rm.users, id)
	return rm.saveUsers()
}

// ListUsers returns all users (optionally filtered by tenant)
func (rm *RBACManager) ListUsers(tenantID string) []*User {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	users := make([]*User, 0)
	for _, user := range rm.users {
		if tenantID == "" || user.TenantID == tenantID {
			users = append(users, user)
		}
	}
	return users
}

// VerifyPassword verifies a user's password
func (rm *RBACManager) VerifyPassword(userID, password string) bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	user, exists := rm.users[userID]
	if !exists {
		return false
	}

	// Use bcrypt's secure comparison to prevent timing attacks
	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	return err == nil
}

// CreateAPIKey creates a new API key for a user
func (rm *RBACManager) CreateAPIKey(userID, name string, expiresAt time.Time) (string, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	user, exists := rm.users[userID]
	if !exists {
		return "", errors.New("user not found")
	}

	// Generate random key
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", err
	}
	key := hex.EncodeToString(keyBytes)

	apiKey := APIKey{
		ID:        generateID(),
		Name:      name,
		KeyHash:   hashAPIKey(key),
		Prefix:    key[:8],
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}

	user.APIKeys = append(user.APIKeys, apiKey)
	user.UpdatedAt = time.Now()

	if err := rm.saveUsers(); err != nil {
		return "", err
	}

	// Return the actual key (only time it's available)
	return key, nil
}

// ValidateAPIKey validates an API key and returns the associated user
func (rm *RBACManager) ValidateAPIKey(key string) (*User, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	keyHash := hashAPIKey(key)
	prefix := key[:8]

	for _, user := range rm.users {
		for i := range user.APIKeys {
			apiKey := &user.APIKeys[i]
			if apiKey.Prefix == prefix && apiKey.KeyHash == keyHash {
				// Check expiration
				if !apiKey.ExpiresAt.IsZero() && time.Now().After(apiKey.ExpiresAt) {
					return nil, errors.New("API key expired")
				}

				// Update last used
				apiKey.LastUsedAt = time.Now()
				return user, nil
			}
		}
	}

	return nil, errors.New("invalid API key")
}

// RevokeAPIKey revokes an API key
func (rm *RBACManager) RevokeAPIKey(userID, keyID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	user, exists := rm.users[userID]
	if !exists {
		return errors.New("user not found")
	}

	for i, key := range user.APIKeys {
		if key.ID == keyID {
			user.APIKeys = append(user.APIKeys[:i], user.APIKeys[i+1:]...)
			user.UpdatedAt = time.Now()
			return rm.saveUsers()
		}
	}

	return errors.New("API key not found")
}

// HasPermission checks if a user has a specific permission
func (rm *RBACManager) HasPermission(userID string, perm Permission) bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	user, exists := rm.users[userID]
	if !exists || user.Status != UserStatusActive {
		return false
	}

	for _, roleID := range user.Roles {
		role, exists := rm.roles[roleID]
		if !exists {
			continue
		}

		for _, p := range role.Permissions {
			if p == perm || p == PermAdminAll {
				return true
			}
		}
	}

	return false
}

// GetUserPermissions returns all permissions for a user
func (rm *RBACManager) GetUserPermissions(userID string) []Permission {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	user, exists := rm.users[userID]
	if !exists {
		return nil
	}

	permSet := make(map[Permission]bool)
	for _, roleID := range user.Roles {
		role, exists := rm.roles[roleID]
		if !exists {
			continue
		}

		for _, p := range role.Permissions {
			permSet[p] = true
		}
	}

	perms := make([]Permission, 0, len(permSet))
	for p := range permSet {
		perms = append(perms, p)
	}
	return perms
}

// AssignRole assigns a role to a user
func (rm *RBACManager) AssignRole(userID, roleID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	user, exists := rm.users[userID]
	if !exists {
		return errors.New("user not found")
	}

	if _, exists := rm.roles[roleID]; !exists {
		return errors.New("role not found")
	}

	// Check if already assigned
	for _, r := range user.Roles {
		if r == roleID {
			return nil
		}
	}

	user.Roles = append(user.Roles, roleID)
	user.UpdatedAt = time.Now()

	return rm.saveUsers()
}

// RemoveRole removes a role from a user
func (rm *RBACManager) RemoveRole(userID, roleID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	user, exists := rm.users[userID]
	if !exists {
		return errors.New("user not found")
	}

	for i, r := range user.Roles {
		if r == roleID {
			user.Roles = append(user.Roles[:i], user.Roles[i+1:]...)
			user.UpdatedAt = time.Now()
			return rm.saveUsers()
		}
	}

	return nil
}

// Helper functions

func hashPassword(password string) string {
	// Use bcrypt for secure password hashing (cost factor 12 provides good security/performance balance)
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		// Fallback should never happen with valid input, but return empty to indicate failure
		return ""
	}
	return string(hash)
}

func hashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

func generateID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
