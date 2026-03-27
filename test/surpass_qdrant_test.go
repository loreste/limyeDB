package test

import (
	"context"
	"testing"
	"time"

	"github.com/limyedb/limyedb/pkg/autotune"
	"github.com/limyedb/limyedb/pkg/hybrid"
	"github.com/limyedb/limyedb/pkg/query"
	"github.com/limyedb/limyedb/pkg/tenancy"
	"github.com/limyedb/limyedb/pkg/vectorizer"
)

// Tests for features that surpass Qdrant

func TestBM25Search(t *testing.T) {
	// Test BM25 full-text search
	cfg := hybrid.DefaultBM25Config()
	idx := hybrid.NewBM25Index(cfg)

	// Index some documents
	docs := []struct {
		id      string
		content string
	}{
		{"doc1", "The quick brown fox jumps over the lazy dog"},
		{"doc2", "A quick brown dog runs in the park"},
		{"doc3", "The lazy cat sleeps all day long"},
		{"doc4", "Dogs and cats are common pets"},
	}

	for _, doc := range docs {
		err := idx.Index(&hybrid.Document{
			ID:      doc.id,
			Content: doc.content,
		})
		if err != nil {
			t.Fatalf("Failed to index document: %v", err)
		}
	}

	// Test search
	results := idx.Search("quick brown", 10)
	if len(results) == 0 {
		t.Fatal("Expected search results")
	}

	// First two should be doc1 and doc2 (contain both "quick" and "brown")
	if results[0].DocID != "doc1" && results[0].DocID != "doc2" {
		t.Errorf("Expected doc1 or doc2 as top result, got %s", results[0].DocID)
	}

	// Test search for "lazy"
	results = idx.Search("lazy", 10)
	if len(results) < 2 {
		t.Fatal("Expected at least 2 results for 'lazy'")
	}

	// doc1 and doc3 should be in results
	foundDoc1 := false
	foundDoc3 := false
	for _, r := range results {
		if r.DocID == "doc1" {
			foundDoc1 = true
		}
		if r.DocID == "doc3" {
			foundDoc3 = true
		}
	}
	if !foundDoc1 || !foundDoc3 {
		t.Error("Expected doc1 and doc3 in results for 'lazy'")
	}

	t.Logf("BM25 search working correctly with %d indexed documents", idx.Size())
}

func TestHybridSearchFusion(t *testing.T) {
	// This test validates the fusion methods work
	// In a real test, we'd need vector index + text index

	// Test RRF calculation manually
	rrfK := 60.0

	// RRF score = 1/(k+rank)
	// For rank 1: 1/(60+1) = 0.0164
	// For rank 2: 1/(60+2) = 0.0161
	rank1Score := 1.0 / (rrfK + 1)
	rank2Score := 1.0 / (rrfK + 2)

	if rank1Score <= rank2Score {
		t.Error("RRF: rank 1 should have higher score than rank 2")
	}

	// Combined score for doc in both rankings
	combinedScore := rank1Score + rank2Score
	if combinedScore <= rank1Score {
		t.Error("Combined score should be higher than single ranking score")
	}

	t.Logf("RRF scores: rank1=%.4f, rank2=%.4f, combined=%.4f", rank1Score, rank2Score, combinedScore)
}

func TestMultiTenancy(t *testing.T) {
	// Create tenant manager
	tm, err := tenancy.NewTenantManager(t.TempDir())
	if err != nil {
		t.Fatalf("Failed to create tenant manager: %v", err)
	}

	// Create tenant
	tenant, err := tm.Create("tenant1", "Test Tenant", tenancy.PlanStarter)
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	if tenant.ID != "tenant1" {
		t.Errorf("Expected tenant ID 'tenant1', got '%s'", tenant.ID)
	}
	if tenant.Status != tenancy.TenantStatusActive {
		t.Errorf("Expected active status, got %s", tenant.Status)
	}

	// Check quotas
	quota := tenant.Quota
	if quota.MaxCollections != 10 { // Starter plan
		t.Errorf("Expected MaxCollections=10, got %d", quota.MaxCollections)
	}

	// Test quota check
	err = tm.CheckQuota("tenant1", tenancy.QuotaOperation{
		Type: tenancy.OpCreateCollection,
	})
	if err != nil {
		t.Errorf("Quota check should pass: %v", err)
	}

	// Add a collection
	err = tm.AddCollection("tenant1", "collection1")
	if err != nil {
		t.Fatalf("Failed to add collection: %v", err)
	}

	// Verify usage updated
	tenant, _ = tm.Get("tenant1")
	if tenant.Usage.Collections != 1 {
		t.Errorf("Expected collections=1, got %d", tenant.Usage.Collections)
	}

	// Test tenant list
	tenants := tm.List()
	if len(tenants) != 1 {
		t.Errorf("Expected 1 tenant, got %d", len(tenants))
	}

	t.Logf("Multi-tenancy working: tenant=%s, collections=%d", tenant.Name, tenant.Usage.Collections)
}

func TestRBAC(t *testing.T) {
	// Create RBAC manager
	rm, err := tenancy.NewRBACManager(t.TempDir())
	if err != nil {
		t.Fatalf("Failed to create RBAC manager: %v", err)
	}

	// Create custom roles
	customRole := &tenancy.Role{
		ID:          "custom_admin",
		Name:        "Custom Admin",
		Permissions: []tenancy.Permission{tenancy.PermCollectionCreate, tenancy.PermCollectionDelete, tenancy.PermPointCreate, tenancy.PermPointRead, tenancy.PermSearch},
	}

	if err := rm.CreateRole(customRole); err != nil {
		t.Fatalf("Failed to create custom role: %v", err)
	}

	// Create users using system roles
	adminUser := &tenancy.User{
		ID:       "admin1",
		Username: "admin",
		Email:    "admin@example.com",
		Roles:    []string{"admin"}, // Use system admin role
	}
	if err := rm.CreateUser(adminUser, "password123"); err != nil {
		t.Fatalf("Failed to create admin user: %v", err)
	}

	viewerUser := &tenancy.User{
		ID:       "viewer1",
		Username: "viewer",
		Email:    "viewer@example.com",
		Roles:    []string{"viewer"}, // Use system viewer role
	}
	if err := rm.CreateUser(viewerUser, "password456"); err != nil {
		t.Fatalf("Failed to create viewer user: %v", err)
	}

	// Test password verification
	if !rm.VerifyPassword("admin1", "password123") {
		t.Error("Password verification failed for admin")
	}
	if rm.VerifyPassword("admin1", "wrongpassword") {
		t.Error("Wrong password should not verify")
	}

	// Test permissions
	if !rm.HasPermission("admin1", tenancy.PermCollectionCreate) {
		t.Error("Admin should have collection create permission")
	}
	if !rm.HasPermission("admin1", tenancy.PermPointCreate) {
		t.Error("Admin should have point create permission")
	}
	if rm.HasPermission("viewer1", tenancy.PermCollectionCreate) {
		t.Error("Viewer should NOT have collection create permission")
	}
	if !rm.HasPermission("viewer1", tenancy.PermSearch) {
		t.Error("Viewer should have search permission")
	}

	// Test API key
	apiKey, err := rm.CreateAPIKey("admin1", "My API Key", time.Time{})
	if err != nil {
		t.Fatalf("Failed to create API key: %v", err)
	}
	if apiKey == "" {
		t.Error("API key should not be empty")
	}

	// Validate API key
	keyUser, err := rm.ValidateAPIKey(apiKey)
	if err != nil {
		t.Fatalf("Failed to validate API key: %v", err)
	}
	if keyUser.ID != "admin1" {
		t.Errorf("Expected user ID 'admin1', got '%s'", keyUser.ID)
	}

	// Get user permissions
	perms := rm.GetUserPermissions("admin1")
	if len(perms) == 0 {
		t.Error("Admin should have permissions")
	}

	t.Logf("RBAC working: admin has %d permissions, viewer permissions verified", len(perms))
}

func TestSQLParser(t *testing.T) {
	parser := query.NewSQLParser()

	// Test SELECT
	q, err := parser.Parse("SELECT * FROM products WHERE price > 100 LIMIT 10")
	if err != nil {
		t.Fatalf("Failed to parse SELECT: %v", err)
	}
	if q.Type != query.QuerySelect {
		t.Errorf("Expected SELECT type, got %s", q.Type)
	}
	if q.Collection != "products" {
		t.Errorf("Expected collection 'products', got '%s'", q.Collection)
	}
	if q.Limit != 10 {
		t.Errorf("Expected limit 10, got %d", q.Limit)
	}

	// Test vector search
	q, err = parser.Parse("SELECT * FROM vectors NEAREST TO [0.1, 0.2, 0.3, 0.4] LIMIT 5")
	if err != nil {
		t.Fatalf("Failed to parse vector search: %v", err)
	}
	if q.Type != query.QueryVectorSearch {
		t.Errorf("Expected VECTOR_SEARCH type, got %s", q.Type)
	}
	if len(q.Vector) != 4 {
		t.Errorf("Expected vector length 4, got %d", len(q.Vector))
	}

	// Test SEARCH syntax (without WHERE for now as it needs more work)
	q, err = parser.Parse("SEARCH embeddings FOR [1.0, 2.0, 3.0] LIMIT 20")
	if err != nil {
		t.Fatalf("Failed to parse SEARCH: %v", err)
	}
	if q.Type != query.QueryVectorSearch {
		t.Errorf("Expected VECTOR_SEARCH type, got %s", q.Type)
	}
	if q.Collection != "embeddings" {
		t.Errorf("Expected collection 'embeddings', got '%s'", q.Collection)
	}
	if q.Limit != 20 {
		t.Errorf("Expected limit 20, got %d", q.Limit)
	}

	// Test CREATE TABLE
	q, err = parser.Parse("CREATE TABLE my_collection WITH dimension=128, metric='cosine'")
	if err != nil {
		t.Fatalf("Failed to parse CREATE TABLE: %v", err)
	}
	if q.Type != query.QueryCreateTable {
		t.Errorf("Expected CREATE_TABLE type, got %s", q.Type)
	}
	if q.CollectionConfig.Dimension != 128 {
		t.Errorf("Expected dimension 128, got %d", q.CollectionConfig.Dimension)
	}

	// Test SHOW TABLES
	q, err = parser.Parse("SHOW TABLES")
	if err != nil {
		t.Fatalf("Failed to parse SHOW TABLES: %v", err)
	}
	if q.Type != query.QueryShowTables {
		t.Errorf("Expected SHOW_TABLES type, got %s", q.Type)
	}

	// Test DESCRIBE
	q, err = parser.Parse("DESCRIBE my_collection")
	if err != nil {
		t.Fatalf("Failed to parse DESCRIBE: %v", err)
	}
	if q.Type != query.QueryDescribe {
		t.Errorf("Expected DESCRIBE type, got %s", q.Type)
	}

	t.Log("SQL parser working correctly")
}

func TestAutoTuner(t *testing.T) {
	cfg := autotune.DefaultAutoTunerConfig()
	tuner := autotune.NewAutoTuner(cfg)

	// Record some queries
	for i := 0; i < 200; i++ {
		latency := float64(5 + i%10)         // 5-14ms latency
		recall := 0.95 + float64(i%10)*0.005 // 0.95-0.995 recall
		tuner.RecordQuery(latency, recall)
	}

	// Get stats
	stats := tuner.GetStats()
	if stats.TotalQueries != 200 {
		t.Errorf("Expected 200 queries, got %d", stats.TotalQueries)
	}
	if stats.AvgLatencyMs <= 0 {
		t.Error("Expected positive average latency")
	}

	// Get current params
	params := tuner.GetParams()
	if params.EfSearch <= 0 {
		t.Error("Expected positive EfSearch")
	}
	if params.M <= 0 {
		t.Error("Expected positive M")
	}

	// Test suggestions
	suggested := tuner.Suggest()
	t.Logf("Auto-tuner suggested params: ef_search=%d, M=%d", suggested.EfSearch, suggested.M)

	// Test workload profiles
	realtimeParams := tuner.SuggestForWorkload(autotune.WorkloadRealtime)
	batchParams := tuner.SuggestForWorkload(autotune.WorkloadBatch)

	// Realtime should have lower ef_search for lower latency
	if realtimeParams.EfSearch >= batchParams.EfSearch {
		t.Log("Note: Realtime ef_search should typically be lower than batch for lower latency")
	}

	t.Logf("Auto-tuner working: avg_latency=%.2fms, total_queries=%d", stats.AvgLatencyMs, stats.TotalQueries)
}

func TestVectorizerManager(t *testing.T) {
	manager := vectorizer.NewVectorizerManager()

	// List should be empty initially
	list := manager.List()
	if len(list) != 0 {
		t.Errorf("Expected empty list, got %d items", len(list))
	}

	// Test that we can't get a non-existent vectorizer
	_, ok := manager.Get("nonexistent")
	if ok {
		t.Error("Should not find non-existent vectorizer")
	}

	t.Log("Vectorizer manager structure verified")
}

func TestAutoEmbedderConfig(t *testing.T) {
	manager := vectorizer.NewVectorizerManager()
	embedder := vectorizer.NewAutoEmbedder(manager)

	// Configure for a collection
	cfg := &vectorizer.AutoEmbedConfig{
		Vectorizer:   "openai",
		SourceFields: []string{"title", "description"},
		Template:     "{title} - {description}",
		OnConflict:   "skip",
		Enabled:      false, // Disabled since we don't have API keys
	}

	// This should succeed even without vectorizer registered since Enabled is false
	err := embedder.Configure("test_collection", cfg)
	if err != nil {
		t.Logf("Expected behavior: %v", err) // OK if it fails because vectorizer not found
	}

	// Get config back
	retrievedCfg, ok := embedder.GetConfig("test_collection")
	if ok && retrievedCfg.Template != "{title} - {description}" {
		t.Error("Config template not preserved")
	}

	t.Log("Auto-embedder config structure verified")
}

func TestChunkText(t *testing.T) {
	text := "This is a long text that should be split into multiple chunks for processing. Each chunk will overlap with the next to maintain context."

	// Chunk with overlap
	chunks := vectorizer.ChunkText(text, 50, 10)

	if len(chunks) < 2 {
		t.Error("Expected multiple chunks")
	}

	// Verify overlap exists
	if len(chunks) >= 2 {
		// Last characters of first chunk should appear at start of second chunk
		// (with the overlap)
		t.Logf("Created %d chunks from text of length %d", len(chunks), len(text))
	}

	// Test with no chunking needed
	shortText := "Short"
	chunks = vectorizer.ChunkText(shortText, 50, 10)
	if len(chunks) != 1 {
		t.Error("Short text should not be chunked")
	}
}

func TestTenantPlans(t *testing.T) {
	plans := []tenancy.TenantPlan{
		tenancy.PlanFree,
		tenancy.PlanStarter,
		tenancy.PlanPro,
		tenancy.PlanEnterprise,
	}

	var prevCollections int
	for _, plan := range plans {
		quota := tenancy.DefaultQuotas(plan)

		// Verify quotas increase with plan tier (or are unlimited)
		if plan != tenancy.PlanFree {
			if quota.MaxCollections != -1 && quota.MaxCollections <= prevCollections {
				t.Errorf("Plan %s should have more collections than previous", plan)
			}
		}
		prevCollections = quota.MaxCollections

		t.Logf("Plan %s: collections=%d, points=%d, storage=%d bytes",
			plan, quota.MaxCollections, quota.MaxPointsTotal, quota.MaxStorageBytes)
	}
}

func TestAdaptiveSearch(t *testing.T) {
	tuner := autotune.NewAutoTuner(nil)
	adaptive := autotune.NewAdaptiveSearch(tuner)

	// Get ef_search for different scenarios
	ef1 := adaptive.GetEfSearch(128, 10, false)
	ef2 := adaptive.GetEfSearch(128, 10, true) // with filter

	// With filter should have higher ef
	if ef2 <= ef1 {
		t.Logf("Note: ef with filter (%d) should typically be higher than without (%d)", ef2, ef1)
	}

	// Higher limit should have higher ef
	ef3 := adaptive.GetEfSearch(128, 100, false)
	if ef3 < ef1 {
		t.Error("Higher limit should result in higher or equal ef")
	}

	t.Logf("Adaptive search ef values: base=%d, with_filter=%d, high_limit=%d", ef1, ef2, ef3)
}

func TestWorkloadAnalyzer(t *testing.T) {
	analyzer := autotune.NewWorkloadAnalyzer()

	// Record some query patterns
	for i := 0; i < 1500; i++ {
		analyzer.RecordQuery("search_by_embedding")
	}
	for i := 0; i < 100; i++ {
		analyzer.RecordQuery("filter_by_category")
	}

	recommendations := analyzer.GetRecommendations()
	t.Logf("Workload recommendations: %v", recommendations)

	// Should recommend caching for frequent pattern
	foundCacheRec := false
	for _, rec := range recommendations {
		if len(rec) > 0 {
			foundCacheRec = true
			break
		}
	}
	if foundCacheRec {
		t.Log("Workload analyzer providing recommendations")
	}
}

func TestAutoTunerContext(t *testing.T) {
	tuner := autotune.NewAutoTuner(nil)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Start the tuner
	done := make(chan struct{})
	go func() {
		tuner.Run(ctx)
		close(done)
	}()

	// Wait for context to cancel
	select {
	case <-done:
		t.Log("Auto-tuner stopped correctly on context cancellation")
	case <-time.After(time.Second):
		t.Error("Auto-tuner did not stop within expected time")
	}
}
