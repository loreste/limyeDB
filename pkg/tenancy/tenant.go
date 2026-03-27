package tenancy

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Tenant represents a tenant in a multi-tenant deployment
type Tenant struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Status      TenantStatus      `json:"status"`
	Plan        TenantPlan        `json:"plan"`
	Quota       *ResourceQuota    `json:"quota"`
	Usage       *ResourceUsage    `json:"usage"`
	Settings    map[string]string `json:"settings"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Collections []string          `json:"collections"` // Collections owned by this tenant
}

// TenantStatus represents the status of a tenant
type TenantStatus string

const (
	TenantStatusActive    TenantStatus = "active"
	TenantStatusSuspended TenantStatus = "suspended"
	TenantStatusDeleted   TenantStatus = "deleted"
)

// TenantPlan represents the subscription plan
type TenantPlan string

const (
	PlanFree       TenantPlan = "free"
	PlanStarter    TenantPlan = "starter"
	PlanPro        TenantPlan = "pro"
	PlanEnterprise TenantPlan = "enterprise"
)

// ResourceQuota defines resource limits for a tenant
type ResourceQuota struct {
	MaxCollections     int      `json:"max_collections"`
	MaxPointsTotal     int64    `json:"max_points_total"`
	MaxPointsPerColl   int64    `json:"max_points_per_collection"`
	MaxStorageBytes    int64    `json:"max_storage_bytes"`
	MaxQueriesPerSec   int      `json:"max_queries_per_second"`
	MaxDimension       int      `json:"max_dimension"`
	MaxPayloadSize     int64    `json:"max_payload_size_bytes"`
	AllowedVectorTypes []string `json:"allowed_vector_types"` // e.g., ["float32", "uint8"]
}

// ResourceUsage tracks current resource usage
type ResourceUsage struct {
	Collections   int       `json:"collections"`
	PointsTotal   int64     `json:"points_total"`
	StorageBytes  int64     `json:"storage_bytes"`
	LastQueryTime time.Time `json:"last_query_time"`
	QueriesCount  int64     `json:"queries_count"`
}

// DefaultQuotas returns default quotas for each plan
func DefaultQuotas(plan TenantPlan) *ResourceQuota {
	switch plan {
	case PlanFree:
		return &ResourceQuota{
			MaxCollections:   3,
			MaxPointsTotal:   100000,
			MaxPointsPerColl: 50000,
			MaxStorageBytes:  100 * 1024 * 1024, // 100MB
			MaxQueriesPerSec: 10,
			MaxDimension:     1536,
			MaxPayloadSize:   1024, // 1KB
		}
	case PlanStarter:
		return &ResourceQuota{
			MaxCollections:   10,
			MaxPointsTotal:   1000000,
			MaxPointsPerColl: 500000,
			MaxStorageBytes:  1024 * 1024 * 1024, // 1GB
			MaxQueriesPerSec: 100,
			MaxDimension:     3072,
			MaxPayloadSize:   10 * 1024, // 10KB
		}
	case PlanPro:
		return &ResourceQuota{
			MaxCollections:   100,
			MaxPointsTotal:   10000000,
			MaxPointsPerColl: 5000000,
			MaxStorageBytes:  10 * 1024 * 1024 * 1024, // 10GB
			MaxQueriesPerSec: 1000,
			MaxDimension:     4096,
			MaxPayloadSize:   100 * 1024, // 100KB
		}
	case PlanEnterprise:
		return &ResourceQuota{
			MaxCollections:   -1, // Unlimited
			MaxPointsTotal:   -1,
			MaxPointsPerColl: -1,
			MaxStorageBytes:  -1,
			MaxQueriesPerSec: -1,
			MaxDimension:     -1,
			MaxPayloadSize:   -1,
		}
	default:
		return DefaultQuotas(PlanFree)
	}
}

// TenantManager manages tenants
type TenantManager struct {
	tenants map[string]*Tenant
	dataDir string
	mu      sync.RWMutex
}

// NewTenantManager creates a new tenant manager
func NewTenantManager(dataDir string) (*TenantManager, error) {
	tm := &TenantManager{
		tenants: make(map[string]*Tenant),
		dataDir: dataDir,
	}

	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return nil, err
	}

	if err := tm.loadTenants(); err != nil {
		return nil, err
	}

	return tm, nil
}

// loadTenants loads tenants from disk
func (tm *TenantManager) loadTenants() error {
	path := filepath.Clean(filepath.Join(tm.dataDir, "tenants.json"))
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &tm.tenants)
}

// saveTenants saves tenants to disk
func (tm *TenantManager) saveTenants() error {
	path := filepath.Join(tm.dataDir, "tenants.json")
	data, err := json.MarshalIndent(tm.tenants, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// Create creates a new tenant
func (tm *TenantManager) Create(id, name string, plan TenantPlan) (*Tenant, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, exists := tm.tenants[id]; exists {
		return nil, errors.New("tenant already exists")
	}

	tenant := &Tenant{
		ID:          id,
		Name:        name,
		Status:      TenantStatusActive,
		Plan:        plan,
		Quota:       DefaultQuotas(plan),
		Usage:       &ResourceUsage{},
		Settings:    make(map[string]string),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Collections: []string{},
	}

	tm.tenants[id] = tenant

	if err := tm.saveTenants(); err != nil {
		delete(tm.tenants, id)
		return nil, err
	}

	return tenant, nil
}

// Get retrieves a tenant by ID
func (tm *TenantManager) Get(id string) (*Tenant, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tenant, exists := tm.tenants[id]
	if !exists {
		return nil, errors.New("tenant not found")
	}

	return tenant, nil
}

// Update updates a tenant
func (tm *TenantManager) Update(id string, updates map[string]interface{}) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tenant, exists := tm.tenants[id]
	if !exists {
		return errors.New("tenant not found")
	}

	if name, ok := updates["name"].(string); ok {
		tenant.Name = name
	}
	if status, ok := updates["status"].(TenantStatus); ok {
		tenant.Status = status
	}
	if plan, ok := updates["plan"].(TenantPlan); ok {
		tenant.Plan = plan
		tenant.Quota = DefaultQuotas(plan)
	}

	tenant.UpdatedAt = time.Now()

	return tm.saveTenants()
}

// Delete deletes a tenant
func (tm *TenantManager) Delete(id string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, exists := tm.tenants[id]; !exists {
		return errors.New("tenant not found")
	}

	delete(tm.tenants, id)
	return tm.saveTenants()
}

// List returns all tenants
func (tm *TenantManager) List() []*Tenant {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tenants := make([]*Tenant, 0, len(tm.tenants))
	for _, t := range tm.tenants {
		tenants = append(tenants, t)
	}
	return tenants
}

// CheckQuota checks if an operation is within quota
func (tm *TenantManager) CheckQuota(tenantID string, op QuotaOperation) error {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tenant, exists := tm.tenants[tenantID]
	if !exists {
		return errors.New("tenant not found")
	}

	if tenant.Status != TenantStatusActive {
		return errors.New("tenant is not active")
	}

	quota := tenant.Quota
	usage := tenant.Usage

	switch op.Type {
	case OpCreateCollection:
		if quota.MaxCollections > 0 && usage.Collections >= quota.MaxCollections {
			return errors.New("collection quota exceeded")
		}
	case OpInsertPoints:
		if quota.MaxPointsTotal > 0 && usage.PointsTotal+op.Count > quota.MaxPointsTotal {
			return errors.New("total points quota exceeded")
		}
	case OpQuery:
		// Rate limiting would be implemented here
	}

	return nil
}

// UpdateUsage updates resource usage for a tenant
func (tm *TenantManager) UpdateUsage(tenantID string, delta *ResourceUsage) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tenant, exists := tm.tenants[tenantID]
	if !exists {
		return errors.New("tenant not found")
	}

	if delta.Collections != 0 {
		tenant.Usage.Collections += delta.Collections
	}
	if delta.PointsTotal != 0 {
		tenant.Usage.PointsTotal += delta.PointsTotal
	}
	if delta.StorageBytes != 0 {
		tenant.Usage.StorageBytes += delta.StorageBytes
	}
	if delta.QueriesCount != 0 {
		tenant.Usage.QueriesCount += delta.QueriesCount
		tenant.Usage.LastQueryTime = time.Now()
	}

	return tm.saveTenants()
}

// AddCollection adds a collection to a tenant
func (tm *TenantManager) AddCollection(tenantID, collectionName string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tenant, exists := tm.tenants[tenantID]
	if !exists {
		return errors.New("tenant not found")
	}

	tenant.Collections = append(tenant.Collections, collectionName)
	tenant.Usage.Collections++
	tenant.UpdatedAt = time.Now()

	return tm.saveTenants()
}

// RemoveCollection removes a collection from a tenant
func (tm *TenantManager) RemoveCollection(tenantID, collectionName string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tenant, exists := tm.tenants[tenantID]
	if !exists {
		return errors.New("tenant not found")
	}

	for i, name := range tenant.Collections {
		if name == collectionName {
			tenant.Collections = append(tenant.Collections[:i], tenant.Collections[i+1:]...)
			tenant.Usage.Collections--
			break
		}
	}

	tenant.UpdatedAt = time.Now()
	return tm.saveTenants()
}

// QuotaOperationType represents the type of operation
type QuotaOperationType string

const (
	OpCreateCollection QuotaOperationType = "create_collection"
	OpInsertPoints     QuotaOperationType = "insert_points"
	OpQuery            QuotaOperationType = "query"
)

// QuotaOperation represents an operation to check against quotas
type QuotaOperation struct {
	Type  QuotaOperationType
	Count int64
}

// IsolationLevel represents the level of tenant isolation
type IsolationLevel string

const (
	// IsolationShared - tenants share resources (namespace isolation only)
	IsolationShared IsolationLevel = "shared"
	// IsolationDedicated - tenant has dedicated resources
	IsolationDedicated IsolationLevel = "dedicated"
)
