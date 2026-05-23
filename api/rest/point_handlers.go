package rest

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/limyedb/limyedb/pkg/cluster"
	"github.com/limyedb/limyedb/pkg/collection"
	"github.com/limyedb/limyedb/pkg/embedder"
	"github.com/limyedb/limyedb/pkg/point"
)

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
		resp, err := s.raft.Write(cluster.OpUpsertPoints, cluster.UpsertPointsData{
			CollectionName: name,
			Points:         points,
		})
		if err != nil {
			respondError(c, http.StatusBadRequest, fmt.Errorf("raft write failed: %w", err))
			return
		}
		// FSM.Apply returns the BatchResult on success so we can report
		// real per-point counts and errors instead of pretending all
		// points succeeded.
		if result, ok := resp.(*collection.BatchResult); ok && result != nil {
			respondSuccess(c, batchResultToResponse(result))
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

	respondSuccess(c, batchResultToResponse(result))
}

func (s *Server) handleGetPoint(c *gin.Context) {
	if s.consistentReadProxy(c) {
		return
	}
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
		if _, err := s.raft.Write(cluster.OpDeletePoints, cluster.DeletePointsData{
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

// V2 Points (Named Vectors)

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

// Scroll/Pagination

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

// Payload Index Configuration API

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
