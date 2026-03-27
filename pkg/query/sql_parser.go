package query

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/limyedb/limyedb/pkg/point"
)

// ErrNullValue is returned when a SQL NULL value is encountered.
var ErrNullValue = errors.New("sql: null value")

// SQLQuery represents a parsed SQL-like query
type SQLQuery struct {
	Type       QueryType
	Collection string
	Columns    []string // SELECT fields
	Vector     point.Vector
	VectorName string
	Limit      int
	Offset     int
	Filter     *FilterExpr
	OrderBy    string
	OrderDesc  bool

	// For INSERT/UPDATE
	Points []*point.Point

	// For CREATE/DROP
	CollectionConfig *CollectionConfigSQL
}

// QueryType represents the type of SQL query
type QueryType string

const (
	QuerySelect       QueryType = "SELECT"
	QueryInsert       QueryType = "INSERT"
	QueryUpdate       QueryType = "UPDATE"
	QueryDelete       QueryType = "DELETE"
	QueryCreateTable  QueryType = "CREATE_TABLE"
	QueryDropTable    QueryType = "DROP_TABLE"
	QueryDescribe     QueryType = "DESCRIBE"
	QueryShowTables   QueryType = "SHOW_TABLES"
	QueryVectorSearch QueryType = "VECTOR_SEARCH"
)

// FilterExpr represents a filter expression
type FilterExpr struct {
	Field    string
	Operator string
	Value    interface{}
	And      []*FilterExpr
	Or       []*FilterExpr
	Not      *FilterExpr
}

// CollectionConfigSQL holds collection config from SQL
type CollectionConfigSQL struct {
	Name       string
	Dimension  int
	Metric     string
	VectorType string
}

// SQLParser parses SQL-like queries for vector database operations
type SQLParser struct {
	// Keywords are case-insensitive
}

// NewSQLParser creates a new SQL parser
func NewSQLParser() *SQLParser {
	return &SQLParser{}
}

// Parse parses a SQL-like query string
func (p *SQLParser) Parse(query string) (*SQLQuery, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("empty query")
	}

	// Normalize whitespace
	query = regexp.MustCompile(`\s+`).ReplaceAllString(query, " ")

	// Determine query type
	upper := strings.ToUpper(query)

	switch {
	case strings.HasPrefix(upper, "SELECT"):
		return p.parseSelect(query)
	case strings.HasPrefix(upper, "INSERT"):
		return p.parseInsert(query)
	case strings.HasPrefix(upper, "UPDATE"):
		return p.parseUpdate(query)
	case strings.HasPrefix(upper, "DELETE"):
		return p.parseDelete(query)
	case strings.HasPrefix(upper, "CREATE TABLE"):
		return p.parseCreateTable(query)
	case strings.HasPrefix(upper, "DROP TABLE"):
		return p.parseDropTable(query)
	case strings.HasPrefix(upper, "DESCRIBE") || strings.HasPrefix(upper, "DESC "):
		return p.parseDescribe(query)
	case strings.HasPrefix(upper, "SHOW TABLES"):
		return p.parseShowTables(query)
	case strings.HasPrefix(upper, "SEARCH"):
		return p.parseVectorSearch(query)
	default:
		return nil, fmt.Errorf("unsupported query type: %s", strings.Split(query, " ")[0])
	}
}

func (p *SQLParser) parseSelect(query string) (*SQLQuery, error) {
	result := &SQLQuery{Type: QuerySelect}

	// Check for vector search syntax
	if strings.Contains(strings.ToUpper(query), "NEAREST TO") ||
		strings.Contains(strings.ToUpper(query), "SIMILAR TO") {
		return p.parseVectorSelectSearch(query)
	}

	// Pattern: SELECT columns FROM collection [WHERE ...] [LIMIT ...] [OFFSET ...]
	selectRe := regexp.MustCompile(`(?i)SELECT\s+(.+?)\s+FROM\s+(\w+)(?:\s+WHERE\s+(.+?))?(?:\s+ORDER\s+BY\s+(\w+)(?:\s+(ASC|DESC))?)?(?:\s+LIMIT\s+(\d+))?(?:\s+OFFSET\s+(\d+))?$`)

	matches := selectRe.FindStringSubmatch(query)
	if matches == nil {
		return nil, errors.New("invalid SELECT syntax")
	}

	// Parse columns
	columnsStr := strings.TrimSpace(matches[1])
	if columnsStr == "*" {
		result.Columns = []string{"*"}
	} else {
		columns := strings.Split(columnsStr, ",")
		for _, col := range columns {
			result.Columns = append(result.Columns, strings.TrimSpace(col))
		}
	}

	result.Collection = strings.TrimSpace(matches[2])

	// Parse WHERE clause
	if matches[3] != "" {
		filter, err := p.parseWhereClause(matches[3])
		if err != nil {
			return nil, err
		}
		result.Filter = filter
	}

	// Parse ORDER BY
	if matches[4] != "" {
		result.OrderBy = matches[4]
		result.OrderDesc = strings.ToUpper(matches[5]) == "DESC"
	}

	// Parse LIMIT
	if matches[6] != "" {
		limit, err := strconv.Atoi(matches[6])
		if err != nil {
			return nil, errors.New("invalid LIMIT value")
		}
		result.Limit = limit
	}

	// Parse OFFSET
	if matches[7] != "" {
		offset, err := strconv.Atoi(matches[7])
		if err != nil {
			return nil, errors.New("invalid OFFSET value")
		}
		result.Offset = offset
	}

	return result, nil
}

func (p *SQLParser) parseVectorSelectSearch(query string) (*SQLQuery, error) {
	result := &SQLQuery{Type: QueryVectorSearch}

	// Pattern: SELECT columns FROM collection NEAREST TO [vector] [WHERE ...] LIMIT n
	// Also supports: SELECT ... SIMILAR TO [vector] ...
	nearestRe := regexp.MustCompile(`(?i)SELECT\s+(.+?)\s+FROM\s+(\w+)\s+(?:NEAREST|SIMILAR)\s+TO\s+\[([^\]]+)\](?:\s+USING\s+(\w+))?(?:\s+WHERE\s+(.+?))?(?:\s+LIMIT\s+(\d+))?`)

	matches := nearestRe.FindStringSubmatch(query)
	if matches == nil {
		return nil, errors.New("invalid vector search syntax. Use: SELECT * FROM collection NEAREST TO [0.1, 0.2, ...] LIMIT 10")
	}

	// Parse columns
	columnsStr := strings.TrimSpace(matches[1])
	if columnsStr == "*" {
		result.Columns = []string{"*"}
	} else {
		columns := strings.Split(columnsStr, ",")
		for _, col := range columns {
			result.Columns = append(result.Columns, strings.TrimSpace(col))
		}
	}

	result.Collection = strings.TrimSpace(matches[2])

	// Parse vector
	vectorStr := strings.TrimSpace(matches[3])
	vec, err := p.parseVector(vectorStr)
	if err != nil {
		return nil, fmt.Errorf("invalid vector: %v", err)
	}
	result.Vector = vec

	// Parse USING (named vector)
	if matches[4] != "" {
		result.VectorName = matches[4]
	}

	// Parse WHERE
	if matches[5] != "" {
		filter, err := p.parseWhereClause(matches[5])
		if err != nil {
			return nil, err
		}
		result.Filter = filter
	}

	// Parse LIMIT
	if matches[6] != "" {
		limit, err := strconv.Atoi(matches[6])
		if err != nil {
			return nil, errors.New("invalid LIMIT value")
		}
		result.Limit = limit
	}

	if result.Limit == 0 {
		result.Limit = 10 // Default limit
	}

	return result, nil
}

func (p *SQLParser) parseVectorSearch(query string) (*SQLQuery, error) {
	result := &SQLQuery{Type: QueryVectorSearch}

	// Pattern: SEARCH collection FOR [vector] [USING vector_name] [WHERE ...] LIMIT n
	searchRe := regexp.MustCompile(`(?i)SEARCH\s+(\w+)\s+FOR\s+\[([^\]]+)\](?:\s+USING\s+(\w+))?(?:\s+WHERE\s+(.+?))?(?:\s+LIMIT\s+(\d+))?`)

	matches := searchRe.FindStringSubmatch(query)
	if matches == nil {
		return nil, errors.New("invalid SEARCH syntax. Use: SEARCH collection FOR [0.1, 0.2, ...] LIMIT 10")
	}

	result.Collection = strings.TrimSpace(matches[1])
	result.Columns = []string{"*"}

	// Parse vector
	vec, err := p.parseVector(matches[2])
	if err != nil {
		return nil, fmt.Errorf("invalid vector: %v", err)
	}
	result.Vector = vec

	// Parse USING
	if matches[3] != "" {
		result.VectorName = matches[3]
	}

	// Parse WHERE
	if matches[4] != "" {
		filter, err := p.parseWhereClause(matches[4])
		if err != nil {
			return nil, err
		}
		result.Filter = filter
	}

	// Parse LIMIT
	if matches[5] != "" {
		limit, err := strconv.Atoi(matches[5])
		if err != nil {
			return nil, errors.New("invalid LIMIT value")
		}
		result.Limit = limit
	}

	if result.Limit == 0 {
		result.Limit = 10
	}

	return result, nil
}

func (p *SQLParser) parseVector(s string) (point.Vector, error) {
	parts := strings.Split(s, ",")
	vec := make(point.Vector, len(parts))

	for i, part := range parts {
		part = strings.TrimSpace(part)
		val, err := strconv.ParseFloat(part, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid vector element at position %d: %s", i, part)
		}
		vec[i] = float32(val)
	}

	return vec, nil
}

func (p *SQLParser) parseWhereClause(clause string) (*FilterExpr, error) {
	clause = strings.TrimSpace(clause)

	// Handle OR first (lower precedence)
	orParts := p.splitByKeyword(clause, "OR")
	if len(orParts) > 1 {
		var orFilters []*FilterExpr
		for _, part := range orParts {
			f, err := p.parseWhereClause(part)
			if err != nil {
				return nil, err
			}
			orFilters = append(orFilters, f)
		}
		return &FilterExpr{Or: orFilters}, nil
	}

	// Handle AND
	andParts := p.splitByKeyword(clause, "AND")
	if len(andParts) > 1 {
		var andFilters []*FilterExpr
		for _, part := range andParts {
			f, err := p.parseWhereClause(part)
			if err != nil {
				return nil, err
			}
			andFilters = append(andFilters, f)
		}
		return &FilterExpr{And: andFilters}, nil
	}

	// Handle NOT
	if strings.HasPrefix(strings.ToUpper(clause), "NOT ") {
		inner, err := p.parseWhereClause(clause[4:])
		if err != nil {
			return nil, err
		}
		return &FilterExpr{Not: inner}, nil
	}

	// Parse single condition
	return p.parseSingleCondition(clause)
}

func (p *SQLParser) splitByKeyword(s, keyword string) []string {
	// Simple split that respects quoted strings and parentheses
	var parts []string
	var current strings.Builder
	depth := 0
	inQuote := false
	quoteChar := rune(0)

	upper := strings.ToUpper(s)
	keywordLen := len(keyword)

	for i := 0; i < len(s); i++ {
		c := rune(s[i])

		if !inQuote && (c == '"' || c == '\'') {
			inQuote = true
			quoteChar = c
			current.WriteRune(c)
			continue
		}

		if inQuote && c == quoteChar {
			inQuote = false
			current.WriteRune(c)
			continue
		}

		if !inQuote {
			if c == '(' {
				depth++
			} else if c == ')' {
				depth--
			}
		}

		// Check for keyword at word boundary
		if depth == 0 && !inQuote && i+keywordLen <= len(upper) {
			prefix := upper[i : i+keywordLen]
			if prefix == keyword {
				// Check word boundaries
				beforeOk := i == 0 || !isWordChar(rune(s[i-1]))
				afterOk := i+keywordLen >= len(s) || !isWordChar(rune(s[i+keywordLen]))

				if beforeOk && afterOk {
					part := strings.TrimSpace(current.String())
					if part != "" {
						parts = append(parts, part)
					}
					current.Reset()
					i += keywordLen - 1
					continue
				}
			}
		}

		current.WriteRune(c)
	}

	part := strings.TrimSpace(current.String())
	if part != "" {
		parts = append(parts, part)
	}

	return parts
}

func isWordChar(c rune) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_'
}

func (p *SQLParser) parseSingleCondition(cond string) (*FilterExpr, error) {
	cond = strings.TrimSpace(cond)

	// Remove surrounding parentheses
	for strings.HasPrefix(cond, "(") && strings.HasSuffix(cond, ")") {
		cond = strings.TrimSpace(cond[1 : len(cond)-1])
	}

	// Try different operators
	operators := []struct {
		sql string
		op  string
	}{
		{"<>", "ne"},
		{"!=", "ne"},
		{"<=", "lte"},
		{">=", "gte"},
		{"<", "lt"},
		{">", "gt"},
		{"=", "eq"},
		{" LIKE ", "like"},
		{" IN ", "in"},
		{" IS NOT NULL", "is_not_null"},
		{" IS NULL", "is_null"},
		{" BETWEEN ", "between"},
	}

	for _, op := range operators {
		var idx int
		if strings.HasPrefix(op.sql, " ") {
			idx = strings.Index(strings.ToUpper(cond), op.sql)
		} else {
			idx = strings.Index(cond, op.sql)
		}

		if idx != -1 {
			field := strings.TrimSpace(cond[:idx])
			valueStr := strings.TrimSpace(cond[idx+len(op.sql):])

			// Handle special cases
			if op.op == "is_null" || op.op == "is_not_null" {
				return &FilterExpr{
					Field:    field,
					Operator: op.op,
				}, nil
			}

			value, err := p.parseValue(valueStr)
			if err != nil {
				return nil, err
			}

			return &FilterExpr{
				Field:    field,
				Operator: op.op,
				Value:    value,
			}, nil
		}
	}

	return nil, fmt.Errorf("cannot parse condition: %s", cond)
}

func (p *SQLParser) parseValue(s string) (interface{}, error) {
	s = strings.TrimSpace(s)

	// Quoted string (must be at least 2 chars for valid quotes)
	if len(s) >= 2 {
		if (strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'")) ||
			(strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")) {
			return s[1 : len(s)-1], nil
		}
	}

	// NULL
	if strings.ToUpper(s) == "NULL" {
		return nil, ErrNullValue
	}

	// Boolean
	if strings.ToUpper(s) == "TRUE" {
		return true, nil
	}
	if strings.ToUpper(s) == "FALSE" {
		return false, nil
	}

	// Array for IN clause
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		inner := s[1 : len(s)-1]
		parts := strings.Split(inner, ",")
		var values []interface{}
		for _, part := range parts {
			v, err := p.parseValue(strings.TrimSpace(part))
			if err != nil {
				return nil, err
			}
			values = append(values, v)
		}
		return values, nil
	}

	// Number
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i, nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f, nil
	}

	// Treat as string
	return s, nil
}

func (p *SQLParser) parseInsert(query string) (*SQLQuery, error) {
	result := &SQLQuery{Type: QueryInsert}

	// Pattern: INSERT INTO collection (id, vector, payload) VALUES (...)
	// Or: INSERT INTO collection SET id = '...', vector = [...], payload.field = ...
	insertRe := regexp.MustCompile(`(?i)INSERT\s+INTO\s+(\w+)`)

	matches := insertRe.FindStringSubmatch(query)
	if matches == nil {
		return nil, errors.New("invalid INSERT syntax")
	}

	result.Collection = strings.TrimSpace(matches[1])

	// For now, return the basic structure - actual parsing would be more complex
	return result, nil
}

func (p *SQLParser) parseUpdate(query string) (*SQLQuery, error) {
	result := &SQLQuery{Type: QueryUpdate}

	updateRe := regexp.MustCompile(`(?i)UPDATE\s+(\w+)`)

	matches := updateRe.FindStringSubmatch(query)
	if matches == nil {
		return nil, errors.New("invalid UPDATE syntax")
	}

	result.Collection = strings.TrimSpace(matches[1])

	return result, nil
}

func (p *SQLParser) parseDelete(query string) (*SQLQuery, error) {
	result := &SQLQuery{Type: QueryDelete}

	// Pattern: DELETE FROM collection WHERE id = '...'
	deleteRe := regexp.MustCompile(`(?i)DELETE\s+FROM\s+(\w+)(?:\s+WHERE\s+(.+))?`)

	matches := deleteRe.FindStringSubmatch(query)
	if matches == nil {
		return nil, errors.New("invalid DELETE syntax")
	}

	result.Collection = strings.TrimSpace(matches[1])

	if matches[2] != "" {
		filter, err := p.parseWhereClause(matches[2])
		if err != nil {
			return nil, err
		}
		result.Filter = filter
	}

	return result, nil
}

func (p *SQLParser) parseCreateTable(query string) (*SQLQuery, error) {
	result := &SQLQuery{Type: QueryCreateTable}

	// Pattern: CREATE TABLE collection (dimension INT, metric VARCHAR)
	// Or: CREATE TABLE collection WITH dimension=128, metric='cosine'
	createRe := regexp.MustCompile(`(?i)CREATE\s+TABLE\s+(\w+)(?:\s+\((.+)\)|\s+WITH\s+(.+))?`)

	matches := createRe.FindStringSubmatch(query)
	if matches == nil {
		return nil, errors.New("invalid CREATE TABLE syntax")
	}

	cfg := &CollectionConfigSQL{
		Name: strings.TrimSpace(matches[1]),
	}

	// Parse options
	options := matches[2]
	if options == "" {
		options = matches[3]
	}

	if options != "" {
		// Parse key=value pairs
		pairs := strings.Split(options, ",")
		for _, pair := range pairs {
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(strings.ToLower(parts[0]))
			value := strings.Trim(strings.TrimSpace(parts[1]), "'\"")

			switch key {
			case "dimension":
				dim, err := strconv.Atoi(value)
				if err != nil {
					return nil, errors.New("invalid dimension value")
				}
				cfg.Dimension = dim
			case "metric":
				cfg.Metric = value
			case "vector_type":
				cfg.VectorType = value
			}
		}
	}

	result.CollectionConfig = cfg
	result.Collection = cfg.Name

	return result, nil
}

func (p *SQLParser) parseDropTable(query string) (*SQLQuery, error) {
	result := &SQLQuery{Type: QueryDropTable}

	dropRe := regexp.MustCompile(`(?i)DROP\s+TABLE\s+(?:IF\s+EXISTS\s+)?(\w+)`)

	matches := dropRe.FindStringSubmatch(query)
	if matches == nil {
		return nil, errors.New("invalid DROP TABLE syntax")
	}

	result.Collection = strings.TrimSpace(matches[1])

	return result, nil
}

func (p *SQLParser) parseDescribe(query string) (*SQLQuery, error) {
	result := &SQLQuery{Type: QueryDescribe}

	descRe := regexp.MustCompile(`(?i)(?:DESCRIBE|DESC)\s+(\w+)`)

	matches := descRe.FindStringSubmatch(query)
	if matches == nil {
		return nil, errors.New("invalid DESCRIBE syntax")
	}

	result.Collection = strings.TrimSpace(matches[1])

	return result, nil
}

func (p *SQLParser) parseShowTables(query string) (*SQLQuery, error) {
	return &SQLQuery{Type: QueryShowTables}, nil
}

// ToPayloadFilter converts FilterExpr to the payload filter format used by the collection
func (fe *FilterExpr) ToPayloadFilter() map[string]interface{} {
	if fe == nil {
		return nil
	}

	result := make(map[string]interface{})

	if len(fe.And) > 0 {
		var conditions []map[string]interface{}
		for _, f := range fe.And {
			conditions = append(conditions, f.ToPayloadFilter())
		}
		result["must"] = conditions
		return result
	}

	if len(fe.Or) > 0 {
		var conditions []map[string]interface{}
		for _, f := range fe.Or {
			conditions = append(conditions, f.ToPayloadFilter())
		}
		result["should"] = conditions
		return result
	}

	if fe.Not != nil {
		result["must_not"] = []map[string]interface{}{fe.Not.ToPayloadFilter()}
		return result
	}

	// Single condition
	condition := map[string]interface{}{
		"key": fe.Field,
	}

	switch fe.Operator {
	case "eq":
		condition["match"] = map[string]interface{}{"value": fe.Value}
	case "ne":
		result["must_not"] = []map[string]interface{}{
			{
				"key":   fe.Field,
				"match": map[string]interface{}{"value": fe.Value},
			},
		}
		return result
	case "lt", "gt", "lte", "gte":
		condition["range"] = map[string]interface{}{fe.Operator: fe.Value}
	case "in":
		condition["match"] = map[string]interface{}{"any": fe.Value}
	case "like":
		condition["match"] = map[string]interface{}{"text": fe.Value}
	case "is_null":
		condition["is_null"] = true
	case "is_not_null":
		condition["is_null"] = false
	}

	result["must"] = []map[string]interface{}{condition}
	return result
}
