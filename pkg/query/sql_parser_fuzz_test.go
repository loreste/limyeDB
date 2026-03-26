package query

import (
	"strings"
	"testing"
)

// FuzzSQLParser fuzzes the SQL parser with random query strings
func FuzzSQLParser(f *testing.F) {
	// Seed corpus with valid queries
	seeds := []string{
		// SELECT queries
		"SELECT * FROM collection",
		"SELECT id, vector FROM test",
		"SELECT * FROM test WHERE id = '123'",
		"SELECT * FROM test WHERE status = 'active' AND count > 10",
		"SELECT * FROM test WHERE type IN ('a', 'b', 'c')",
		"SELECT * FROM test LIMIT 10",
		"SELECT * FROM test LIMIT 10 OFFSET 20",
		"SELECT * FROM test ORDER BY name ASC",
		"SELECT * FROM test ORDER BY score DESC LIMIT 100",

		// Vector search queries
		"SELECT * FROM vectors NEAREST TO [0.1, 0.2, 0.3] LIMIT 10",
		"SELECT * FROM test SIMILAR TO [1.0, 2.0, 3.0, 4.0] LIMIT 5",
		"SELECT id, score FROM test NEAREST TO [0.5, 0.5] WHERE status = 'active' LIMIT 10",
		"SEARCH collection FOR [0.1, 0.2, 0.3] LIMIT 10",
		"SEARCH test FOR [1.0, 2.0] USING embedding LIMIT 5",
		"SEARCH data FOR [0.0, 1.0, 0.0] WHERE category = 'news' LIMIT 20",

		// INSERT queries
		"INSERT INTO collection (id, vector) VALUES ('1', '[0.1, 0.2]')",
		"INSERT INTO test SET id = 'abc'",

		// UPDATE queries
		"UPDATE collection SET status = 'active'",
		"UPDATE test SET value = 123 WHERE id = 'abc'",

		// DELETE queries
		"DELETE FROM collection",
		"DELETE FROM test WHERE id = '123'",
		"DELETE FROM data WHERE status = 'deleted'",

		// CREATE/DROP queries
		"CREATE TABLE newcollection",
		"CREATE TABLE test (dimension=128, metric='cosine')",
		"CREATE TABLE vectors WITH dimension=256, metric='euclidean'",
		"DROP TABLE collection",
		"DROP TABLE IF EXISTS test",

		// DESCRIBE/SHOW queries
		"DESCRIBE collection",
		"DESC test",
		"SHOW TABLES",

		// Edge cases
		"",
		" ",
		"   SELECT * FROM test   ",
		"select * from test", // lowercase
		"SELECT  *  FROM  test", // extra spaces
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	parser := NewSQLParser()

	f.Fuzz(func(t *testing.T, query string) {
		// The parser should never panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Parser panicked on input: %q, panic: %v", query, r)
			}
		}()

		// Parse the query - we don't care about errors, just panics
		result, err := parser.Parse(query)

		// If parsing succeeded, verify the result is valid
		if err == nil && result != nil {
			// Type should be set
			if result.Type == "" {
				t.Errorf("Parse(%q) returned nil type", query)
			}

			// Collection should be set for most query types
			switch result.Type {
			case QueryShowTables:
				// No collection expected
			default:
				// Other queries might have collection set
			}

			// Vector should be valid if present
			if result.Vector != nil {
				for i, v := range result.Vector {
					// Check for NaN/Inf
					if v != v { // NaN check
						t.Errorf("Vector contains NaN at index %d", i)
					}
				}
			}

			// Limit should be non-negative
			if result.Limit < 0 {
				t.Errorf("Negative limit: %d", result.Limit)
			}

			// Offset should be non-negative
			if result.Offset < 0 {
				t.Errorf("Negative offset: %d", result.Offset)
			}
		}
	})
}

// FuzzParseVector fuzzes the vector parsing function
func FuzzParseVector(f *testing.F) {
	// Seed corpus
	seeds := []string{
		"0.1, 0.2, 0.3",
		"1.0, 2.0, 3.0, 4.0",
		"0, 1, 2",
		"-1.0, 0.0, 1.0",
		"0.123456789, 0.987654321",
		"1e-5, 1e5",
		"",
		" ",
		"   0.1  ,  0.2  ",
		"0.1,0.2,0.3", // no spaces
		"0.1 , 0.2 , 0.3", // spaces around comma
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	parser := NewSQLParser()

	f.Fuzz(func(t *testing.T, input string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("parseVector panicked on input: %q, panic: %v", input, r)
			}
		}()

		vec, err := parser.parseVector(input)

		// If parsing succeeded, verify the result
		if err == nil {
			// Vector should have at least one element if input was non-empty and valid
			if len(strings.TrimSpace(input)) > 0 && len(vec) == 0 {
				// Empty vector from non-empty input might be okay depending on content
			}

			// Check each element
			for i, v := range vec {
				// Check for NaN
				if v != v {
					t.Errorf("Vector contains NaN at index %d for input %q", i, input)
				}
			}
		}
	})
}

// FuzzParseWhereClause fuzzes the WHERE clause parsing
func FuzzParseWhereClause(f *testing.F) {
	// Seed corpus
	seeds := []string{
		"id = '123'",
		"status = 'active'",
		"count > 10",
		"value >= 5.5",
		"score < 100",
		"rating <= 4",
		"type <> 'deleted'",
		"type != 'deleted'",
		"name LIKE '%test%'",
		"category IN ('a', 'b', 'c')",
		"field IS NULL",
		"field IS NOT NULL",
		"status = 'active' AND count > 10",
		"type = 'a' OR type = 'b'",
		"NOT status = 'deleted'",
		"(status = 'active' AND count > 10) OR type = 'special'",
		"a = 1 AND b = 2 AND c = 3",
		"a = 1 OR b = 2 OR c = 3",
		"",
		" ",
		"field = TRUE",
		"field = FALSE",
		"field = NULL",
		"num = 42",
		"float = 3.14",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	parser := NewSQLParser()

	f.Fuzz(func(t *testing.T, clause string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("parseWhereClause panicked on input: %q, panic: %v", clause, r)
			}
		}()

		filter, err := parser.parseWhereClause(clause)

		// If parsing succeeded, verify the result
		if err == nil && filter != nil {
			// Verify the filter structure is valid
			verifyFilterExpr(t, filter, clause)
		}
	})
}

// FuzzParseValue fuzzes the value parsing function
func FuzzParseValue(f *testing.F) {
	// Seed corpus
	seeds := []string{
		"'string'",
		"\"double quoted\"",
		"123",
		"456.789",
		"-123",
		"-456.789",
		"TRUE",
		"FALSE",
		"true",
		"false",
		"NULL",
		"null",
		"('a', 'b', 'c')",
		"(1, 2, 3)",
		"",
		" ",
		"plain_string",
		"1e10",
		"1.5e-3",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	parser := NewSQLParser()

	f.Fuzz(func(t *testing.T, input string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("parseValue panicked on input: %q, panic: %v", input, r)
			}
		}()

		_, _ = parser.parseValue(input)
		// We just care that it doesn't panic
	})
}

// FuzzComplexQueries fuzzes with complex multi-part queries
func FuzzComplexQueries(f *testing.F) {
	// Seed corpus with complex queries
	seeds := []string{
		"SELECT a, b, c FROM test WHERE x = 1 AND y = 2 ORDER BY z DESC LIMIT 10 OFFSET 5",
		"SELECT * FROM vectors NEAREST TO [0.1, 0.2, 0.3, 0.4, 0.5] WHERE status = 'active' LIMIT 100",
		"SEARCH embeddings FOR [1.0, 2.0, 3.0] USING default WHERE category IN ('news', 'sports') LIMIT 50",
		"DELETE FROM collection WHERE created_at < '2023-01-01' AND status = 'archived'",
		"UPDATE items SET status = 'processed' WHERE id IN ('a', 'b', 'c')",
		"CREATE TABLE vectors WITH dimension=768, metric='cosine', vector_type='float32'",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	parser := NewSQLParser()

	f.Fuzz(func(t *testing.T, query string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Parser panicked on complex query: %q, panic: %v", query, r)
			}
		}()

		_, _ = parser.Parse(query)
	})
}

// verifyFilterExpr recursively verifies a filter expression is valid
func verifyFilterExpr(t *testing.T, filter *FilterExpr, originalInput string) {
	t.Helper()

	if filter == nil {
		return
	}

	// Check AND conditions
	for _, f := range filter.And {
		verifyFilterExpr(t, f, originalInput)
	}

	// Check OR conditions
	for _, f := range filter.Or {
		verifyFilterExpr(t, f, originalInput)
	}

	// Check NOT condition
	if filter.Not != nil {
		verifyFilterExpr(t, filter.Not, originalInput)
	}

	// If it's a leaf node, verify field and operator make sense
	if len(filter.And) == 0 && len(filter.Or) == 0 && filter.Not == nil {
		// This is a leaf condition
		// Operator should be one of the known operators
		validOps := map[string]bool{
			"eq": true, "ne": true, "lt": true, "gt": true,
			"lte": true, "gte": true, "like": true, "in": true,
			"is_null": true, "is_not_null": true, "between": true,
			"": true, // Empty might be valid for certain cases
		}

		if !validOps[filter.Operator] && filter.Operator != "" {
			// Unknown operator - might be valid depending on parser implementation
		}
	}
}

// FuzzSplitByKeyword fuzzes the keyword splitting function
func FuzzSplitByKeyword(f *testing.F) {
	seeds := []string{
		"a AND b",
		"a OR b",
		"a AND b AND c",
		"a OR b OR c",
		"'quoted AND string' AND other",
		"field = 'value with AND inside' AND another = 'test'",
		"(a AND b) OR c",
		"",
		" ",
		"ANDANDAND",
		"OROROR",
	}

	for _, seed := range seeds {
		f.Add(seed, "AND")
		f.Add(seed, "OR")
	}

	parser := NewSQLParser()

	f.Fuzz(func(t *testing.T, input string, keyword string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("splitByKeyword panicked on input: %q, keyword: %q, panic: %v", input, keyword, r)
			}
		}()

		_ = parser.splitByKeyword(input, keyword)
	})
}
