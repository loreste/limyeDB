package payload

import (
	"sort"
	"strings"
	"sync"
)

// Index manages payload indexing for fast filtering
type Index struct {
	// Field-specific indexes
	numericIndexes  map[string]*NumericIndex
	keywordIndexes  map[string]*KeywordIndex
	booleanIndexes  map[string]*BooleanIndex

	mu sync.RWMutex
}

// NewIndex creates a new payload index
func NewIndex() *Index {
	return &Index{
		numericIndexes:  make(map[string]*NumericIndex),
		keywordIndexes:  make(map[string]*KeywordIndex),
		booleanIndexes:  make(map[string]*BooleanIndex),
	}
}

// IndexPoint adds a point's payload to the index
func (idx *Index) IndexPoint(pointID uint32, payload map[string]interface{}) {
	if payload == nil {
		return
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	for field, value := range payload {
		switch v := value.(type) {
		case float64:
			idx.getOrCreateNumeric(field).Add(pointID, v)
		case float32:
			idx.getOrCreateNumeric(field).Add(pointID, float64(v))
		case int:
			idx.getOrCreateNumeric(field).Add(pointID, float64(v))
		case int64:
			idx.getOrCreateNumeric(field).Add(pointID, float64(v))
		case string:
			idx.getOrCreateKeyword(field).Add(pointID, v)
		case bool:
			idx.getOrCreateBoolean(field).Add(pointID, v)
		case []interface{}:
			// Handle arrays of keywords
			for _, item := range v {
				if s, ok := item.(string); ok {
					idx.getOrCreateKeyword(field).Add(pointID, s)
				}
			}
		}
	}
}

// RemovePoint removes a point from all indexes
func (idx *Index) RemovePoint(pointID uint32, payload map[string]interface{}) {
	if payload == nil {
		return
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	for field, value := range payload {
		switch v := value.(type) {
		case float64:
			if numIdx, ok := idx.numericIndexes[field]; ok {
				numIdx.Remove(pointID, v)
			}
		case string:
			if kwIdx, ok := idx.keywordIndexes[field]; ok {
				kwIdx.Remove(pointID, v)
			}
		case bool:
			if boolIdx, ok := idx.booleanIndexes[field]; ok {
				boolIdx.Remove(pointID, v)
			}
		}
	}
}

func (idx *Index) getOrCreateNumeric(field string) *NumericIndex {
	if numIdx, ok := idx.numericIndexes[field]; ok {
		return numIdx
	}
	numIdx := NewNumericIndex()
	idx.numericIndexes[field] = numIdx
	return numIdx
}

func (idx *Index) getOrCreateKeyword(field string) *KeywordIndex {
	if kwIdx, ok := idx.keywordIndexes[field]; ok {
		return kwIdx
	}
	kwIdx := NewKeywordIndex()
	idx.keywordIndexes[field] = kwIdx
	return kwIdx
}

func (idx *Index) getOrCreateBoolean(field string) *BooleanIndex {
	if boolIdx, ok := idx.booleanIndexes[field]; ok {
		return boolIdx
	}
	boolIdx := NewBooleanIndex()
	idx.booleanIndexes[field] = boolIdx
	return boolIdx
}

// Filter returns point IDs matching the filter
func (idx *Index) Filter(filter *Filter) []uint32 {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.evaluateFilter(filter)
}

func (idx *Index) evaluateFilter(filter *Filter) []uint32 {
	if filter == nil {
		return nil
	}

	switch filter.Type {
	case FilterTypeAnd:
		return idx.evaluateAnd(filter.Filters)
	case FilterTypeOr:
		return idx.evaluateOr(filter.Filters)
	case FilterTypeNot:
		if len(filter.Filters) > 0 {
			return idx.evaluateNot(filter.Filters[0])
		}
	case FilterTypeField:
		return idx.evaluateFieldCondition(filter.Field, filter.Condition)
	}

	return nil
}

func (idx *Index) evaluateAnd(filters []*Filter) []uint32 {
	if len(filters) == 0 {
		return nil
	}

	result := idx.evaluateFilter(filters[0])
	if len(result) == 0 {
		return nil
	}

	for i := 1; i < len(filters); i++ {
		other := idx.evaluateFilter(filters[i])
		result = intersect(result, other)
		if len(result) == 0 {
			return nil
		}
	}

	return result
}

func (idx *Index) evaluateOr(filters []*Filter) []uint32 {
	if len(filters) == 0 {
		return nil
	}

	resultSet := make(map[uint32]struct{})
	for _, f := range filters {
		ids := idx.evaluateFilter(f)
		for _, id := range ids {
			resultSet[id] = struct{}{}
		}
	}

	result := make([]uint32, 0, len(resultSet))
	for id := range resultSet {
		result = append(result, id)
	}
	return result
}

func (idx *Index) evaluateNot(filter *Filter) []uint32 {
	// NOT requires knowledge of all point IDs
	// For now, return nil (handled at search time)
	return nil
}

func (idx *Index) evaluateFieldCondition(field string, cond *Condition) []uint32 {
	if cond == nil {
		return nil
	}

	switch cond.Op {
	case OpEqual:
		return idx.findEqual(field, cond.Value)
	case OpNotEqual:
		return nil // Handled at search time
	case OpGreater, OpGreaterEqual, OpLess, OpLessEqual:
		return idx.findRange(field, cond)
	case OpIn:
		return idx.findIn(field, cond.Values)
	case OpContains:
		return idx.findContains(field, cond.Value)
	case OpIsNull, OpIsNotNull:
		return nil // Handled at search time
	}

	return nil
}

func (idx *Index) findEqual(field string, value interface{}) []uint32 {
	switch v := value.(type) {
	case float64:
		if numIdx, ok := idx.numericIndexes[field]; ok {
			return numIdx.Range(v, v)
		}
	case string:
		if kwIdx, ok := idx.keywordIndexes[field]; ok {
			return kwIdx.Match(v)
		}
	case bool:
		if boolIdx, ok := idx.booleanIndexes[field]; ok {
			return boolIdx.Match(v)
		}
	}
	return nil
}

func (idx *Index) findRange(field string, cond *Condition) []uint32 {
	numIdx, ok := idx.numericIndexes[field]
	if !ok {
		return nil
	}

	val, ok := toFloat64(cond.Value)
	if !ok {
		return nil
	}

	switch cond.Op {
	case OpGreater:
		return numIdx.GreaterThan(val)
	case OpGreaterEqual:
		return numIdx.GreaterOrEqual(val)
	case OpLess:
		return numIdx.LessThan(val)
	case OpLessEqual:
		return numIdx.LessOrEqual(val)
	}

	return nil
}

func (idx *Index) findIn(field string, values []interface{}) []uint32 {
	resultSet := make(map[uint32]struct{})

	for _, value := range values {
		ids := idx.findEqual(field, value)
		for _, id := range ids {
			resultSet[id] = struct{}{}
		}
	}

	result := make([]uint32, 0, len(resultSet))
	for id := range resultSet {
		result = append(result, id)
	}
	return result
}

func (idx *Index) findContains(field string, value interface{}) []uint32 {
	str, ok := value.(string)
	if !ok {
		return nil
	}

	kwIdx, ok := idx.keywordIndexes[field]
	if !ok {
		return nil
	}

	return kwIdx.Contains(str)
}

// NumericIndex indexes numeric values using a sorted list
type NumericIndex struct {
	entries []numericEntry
	mu      sync.RWMutex
}

type numericEntry struct {
	value   float64
	pointID uint32
}

// NewNumericIndex creates a new numeric index
func NewNumericIndex() *NumericIndex {
	return &NumericIndex{
		entries: make([]numericEntry, 0),
	}
}

// Add adds a value to the index
func (n *NumericIndex) Add(pointID uint32, value float64) {
	n.mu.Lock()
	defer n.mu.Unlock()

	entry := numericEntry{value: value, pointID: pointID}

	// Binary search for insertion point
	i := sort.Search(len(n.entries), func(i int) bool {
		return n.entries[i].value >= value
	})

	n.entries = append(n.entries, numericEntry{})
	copy(n.entries[i+1:], n.entries[i:])
	n.entries[i] = entry
}

// Remove removes a value from the index
func (n *NumericIndex) Remove(pointID uint32, value float64) {
	n.mu.Lock()
	defer n.mu.Unlock()

	for i, e := range n.entries {
		if e.pointID == pointID && e.value == value {
			n.entries = append(n.entries[:i], n.entries[i+1:]...)
			return
		}
	}
}

// Range returns points with values in [min, max]
func (n *NumericIndex) Range(min, max float64) []uint32 {
	n.mu.RLock()
	defer n.mu.RUnlock()

	var result []uint32
	for _, e := range n.entries {
		if e.value >= min && e.value <= max {
			result = append(result, e.pointID)
		} else if e.value > max {
			break
		}
	}
	return result
}

// GreaterThan returns points with values > threshold
func (n *NumericIndex) GreaterThan(threshold float64) []uint32 {
	n.mu.RLock()
	defer n.mu.RUnlock()

	// Binary search for first entry > threshold
	i := sort.Search(len(n.entries), func(i int) bool {
		return n.entries[i].value > threshold
	})

	result := make([]uint32, 0, len(n.entries)-i)
	for ; i < len(n.entries); i++ {
		result = append(result, n.entries[i].pointID)
	}
	return result
}

// GreaterOrEqual returns points with values >= threshold
func (n *NumericIndex) GreaterOrEqual(threshold float64) []uint32 {
	n.mu.RLock()
	defer n.mu.RUnlock()

	i := sort.Search(len(n.entries), func(i int) bool {
		return n.entries[i].value >= threshold
	})

	result := make([]uint32, 0, len(n.entries)-i)
	for ; i < len(n.entries); i++ {
		result = append(result, n.entries[i].pointID)
	}
	return result
}

// LessThan returns points with values < threshold
func (n *NumericIndex) LessThan(threshold float64) []uint32 {
	n.mu.RLock()
	defer n.mu.RUnlock()

	var result []uint32
	for _, e := range n.entries {
		if e.value < threshold {
			result = append(result, e.pointID)
		} else {
			break
		}
	}
	return result
}

// LessOrEqual returns points with values <= threshold
func (n *NumericIndex) LessOrEqual(threshold float64) []uint32 {
	n.mu.RLock()
	defer n.mu.RUnlock()

	var result []uint32
	for _, e := range n.entries {
		if e.value <= threshold {
			result = append(result, e.pointID)
		} else {
			break
		}
	}
	return result
}

// KeywordIndex indexes string values using an inverted index
type KeywordIndex struct {
	postings map[string][]uint32
	mu       sync.RWMutex
}

// NewKeywordIndex creates a new keyword index
func NewKeywordIndex() *KeywordIndex {
	return &KeywordIndex{
		postings: make(map[string][]uint32),
	}
}

// Add adds a keyword to the index
func (k *KeywordIndex) Add(pointID uint32, value string) {
	k.mu.Lock()
	defer k.mu.Unlock()

	value = strings.ToLower(value)
	k.postings[value] = append(k.postings[value], pointID)
}

// Remove removes a keyword from the index
func (k *KeywordIndex) Remove(pointID uint32, value string) {
	k.mu.Lock()
	defer k.mu.Unlock()

	value = strings.ToLower(value)
	ids := k.postings[value]
	for i, id := range ids {
		if id == pointID {
			k.postings[value] = append(ids[:i], ids[i+1:]...)
			return
		}
	}
}

// Match returns points with exact keyword match
func (k *KeywordIndex) Match(value string) []uint32 {
	k.mu.RLock()
	defer k.mu.RUnlock()

	value = strings.ToLower(value)
	if ids, ok := k.postings[value]; ok {
		result := make([]uint32, len(ids))
		copy(result, ids)
		return result
	}
	return nil
}

// Contains returns points where the keyword contains the substring
func (k *KeywordIndex) Contains(substr string) []uint32 {
	k.mu.RLock()
	defer k.mu.RUnlock()

	substr = strings.ToLower(substr)
	resultSet := make(map[uint32]struct{})

	for keyword, ids := range k.postings {
		if strings.Contains(keyword, substr) {
			for _, id := range ids {
				resultSet[id] = struct{}{}
			}
		}
	}

	result := make([]uint32, 0, len(resultSet))
	for id := range resultSet {
		result = append(result, id)
	}
	return result
}

// BooleanIndex indexes boolean values
type BooleanIndex struct {
	trueIDs  []uint32
	falseIDs []uint32
	mu       sync.RWMutex
}

// NewBooleanIndex creates a new boolean index
func NewBooleanIndex() *BooleanIndex {
	return &BooleanIndex{}
}

// Add adds a boolean value to the index
func (b *BooleanIndex) Add(pointID uint32, value bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if value {
		b.trueIDs = append(b.trueIDs, pointID)
	} else {
		b.falseIDs = append(b.falseIDs, pointID)
	}
}

// Remove removes a boolean value from the index
func (b *BooleanIndex) Remove(pointID uint32, value bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	var ids *[]uint32
	if value {
		ids = &b.trueIDs
	} else {
		ids = &b.falseIDs
	}

	for i, id := range *ids {
		if id == pointID {
			*ids = append((*ids)[:i], (*ids)[i+1:]...)
			return
		}
	}
}

// Match returns points with the given boolean value
func (b *BooleanIndex) Match(value bool) []uint32 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var ids []uint32
	if value {
		ids = b.trueIDs
	} else {
		ids = b.falseIDs
	}

	result := make([]uint32, len(ids))
	copy(result, ids)
	return result
}

// Helper functions

func intersect(a, b []uint32) []uint32 {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}

	// Use map for O(n+m) intersection
	set := make(map[uint32]struct{}, len(a))
	for _, id := range a {
		set[id] = struct{}{}
	}

	result := make([]uint32, 0)
	for _, id := range b {
		if _, ok := set[id]; ok {
			result = append(result, id)
		}
	}
	return result
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
	default:
		return 0, false
	}
}

// IndexType represents the type of payload index
type IndexType int

const (
	IndexTypeHash IndexType = iota
	IndexTypeNumeric
	IndexTypeFullText
	IndexTypeGeo
)

// IndexStats holds statistics about a payload index
type IndexStats struct {
	PointCount int64
	SizeBytes  int64
}

// IndexedFields returns a list of all indexed field names
func (idx *Index) IndexedFields() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	fieldSet := make(map[string]struct{})

	for field := range idx.numericIndexes {
		fieldSet[field] = struct{}{}
	}
	for field := range idx.keywordIndexes {
		fieldSet[field] = struct{}{}
	}
	for field := range idx.booleanIndexes {
		fieldSet[field] = struct{}{}
	}

	fields := make([]string, 0, len(fieldSet))
	for field := range fieldSet {
		fields = append(fields, field)
	}
	return fields
}

// CreateIndex creates a new index for a field with specified type
func (idx *Index) CreateIndex(fieldName string, indexType IndexType) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	switch indexType {
	case IndexTypeNumeric:
		if _, exists := idx.numericIndexes[fieldName]; !exists {
			idx.numericIndexes[fieldName] = NewNumericIndex()
		}
	case IndexTypeHash, IndexTypeFullText:
		if _, exists := idx.keywordIndexes[fieldName]; !exists {
			idx.keywordIndexes[fieldName] = NewKeywordIndex()
		}
	}
}

// DeleteIndex removes an index for a field
func (idx *Index) DeleteIndex(fieldName string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	delete(idx.numericIndexes, fieldName)
	delete(idx.keywordIndexes, fieldName)
	delete(idx.booleanIndexes, fieldName)
}

// GetIndexStats returns statistics for a field's index
func (idx *Index) GetIndexStats(fieldName string) *IndexStats {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	stats := &IndexStats{}

	if numIdx, ok := idx.numericIndexes[fieldName]; ok {
		numIdx.mu.RLock()
		stats.PointCount = int64(len(numIdx.entries))
		stats.SizeBytes = int64(len(numIdx.entries) * 12) // estimate: 8 bytes float64 + 4 bytes uint32
		numIdx.mu.RUnlock()
		return stats
	}

	if kwIdx, ok := idx.keywordIndexes[fieldName]; ok {
		kwIdx.mu.RLock()
		for _, ids := range kwIdx.postings {
			stats.PointCount += int64(len(ids))
			stats.SizeBytes += int64(len(ids) * 4) // 4 bytes per uint32
		}
		kwIdx.mu.RUnlock()
		return stats
	}

	if boolIdx, ok := idx.booleanIndexes[fieldName]; ok {
		boolIdx.mu.RLock()
		stats.PointCount = int64(len(boolIdx.trueIDs) + len(boolIdx.falseIDs))
		stats.SizeBytes = stats.PointCount * 4
		boolIdx.mu.RUnlock()
		return stats
	}

	return nil
}

// IndexField indexes a single field value for a point
func (idx *Index) IndexField(pointID uint32, fieldName string, value interface{}) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	switch v := value.(type) {
	case float64:
		idx.getOrCreateNumeric(fieldName).Add(pointID, v)
	case float32:
		idx.getOrCreateNumeric(fieldName).Add(pointID, float64(v))
	case int:
		idx.getOrCreateNumeric(fieldName).Add(pointID, float64(v))
	case int64:
		idx.getOrCreateNumeric(fieldName).Add(pointID, float64(v))
	case string:
		idx.getOrCreateKeyword(fieldName).Add(pointID, v)
	case bool:
		idx.getOrCreateBoolean(fieldName).Add(pointID, v)
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				idx.getOrCreateKeyword(fieldName).Add(pointID, s)
			}
		}
	}
}
