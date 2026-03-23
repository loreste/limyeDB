package payload

import (
	"encoding/json"
	"errors"
	"strings"
)

// FilterType represents the type of filter operation
type FilterType string

const (
	FilterTypeAnd      FilterType = "and"
	FilterTypeOr       FilterType = "or"
	FilterTypeNot      FilterType = "not"
	FilterTypeField    FilterType = "field"
	FilterMatch        FilterType = "match"
	FilterRange        FilterType = "range"
	FilterIsNull       FilterType = "is_null"
	FilterIsNotNull    FilterType = "is_not_null"
	FilterAnd          FilterType = "and"    // Alias
	FilterOr           FilterType = "or"     // Alias
)

// Operator represents a comparison operator
type Operator string

const (
	OpEqual        Operator = "eq"
	OpNotEqual     Operator = "ne"
	OpGreater      Operator = "gt"
	OpGreaterEqual Operator = "gte"
	OpLess         Operator = "lt"
	OpLessEqual    Operator = "lte"
	OpIn           Operator = "in"
	OpNotIn        Operator = "nin"
	OpContains     Operator = "contains"
	OpStartsWith   Operator = "starts_with"
	OpEndsWith     Operator = "ends_with"
	OpIsNull       Operator = "is_null"
	OpIsNotNull    Operator = "is_not_null"
	OpRange        Operator = "range"
)

// Filter represents a filter expression
type Filter struct {
	Type       FilterType  `json:"type,omitempty"`
	Field      string      `json:"field,omitempty"`
	Condition  *Condition  `json:"condition,omitempty"`
	Filters    []*Filter   `json:"filters,omitempty"`
	Conditions []*Filter   `json:"conditions,omitempty"` // Alias for Filters (Qdrant compatibility)
}

// Condition represents a field condition
type Condition struct {
	Op     Operator      `json:"op"`
	Value  interface{}   `json:"value,omitempty"`
	Values []interface{} `json:"values,omitempty"`
	Min    interface{}   `json:"min,omitempty"`
	Max    interface{}   `json:"max,omitempty"`
}

// And creates an AND filter
func And(filters ...*Filter) *Filter {
	return &Filter{
		Type:    FilterTypeAnd,
		Filters: filters,
	}
}

// Or creates an OR filter
func Or(filters ...*Filter) *Filter {
	return &Filter{
		Type:    FilterTypeOr,
		Filters: filters,
	}
}

// Not creates a NOT filter
func Not(filter *Filter) *Filter {
	return &Filter{
		Type:    FilterTypeNot,
		Filters: []*Filter{filter},
	}
}

// Field creates a field filter
func Field(name string, cond *Condition) *Filter {
	return &Filter{
		Type:      FilterTypeField,
		Field:     name,
		Condition: cond,
	}
}

// Eq creates an equality condition
func Eq(value interface{}) *Condition {
	return &Condition{Op: OpEqual, Value: value}
}

// Ne creates a not-equal condition
func Ne(value interface{}) *Condition {
	return &Condition{Op: OpNotEqual, Value: value}
}

// Gt creates a greater-than condition
func Gt(value interface{}) *Condition {
	return &Condition{Op: OpGreater, Value: value}
}

// Gte creates a greater-than-or-equal condition
func Gte(value interface{}) *Condition {
	return &Condition{Op: OpGreaterEqual, Value: value}
}

// Lt creates a less-than condition
func Lt(value interface{}) *Condition {
	return &Condition{Op: OpLess, Value: value}
}

// Lte creates a less-than-or-equal condition
func Lte(value interface{}) *Condition {
	return &Condition{Op: OpLessEqual, Value: value}
}

// In creates an in-list condition
func In(values ...interface{}) *Condition {
	return &Condition{Op: OpIn, Values: values}
}

// NotIn creates a not-in-list condition
func NotIn(values ...interface{}) *Condition {
	return &Condition{Op: OpNotIn, Values: values}
}

// Contains creates a contains condition
func Contains(value string) *Condition {
	return &Condition{Op: OpContains, Value: value}
}

// StartsWith creates a starts-with condition
func StartsWith(value string) *Condition {
	return &Condition{Op: OpStartsWith, Value: value}
}

// EndsWith creates an ends-with condition
func EndsWith(value string) *Condition {
	return &Condition{Op: OpEndsWith, Value: value}
}

// IsNull creates an is-null condition
func IsNull() *Condition {
	return &Condition{Op: OpIsNull}
}

// IsNotNull creates an is-not-null condition
func IsNotNull() *Condition {
	return &Condition{Op: OpIsNotNull}
}

// Range creates a range condition
func Range(min, max interface{}) *Condition {
	return &Condition{Op: OpRange, Min: min, Max: max}
}

// Evaluator evaluates filters against payloads
type Evaluator struct{}

// NewEvaluator creates a new filter evaluator
func NewEvaluator() *Evaluator {
	return &Evaluator{}
}

// Evaluate checks if a payload matches the filter
func (e *Evaluator) Evaluate(filter *Filter, payload map[string]interface{}) bool {
	if filter == nil {
		return true
	}

	switch filter.Type {
	case FilterTypeAnd:
		return e.evaluateAnd(filter.Filters, payload)
	case FilterTypeOr:
		return e.evaluateOr(filter.Filters, payload)
	case FilterTypeNot:
		if len(filter.Filters) > 0 {
			return !e.Evaluate(filter.Filters[0], payload)
		}
		return true
	case FilterTypeField:
		return e.evaluateCondition(filter.Field, filter.Condition, payload)
	default:
		return true
	}
}

func (e *Evaluator) evaluateAnd(filters []*Filter, payload map[string]interface{}) bool {
	for _, f := range filters {
		if !e.Evaluate(f, payload) {
			return false
		}
	}
	return true
}

func (e *Evaluator) evaluateOr(filters []*Filter, payload map[string]interface{}) bool {
	if len(filters) == 0 {
		return true
	}
	for _, f := range filters {
		if e.Evaluate(f, payload) {
			return true
		}
	}
	return false
}

func (e *Evaluator) evaluateCondition(field string, cond *Condition, payload map[string]interface{}) bool {
	if cond == nil {
		return true
	}

	// Handle nested field access
	value := e.getFieldValue(field, payload)

	switch cond.Op {
	case OpEqual:
		return e.compareEqual(value, cond.Value)
	case OpNotEqual:
		return !e.compareEqual(value, cond.Value)
	case OpGreater:
		return e.compareNumeric(value, cond.Value) > 0
	case OpGreaterEqual:
		return e.compareNumeric(value, cond.Value) >= 0
	case OpLess:
		return e.compareNumeric(value, cond.Value) < 0
	case OpLessEqual:
		return e.compareNumeric(value, cond.Value) <= 0
	case OpIn:
		return e.valueIn(value, cond.Values)
	case OpNotIn:
		return !e.valueIn(value, cond.Values)
	case OpContains:
		return e.stringContains(value, cond.Value)
	case OpStartsWith:
		return e.stringStartsWith(value, cond.Value)
	case OpEndsWith:
		return e.stringEndsWith(value, cond.Value)
	case OpIsNull:
		return value == nil
	case OpIsNotNull:
		return value != nil
	case OpRange:
		return e.inRange(value, cond.Min, cond.Max)
	default:
		return true
	}
}

func (e *Evaluator) getFieldValue(field string, payload map[string]interface{}) interface{} {
	parts := strings.Split(field, ".")
	var current interface{} = payload

	for _, part := range parts {
		if current == nil {
			return nil
		}
		if m, ok := current.(map[string]interface{}); ok {
			current = m[part]
		} else {
			return nil
		}
	}

	return current
}

func (e *Evaluator) compareEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Handle numeric comparisons
	aNum, aOk := toFloat64(a)
	bNum, bOk := toFloat64(b)
	if aOk && bOk {
		return aNum == bNum
	}

	// Handle string comparisons (case-insensitive)
	aStr, aOk := a.(string)
	bStr, bOk := b.(string)
	if aOk && bOk {
		return strings.EqualFold(aStr, bStr)
	}

	return a == b
}

func (e *Evaluator) compareNumeric(a, b interface{}) int {
	aNum, aOk := toFloat64(a)
	bNum, bOk := toFloat64(b)
	if !aOk || !bOk {
		return 0
	}

	if aNum < bNum {
		return -1
	}
	if aNum > bNum {
		return 1
	}
	return 0
}

func (e *Evaluator) valueIn(value interface{}, values []interface{}) bool {
	for _, v := range values {
		if e.compareEqual(value, v) {
			return true
		}
	}
	return false
}

func (e *Evaluator) stringContains(value, substr interface{}) bool {
	vStr, vOk := value.(string)
	sStr, sOk := substr.(string)
	if !vOk || !sOk {
		return false
	}
	return strings.Contains(strings.ToLower(vStr), strings.ToLower(sStr))
}

func (e *Evaluator) stringStartsWith(value, prefix interface{}) bool {
	vStr, vOk := value.(string)
	pStr, pOk := prefix.(string)
	if !vOk || !pOk {
		return false
	}
	return strings.HasPrefix(strings.ToLower(vStr), strings.ToLower(pStr))
}

func (e *Evaluator) stringEndsWith(value, suffix interface{}) bool {
	vStr, vOk := value.(string)
	sStr, sOk := suffix.(string)
	if !vOk || !sOk {
		return false
	}
	return strings.HasSuffix(strings.ToLower(vStr), strings.ToLower(sStr))
}

func (e *Evaluator) inRange(value, min, max interface{}) bool {
	vNum, vOk := toFloat64(value)
	minNum, minOk := toFloat64(min)
	maxNum, maxOk := toFloat64(max)

	if !vOk || !minOk || !maxOk {
		return false
	}

	return vNum >= minNum && vNum <= maxNum
}

// ParseFilter parses a JSON filter expression
func ParseFilter(data []byte) (*Filter, error) {
	var filter Filter
	if err := json.Unmarshal(data, &filter); err != nil {
		return nil, err
	}
	return &filter, nil
}

// FilterBuilder provides a fluent interface for building filters
type FilterBuilder struct {
	filter *Filter
	err    error
}

// NewFilterBuilder creates a new filter builder
func NewFilterBuilder() *FilterBuilder {
	return &FilterBuilder{
		filter: &Filter{Type: FilterTypeAnd, Filters: []*Filter{}},
	}
}

// Where adds a field condition
func (fb *FilterBuilder) Where(field string, cond *Condition) *FilterBuilder {
	if fb.err != nil {
		return fb
	}
	fb.filter.Filters = append(fb.filter.Filters, Field(field, cond))
	return fb
}

// And adds an AND condition
func (fb *FilterBuilder) And(other *FilterBuilder) *FilterBuilder {
	if fb.err != nil {
		return fb
	}
	if other.err != nil {
		fb.err = other.err
		return fb
	}
	fb.filter.Filters = append(fb.filter.Filters, other.filter)
	return fb
}

// OrWhere adds an OR condition
func (fb *FilterBuilder) OrWhere(field string, cond *Condition) *FilterBuilder {
	if fb.err != nil {
		return fb
	}
	// Wrap current filter in OR with new condition
	currentFilter := fb.filter
	fb.filter = &Filter{
		Type: FilterTypeOr,
		Filters: []*Filter{
			currentFilter,
			Field(field, cond),
		},
	}
	return fb
}

// Build returns the constructed filter
func (fb *FilterBuilder) Build() (*Filter, error) {
	if fb.err != nil {
		return nil, fb.err
	}
	return fb.filter, nil
}

// QdrantFilter provides a Qdrant-compatible filter structure
type QdrantFilter struct {
	Must    []QdrantCondition `json:"must,omitempty"`
	Should  []QdrantCondition `json:"should,omitempty"`
	MustNot []QdrantCondition `json:"must_not,omitempty"`
}

// QdrantCondition represents a Qdrant-compatible condition
type QdrantCondition struct {
	Field  string  `json:"key,omitempty"`
	Match  *Match  `json:"match,omitempty"`
	Range  *QRange `json:"range,omitempty"`
	IsNull *bool   `json:"is_null,omitempty"`
}

// Match represents a match condition
type Match struct {
	Value interface{}   `json:"value,omitempty"`
	Any   []interface{} `json:"any,omitempty"`
	Text  string        `json:"text,omitempty"`
}

// QRange represents a range condition
type QRange struct {
	Lt  *float64 `json:"lt,omitempty"`
	Lte *float64 `json:"lte,omitempty"`
	Gt  *float64 `json:"gt,omitempty"`
	Gte *float64 `json:"gte,omitempty"`
}

// ToFilter converts a QdrantFilter to the internal Filter format
func (qf *QdrantFilter) ToFilter() *Filter {
	if qf == nil {
		return nil
	}

	var filters []*Filter

	// Process Must conditions
	for _, cond := range qf.Must {
		filters = append(filters, conditionToFilter(&cond))
	}

	// Wrap in AND if multiple must conditions
	result := &Filter{Type: FilterTypeAnd, Filters: filters}

	// Process Should conditions
	if len(qf.Should) > 0 {
		var shouldFilters []*Filter
		for _, cond := range qf.Should {
			shouldFilters = append(shouldFilters, conditionToFilter(&cond))
		}
		result.Filters = append(result.Filters, &Filter{
			Type:    FilterTypeOr,
			Filters: shouldFilters,
		})
	}

	// Process MustNot conditions
	for _, cond := range qf.MustNot {
		result.Filters = append(result.Filters, Not(conditionToFilter(&cond)))
	}

	return result
}

func conditionToFilter(cond *QdrantCondition) *Filter {
	if cond == nil {
		return nil
	}

	filter := &Filter{
		Type:  FilterTypeField,
		Field: cond.Field,
	}

	if cond.IsNull != nil {
		if *cond.IsNull {
			filter.Condition = IsNull()
		} else {
			filter.Condition = IsNotNull()
		}
		return filter
	}

	if cond.Match != nil {
		if cond.Match.Value != nil {
			filter.Condition = Eq(cond.Match.Value)
		} else if len(cond.Match.Any) > 0 {
			filter.Condition = In(cond.Match.Any...)
		} else if cond.Match.Text != "" {
			filter.Condition = Contains(cond.Match.Text)
		}
		return filter
	}

	if cond.Range != nil {
		if cond.Range.Lt != nil {
			filter.Condition = Lt(*cond.Range.Lt)
		} else if cond.Range.Lte != nil {
			filter.Condition = Lte(*cond.Range.Lte)
		} else if cond.Range.Gt != nil {
			filter.Condition = Gt(*cond.Range.Gt)
		} else if cond.Range.Gte != nil {
			filter.Condition = Gte(*cond.Range.Gte)
		}
		return filter
	}

	return filter
}

// Validate validates a filter
func Validate(filter *Filter) error {
	if filter == nil {
		return nil
	}

	switch filter.Type {
	case FilterTypeAnd, FilterTypeOr:
		if len(filter.Filters) == 0 {
			return errors.New("and/or filter requires at least one sub-filter")
		}
		for _, f := range filter.Filters {
			if err := Validate(f); err != nil {
				return err
			}
		}
	case FilterTypeNot:
		if len(filter.Filters) != 1 {
			return errors.New("not filter requires exactly one sub-filter")
		}
		return Validate(filter.Filters[0])
	case FilterTypeField:
		if filter.Field == "" {
			return errors.New("field filter requires a field name")
		}
		if filter.Condition == nil {
			return errors.New("field filter requires a condition")
		}
	default:
		return errors.New("unknown filter type")
	}

	return nil
}
