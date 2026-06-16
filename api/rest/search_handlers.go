package rest

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/limyedb/limyedb/pkg/collection"
	"github.com/limyedb/limyedb/pkg/point"
)

// Search handlers

// SearchRequest represents a search request
type SearchRequest struct {
	Vector      []float32              `json:"vector" binding:"required"`
	Limit       int                    `json:"limit"`
	Offset      int                    `json:"offset"`
	Ef          int                    `json:"ef"`
	Filter      map[string]interface{} `json:"filter,omitempty"`
	WithVector  bool                   `json:"with_vector"`
	WithPayload bool                   `json:"with_payload"`
}

func (s *Server) handleSearch(c *gin.Context) {
	if s.consistentReadProxy(c) {
		return
	}
	name := c.Param("name")

	coll, err := s.collections.Get(name)
	if err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	var req SearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	if req.Limit <= 0 {
		req.Limit = 10
	}

	params := &collection.SearchParams{
		K:           req.Limit,
		Ef:          req.Ef,
		WithVector:  req.WithVector,
		WithPayload: req.WithPayload,
	}

	// Parse filter if present
	if req.Filter != nil {
		params.Filter = parseFilter(req.Filter)
	}

	result, err := coll.SearchWithParams(req.Vector, params)
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"result":  result.Points,
		"took_ms": result.TookMs,
	})
}

// SearchV2 (Named Vectors)

type SearchV2Request struct {
	Vector       []float32              `json:"vector" binding:"required"`
	VectorName   string                 `json:"vector_name,omitempty"`
	Limit        int                    `json:"limit"`
	Ef           int                    `json:"ef"`
	Filter       map[string]interface{} `json:"filter,omitempty"`
	WithVector   bool                   `json:"with_vector"`
	WithPayload  bool                   `json:"with_payload"`
	SparseVector *point.SparseVector    `json:"sparse_vector,omitempty"`
}

func (s *Server) handleSearchV2(c *gin.Context) {
	if s.consistentReadProxy(c) {
		return
	}
	name := c.Param("name")

	coll, err := s.collections.Get(name)
	if err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	var req SearchV2Request
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	if req.Limit <= 0 {
		req.Limit = 10
	}

	params := &collection.SearchParams{
		K:           req.Limit,
		Ef:          req.Ef,
		WithVector:  req.WithVector,
		WithPayload: req.WithPayload,
		SparseQuery: req.SparseVector,
	}

	if req.Filter != nil {
		params.Filter = parseFilter(req.Filter)
	}

	result, err := coll.SearchV2WithParams(req.Vector, req.VectorName, params)
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"result":      result.Points,
		"vector_name": result.VectorName,
		"took_ms":     result.TookMs,
	})
}

// Recommend handlers

// RecommendRequest represents a recommendation request
type RecommendRequest struct {
	PositiveIDs []string `json:"positive" binding:"required"`
	NegativeIDs []string `json:"negative,omitempty"`
	Limit       int      `json:"limit"`
}

func (s *Server) handleRecommend(c *gin.Context) {
	if s.consistentReadProxy(c) {
		return
	}
	name := c.Param("name")

	coll, err := s.collections.Get(name)
	if err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	var req RecommendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	if len(req.PositiveIDs) == 0 {
		respondError(c, http.StatusBadRequest, errors.New("at least one positive ID required"))
		return
	}

	if req.Limit <= 0 {
		req.Limit = 10
	}

	result, err := coll.Recommend(req.PositiveIDs[0], req.Limit)
	if err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"result":  result.Points,
		"took_ms": result.TookMs,
	})
}

// RecommendV2Request supports positive and negative examples
type RecommendV2Request struct {
	Positive    []string               `json:"positive" binding:"required"`
	Negative    []string               `json:"negative,omitempty"`
	Limit       int                    `json:"limit"`
	Ef          int                    `json:"ef,omitempty"`
	Filter      map[string]interface{} `json:"filter,omitempty"`
	VectorName  string                 `json:"vector_name,omitempty"`
	WithVector  bool                   `json:"with_vector"`
	WithPayload bool                   `json:"with_payload"`
}

func (s *Server) handleRecommendV2(c *gin.Context) {
	if s.consistentReadProxy(c) {
		return
	}
	name := c.Param("name")

	coll, err := s.collections.Get(name)
	if err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	var req RecommendV2Request
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	if req.Limit <= 0 {
		req.Limit = 10
	}

	params := &collection.RecommendParams{
		Positive:    req.Positive,
		Negative:    req.Negative,
		K:           req.Limit,
		Ef:          req.Ef,
		VectorName:  req.VectorName,
		WithVector:  req.WithVector,
		WithPayload: req.WithPayload,
	}

	if req.Filter != nil {
		params.Filter = parseFilter(req.Filter)
	}

	result, err := coll.RecommendV2(params)
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"points":  result.Points,
		"took_ms": result.TookMs,
	})
}

// Discovery/Context Search API

// DiscoverRequest represents a discovery request
type DiscoverRequest struct {
	Target      []float32              `json:"target,omitempty"`
	Context     *DiscoverContextInput  `json:"context,omitempty"`
	Limit       int                    `json:"limit"`
	Ef          int                    `json:"ef,omitempty"`
	Filter      map[string]interface{} `json:"filter,omitempty"`
	VectorName  string                 `json:"vector_name,omitempty"`
	WithVector  bool                   `json:"with_vector"`
	WithPayload bool                   `json:"with_payload"`
}

// DiscoverContextInput represents context examples
type DiscoverContextInput struct {
	Positive []ContextExampleInput `json:"positive,omitempty"`
	Negative []ContextExampleInput `json:"negative,omitempty"`
}

// ContextExampleInput represents a context example
type ContextExampleInput struct {
	ID     string    `json:"id,omitempty"`
	Vector []float32 `json:"vector,omitempty"`
}

func (s *Server) handleDiscover(c *gin.Context) {
	name := c.Param("name")

	coll, err := s.collections.Get(name)
	if err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	var req DiscoverRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	if req.Limit <= 0 {
		req.Limit = 10
	}

	params := &collection.DiscoveryParams{
		Target:      req.Target,
		K:           req.Limit,
		Ef:          req.Ef,
		VectorName:  req.VectorName,
		WithVector:  req.WithVector,
		WithPayload: req.WithPayload,
	}

	if req.Filter != nil {
		params.Filter = parseFilter(req.Filter)
	}

	if req.Context != nil {
		params.Context = &collection.DiscoveryContext{}
		for _, p := range req.Context.Positive {
			params.Context.Positive = append(params.Context.Positive, collection.ContextExample{
				ID:     p.ID,
				Vector: p.Vector,
			})
		}
		for _, n := range req.Context.Negative {
			params.Context.Negative = append(params.Context.Negative, collection.ContextExample{
				ID:     n.ID,
				Vector: n.Vector,
			})
		}
	}

	result, err := coll.Discover(params)
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"points":  result.Points,
		"took_ms": result.TookMs,
	})
}

// Group Search API

// GroupSearchRequest represents a grouped search request
type GroupSearchRequest struct {
	Vector     []float32              `json:"vector" binding:"required"`
	GroupBy    string                 `json:"group_by" binding:"required"`
	GroupSize  int                    `json:"group_size"`
	Limit      int                    `json:"limit"`
	Filter     map[string]interface{} `json:"filter,omitempty"`
	VectorName string                 `json:"vector_name,omitempty"`
	WithVector bool                   `json:"with_vector"`
}

func (s *Server) handleGroupSearch(c *gin.Context) {
	name := c.Param("name")

	coll, err := s.collections.Get(name)
	if err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	var req GroupSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	params := &collection.GroupSearchParams{
		Query:      req.Vector,
		GroupBy:    req.GroupBy,
		GroupSize:  req.GroupSize,
		Limit:      req.Limit,
		VectorName: req.VectorName,
		WithVector: req.WithVector,
	}

	if req.Filter != nil {
		params.Filter = parseFilter(req.Filter)
	}

	result, err := coll.GroupSearch(params)
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// Faceted Search API

// FacetRequest represents a facet request
type FacetRequest struct {
	Field      string                 `json:"field" binding:"required"`
	Limit      int                    `json:"limit"`
	Filter     map[string]interface{} `json:"filter,omitempty"`
	MinCount   int                    `json:"min_count"`
	OrderBy    string                 `json:"order_by"`
	Descending bool                   `json:"descending"`
}

func (s *Server) handleFacet(c *gin.Context) {
	name := c.Param("name")

	coll, err := s.collections.Get(name)
	if err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	var req FacetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	params := &collection.FacetParams{
		Field:      req.Field,
		Limit:      req.Limit,
		MinCount:   req.MinCount,
		OrderBy:    req.OrderBy,
		Descending: req.Descending,
	}

	if req.Filter != nil {
		params.Filter = parseFilter(req.Filter)
	}

	result, err := coll.Facet(params)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// MultiFacetRequest represents multiple facet queries
type MultiFacetRequest struct {
	Facets []FacetRequest         `json:"facets" binding:"required"`
	Filter map[string]interface{} `json:"filter,omitempty"`
}

func (s *Server) handleMultiFacet(c *gin.Context) {
	name := c.Param("name")

	coll, err := s.collections.Get(name)
	if err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	var req MultiFacetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	params := &collection.MultiFacetParams{
		Facets: make([]*collection.FacetParams, len(req.Facets)),
	}

	if req.Filter != nil {
		params.Filter = parseFilter(req.Filter)
	}

	for i, f := range req.Facets {
		fp := &collection.FacetParams{
			Field:      f.Field,
			Limit:      f.Limit,
			MinCount:   f.MinCount,
			OrderBy:    f.OrderBy,
			Descending: f.Descending,
		}
		if f.Filter != nil {
			fp.Filter = parseFilter(f.Filter)
		}
		params.Facets[i] = fp
	}

	result, err := coll.MultiFacet(params)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// Query Explain/Planning API

// ExplainRequest represents an explain request
type ExplainRequest struct {
	Vector     []float32              `json:"vector,omitempty"`
	Limit      int                    `json:"limit"`
	Ef         int                    `json:"ef,omitempty"`
	Filter     map[string]interface{} `json:"filter,omitempty"`
	VectorName string                 `json:"vector_name,omitempty"`
	Analyze    bool                   `json:"analyze"`
}

func (s *Server) handleExplain(c *gin.Context) {
	name := c.Param("name")

	coll, err := s.collections.Get(name)
	if err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	var req ExplainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	params := &collection.ExplainParams{
		Query:      req.Vector,
		K:          req.Limit,
		Ef:         req.Ef,
		VectorName: req.VectorName,
		Analyze:    req.Analyze,
	}

	if req.Filter != nil {
		params.Filter = parseFilter(req.Filter)
	}

	plan, err := coll.Explain(params)
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	c.JSON(http.StatusOK, plan)
}
