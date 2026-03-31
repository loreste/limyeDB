package payload

import (
	"encoding/json"
	"testing"
)

func TestNewEvaluator(t *testing.T) {
	t.Parallel()

	e := NewEvaluator()
	if e == nil {
		t.Fatal("NewEvaluator returned nil")
	}
}

func TestEvaluateNilFilter(t *testing.T) {
	t.Parallel()

	e := NewEvaluator()
	payload := map[string]interface{}{"key": "value"}

	// Nil filter should match everything
	if !e.Evaluate(nil, payload) {
		t.Error("Nil filter should match")
	}
}

func TestEvaluateEqual(t *testing.T) {
	t.Parallel()

	e := NewEvaluator()
	payload := map[string]interface{}{
		"name":  "John",
		"age":   30,
		"score": 95.5,
	}

	testCases := []struct {
		name     string
		filter   *Filter
		expected bool
	}{
		{
			name:     "string equal match",
			filter:   Field("name", Eq("John")),
			expected: true,
		},
		{
			name:     "string equal no match",
			filter:   Field("name", Eq("Jane")),
			expected: false,
		},
		{
			name:     "number equal match",
			filter:   Field("age", Eq(30)),
			expected: true,
		},
		{
			name:     "float equal match",
			filter:   Field("score", Eq(95.5)),
			expected: true,
		},
		{
			name:     "case insensitive match",
			filter:   Field("name", Eq("john")),
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := e.Evaluate(tc.filter, payload)
			if result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestEvaluateNotEqual(t *testing.T) {
	t.Parallel()

	e := NewEvaluator()
	payload := map[string]interface{}{"name": "John"}

	filter := Field("name", Ne("Jane"))
	if !e.Evaluate(filter, payload) {
		t.Error("Expected true for not equal")
	}

	filter = Field("name", Ne("John"))
	if e.Evaluate(filter, payload) {
		t.Error("Expected false for equal value")
	}
}

func TestEvaluateComparisons(t *testing.T) {
	t.Parallel()

	e := NewEvaluator()
	payload := map[string]interface{}{"age": 25}

	testCases := []struct {
		name     string
		filter   *Filter
		expected bool
	}{
		{"greater than (true)", Field("age", Gt(20)), true},
		{"greater than (false)", Field("age", Gt(30)), false},
		{"greater or equal (true)", Field("age", Gte(25)), true},
		{"greater or equal (false)", Field("age", Gte(30)), false},
		{"less than (true)", Field("age", Lt(30)), true},
		{"less than (false)", Field("age", Lt(20)), false},
		{"less or equal (true)", Field("age", Lte(25)), true},
		{"less or equal (false)", Field("age", Lte(20)), false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := e.Evaluate(tc.filter, payload)
			if result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestEvaluateIn(t *testing.T) {
	t.Parallel()

	e := NewEvaluator()
	payload := map[string]interface{}{"status": "active"}

	filter := Field("status", In("active", "pending", "completed"))
	if !e.Evaluate(filter, payload) {
		t.Error("Expected true for value in list")
	}

	filter = Field("status", In("inactive", "deleted"))
	if e.Evaluate(filter, payload) {
		t.Error("Expected false for value not in list")
	}
}

func TestEvaluateNotIn(t *testing.T) {
	t.Parallel()

	e := NewEvaluator()
	payload := map[string]interface{}{"status": "active"}

	filter := Field("status", NotIn("inactive", "deleted"))
	if !e.Evaluate(filter, payload) {
		t.Error("Expected true for value not in list")
	}

	filter = Field("status", NotIn("active", "pending"))
	if e.Evaluate(filter, payload) {
		t.Error("Expected false for value in list")
	}
}

func TestEvaluateStringOperations(t *testing.T) {
	t.Parallel()

	e := NewEvaluator()
	payload := map[string]interface{}{"text": "Hello World"}

	testCases := []struct {
		name     string
		filter   *Filter
		expected bool
	}{
		{"contains (true)", Field("text", Contains("World")), true},
		{"contains (false)", Field("text", Contains("xyz")), false},
		{"contains (case insensitive)", Field("text", Contains("WORLD")), true},
		{"starts with (true)", Field("text", StartsWith("Hello")), true},
		{"starts with (false)", Field("text", StartsWith("World")), false},
		{"ends with (true)", Field("text", EndsWith("World")), true},
		{"ends with (false)", Field("text", EndsWith("Hello")), false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := e.Evaluate(tc.filter, payload)
			if result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestEvaluateIsNull(t *testing.T) {
	t.Parallel()

	e := NewEvaluator()
	payload := map[string]interface{}{
		"present": "value",
		"null":    nil,
	}

	// Test IsNull
	filter := Field("missing", IsNull())
	if !e.Evaluate(filter, payload) {
		t.Error("Expected true for missing field")
	}

	filter = Field("null", IsNull())
	if !e.Evaluate(filter, payload) {
		t.Error("Expected true for null field")
	}

	filter = Field("present", IsNull())
	if e.Evaluate(filter, payload) {
		t.Error("Expected false for present field")
	}

	// Test IsNotNull
	filter = Field("present", IsNotNull())
	if !e.Evaluate(filter, payload) {
		t.Error("Expected true for present field")
	}

	filter = Field("missing", IsNotNull())
	if e.Evaluate(filter, payload) {
		t.Error("Expected false for missing field")
	}
}

func TestEvaluateRange(t *testing.T) {
	t.Parallel()

	e := NewEvaluator()
	payload := map[string]interface{}{"score": 75}

	filter := Field("score", Range(50, 100))
	if !e.Evaluate(filter, payload) {
		t.Error("Expected true for value in range")
	}

	filter = Field("score", Range(80, 100))
	if e.Evaluate(filter, payload) {
		t.Error("Expected false for value below range")
	}
}

func TestEvaluateAnd(t *testing.T) {
	t.Parallel()

	e := NewEvaluator()
	payload := map[string]interface{}{
		"age":    25,
		"status": "active",
	}

	// Both conditions match
	filter := And(
		Field("age", Gte(18)),
		Field("status", Eq("active")),
	)
	if !e.Evaluate(filter, payload) {
		t.Error("Expected true for both conditions matching")
	}

	// One condition fails
	filter = And(
		Field("age", Gte(30)),
		Field("status", Eq("active")),
	)
	if e.Evaluate(filter, payload) {
		t.Error("Expected false when one condition fails")
	}
}

func TestEvaluateOr(t *testing.T) {
	t.Parallel()

	e := NewEvaluator()
	payload := map[string]interface{}{
		"status": "active",
	}

	// One condition matches
	filter := Or(
		Field("status", Eq("active")),
		Field("status", Eq("pending")),
	)
	if !e.Evaluate(filter, payload) {
		t.Error("Expected true for one condition matching")
	}

	// No conditions match
	filter = Or(
		Field("status", Eq("inactive")),
		Field("status", Eq("deleted")),
	)
	if e.Evaluate(filter, payload) {
		t.Error("Expected false when no conditions match")
	}
}

func TestEvaluateNot(t *testing.T) {
	t.Parallel()

	e := NewEvaluator()
	payload := map[string]interface{}{"status": "active"}

	filter := Not(Field("status", Eq("inactive")))
	if !e.Evaluate(filter, payload) {
		t.Error("Expected true for NOT matching")
	}

	filter = Not(Field("status", Eq("active")))
	if e.Evaluate(filter, payload) {
		t.Error("Expected false for NOT non-matching")
	}
}

func TestEvaluateNestedField(t *testing.T) {
	t.Parallel()

	e := NewEvaluator()
	payload := map[string]interface{}{
		"user": map[string]interface{}{
			"profile": map[string]interface{}{
				"age": 25,
			},
		},
	}

	filter := Field("user.profile.age", Eq(25))
	if !e.Evaluate(filter, payload) {
		t.Error("Expected true for nested field match")
	}

	filter = Field("user.profile.age", Eq(30))
	if e.Evaluate(filter, payload) {
		t.Error("Expected false for nested field non-match")
	}
}

func TestFilterBuilder(t *testing.T) {
	t.Parallel()

	e := NewEvaluator()
	payload := map[string]interface{}{
		"age":    25,
		"status": "active",
	}

	fb := NewFilterBuilder()
	fb.Where("age", Gte(18))
	fb.Where("status", Eq("active"))

	filter, err := fb.Build()
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	if !e.Evaluate(filter, payload) {
		t.Error("Expected filter to match")
	}
}

func TestFilterBuilderOrWhere(t *testing.T) {
	t.Parallel()

	e := NewEvaluator()
	payload := map[string]interface{}{"status": "pending"}

	fb := NewFilterBuilder()
	fb.Where("status", Eq("active"))
	fb.OrWhere("status", Eq("pending"))

	filter, err := fb.Build()
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	if !e.Evaluate(filter, payload) {
		t.Error("Expected filter to match with OR")
	}
}

func TestParseFilter(t *testing.T) {
	t.Parallel()

	jsonFilter := `{
		"type": "field",
		"field": "age",
		"condition": {
			"op": "gte",
			"value": 18
		}
	}`

	filter, err := ParseFilter([]byte(jsonFilter))
	if err != nil {
		t.Fatalf("ParseFilter() failed: %v", err)
	}

	if filter.Type != FilterTypeField {
		t.Errorf("Expected type=field, got %s", filter.Type)
	}
	if filter.Field != "age" {
		t.Errorf("Expected field=age, got %s", filter.Field)
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		filter    *Filter
		expectErr bool
	}{
		{"nil filter", nil, false},
		{"valid field filter", Field("age", Eq(25)), false},
		{"empty AND filter", And(), true},
		{"valid AND filter", And(Field("age", Eq(25))), false},
		{"valid OR filter", Or(Field("a", Eq(1)), Field("b", Eq(2))), false},
		{"empty NOT filter", &Filter{Type: FilterTypeNot}, true},
		{"valid NOT filter", Not(Field("a", Eq(1))), false},
		{"missing field name", &Filter{Type: FilterTypeField, Condition: Eq(1)}, true},
		{"missing condition", &Filter{Type: FilterTypeField, Field: "a"}, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.filter)
			if tc.expectErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestQdrantFilterToFilter(t *testing.T) {
	t.Parallel()

	// Test Must conditions
	qf := &QdrantFilter{
		Must: []QdrantCondition{
			{Field: "status", Match: &Match{Value: "active"}},
		},
	}

	filter := qf.ToFilter()
	if filter == nil {
		t.Fatal("Expected non-nil filter")
	}

	e := NewEvaluator()
	payload := map[string]interface{}{"status": "active"}
	if !e.Evaluate(filter, payload) {
		t.Error("Expected filter to match")
	}
}

func TestQdrantFilterRange(t *testing.T) {
	t.Parallel()

	gt := 10.0
	lt := 20.0
	qf := &QdrantFilter{
		Must: []QdrantCondition{
			{Field: "age", Range: &QRange{Gt: &gt, Lt: &lt}},
		},
	}

	filter := qf.ToFilter()
	if filter == nil {
		t.Fatal("Expected non-nil filter")
	}

	e := NewEvaluator()

	payload := map[string]interface{}{"age": 15}
	if !e.Evaluate(filter, payload) {
		t.Error("Expected filter to match for age=15")
	}

	payload = map[string]interface{}{"age": 25}
	if e.Evaluate(filter, payload) {
		t.Error("Expected filter to not match for age=25")
	}
}

func TestQdrantFilterShouldConditions(t *testing.T) {
	t.Parallel()

	qf := &QdrantFilter{
		Should: []QdrantCondition{
			{Field: "status", Match: &Match{Value: "active"}},
			{Field: "status", Match: &Match{Value: "pending"}},
		},
	}

	filter := qf.ToFilter()
	if filter == nil {
		t.Fatal("Expected non-nil filter")
	}

	e := NewEvaluator()

	payload := map[string]interface{}{"status": "pending"}
	if !e.Evaluate(filter, payload) {
		t.Error("Expected filter to match for status=pending")
	}

	payload = map[string]interface{}{"status": "inactive"}
	if e.Evaluate(filter, payload) {
		t.Error("Expected filter to not match for status=inactive")
	}
}

func TestQdrantFilterMustNot(t *testing.T) {
	t.Parallel()

	qf := &QdrantFilter{
		MustNot: []QdrantCondition{
			{Field: "deleted", Match: &Match{Value: true}},
		},
	}

	filter := qf.ToFilter()
	if filter == nil {
		t.Fatal("Expected non-nil filter")
	}

	e := NewEvaluator()

	payload := map[string]interface{}{"deleted": false}
	if !e.Evaluate(filter, payload) {
		t.Error("Expected filter to match when deleted=false")
	}

	payload = map[string]interface{}{"deleted": true}
	if e.Evaluate(filter, payload) {
		t.Error("Expected filter to not match when deleted=true")
	}
}

func TestQdrantFilterMatchAny(t *testing.T) {
	t.Parallel()

	qf := &QdrantFilter{
		Must: []QdrantCondition{
			{Field: "status", Match: &Match{Any: []interface{}{"active", "pending"}}},
		},
	}

	filter := qf.ToFilter()
	if filter == nil {
		t.Fatal("Expected non-nil filter")
	}

	e := NewEvaluator()

	payload := map[string]interface{}{"status": "pending"}
	if !e.Evaluate(filter, payload) {
		t.Error("Expected filter to match")
	}
}

func TestQdrantFilterIsNull(t *testing.T) {
	t.Parallel()

	isNull := true
	qf := &QdrantFilter{
		Must: []QdrantCondition{
			{Field: "deleted_at", IsNull: &isNull},
		},
	}

	filter := qf.ToFilter()
	if filter == nil {
		t.Fatal("Expected non-nil filter")
	}

	e := NewEvaluator()

	payload := map[string]interface{}{"deleted_at": nil}
	if !e.Evaluate(filter, payload) {
		t.Error("Expected filter to match for null field")
	}
}

func TestFilterJSON(t *testing.T) {
	t.Parallel()

	original := And(
		Field("age", Gte(18)),
		Or(
			Field("status", Eq("active")),
			Field("status", Eq("pending")),
		),
	)

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Unmarshal
	var restored Filter
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify
	if restored.Type != original.Type {
		t.Errorf("Type mismatch: %s vs %s", restored.Type, original.Type)
	}
}
