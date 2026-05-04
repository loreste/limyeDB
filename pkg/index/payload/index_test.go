package payload

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewIndex(t *testing.T) {
	t.Parallel()

	// Test in-memory index
	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	if idx == nil {
		t.Fatal("NewIndex returned nil")
	}
}

func TestNewIndexWithPath(t *testing.T) {
	t.Parallel()

	// Create temp directory
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	idx, err := NewIndex(dbPath)
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	// Check file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Expected database file to exist")
	}
}

func TestIndexPoint(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	payload := map[string]interface{}{
		"name": "John",
		"age":  30,
	}

	idx.IndexPoint(1, payload)
	// Should not panic
}

func TestIndexPointEmpty(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	// Empty payload should be ignored
	idx.IndexPoint(1, nil)
	idx.IndexPoint(2, map[string]interface{}{})
	// Should not panic
}

func TestRemovePoint(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	payload := map[string]interface{}{"key": "value"}
	idx.IndexPoint(1, payload)
	idx.RemovePoint(1, payload)
	// Should not panic
}

func TestFilter(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	// Index some points
	idx.IndexPoint(1, map[string]interface{}{"category": "A", "price": 100})
	idx.IndexPoint(2, map[string]interface{}{"category": "B", "price": 200})
	idx.IndexPoint(3, map[string]interface{}{"category": "A", "price": 150})

	// Filter by category
	filter := Field("category", Eq("A"))
	results := idx.Filter(filter)

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestFilterNil(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	results := idx.Filter(nil)
	if results != nil {
		t.Error("Expected nil for nil filter")
	}
}

func TestFilterAnd(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	idx.IndexPoint(1, map[string]interface{}{"category": "A", "price": 100})
	idx.IndexPoint(2, map[string]interface{}{"category": "A", "price": 200})
	idx.IndexPoint(3, map[string]interface{}{"category": "B", "price": 100})

	filter := And(
		Field("category", Eq("A")),
		Field("price", Eq(100)),
	)
	results := idx.Filter(filter)

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	if len(results) > 0 && results[0] != 1 {
		t.Errorf("Expected point ID 1, got %d", results[0])
	}
}

func TestFilterOr(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	idx.IndexPoint(1, map[string]interface{}{"status": "active"})
	idx.IndexPoint(2, map[string]interface{}{"status": "pending"})
	idx.IndexPoint(3, map[string]interface{}{"status": "inactive"})

	filter := Or(
		Field("status", Eq("active")),
		Field("status", Eq("pending")),
	)
	results := idx.Filter(filter)

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestFilterNot(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	idx.IndexPoint(1, map[string]interface{}{"status": "active"})
	idx.IndexPoint(2, map[string]interface{}{"status": "inactive"})
	idx.IndexPoint(3, map[string]interface{}{"status": "deleted"})

	filter := Not(Field("status", Eq("deleted")))
	results := idx.Filter(filter)

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestFilterRange(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	idx.IndexPoint(1, map[string]interface{}{"score": 10})
	idx.IndexPoint(2, map[string]interface{}{"score": 50})
	idx.IndexPoint(3, map[string]interface{}{"score": 90})

	filter := Field("score", Range(20, 80))
	results := idx.Filter(filter)

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
}

func TestFilterIn(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	idx.IndexPoint(1, map[string]interface{}{"type": "A"})
	idx.IndexPoint(2, map[string]interface{}{"type": "B"})
	idx.IndexPoint(3, map[string]interface{}{"type": "C"})

	filter := Field("type", In("A", "C"))
	results := idx.Filter(filter)

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestFilterNotIn(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	idx.IndexPoint(1, map[string]interface{}{"type": "A"})
	idx.IndexPoint(2, map[string]interface{}{"type": "B"})
	idx.IndexPoint(3, map[string]interface{}{"type": "C"})

	filter := Field("type", NotIn("A", "C"))
	results := idx.Filter(filter)

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
}

func TestFilterContains(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	idx.IndexPoint(1, map[string]interface{}{"title": "Hello World"})
	idx.IndexPoint(2, map[string]interface{}{"title": "Goodbye World"})
	idx.IndexPoint(3, map[string]interface{}{"title": "Hello There"})

	filter := Field("title", Contains("World"))
	results := idx.Filter(filter)

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestFilterStartsWith(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	idx.IndexPoint(1, map[string]interface{}{"name": "John Smith"})
	idx.IndexPoint(2, map[string]interface{}{"name": "Jane Doe"})
	idx.IndexPoint(3, map[string]interface{}{"name": "Johnny Cash"})

	filter := Field("name", StartsWith("John"))
	results := idx.Filter(filter)

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestFilterEndsWith(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	idx.IndexPoint(1, map[string]interface{}{"email": "user@example.com"})
	idx.IndexPoint(2, map[string]interface{}{"email": "admin@example.org"})
	idx.IndexPoint(3, map[string]interface{}{"email": "test@test.com"})

	filter := Field("email", EndsWith(".com"))
	results := idx.Filter(filter)

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestFilterIsNull(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	idx.IndexPoint(1, map[string]interface{}{"field": "value"})
	idx.IndexPoint(2, map[string]interface{}{"field": nil})
	idx.IndexPoint(3, map[string]interface{}{"other": "data"})

	filter := Field("field", IsNull())
	results := idx.Filter(filter)

	// Point 2 has null, point 3 doesn't have the field
	if len(results) < 1 {
		t.Error("Expected at least 1 result for null field")
	}
}

func TestFilterIsNotNull(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	idx.IndexPoint(1, map[string]interface{}{"field": "value"})
	idx.IndexPoint(2, map[string]interface{}{"field": nil})

	filter := Field("field", IsNotNull())
	results := idx.Filter(filter)

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
}

func TestCreateIndex(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	idx.CreateIndex("status", IndexTypeHash)

	fields := idx.IndexedFields()
	if len(fields) != 1 {
		t.Errorf("Expected 1 indexed field, got %d", len(fields))
	}
}

func TestDeleteIndex(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	idx.CreateIndex("status", IndexTypeHash)
	idx.DeleteIndex("status")

	fields := idx.IndexedFields()
	if len(fields) != 0 {
		t.Errorf("Expected 0 indexed fields, got %d", len(fields))
	}
}

func TestGetIndexStats(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	// Non-existent index
	stats := idx.GetIndexStats("nonexistent")
	if stats != nil {
		t.Error("Expected nil stats for non-existent index")
	}

	// Create and get stats
	idx.CreateIndex("status", IndexTypeHash)
	stats = idx.GetIndexStats("status")
	if stats == nil {
		t.Error("Expected non-nil stats for existing index")
	}
}

func TestIndexField(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	// Should not panic
	idx.IndexField(1, "status", "active")
}

func TestClose(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}

	err = idx.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}
}

func TestFilterGreaterLess(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	idx.IndexPoint(1, map[string]interface{}{"value": 10})
	idx.IndexPoint(2, map[string]interface{}{"value": 20})
	idx.IndexPoint(3, map[string]interface{}{"value": 30})

	// Greater
	filter := Field("value", Gt(15))
	results := idx.Filter(filter)
	if len(results) != 2 {
		t.Errorf("GT: Expected 2 results, got %d", len(results))
	}

	// Greater or equal
	filter = Field("value", Gte(20))
	results = idx.Filter(filter)
	if len(results) != 2 {
		t.Errorf("GTE: Expected 2 results, got %d", len(results))
	}

	// Less
	filter = Field("value", Lt(25))
	results = idx.Filter(filter)
	if len(results) != 2 {
		t.Errorf("LT: Expected 2 results, got %d", len(results))
	}

	// Less or equal
	filter = Field("value", Lte(20))
	results = idx.Filter(filter)
	if len(results) != 2 {
		t.Errorf("LTE: Expected 2 results, got %d", len(results))
	}
}

func TestFilterNotEqual(t *testing.T) {
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	idx.IndexPoint(1, map[string]interface{}{"status": "active"})
	idx.IndexPoint(2, map[string]interface{}{"status": "inactive"})
	idx.IndexPoint(3, map[string]interface{}{"status": "pending"})

	filter := Field("status", Ne("active"))
	results := idx.Filter(filter)

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}
