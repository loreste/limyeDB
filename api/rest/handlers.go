package rest

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/hashicorp/raft"

	"github.com/limyedb/limyedb/pkg/collection"
	"github.com/limyedb/limyedb/pkg/index/payload"
)

// consistentReadProxy honors the ?consistent=true query param on read
// endpoints by routing the request to the Raft leader. The default,
// without the flag, is the existing eventually-consistent local-FSM read
// behavior. Routing to the leader gives read-after-write semantics for
// the calling client, because Raft.Apply runs on the leader before the
// preceding write returned success — so a mutation that succeeded is
// guaranteed to be visible on the leader's local state.
//
// Returns true when the request was proxied (handler must return).
// Returns false when the request can be served locally (we're already
// the leader, or the flag was not set, or this server has no Raft).
func (s *Server) consistentReadProxy(c *gin.Context) bool {
	if c.Query("consistent") != "true" {
		return false
	}
	return s.proxyToLeader(c)
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

// batchResultToResponse formats a BatchResult into the upsert response
// shape. The fields are additive vs. the historical {succeeded, failed}
// payload; old clients that only read those two keys keep working.
func batchResultToResponse(r *collection.BatchResult) gin.H {
	out := gin.H{
		"succeeded": r.Succeeded,
		"failed":    r.Failed,
	}
	if len(r.Errors) > 0 {
		errs := make([]gin.H, len(r.Errors))
		for i, e := range r.Errors {
			errs[i] = gin.H{"id": e.ID, "error": e.Err.Error()}
		}
		out["errors"] = errs
	}
	return out
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

// handleWebSocket transparently passes the Gin HTTP structures into the pure Real-Time WebSocket hub upgrader.
func (s *Server) handleWebSocket(c *gin.Context) {
	s.realtimeHub.ServeWS(c.Writer, c.Request)
}
