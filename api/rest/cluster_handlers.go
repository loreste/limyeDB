package rest

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/limyedb/limyedb/pkg/cdc"
)

// Cluster handlers

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

// Change Data Capture API

func (s *Server) handleCreateWebhook(c *gin.Context) {
	var req cdc.WebhookSubscription
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	collectionName := c.Param("name")
	if err := cdc.GetDispatcher().Subscribe(collectionName, req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	respondSuccess(c, gin.H{
		"message":    "webhook subscribed successfully",
		"collection": collectionName,
	})
}
