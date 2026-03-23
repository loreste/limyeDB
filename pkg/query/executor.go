package query

import (
	"context"
	"errors"
	"fmt"

	"github.com/limyedb/limyedb/pkg/collection"
	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/index/payload"
	"github.com/limyedb/limyedb/pkg/point"
)

// QueryResult represents the result of a SQL query
type QueryResult struct {
	Type       QueryType              `json:"type"`
	Collection string                 `json:"collection,omitempty"`
	Rows       []map[string]interface{} `json:"rows,omitempty"`
	Points     []*point.Point         `json:"points,omitempty"`
	Affected   int                    `json:"affected,omitempty"`
	Message    string                 `json:"message,omitempty"`
	Columns    []string               `json:"columns,omitempty"`
}

// SQLExecutor executes parsed SQL queries against collections
type SQLExecutor struct {
	manager *collection.Manager
}

// NewSQLExecutor creates a new SQL executor
func NewSQLExecutor(manager *collection.Manager) *SQLExecutor {
	return &SQLExecutor{
		manager: manager,
	}
}

// Execute executes a parsed SQL query
func (e *SQLExecutor) Execute(ctx context.Context, query *SQLQuery) (*QueryResult, error) {
	switch query.Type {
	case QuerySelect:
		return e.executeSelect(ctx, query)
	case QueryVectorSearch:
		return e.executeVectorSearch(ctx, query)
	case QueryInsert:
		return e.executeInsert(ctx, query)
	case QueryUpdate:
		return e.executeUpdate(ctx, query)
	case QueryDelete:
		return e.executeDelete(ctx, query)
	case QueryCreateTable:
		return e.executeCreateTable(ctx, query)
	case QueryDropTable:
		return e.executeDropTable(ctx, query)
	case QueryDescribe:
		return e.executeDescribe(ctx, query)
	case QueryShowTables:
		return e.executeShowTables(ctx, query)
	default:
		return nil, fmt.Errorf("unsupported query type: %s", query.Type)
	}
}

// ExecuteRaw parses and executes a SQL query string
func (e *SQLExecutor) ExecuteRaw(ctx context.Context, sql string) (*QueryResult, error) {
	parser := NewSQLParser()
	query, err := parser.Parse(sql)
	if err != nil {
		return nil, err
	}
	return e.Execute(ctx, query)
}

func (e *SQLExecutor) executeSelect(ctx context.Context, query *SQLQuery) (*QueryResult, error) {
	coll, err := e.manager.Get(query.Collection)
	if err != nil {
		return nil, fmt.Errorf("collection not found: %s", query.Collection)
	}

	// Build filter
	var filter *payload.Filter
	if query.Filter != nil {
		filter = e.buildFilter(query.Filter)
	}

	// Get limit
	limit := query.Limit
	if limit <= 0 {
		limit = 100 // Default limit
	}

	// Use scroll to get points
	scrollParams := &collection.ScrollParams{
		Limit:       limit,
		Offset:      "",
		Filter:      filter,
		WithPayload: true,
		WithVector:  e.hasColumn(query.Columns, "vector"),
	}

	result, err := coll.Scroll(scrollParams)
	if err != nil {
		return nil, err
	}

	// Convert to rows
	rows := make([]map[string]interface{}, len(result.Points))
	for i, p := range result.Points {
		row := make(map[string]interface{})
		row["id"] = p.ID

		// Add selected columns
		if e.hasColumn(query.Columns, "*") || e.hasColumn(query.Columns, "payload") {
			for k, v := range p.Payload {
				row[k] = v
			}
		} else {
			for _, col := range query.Columns {
				if col == "id" {
					continue
				}
				if col == "vector" {
					row["vector"] = p.Vector
				} else if val, ok := p.Payload[col]; ok {
					row[col] = val
				}
			}
		}

		rows[i] = row
	}

	return &QueryResult{
		Type:       QuerySelect,
		Collection: query.Collection,
		Rows:       rows,
		Columns:    query.Columns,
	}, nil
}

func (e *SQLExecutor) executeVectorSearch(ctx context.Context, query *SQLQuery) (*QueryResult, error) {
	coll, err := e.manager.Get(query.Collection)
	if err != nil {
		return nil, fmt.Errorf("collection not found: %s", query.Collection)
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 10
	}

	// Build filter
	var filter *payload.Filter
	if query.Filter != nil {
		filter = e.buildFilter(query.Filter)
	}

	// Perform search
	var searchResult *collection.SearchResult
	if query.VectorName != "" {
		// Named vector search
		namedResult, err := coll.SearchV2(query.Vector, query.VectorName, limit)
		if err != nil {
			return nil, err
		}
		// Convert to standard SearchResult
		searchResult = &collection.SearchResult{
			Points: make([]collection.ScoredPoint, len(namedResult.Points)),
			TookMs: namedResult.TookMs,
		}
		for i, p := range namedResult.Points {
			searchResult.Points[i] = collection.ScoredPoint{
				ID:      p.ID,
				Score:   p.Score,
				Vector:  p.Vector,
				Payload: p.Payload,
			}
		}
	} else {
		// Default vector search with filter
		searchResult, err = coll.SearchWithParams(query.Vector, &collection.SearchParams{
			K:           limit,
			Filter:      filter,
			WithPayload: true,
			WithVector:  e.hasColumn(query.Columns, "vector"),
		})
		if err != nil {
			return nil, err
		}
	}

	// Convert to rows
	rows := make([]map[string]interface{}, len(searchResult.Points))
	for i, r := range searchResult.Points {
		row := make(map[string]interface{})
		row["id"] = r.ID
		row["_score"] = r.Score

		// Add payload fields
		if e.hasColumn(query.Columns, "*") || e.hasColumn(query.Columns, "payload") {
			for k, v := range r.Payload {
				row[k] = v
			}
		} else {
			for _, col := range query.Columns {
				switch col {
				case "id", "_score":
					continue
				case "vector":
					row["vector"] = r.Vector
				default:
					if val, ok := r.Payload[col]; ok {
						row[col] = val
					}
				}
			}
		}

		rows[i] = row
	}

	return &QueryResult{
		Type:       QueryVectorSearch,
		Collection: query.Collection,
		Rows:       rows,
		Columns:    append([]string{"id", "_score"}, query.Columns...),
	}, nil
}

func (e *SQLExecutor) executeInsert(ctx context.Context, query *SQLQuery) (*QueryResult, error) {
	if len(query.Points) == 0 {
		return nil, errors.New("no points to insert")
	}

	coll, err := e.manager.Get(query.Collection)
	if err != nil {
		return nil, fmt.Errorf("collection not found: %s", query.Collection)
	}

	// Insert points
	for _, p := range query.Points {
		if err := coll.Insert(p); err != nil {
			return nil, err
		}
	}

	return &QueryResult{
		Type:       QueryInsert,
		Collection: query.Collection,
		Affected:   len(query.Points),
		Message:    fmt.Sprintf("Inserted %d point(s)", len(query.Points)),
	}, nil
}

func (e *SQLExecutor) executeUpdate(ctx context.Context, query *SQLQuery) (*QueryResult, error) {
	return nil, errors.New("UPDATE not fully implemented - use REST API")
}

func (e *SQLExecutor) executeDelete(ctx context.Context, query *SQLQuery) (*QueryResult, error) {
	coll, err := e.manager.Get(query.Collection)
	if err != nil {
		return nil, fmt.Errorf("collection not found: %s", query.Collection)
	}

	// For now, delete requires filter with id
	if query.Filter == nil {
		return nil, errors.New("DELETE requires WHERE clause with id")
	}

	if query.Filter.Field == "id" && query.Filter.Operator == "eq" {
		id, ok := query.Filter.Value.(string)
		if !ok {
			return nil, errors.New("id must be a string")
		}

		if err := coll.Delete(id); err != nil {
			return nil, err
		}

		return &QueryResult{
			Type:       QueryDelete,
			Collection: query.Collection,
			Affected:   1,
			Message:    "Deleted 1 point",
		}, nil
	}

	return nil, errors.New("DELETE currently only supports WHERE id = 'value'")
}

func (e *SQLExecutor) executeCreateTable(ctx context.Context, query *SQLQuery) (*QueryResult, error) {
	cfg := query.CollectionConfig
	if cfg == nil {
		return nil, errors.New("missing collection configuration")
	}

	if cfg.Dimension <= 0 {
		cfg.Dimension = 128 // Default
	}

	metric := config.MetricCosine
	switch cfg.Metric {
	case "euclidean", "l2":
		metric = config.MetricEuclidean
	case "dot", "dotproduct":
		metric = config.MetricDotProduct
	}

	collConfig := &config.CollectionConfig{
		Name:      cfg.Name,
		Dimension: cfg.Dimension,
		Metric:    metric,
	}

	if _, err := e.manager.Create(collConfig); err != nil {
		return nil, err
	}

	return &QueryResult{
		Type:       QueryCreateTable,
		Collection: cfg.Name,
		Message:    fmt.Sprintf("Collection '%s' created", cfg.Name),
	}, nil
}

func (e *SQLExecutor) executeDropTable(ctx context.Context, query *SQLQuery) (*QueryResult, error) {
	if err := e.manager.Delete(query.Collection); err != nil {
		return nil, err
	}

	return &QueryResult{
		Type:       QueryDropTable,
		Collection: query.Collection,
		Message:    fmt.Sprintf("Collection '%s' dropped", query.Collection),
	}, nil
}

func (e *SQLExecutor) executeDescribe(ctx context.Context, query *SQLQuery) (*QueryResult, error) {
	coll, err := e.manager.Get(query.Collection)
	if err != nil {
		return nil, fmt.Errorf("collection not found: %s", query.Collection)
	}

	info := coll.Info()

	rows := []map[string]interface{}{
		{"field": "name", "type": "string", "value": info.Name},
		{"field": "dimension", "type": "int", "value": info.Config.Dimension},
		{"field": "metric", "type": "string", "value": string(info.Config.Metric)},
		{"field": "size", "type": "int", "value": info.Size},
	}

	// Add named vectors info
	for name, vecInfo := range info.Vectors {
		rows = append(rows, map[string]interface{}{
			"field": fmt.Sprintf("vector.%s.dimension", name),
			"type":  "int",
			"value": vecInfo.Dimension,
		})
	}

	return &QueryResult{
		Type:       QueryDescribe,
		Collection: query.Collection,
		Rows:       rows,
		Columns:    []string{"field", "type", "value"},
	}, nil
}

func (e *SQLExecutor) executeShowTables(ctx context.Context, query *SQLQuery) (*QueryResult, error) {
	collections := e.manager.List()

	rows := make([]map[string]interface{}, len(collections))
	for i, name := range collections {
		coll, _ := e.manager.Get(name)
		info := coll.Info()
		rows[i] = map[string]interface{}{
			"name":      name,
			"size":      info.Size,
			"dimension": info.Config.Dimension,
			"metric":    string(info.Config.Metric),
		}
	}

	return &QueryResult{
		Type:    QueryShowTables,
		Rows:    rows,
		Columns: []string{"name", "points_count", "dimension", "metric"},
	}, nil
}

func (e *SQLExecutor) buildFilter(fe *FilterExpr) *payload.Filter {
	if fe == nil {
		return nil
	}

	// Handle AND
	if len(fe.And) > 0 {
		var filters []*payload.Filter
		for _, f := range fe.And {
			subFilter := e.buildFilter(f)
			if subFilter != nil {
				filters = append(filters, subFilter)
			}
		}
		return payload.And(filters...)
	}

	// Handle OR
	if len(fe.Or) > 0 {
		var filters []*payload.Filter
		for _, f := range fe.Or {
			subFilter := e.buildFilter(f)
			if subFilter != nil {
				filters = append(filters, subFilter)
			}
		}
		return payload.Or(filters...)
	}

	// Handle NOT
	if fe.Not != nil {
		subFilter := e.buildFilter(fe.Not)
		if subFilter != nil {
			return payload.Not(subFilter)
		}
		return nil
	}

	// Single condition
	var cond *payload.Condition

	switch fe.Operator {
	case "eq":
		cond = payload.Eq(fe.Value)
	case "ne":
		return payload.Not(payload.Field(fe.Field, payload.Eq(fe.Value)))
	case "lt":
		cond = payload.Lt(fe.Value)
	case "lte":
		cond = payload.Lte(fe.Value)
	case "gt":
		cond = payload.Gt(fe.Value)
	case "gte":
		cond = payload.Gte(fe.Value)
	case "in":
		if arr, ok := fe.Value.([]interface{}); ok {
			cond = payload.In(arr...)
		}
	case "like":
		if s, ok := fe.Value.(string); ok {
			cond = payload.Contains(s)
		}
	case "is_null":
		cond = payload.IsNull()
	case "is_not_null":
		cond = payload.IsNotNull()
	default:
		cond = payload.Eq(fe.Value)
	}

	return payload.Field(fe.Field, cond)
}

func (e *SQLExecutor) hasColumn(columns []string, col string) bool {
	for _, c := range columns {
		if c == col || c == "*" {
			return true
		}
	}
	return false
}

// Condition adds missing IsNull field to payload.Condition for SQL integration
// This is handled in the buildFilter function above
