package rest

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/limyedb/limyedb/pkg/version"
)

// Health handlers

func (s *Server) handleHealth(c *gin.Context) {
	uptime := time.Since(s.startTime).Truncate(time.Second).String()

	collectionCount := s.collections.Count()
	collectionStatus := "healthy"
	if collectionCount < 0 {
		collectionStatus = "degraded"
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"version": version.Version,
		"uptime":  uptime,
		"components": gin.H{
			"storage": "healthy",
			"collections": gin.H{
				"count":  collectionCount,
				"status": collectionStatus,
			},
		},
	})
}

func (s *Server) handleReadiness(c *gin.Context) {
	if s.collections == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"ready":  false,
			"reason": "collection manager not initialized",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ready": true,
	})
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
