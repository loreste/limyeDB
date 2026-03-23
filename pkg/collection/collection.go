package collection

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/distance"
	"github.com/limyedb/limyedb/pkg/index/hnsw"
	"github.com/limyedb/limyedb/pkg/index/payload"
	"github.com/limyedb/limyedb/pkg/point"
)

// Collection represents a vector collection
type Collection struct {
	config *config.CollectionConfig
	index  *hnsw.HNSW                // Default/single vector index (backwards compat)
	indices map[string]*hnsw.HNSW    // Named vector indices
	payloadIndex *payload.Index
	distCalc distance.Calculator
	distCalcs map[string]distance.Calculator // Distance calculators per vector

	// Metadata
	createdAt time.Time
	updatedAt atomic.Value // time.Time

	// Concurrency
	mu sync.RWMutex
}

// New creates a new collection
func New(cfg *config.CollectionConfig) (*Collection, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	c := &Collection{
		config:       cfg,
		indices:      make(map[string]*hnsw.HNSW),
		payloadIndex: payload.NewIndex(),
		distCalcs:    make(map[string]distance.Calculator),
		createdAt:    time.Now(),
	}

	// Check if using named vectors
	if cfg.HasNamedVectors() {
		// Create an HNSW index for each named vector
		for name, vc := range cfg.Vectors {
			metric := vc.Metric
			if metric == "" {
				metric = config.MetricCosine
			}
			hnswCfg := &hnsw.Config{
				M:              vc.HNSW.M,
				EfConstruction: vc.HNSW.EfConstruction,
				EfSearch:       vc.HNSW.EfSearch,
				MaxElements:    vc.HNSW.MaxElements,
				Metric:         metric,
				Dimension:      vc.Dimension,
			}
			// Apply defaults
			if hnswCfg.M == 0 {
				hnswCfg.M = 16
			}
			if hnswCfg.EfConstruction == 0 {
				hnswCfg.EfConstruction = 200
			}
			if hnswCfg.EfSearch == 0 {
				hnswCfg.EfSearch = 100
			}
			if hnswCfg.MaxElements == 0 {
				hnswCfg.MaxElements = 100000
			}

			index, err := hnsw.New(hnswCfg)
			if err != nil {
				return nil, err
			}
			c.indices[name] = index
			c.distCalcs[name] = distance.New(metric)
		}
	} else {
		// Legacy single vector mode
		hnswCfg := &hnsw.Config{
			M:              cfg.HNSW.M,
			EfConstruction: cfg.HNSW.EfConstruction,
			EfSearch:       cfg.HNSW.EfSearch,
			MaxElements:    cfg.HNSW.MaxElements,
			Metric:         cfg.Metric,
			Dimension:      cfg.Dimension,
		}

		index, err := hnsw.New(hnswCfg)
		if err != nil {
			return nil, err
		}
		c.index = index
		c.distCalc = distance.New(cfg.Metric)
	}

	c.updatedAt.Store(time.Now())
	return c, nil
}

// Name returns the collection name
func (c *Collection) Name() string {
	return c.config.Name
}

// Dimension returns the default vector dimension
func (c *Collection) Dimension() int {
	return c.config.Dimension
}

// DimensionFor returns the dimension for a specific named vector
func (c *Collection) DimensionFor(vectorName string) int {
	if c.config.HasNamedVectors() {
		if vc := c.config.GetVectorConfig(vectorName); vc != nil {
			return vc.Dimension
		}
		return 0
	}
	return c.config.Dimension
}

// VectorNames returns all vector names in the collection
func (c *Collection) VectorNames() []string {
	return c.config.VectorNames()
}

// HasNamedVectors returns true if the collection uses named vectors
func (c *Collection) HasNamedVectors() bool {
	return c.config.HasNamedVectors()
}

// Metric returns the distance metric
func (c *Collection) Metric() config.MetricType {
	return c.config.Metric
}

// Config returns the collection configuration
func (c *Collection) Config() *config.CollectionConfig {
	return c.config
}

// Insert adds a point to the collection
func (c *Collection) Insert(p *point.Point) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := p.Validate(); err != nil {
		return err
	}

	if len(p.Vector) != c.config.Dimension {
		return ErrDimensionMismatch
	}

	// Normalize for cosine similarity
	if c.config.Metric == config.MetricCosine {
		p.Normalize()
	}

	// Insert into HNSW index
	if err := c.index.Insert(p); err != nil {
		return err
	}

	// Index payload
	nodeID, _ := c.getNodeID(p.ID)
	c.payloadIndex.IndexPoint(nodeID, p.Payload)

	c.updatedAt.Store(time.Now())
	return nil
}

// InsertBatch adds multiple points to the collection
func (c *Collection) InsertBatch(points []*point.Point) (*BatchResult, error) {
	result := &BatchResult{
		Total: len(points),
	}

	for _, p := range points {
		if err := c.Insert(p); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, BatchError{
				ID:  p.ID,
				Err: err,
			})
		} else {
			result.Succeeded++
		}
	}

	return result, nil
}

// InsertV2 adds a point with named vectors to the collection
func (c *Collection) InsertV2(p *point.PointV2) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := p.Validate(); err != nil {
		return err
	}

	// For named vectors collection
	if c.config.HasNamedVectors() {
		// Insert each vector into its corresponding index
		for name, vec := range p.Vectors {
			idx, ok := c.indices[name]
			if !ok {
				return CollectionError("unknown vector name: " + name)
			}

			vc := c.config.GetVectorConfig(name)
			if vc == nil {
				return CollectionError("vector config not found: " + name)
			}

			if len(vec) != vc.Dimension {
				return ErrDimensionMismatch
			}

			// Normalize for cosine similarity
			if vc.Metric == config.MetricCosine {
				vec.Normalize()
			}

			// Create legacy point for HNSW index
			legacyPoint := &point.Point{
				ID:      p.ID,
				Vector:  vec,
				Payload: p.Payload,
			}

			if err := idx.Insert(legacyPoint); err != nil {
				// If insertion fails due to existing point, that's OK for other vectors
				if err.Error() != "point with this ID already exists" {
					return err
				}
			}
		}

		// Handle default vector if present
		if len(p.Vector) > 0 {
			if idx, ok := c.indices["default"]; ok {
				legacyPoint := &point.Point{
					ID:      p.ID,
					Vector:  p.Vector,
					Payload: p.Payload,
				}
				if err := idx.Insert(legacyPoint); err != nil {
					if err.Error() != "point with this ID already exists" {
						return err
					}
				}
			}
		}

		// Index payload using first available index's node ID
		for _, idx := range c.indices {
			if nodeID, ok := idx.GetNodeID(p.ID); ok {
				c.payloadIndex.IndexPoint(nodeID, p.Payload)
				break
			}
		}
	} else {
		// Legacy single vector mode - use default vector
		vec, ok := p.GetVector("")
		if !ok {
			return ErrEmptyVector
		}

		if len(vec) != c.config.Dimension {
			return ErrDimensionMismatch
		}

		if c.config.Metric == config.MetricCosine {
			vec = distance.Normalize(vec)
		}

		legacyPoint := &point.Point{
			ID:      p.ID,
			Vector:  vec,
			Payload: p.Payload,
		}

		if err := c.index.Insert(legacyPoint); err != nil {
			return err
		}

		nodeID, _ := c.getNodeID(p.ID)
		c.payloadIndex.IndexPoint(nodeID, p.Payload)
	}

	c.updatedAt.Store(time.Now())
	return nil
}

// SearchV2 performs k-NN search with a specific named vector
func (c *Collection) SearchV2(query point.Vector, vectorName string, k int) (*SearchResultV2, error) {
	return c.SearchV2WithParams(query, vectorName, &SearchParams{K: k})
}

// SearchResultV2 holds search results for named vector queries
type SearchResultV2 struct {
	Points     []ScoredPointV2 `json:"points"`
	VectorName string          `json:"vector_name"`
	TookMs     int64           `json:"took_ms"`
}

// ScoredPointV2 is a point with named vectors and a similarity score
type ScoredPointV2 struct {
	ID         string                 `json:"id"`
	Score      float32                `json:"score"`
	Vector     point.Vector           `json:"vector,omitempty"`
	Vectors    point.NamedVectors     `json:"vectors,omitempty"`
	Payload    map[string]interface{} `json:"payload,omitempty"`
	VectorName string                 `json:"vector_name,omitempty"`
}

// SearchV2WithParams performs search with custom parameters for named vectors
func (c *Collection) SearchV2WithParams(query point.Vector, vectorName string, params *SearchParams) (*SearchResultV2, error) {
	start := time.Now()

	c.mu.RLock()
	defer c.mu.RUnlock()

	var idx *hnsw.HNSW
	var distCalc distance.Calculator
	var vc *config.VectorConfig

	if c.config.HasNamedVectors() {
		if vectorName == "" {
			vectorName = "default"
		}
		var ok bool
		idx, ok = c.indices[vectorName]
		if !ok {
			return nil, CollectionError("unknown vector name: " + vectorName)
		}
		distCalc = c.distCalcs[vectorName]
		vc = c.config.GetVectorConfig(vectorName)
	} else {
		idx = c.index
		distCalc = c.distCalc
		vc = c.config.GetVectorConfig("")
	}

	if vc == nil {
		return nil, CollectionError("vector config not found")
	}

	if len(query) != vc.Dimension {
		return nil, ErrDimensionMismatch
	}

	// Normalize query for cosine similarity
	if vc.Metric == config.MetricCosine {
		query = distance.Normalize(query)
	}
	_ = distCalc // Will be used for future optimizations

	var candidates []hnsw.Candidate
	var err error

	if params.Filter != nil {
		// Filtered search
		hnswParams := &hnsw.SearchParams{
			K:  params.K,
			Ef: params.Ef,
		}
		if hnswParams.Ef == 0 {
			hnswParams.Ef = 100
		}

		evaluator := payload.NewEvaluator()
		hnswParams.Filter = func(id string, pl map[string]interface{}) bool {
			return evaluator.Evaluate(params.Filter, pl)
		}

		candidates, err = idx.SearchWithFilter(query, hnswParams)
	} else {
		ef := params.Ef
		if ef == 0 {
			ef = 100
		}
		candidates, err = idx.SearchWithEf(query, params.K, ef)
	}

	if err != nil {
		return nil, err
	}

	// Build results
	result := &SearchResultV2{
		Points:     make([]ScoredPointV2, 0, len(candidates)),
		VectorName: vectorName,
		TookMs:     time.Since(start).Milliseconds(),
	}

	for _, cand := range candidates {
		p, err := idx.Get(idx.GetPointID(cand.ID))
		if err != nil {
			continue
		}

		sp := ScoredPointV2{
			ID:         p.ID,
			Score:      1.0 - cand.Distance,
			VectorName: vectorName,
		}

		if params.WithVector {
			sp.Vector = p.Vector
		}
		if params.WithPayload {
			sp.Payload = p.Payload
		}

		result.Points = append(result.Points, sp)
	}

	return result, nil
}

// Upsert inserts or updates a point
func (c *Collection) Upsert(p *point.Point) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := p.Validate(); err != nil {
		return err
	}

	if len(p.Vector) != c.config.Dimension {
		return ErrDimensionMismatch
	}

	// Normalize for cosine similarity
	if c.config.Metric == config.MetricCosine {
		p.Normalize()
	}

	// Try to delete existing (ignore error if not found)
	_ = c.index.Delete(p.ID)

	// Insert new
	if err := c.index.Insert(p); err != nil {
		return err
	}

	nodeID, _ := c.getNodeID(p.ID)
	c.payloadIndex.IndexPoint(nodeID, p.Payload)

	c.updatedAt.Store(time.Now())
	return nil
}

// Get retrieves a point by ID
func (c *Collection) Get(id string) (*point.Point, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.index.Get(id)
}

// Delete removes a point by ID
func (c *Collection) Delete(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.index.Delete(id); err != nil {
		return err
	}

	c.updatedAt.Store(time.Now())
	return nil
}

// Search performs k-NN search
func (c *Collection) Search(query point.Vector, k int) (*SearchResult, error) {
	return c.SearchWithParams(query, &SearchParams{K: k})
}

// SearchParams holds search parameters
type SearchParams struct {
	K          int
	Ef         int
	Filter     *payload.Filter
	WithVector bool
	WithPayload bool
}

// SearchResult holds search results
type SearchResult struct {
	Points []ScoredPoint `json:"points"`
	TookMs int64         `json:"took_ms"`
}

// ScoredPoint is a point with a similarity score
type ScoredPoint struct {
	ID      string                 `json:"id"`
	Score   float32                `json:"score"`
	Vector  point.Vector           `json:"vector,omitempty"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

// SearchWithParams performs search with custom parameters
func (c *Collection) SearchWithParams(query point.Vector, params *SearchParams) (*SearchResult, error) {
	start := time.Now()

	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(query) != c.config.Dimension {
		return nil, ErrDimensionMismatch
	}

	// Normalize query for cosine similarity
	if c.config.Metric == config.MetricCosine {
		query = distance.Normalize(query)
	}

	var candidates []hnsw.Candidate
	var err error

	if params.Filter != nil {
		// Filtered search
		hnswParams := &hnsw.SearchParams{
			K:  params.K,
			Ef: params.Ef,
		}
		if hnswParams.Ef == 0 {
			hnswParams.Ef = 100
		}

		// Create filter function
		evaluator := payload.NewEvaluator()
		hnswParams.Filter = func(id string, pl map[string]interface{}) bool {
			return evaluator.Evaluate(params.Filter, pl)
		}

		candidates, err = c.index.SearchWithFilter(query, hnswParams)
	} else {
		// Unfiltered search
		ef := params.Ef
		if ef == 0 {
			ef = 100
		}
		candidates, err = c.index.SearchWithEf(query, params.K, ef)
	}

	if err != nil {
		return nil, err
	}

	// Build results
	result := &SearchResult{
		Points: make([]ScoredPoint, 0, len(candidates)),
		TookMs: time.Since(start).Milliseconds(),
	}

	for _, cand := range candidates {
		p, err := c.index.Get(c.getPointID(cand.ID))
		if err != nil {
			continue
		}

		sp := ScoredPoint{
			ID:    p.ID,
			Score: 1.0 - cand.Distance, // Convert distance to similarity
		}

		if params.WithVector {
			sp.Vector = p.Vector
		}
		if params.WithPayload {
			sp.Payload = p.Payload
		}

		result.Points = append(result.Points, sp)
	}

	return result, nil
}

// Recommend finds similar points to a given point
func (c *Collection) Recommend(id string, k int) (*SearchResult, error) {
	start := time.Now()

	c.mu.RLock()
	defer c.mu.RUnlock()

	candidates, err := c.index.Recommend(id, k)
	if err != nil {
		return nil, err
	}

	result := &SearchResult{
		Points: make([]ScoredPoint, 0, len(candidates)),
		TookMs: time.Since(start).Milliseconds(),
	}

	for _, cand := range candidates {
		p, err := c.index.Get(c.getPointID(cand.ID))
		if err != nil {
			continue
		}

		result.Points = append(result.Points, ScoredPoint{
			ID:      p.ID,
			Score:   1.0 - cand.Distance,
			Payload: p.Payload,
		})
	}

	return result, nil
}

// Size returns the number of points in the collection
func (c *Collection) Size() int64 {
	if c.config.HasNamedVectors() {
		// Return the size of the first index (all should have same points)
		for _, idx := range c.indices {
			return idx.Size()
		}
		return 0
	}
	return c.index.Size()
}

// Info returns collection information
func (c *Collection) Info() *Info {
	info := &Info{
		Name:      c.config.Name,
		Dimension: c.config.Dimension,
		Metric:    string(c.config.Metric),
		Size:      c.Size(),
		Config:    c.config,
		CreatedAt: c.createdAt,
		UpdatedAt: c.updatedAt.Load().(time.Time),
	}

	if c.config.HasNamedVectors() {
		info.Vectors = make(map[string]VectorInfo)
		for name, vc := range c.config.Vectors {
			vi := VectorInfo{
				Dimension: vc.Dimension,
				Metric:    string(vc.Metric),
			}
			if idx, ok := c.indices[name]; ok {
				vi.Size = idx.Size()
			}
			info.Vectors[name] = vi
		}
	}

	return info
}

// Info holds collection information
type Info struct {
	Name      string                   `json:"name"`
	Dimension int                      `json:"dimension"`
	Metric    string                   `json:"metric"`
	Size      int64                    `json:"size"`
	Config    *config.CollectionConfig `json:"config"`
	CreatedAt time.Time                `json:"created_at"`
	UpdatedAt time.Time                `json:"updated_at"`
	Vectors   map[string]VectorInfo    `json:"vectors,omitempty"`
}

// VectorInfo holds information about a named vector
type VectorInfo struct {
	Dimension int    `json:"dimension"`
	Metric    string `json:"metric"`
	Size      int64  `json:"size"`
}

// SetEfSearch sets the search quality parameter
func (c *Collection) SetEfSearch(ef int) {
	c.index.SetEfSearch(ef)
}

// Iterate iterates over all points in the collection
func (c *Collection) Iterate(fn func(*point.Point) error) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.index.Iterate(func(id string) error {
		p, err := c.index.Get(id)
		if err != nil {
			return nil // Skip deleted/invalid points
		}
		return fn(p)
	})
}

// Export exports collection data
func (c *Collection) Export() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data := struct {
		Config *config.CollectionConfig `json:"config"`
		Size   int64                    `json:"size"`
	}{
		Config: c.config,
		Size:   c.index.Size(),
	}

	return json.Marshal(data)
}

// Helper methods

func (c *Collection) getNodeID(pointID string) (uint32, bool) {
	return c.index.GetNodeID(pointID)
}

func (c *Collection) getPointID(nodeID uint32) string {
	return c.index.GetPointID(nodeID)
}

// BatchResult holds the result of a batch operation
type BatchResult struct {
	Total     int
	Succeeded int
	Failed    int
	Errors    []BatchError
}

// BatchError represents an error for a specific point
type BatchError struct {
	ID  string
	Err error
}

// Errors
type CollectionError string

func (e CollectionError) Error() string { return string(e) }

const (
	ErrDimensionMismatch  CollectionError = "vector dimension mismatch"
	ErrCollectionExists   CollectionError = "collection already exists"
	ErrCollectionNotFound CollectionError = "collection not found"
	ErrPointNotFound      CollectionError = "point not found"
	ErrEmptyVector        CollectionError = "vector cannot be empty"
)

// ScrollParams holds parameters for scroll/pagination
type ScrollParams struct {
	Offset      string                 // Offset point ID (exclusive)
	Limit       int                    // Maximum number of points to return
	Filter      *payload.Filter        // Optional filter
	WithVector  bool                   // Include vectors in response
	WithPayload bool                   // Include payload in response
	OrderBy     string                 // Field to order by (optional)
}

// ScrollResult holds the result of a scroll operation
type ScrollResult struct {
	Points     []ScoredPoint `json:"points"`
	NextOffset string        `json:"next_offset,omitempty"`
	TookMs     int64         `json:"took_ms"`
}

// Scroll retrieves points with pagination support
func (c *Collection) Scroll(params *ScrollParams) (*ScrollResult, error) {
	start := time.Now()

	c.mu.RLock()
	defer c.mu.RUnlock()

	if params.Limit <= 0 {
		params.Limit = 10
	}
	if params.Limit > 1000 {
		params.Limit = 1000
	}

	var allPoints []*point.Point
	var idx *hnsw.HNSW

	if c.config.HasNamedVectors() {
		// Use first available index
		for _, i := range c.indices {
			idx = i
			break
		}
	} else {
		idx = c.index
	}

	if idx == nil {
		return &ScrollResult{Points: []ScoredPoint{}, TookMs: time.Since(start).Milliseconds()}, nil
	}

	allPoints = idx.GetAllPoints()

	// Filter points if filter is provided
	var filteredPoints []*point.Point
	if params.Filter != nil {
		evaluator := payload.NewEvaluator()
		for _, p := range allPoints {
			if evaluator.Evaluate(params.Filter, p.Payload) {
				filteredPoints = append(filteredPoints, p)
			}
		}
	} else {
		filteredPoints = allPoints
	}

	// Find offset position
	startIdx := 0
	if params.Offset != "" {
		for i, p := range filteredPoints {
			if p.ID == params.Offset {
				startIdx = i + 1
				break
			}
		}
	}

	// Get the page
	endIdx := startIdx + params.Limit
	if endIdx > len(filteredPoints) {
		endIdx = len(filteredPoints)
	}

	pagePoints := filteredPoints[startIdx:endIdx]

	result := &ScrollResult{
		Points: make([]ScoredPoint, 0, len(pagePoints)),
		TookMs: time.Since(start).Milliseconds(),
	}

	for _, p := range pagePoints {
		sp := ScoredPoint{
			ID: p.ID,
		}
		if params.WithVector {
			sp.Vector = p.Vector
		}
		if params.WithPayload {
			sp.Payload = p.Payload
		}
		result.Points = append(result.Points, sp)
	}

	// Set next offset if there are more results
	if endIdx < len(filteredPoints) && len(pagePoints) > 0 {
		result.NextOffset = pagePoints[len(pagePoints)-1].ID
	}

	return result, nil
}
