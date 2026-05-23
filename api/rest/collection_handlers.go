package rest

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/limyedb/limyedb/pkg/cluster"
	"github.com/limyedb/limyedb/pkg/collection"
	"github.com/limyedb/limyedb/pkg/config"
)

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
		if _, err := s.raft.Write(cluster.OpCreateCollection, cluster.CreateCollectionData{Config: cfg}); err != nil {
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
		if errors.Is(err, collection.ErrCollectionExists) {
			respondStructuredError(c, http.StatusConflict, "ALREADY_EXISTS", "collection already exists: "+cfg.Name)
		} else {
			respondStructuredError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		}
		return
	}

	respondCreated(c, coll.Info())
}

func (s *Server) handleListCollections(c *gin.Context) {
	if s.consistentReadProxy(c) {
		return
	}
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
	if s.consistentReadProxy(c) {
		return
	}
	name := c.Param("name")

	coll, err := s.collections.Get(name)
	if err != nil {
		respondStructuredError(c, http.StatusNotFound, "NOT_FOUND", "collection not found: "+name)
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
		if _, err := s.raft.Write(cluster.OpDeleteCollection, cluster.DeleteCollectionData{Name: name}); err != nil {
			respondError(c, http.StatusBadRequest, fmt.Errorf("raft write failed: %w", err))
			return
		}
		respondSuccess(c, gin.H{"deleted": name})
		return
	}

	if err := s.collections.Delete(name); err != nil {
		respondStructuredError(c, http.StatusNotFound, "NOT_FOUND", "collection not found: "+name)
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

	// A concurrent Delete between UpdateConfig and Get returns nil; the
	// previous code dereferenced it and crashed the request goroutine.
	coll, err := s.collections.Get(name)
	if err != nil || coll == nil {
		respondStructuredError(c, http.StatusNotFound, "NOT_FOUND", "collection deleted concurrently")
		return
	}
	respondSuccess(c, coll.Info())
}

// CreateCollectionV2Request supports named vectors
type CreateCollectionV2Request struct {
	Name    string                       `json:"name" binding:"required"`
	Vectors map[string]VectorConfigInput `json:"vectors"`
	OnDisk  bool                         `json:"on_disk"`
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

// Alias handlers

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
