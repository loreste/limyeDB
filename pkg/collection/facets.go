package collection

import (
	"sort"
	"time"

	"github.com/limyedb/limyedb/pkg/index/payload"
	"github.com/limyedb/limyedb/pkg/point"
)

// FacetParams holds parameters for faceted search
type FacetParams struct {
	Field      string          // Field to facet on
	Limit      int             // Maximum number of facet values to return
	Filter     *payload.Filter // Optional pre-filter
	MinCount   int             // Minimum count to include in results
	OrderBy    string          // "count" (default) or "value"
	Descending bool            // Sort order (default: descending for count)
}

// FacetResult holds the result of a facet query
type FacetResult struct {
	Field  string       `json:"field"`
	Values []FacetValue `json:"values"`
	TookMs int64        `json:"took_ms"`
}

// FacetValue represents a single facet value with its count
type FacetValue struct {
	Value interface{} `json:"value"`
	Count int64       `json:"count"`
}

// MultiFacetParams holds parameters for multiple facet queries
type MultiFacetParams struct {
	Facets []*FacetParams  // Facets to compute
	Filter *payload.Filter // Global pre-filter
}

// MultiFacetResult holds the results of multiple facet queries
type MultiFacetResult struct {
	Facets map[string]*FacetResult `json:"facets"`
	TookMs int64                   `json:"took_ms"`
}

// Facet computes facets for a single field
func (c *Collection) Facet(params *FacetParams) (*FacetResult, error) {
	start := time.Now()

	c.mu.RLock()
	defer c.mu.RUnlock()

	if params.Limit <= 0 {
		params.Limit = 10
	}

	// Get all points
	var allPoints []*point.Point
	if c.config.HasNamedVectors() {
		for _, idx := range c.indices {
			allPoints = idx.GetAllPoints()
			break
		}
	} else {
		allPoints = c.index.GetAllPoints()
	}

	// Count occurrences of each value
	valueCounts := make(map[interface{}]int64)
	evaluator := payload.NewEvaluator()

	for _, p := range allPoints {
		// Apply filter if provided
		if params.Filter != nil && !evaluator.Evaluate(params.Filter, p.Payload) {
			continue
		}

		// Get field value
		if p.Payload == nil {
			continue
		}
		value, exists := p.Payload[params.Field]
		if !exists {
			continue
		}

		// Handle arrays - count each element separately
		if arr, ok := value.([]interface{}); ok {
			for _, v := range arr {
				valueCounts[v]++
			}
		} else {
			valueCounts[value]++
		}
	}

	// Convert to slice and sort
	values := make([]FacetValue, 0, len(valueCounts))
	for val, count := range valueCounts {
		if params.MinCount > 0 && count < int64(params.MinCount) {
			continue
		}
		values = append(values, FacetValue{Value: val, Count: count})
	}

	// Sort
	if params.OrderBy == "value" {
		sort.Slice(values, func(i, j int) bool {
			vi := toString(values[i].Value)
			vj := toString(values[j].Value)
			if params.Descending {
				return vi > vj
			}
			return vi < vj
		})
	} else {
		// Default: sort by count
		sort.Slice(values, func(i, j int) bool {
			if params.Descending || params.OrderBy == "" {
				return values[i].Count > values[j].Count
			}
			return values[i].Count < values[j].Count
		})
	}

	// Limit results
	if len(values) > params.Limit {
		values = values[:params.Limit]
	}

	return &FacetResult{
		Field:  params.Field,
		Values: values,
		TookMs: time.Since(start).Milliseconds(),
	}, nil
}

// MultiFacet computes multiple facets in one operation
func (c *Collection) MultiFacet(params *MultiFacetParams) (*MultiFacetResult, error) {
	start := time.Now()

	result := &MultiFacetResult{
		Facets: make(map[string]*FacetResult),
	}

	for _, fp := range params.Facets {
		// Apply global filter if not overridden
		if fp.Filter == nil && params.Filter != nil {
			fp.Filter = params.Filter
		}

		facetResult, err := c.Facet(fp)
		if err != nil {
			return nil, err
		}
		result.Facets[fp.Field] = facetResult
	}

	result.TookMs = time.Since(start).Milliseconds()
	return result, nil
}

// RangeFacetParams holds parameters for range/histogram facets
type RangeFacetParams struct {
	Field    string          // Numeric field to create ranges for
	Ranges   []Range         // Custom ranges
	Interval float64         // Auto-generate ranges with this interval
	Filter   *payload.Filter // Optional pre-filter
}

// Range represents a facet range
type Range struct {
	From *float64 `json:"from,omitempty"`
	To   *float64 `json:"to,omitempty"`
	Key  string   `json:"key,omitempty"`
}

// RangeFacetResult holds the result of a range facet query
type RangeFacetResult struct {
	Field  string            `json:"field"`
	Ranges []RangeFacetValue `json:"ranges"`
	TookMs int64             `json:"took_ms"`
}

// RangeFacetValue represents a range with its count
type RangeFacetValue struct {
	Key   string  `json:"key"`
	From  float64 `json:"from"`
	To    float64 `json:"to"`
	Count int64   `json:"count"`
}

// RangeFacet computes range/histogram facets for numeric fields
func (c *Collection) RangeFacet(params *RangeFacetParams) (*RangeFacetResult, error) {
	start := time.Now()

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Get all points
	var allPoints []*point.Point
	if c.config.HasNamedVectors() {
		for _, idx := range c.indices {
			allPoints = idx.GetAllPoints()
			break
		}
	} else {
		allPoints = c.index.GetAllPoints()
	}

	// If interval is specified, auto-generate ranges
	ranges := params.Ranges
	if params.Interval > 0 && len(ranges) == 0 {
		// Find min and max values
		var minVal, maxVal float64
		first := true
		evaluator := payload.NewEvaluator()

		for _, p := range allPoints {
			if params.Filter != nil && !evaluator.Evaluate(params.Filter, p.Payload) {
				continue
			}
			if p.Payload == nil {
				continue
			}
			val, ok := toFloat64(p.Payload[params.Field])
			if !ok {
				continue
			}
			if first {
				minVal, maxVal = val, val
				first = true
			}
			if val < minVal {
				minVal = val
			}
			if val > maxVal {
				maxVal = val
			}
		}

		// Generate ranges
		for v := minVal; v < maxVal; v += params.Interval {
			from := v
			to := v + params.Interval
			ranges = append(ranges, Range{From: &from, To: &to})
		}
	}

	// Count values in each range
	rangeCounts := make([]int64, len(ranges))
	evaluator := payload.NewEvaluator()

	for _, p := range allPoints {
		if params.Filter != nil && !evaluator.Evaluate(params.Filter, p.Payload) {
			continue
		}
		if p.Payload == nil {
			continue
		}
		val, ok := toFloat64(p.Payload[params.Field])
		if !ok {
			continue
		}

		// Check which range this value belongs to
		for i, r := range ranges {
			inRange := true
			if r.From != nil && val < *r.From {
				inRange = false
			}
			if r.To != nil && val >= *r.To {
				inRange = false
			}
			if inRange {
				rangeCounts[i]++
			}
		}
	}

	// Build result
	result := &RangeFacetResult{
		Field:  params.Field,
		Ranges: make([]RangeFacetValue, len(ranges)),
		TookMs: time.Since(start).Milliseconds(),
	}

	for i, r := range ranges {
		key := r.Key
		if key == "" {
			if r.From != nil && r.To != nil {
				key = toString(*r.From) + "-" + toString(*r.To)
			} else if r.From != nil {
				key = ">=" + toString(*r.From)
			} else if r.To != nil {
				key = "<" + toString(*r.To)
			}
		}

		rv := RangeFacetValue{
			Key:   key,
			Count: rangeCounts[i],
		}
		if r.From != nil {
			rv.From = *r.From
		}
		if r.To != nil {
			rv.To = *r.To
		}
		result.Ranges[i] = rv
	}

	return result, nil
}

// Helper functions

func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return floatToString(val)
	case int:
		return intToString(val)
	case int64:
		return int64ToString(val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case int32:
		return float64(val), true
	default:
		return 0, false
	}
}

func floatToString(f float64) string {
	return stringFromFloat(f)
}

func intToString(i int) string {
	return stringFromInt(i)
}

func int64ToString(i int64) string {
	return stringFromInt64(i)
}

// Simple string conversion without fmt dependency
func stringFromFloat(f float64) string {
	if f == 0 {
		return "0"
	}
	// Simple integer check
	if f == float64(int64(f)) {
		return stringFromInt64(int64(f))
	}
	// For simplicity, use limited precision
	intPart := int64(f)
	fracPart := int64((f - float64(intPart)) * 100)
	if fracPart < 0 {
		fracPart = -fracPart
	}
	if fracPart == 0 {
		return stringFromInt64(intPart)
	}
	return stringFromInt64(intPart) + "." + stringFromInt64(fracPart)
}

func stringFromInt(i int) string {
	return stringFromInt64(int64(i))
}

func stringFromInt64(i int64) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
