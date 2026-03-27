package rest

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/hashicorp/raft"
	"github.com/limyedb/limyedb/pkg/cluster"
	"github.com/limyedb/limyedb/pkg/collection"
	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/embedder"
	"github.com/limyedb/limyedb/pkg/index/payload"
	"github.com/limyedb/limyedb/pkg/cdc"
	"github.com/limyedb/limyedb/pkg/point"
	"github.com/limyedb/limyedb/pkg/version"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Health handlers

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"version": version.Version,
	})
}

func (s *Server) handleReadiness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"ready": true,
	})
}

// Collection handlers

// CreateCollectionRequest represents a collection creation request
type CreateCollectionRequest struct {
	Name      string            `json:"name" binding:"required"`
	Dimension int               `json:"dimension" binding:"required,min=1,max=65536"`
	Metric    config.MetricType `json:"metric"`
	OnDisk    bool              `json:"on_disk"`
	HNSW      *HNSWParams       `json:"hnsw,omitempty"`
}

// HNSWParams represents HNSW configuration
type HNSWParams struct {
	M              int `json:"m"`
	EfConstruction int `json:"ef_construction"`
	EfSearch       int `json:"ef_search"`
}

// proxyToLeader transparently forwards HTTP mutation requests to the active Raft Leader
func (s *Server) proxyToLeader(c *gin.Context) bool {
	if s.raft == nil {
		return false
	}
	if s.raft.Raft.State() == raft.Leader {
		return false
	}

	leaderAddr := s.raft.GetLeaderRestAddr()
	if leaderAddr == "" {
		respondError(c, http.StatusServiceUnavailable, errors.New("cluster leader election in progress or leader rest address unknown"))
		return true
	}

	target, err := url.Parse(leaderAddr)
	if err != nil {
		respondError(c, http.StatusInternalServerError, fmt.Errorf("invalid leader rest address: %w", err))
		return true
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ServeHTTP(c.Writer, c.Request)
	return true
}

func (s *Server) handleCreateCollection(c *gin.Context) {
	if s.proxyToLeader(c) {
		return
	}

	var req CreateCollectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	if !s.checkPermission(c, req.Name, "write") {
		respondError(c, http.StatusForbidden, errors.New("insufficient permissions to create collection"))
		return
	}

	if req.Metric == "" {
		req.Metric = config.MetricCosine
	}

	cfg := &config.CollectionConfig{
		Name:      req.Name,
		Dimension: req.Dimension,
		Metric:    req.Metric,
		OnDisk:    req.OnDisk,
		HNSW: config.HNSWConfig{
			M:              16,
			EfConstruction: 200,
			EfSearch:       100,
			MaxElements:    100000,
		},
	}

	if req.HNSW != nil {
		if req.HNSW.M > 0 {
			cfg.HNSW.M = req.HNSW.M
		}
		if req.HNSW.EfConstruction > 0 {
			cfg.HNSW.EfConstruction = req.HNSW.EfConstruction
		}
		if req.HNSW.EfSearch > 0 {
			cfg.HNSW.EfSearch = req.HNSW.EfSearch
		}
	}

	if s.raft != nil {
		if err := s.raft.Write(cluster.OpCreateCollection, cluster.CreateCollectionData{Config: cfg}); err != nil {
			respondError(c, http.StatusBadRequest, fmt.Errorf("raft write failed: %w", err))
			return
		}
		coll, err := s.collections.Get(cfg.Name)
		if err != nil {
			respondError(c, http.StatusInternalServerError, err)
			return
		}
		respondCreated(c, coll.Info())
		return
	}

	coll, err := s.collections.Create(cfg)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, collection.ErrCollectionExists) {
			status = http.StatusConflict
		}
		respondError(c, status, err)
		return
	}

	respondCreated(c, coll.Info())
}

func (s *Server) handleListCollections(c *gin.Context) {
	infos := s.collections.ListInfo()
	
	filtered := make([]*collection.Info, 0, len(infos))
	for _, info := range infos {
		if s.checkPermission(c, info.Name, "read") {
			filtered = append(filtered, info)
		}
	}
	
	c.JSON(http.StatusOK, gin.H{
		"collections": filtered,
	})
}

func (s *Server) handleGetCollection(c *gin.Context) {
	name := c.Param("name")

	coll, err := s.collections.Get(name)
	if err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	respondSuccess(c, coll.Info())
}

func (s *Server) handleDeleteCollection(c *gin.Context) {
	if s.proxyToLeader(c) {
		return
	}

	name := c.Param("name")

	if s.raft != nil {
		if err := s.raft.Write(cluster.OpDeleteCollection, cluster.DeleteCollectionData{Name: name}); err != nil {
			respondError(c, http.StatusBadRequest, fmt.Errorf("raft write failed: %w", err))
			return
		}
		respondSuccess(c, gin.H{"deleted": name})
		return
	}

	if err := s.collections.Delete(name); err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	respondSuccess(c, gin.H{"deleted": name})
}

func (s *Server) handleUpdateCollection(c *gin.Context) {
	name := c.Param("name")

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	if err := s.collections.UpdateConfig(name, updates); err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	coll, _ := s.collections.Get(name)
	respondSuccess(c, coll.Info())
}

// Point handlers

// UpsertPointsRequest represents a point upsert request
type UpsertPointsRequest struct {
	Points []PointInput `json:"points" binding:"required"`
}

// PointInput represents a point in a request
type PointInput struct {
	ID      string                 `json:"id" binding:"required"`
	Vector  []float32              `json:"vector" binding:"required"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

func (s *Server) handleUpsertPoints(c *gin.Context) {
	if s.proxyToLeader(c) {
		return
	}

	name := c.Param("name")

	coll, err := s.collections.Get(name)
	if err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	var req UpsertPointsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	points := make([]*point.Point, len(req.Points))
	for i, p := range req.Points {
		points[i] = point.NewPointWithID(p.ID, p.Vector, p.Payload)
	}

	if s.raft != nil {
		if err := s.raft.Write(cluster.OpUpsertPoints, cluster.UpsertPointsData{
			CollectionName: name,
			Points:         points,
		}); err != nil {
			respondError(c, http.StatusBadRequest, fmt.Errorf("raft write failed: %w", err))
			return
		}
		respondSuccess(c, gin.H{
			"succeeded": len(points),
			"failed":    0,
		})
		return
	}

	result, err := coll.InsertBatch(points)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	respondSuccess(c, gin.H{
		"succeeded": result.Succeeded,
		"failed":    result.Failed,
	})
}

func (s *Server) handleGetPoint(c *gin.Context) {
	name := c.Param("name")
	id := c.Param("id")

	coll, err := s.collections.Get(name)
	if err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	p, err := coll.Get(id)
	if err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	respondSuccess(c, gin.H{
		"id":      p.ID,
		"vector":  p.Vector,
		"payload": p.Payload,
	})
}

func (s *Server) handleDeletePoint(c *gin.Context) {
	if s.proxyToLeader(c) {
		return
	}

	name := c.Param("name")
	id := c.Param("id")

	coll, err := s.collections.Get(name)
	if err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	if s.raft != nil {
		if err := s.raft.Write(cluster.OpDeletePoints, cluster.DeletePointsData{
			CollectionName: name,
			IDs:            []string{id},
		}); err != nil {
			respondError(c, http.StatusBadRequest, fmt.Errorf("raft write failed: %w", err))
			return
		}
		respondSuccess(c, gin.H{"deleted": id})
		return
	}

	if err := coll.Delete(id); err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	respondSuccess(c, gin.H{"deleted": id})
}

func (s *Server) handleBatchUpsert(c *gin.Context) {
	s.handleUpsertPoints(c)
}

// BatchDeleteRequest represents a batch delete request
type BatchDeleteRequest struct {
	IDs []string `json:"ids" binding:"required"`
}

func (s *Server) handleBatchDelete(c *gin.Context) {
	name := c.Param("name")

	coll, err := s.collections.Get(name)
	if err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	var req BatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	deleted := 0
	for _, id := range req.IDs {
		if err := coll.Delete(id); err == nil {
			deleted++
		}
	}

	respondSuccess(c, gin.H{
		"deleted": deleted,
		"total":   len(req.IDs),
	})
}

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

// parseFilter recursively converts a nested map to a payload.Filter AST
func parseFilter(m map[string]interface{}) *payload.Filter {
	if len(m) == 0 {
		return nil
	}

	var conditions []*payload.Filter

	if andList, ok := m["$and"].([]interface{}); ok {
		var subFilters []*payload.Filter
		for _, item := range andList {
			if im, ok := item.(map[string]interface{}); ok {
				if f := parseFilter(im); f != nil {
					subFilters = append(subFilters, f)
				}
			}
		}
		if len(subFilters) > 0 {
			conditions = append(conditions, payload.And(subFilters...))
		}
	}

	if orList, ok := m["$or"].([]interface{}); ok {
		var subFilters []*payload.Filter
		for _, item := range orList {
			if im, ok := item.(map[string]interface{}); ok {
				if f := parseFilter(im); f != nil {
					subFilters = append(subFilters, f)
				}
			}
		}
		if len(subFilters) > 0 {
			conditions = append(conditions, payload.Or(subFilters...))
		}
	}

	if notItem, ok := m["$not"].(map[string]interface{}); ok {
		if f := parseFilter(notItem); f != nil {
			conditions = append(conditions, payload.Not(f))
		}
	}

	// Legacy Qdrant compatibility
	if must, ok := m["must"].([]interface{}); ok {
		var subFilters []*payload.Filter
		for _, cond := range must {
			if condMap, ok := cond.(map[string]interface{}); ok {
				if f := parseFilter(condMap); f != nil {
					subFilters = append(subFilters, f)
				}
			}
		}
		if len(subFilters) > 0 {
			conditions = append(conditions, payload.And(subFilters...))
		}
	}

	if should, ok := m["should"].([]interface{}); ok {
		var subFilters []*payload.Filter
		for _, cond := range should {
			if condMap, ok := cond.(map[string]interface{}); ok {
				if f := parseFilter(condMap); f != nil {
					subFilters = append(subFilters, f)
				}
			}
		}
		if len(subFilters) > 0 {
			conditions = append(conditions, payload.Or(subFilters...))
		}
	}

	for field, value := range m {
		if field == "$and" || field == "$or" || field == "$not" || field == "must" || field == "should" || field == "must_not" {
			continue
		}

		switch v := value.(type) {
		case map[string]interface{}:
			cond := parseOperators(v)
			if cond != nil {
				conditions = append(conditions, payload.Field(field, cond))
			}
		default:
			conditions = append(conditions, payload.Field(field, payload.Eq(v)))
		}
	}

	if len(conditions) == 0 {
		return nil
	}
	if len(conditions) == 1 {
		return conditions[0]
	}
	return payload.And(conditions...)
}

func parseOperators(v map[string]interface{}) *payload.Condition {
	if gte, ok := v["$gte"]; ok {
		return payload.Gte(gte)
	}
	if gt, ok := v["$gt"]; ok {
		return payload.Gt(gt)
	}
	if lte, ok := v["$lte"]; ok {
		return payload.Lte(lte)
	}
	if lt, ok := v["$lt"]; ok {
		return payload.Lt(lt)
	}
	if eq, ok := v["$eq"]; ok {
		return payload.Eq(eq)
	}
	if ne, ok := v["$ne"]; ok {
		return payload.Ne(ne)
	}
	if in, ok := v["$in"].([]interface{}); ok {
		return payload.In(in...)
	}
	if nin, ok := v["$nin"].([]interface{}); ok {
		return payload.NotIn(nin...)
	}
	if contains, ok := v["$contains"].(string); ok {
		return payload.Contains(contains)
	}
	if startsWith, ok := v["$startsWith"].(string); ok {
		return payload.StartsWith(startsWith)
	}
	if endsWith, ok := v["$endsWith"].(string); ok {
		return payload.EndsWith(endsWith)
	}

	if match, ok := v["match"].(map[string]interface{}); ok {
		if val, ok := match["value"]; ok {
			return payload.Eq(val)
		}
		if anyVal, ok := match["any"].([]interface{}); ok {
			return payload.In(anyVal...)
		}
		if text, ok := match["text"].(string); ok {
			return payload.Contains(text)
		}
	}

	if qRange, ok := v["range"].(map[string]interface{}); ok {
		min, hasMin := qRange["gte"]
		if !hasMin {
			min = qRange["gt"]
		}
		max, hasMax := qRange["lte"]
		if !hasMax {
			max = qRange["lt"]
		}
		if hasMin || hasMax {
			return payload.Range(min, max)
		}
	}

	return nil
}

// RecommendRequest represents a recommendation request
type RecommendRequest struct {
	PositiveIDs []string `json:"positive" binding:"required"`
	NegativeIDs []string `json:"negative,omitempty"`
	Limit       int      `json:"limit"`
}

func (s *Server) handleRecommend(c *gin.Context) {
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

	// Use first positive ID for simple recommendation
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

// Snapshot handlers

func (s *Server) handleCreateSnapshot(c *gin.Context) {
	if s.snapshots == nil {
		respondError(c, http.StatusServiceUnavailable, errors.New("snapshots not configured"))
		return
	}

	snap, err := s.collections.CreateSnapshot(s.snapshots)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	respondCreated(c, snap)
}

func (s *Server) handleListSnapshots(c *gin.Context) {
	if s.snapshots == nil {
		respondError(c, http.StatusServiceUnavailable, errors.New("snapshots not configured"))
		return
	}

	snaps, err := s.snapshots.List()
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"snapshots": snaps,
	})
}

func (s *Server) handleRestoreSnapshot(c *gin.Context) {
	if s.snapshots == nil {
		respondError(c, http.StatusServiceUnavailable, errors.New("snapshots not configured"))
		return
	}

	id := c.Param("id")

	if err := s.collections.RestoreSnapshot(s.snapshots, id); err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	respondSuccess(c, gin.H{"restored": id})
}

func (s *Server) handleDeleteSnapshot(c *gin.Context) {
	if s.snapshots == nil {
		respondError(c, http.StatusServiceUnavailable, errors.New("snapshots not configured"))
		return
	}

	id := c.Param("id")

	if err := s.snapshots.Delete(id); err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	respondSuccess(c, gin.H{"deleted": id})
}

// Metrics handler

var (
	TotalCollections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "limyedb_collections_total",
		Help: "Total number of collections hosted on this node",
	})
	TotalPoints = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "limyedb_points_total",
		Help: "Total number of embedded points tracked globally across all namespaces",
	})
)

func init() {
	prometheus.MustRegister(TotalCollections)
	prometheus.MustRegister(TotalPoints)
}

func (s *Server) handleMetrics(c *gin.Context) {
	TotalCollections.Set(float64(s.collections.Count()))

	points := 0
	for _, name := range s.collections.List() {
		if coll, err := s.collections.Get(name); err == nil {
			points += int(coll.Size())
		}
	}
	TotalPoints.Set(float64(points))

	promhttp.Handler().ServeHTTP(c.Writer, c.Request)
}

// ============================================================================
// Scroll/Pagination API
// ============================================================================

// ScrollRequest represents a scroll request
type ScrollRequest struct {
	Offset      string                 `json:"offset,omitempty"`
	Limit       int                    `json:"limit"`
	Filter      map[string]interface{} `json:"filter,omitempty"`
	WithVector  bool                   `json:"with_vector"`
	WithPayload bool                   `json:"with_payload"`
}

func (s *Server) handleScroll(c *gin.Context) {
	name := c.Param("name")

	coll, err := s.collections.Get(name)
	if err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	var req ScrollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	params := &collection.ScrollParams{
		Offset:      req.Offset,
		Limit:       req.Limit,
		WithVector:  req.WithVector,
		WithPayload: req.WithPayload,
	}

	if req.Filter != nil {
		params.Filter = parseFilter(req.Filter)
	}

	result, err := coll.Scroll(params)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"points":      result.Points,
		"next_offset": result.NextOffset,
		"took_ms":     result.TookMs,
	})
}

// ============================================================================
// Named Vectors API
// ============================================================================

// CreateCollectionV2Request supports named vectors
type CreateCollectionV2Request struct {
	Name    string                        `json:"name" binding:"required"`
	Vectors map[string]VectorConfigInput  `json:"vectors"`
	OnDisk  bool                          `json:"on_disk"`
}

// VectorConfigInput represents vector configuration
type VectorConfigInput struct {
	Dimension int               `json:"dimension" binding:"required"`
	Metric    config.MetricType `json:"metric"`
	HNSW      *HNSWParams       `json:"hnsw,omitempty"`
}

func (s *Server) handleCreateCollectionV2(c *gin.Context) {
	var req CreateCollectionV2Request
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	if !s.checkPermission(c, req.Name, "write") {
		respondError(c, http.StatusForbidden, errors.New("insufficient permissions to create collection"))
		return
	}

	cfg := &config.CollectionConfig{
		Name:    req.Name,
		OnDisk:  req.OnDisk,
		Vectors: make(map[string]config.VectorConfig),
	}

	for name, vc := range req.Vectors {
		metric := vc.Metric
		if metric == "" {
			metric = config.MetricCosine
		}

		vcfg := config.VectorConfig{
			Dimension: vc.Dimension,
			Metric:    metric,
			HNSW: config.HNSWConfig{
				M:              16,
				EfConstruction: 200,
				EfSearch:       100,
				MaxElements:    100000,
			},
		}

		if vc.HNSW != nil {
			if vc.HNSW.M > 0 {
				vcfg.HNSW.M = vc.HNSW.M
			}
			if vc.HNSW.EfConstruction > 0 {
				vcfg.HNSW.EfConstruction = vc.HNSW.EfConstruction
			}
			if vc.HNSW.EfSearch > 0 {
				vcfg.HNSW.EfSearch = vc.HNSW.EfSearch
			}
		}

		cfg.Vectors[name] = vcfg
	}

	coll, err := s.collections.Create(cfg)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, collection.ErrCollectionExists) {
			status = http.StatusConflict
		}
		respondError(c, status, err)
		return
	}

	respondCreated(c, coll.Info())
}

// UpsertPointsV2Request supports named vectors
type UpsertPointsV2Request struct {
	Points []PointV2Input `json:"points" binding:"required"`
}

type PointV2Input struct {
	ID      string                 `json:"id" binding:"required"`
	Vector  []float32              `json:"vector,omitempty"`
	Vectors map[string][]float32   `json:"vectors,omitempty"`
	Payload map[string]interface{} `json:"payload,omitempty"`
	Sparse  *point.SparseVector    `json:"sparse,omitempty"`
}

func (s *Server) handleUpsertPointsV2(c *gin.Context) {
	name := c.Param("name")

	coll, err := s.collections.Get(name)
	if err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	var req UpsertPointsV2Request
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	succeeded := 0
	failed := 0

	for _, pi := range req.Points {
		p := &point.PointV2{
			ID:      pi.ID,
			Vector:  pi.Vector,
			Payload: pi.Payload,
			Sparse:  pi.Sparse,
		}

		if len(pi.Vectors) > 0 {
			p.Vectors = make(point.NamedVectors)
			for vn, v := range pi.Vectors {
				p.Vectors[vn] = v
			}
		}

		if err := coll.InsertV2(p); err != nil {
			failed++
		} else {
			succeeded++
		}
	}

	respondSuccess(c, gin.H{
		"succeeded": succeeded,
		"failed":    failed,
	})
}

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

// ============================================================================
// Collection Aliases API
// ============================================================================

// CreateAliasRequest represents an alias creation request
type CreateAliasRequest struct {
	Alias          string `json:"alias" binding:"required"`
	CollectionName string `json:"collection_name" binding:"required"`
}

func (s *Server) handleCreateAlias(c *gin.Context) {
	var req CreateAliasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	if s.aliases == nil {
		respondError(c, http.StatusServiceUnavailable, errors.New("aliases not configured"))
		return
	}

	if err := s.aliases.Create(req.Alias, req.CollectionName); err != nil {
		respondError(c, http.StatusConflict, err)
		return
	}

	respondCreated(c, gin.H{
		"alias":      req.Alias,
		"collection": req.CollectionName,
	})
}

func (s *Server) handleListAliases(c *gin.Context) {
	if s.aliases == nil {
		respondError(c, http.StatusServiceUnavailable, errors.New("aliases not configured"))
		return
	}

	aliases := s.aliases.List()
	c.JSON(http.StatusOK, gin.H{"aliases": aliases})
}

func (s *Server) handleDeleteAlias(c *gin.Context) {
	alias := c.Param("alias")

	if s.aliases == nil {
		respondError(c, http.StatusServiceUnavailable, errors.New("aliases not configured"))
		return
	}

	if err := s.aliases.Delete(alias); err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	respondSuccess(c, gin.H{"deleted": alias})
}

// SwitchAliasRequest represents an alias switch request
type SwitchAliasRequest struct {
	CollectionName string `json:"collection_name" binding:"required"`
}

func (s *Server) handleSwitchAlias(c *gin.Context) {
	alias := c.Param("alias")

	var req SwitchAliasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	if s.aliases == nil {
		respondError(c, http.StatusServiceUnavailable, errors.New("aliases not configured"))
		return
	}

	if err := s.aliases.Switch(alias, req.CollectionName); err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	respondSuccess(c, gin.H{
		"alias":      alias,
		"collection": req.CollectionName,
	})
}

// ============================================================================
// Faceted Search API
// ============================================================================

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

// ============================================================================
// Discovery/Context Search API
// ============================================================================

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

// ============================================================================
// Query Explain/Planning API
// ============================================================================

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

// ============================================================================
// Payload Index Configuration API
// ============================================================================

// CreatePayloadIndexRequest represents a payload index creation request
type CreatePayloadIndexRequest struct {
	FieldName string                 `json:"field_name" binding:"required"`
	FieldType string                 `json:"field_type" binding:"required"`
	IndexType string                 `json:"index_type,omitempty"`
	Options   map[string]interface{} `json:"options,omitempty"`
}

func (s *Server) handleCreatePayloadIndex(c *gin.Context) {
	if s.proxyToLeader(c) {
		return
	}

	name := c.Param("name")

	coll, err := s.collections.Get(name)
	if err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	var req CreatePayloadIndexRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	cfg := &collection.PayloadIndexConfig{
		FieldName: req.FieldName,
		FieldType: collection.PayloadFieldType(req.FieldType),
		IndexType: collection.PayloadIndexType(req.IndexType),
		Options:   req.Options,
	}

	if err := coll.CreatePayloadIndex(cfg); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	respondCreated(c, gin.H{
		"field_name": req.FieldName,
		"status":     "created",
	})
}

func (s *Server) handleListPayloadIndexes(c *gin.Context) {
	name := c.Param("name")

	coll, err := s.collections.Get(name)
	if err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	indexes := coll.GetPayloadIndexes()
	c.JSON(http.StatusOK, gin.H{"indexes": indexes})
}

func (s *Server) handleDeletePayloadIndex(c *gin.Context) {
	if s.proxyToLeader(c) {
		return
	}

	name := c.Param("name")
	fieldName := c.Param("field")

	coll, err := s.collections.Get(name)
	if err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	if err := coll.DeletePayloadIndex(fieldName); err != nil {
		respondError(c, http.StatusNotFound, err)
		return
	}

	respondSuccess(c, gin.H{"deleted": fieldName})
}

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

// ============================================================================
// Cluster API
// ============================================================================

type JoinClusterRequest struct {
	NodeID   string `json:"node_id" binding:"required"`
	RaftAddr string `json:"raft_addr" binding:"required"`
}

func (s *Server) handleJoinCluster(c *gin.Context) {
	if s.raft == nil {
		respondError(c, http.StatusBadRequest, errors.New("raft clustering is not enabled on this node"))
		return
	}

	var req JoinClusterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	if err := s.raft.Join(req.NodeID, req.RaftAddr); err != nil {
		respondError(c, http.StatusInternalServerError, fmt.Errorf("failed to join cluster: %w", err))
		return
	}

	respondSuccess(c, gin.H{
		"message":   fmt.Sprintf("node %s joined successfully", req.NodeID),
		"node_id":   req.NodeID,
		"raft_addr": req.RaftAddr,
	})
}

// Auto-Embed API

type AutoEmbedRequest struct {
	Provider     string         `json:"provider"`
	Model        string         `json:"model"`
	APIKey       string         `json:"api_key"`
	SourceFields []string       `json:"source_fields"`
	Points       []PointV2Input `json:"points"`
}

func (s *Server) handleAutoEmbed(c *gin.Context) {
	if s.proxyToLeader(c) {
		return
	}

	name := c.Param("name")
	coll, err := s.collections.Get(name)
	if err != nil {
		respondError(c, http.StatusNotFound, fmt.Errorf("collection not found: %w", err))
		return
	}

	var req AutoEmbedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	e, err := embedder.New(embedder.Config{
		Provider: req.Provider,
		Model:    req.Model,
		APIKey:   req.APIKey,
	})
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	// Extract text strings
	texts := make([]string, len(req.Points))
	for i, p := range req.Points {
		var combined string
		for _, field := range req.SourceFields {
			if val, ok := p.Payload[field]; ok {
				if str, isStr := val.(string); isStr {
					combined += str + " "
				}
			}
		}
		if combined == "" {
			combined = "unknown context"
		}
		texts[i] = combined
	}

	// Batch remote inference
	vectors, err := e.EmbedBatch(c.Request.Context(), texts)
	if err != nil {
		respondError(c, http.StatusInternalServerError, fmt.Errorf("embedding failed: %w", err))
		return
	}

	succeeded := 0
	failed := 0

	for i, pi := range req.Points {
		p := &point.PointV2{
			ID:      pi.ID,
			Payload: pi.Payload,
			Sparse:  pi.Sparse,
		}

		if len(pi.Vector) == 0 {
			p.Vector = vectors[i]
		} else {
			p.Vector = pi.Vector
		}

		if len(pi.Vectors) > 0 {
			p.Vectors = make(point.NamedVectors)
			for vn, v := range pi.Vectors {
				p.Vectors[vn] = v
			}
		}

		if err := coll.InsertV2(p); err != nil {
			failed++
		} else {
			succeeded++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   failed == 0,
		"succeeded": succeeded,
		"failed":    failed,
	})
}

// ============================================================================
// Change Data Capture API
// ============================================================================

func (s *Server) handleCreateWebhook(c *gin.Context) {
	var req cdc.WebhookSubscription
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	collectionName := c.Param("name")
	cdc.GetDispatcher().Subscribe(collectionName, req)

	respondSuccess(c, gin.H{
		"message":    "webhook subscribed successfully",
		"collection": collectionName,
	})
}


