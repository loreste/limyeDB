package grpc

import (
	"context"
	"io"
	"math"

	"github.com/limyedb/limyedb/pkg/auth"
	"github.com/limyedb/limyedb/pkg/collection"
	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/index/payload"
	"github.com/limyedb/limyedb/pkg/point"
	"github.com/limyedb/limyedb/pkg/storage/snapshot"
	pb "github.com/limyedb/limyedb/api/grpc/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// safeIntToInt32 safely converts int to int32 with bounds checking
func safeIntToInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}

// LimyeDBService implements the LimyeDB gRPC service
type LimyeDBService struct {
	collections *collection.Manager
	snapshots   *snapshot.Manager
	pb.UnimplementedLimyeDBServer
}

// checkPermission verifies if the current request context has adequate JWT roles.
func checkPermission(ctx context.Context, collection string, action string) bool {
	claimsRaw := ctx.Value("token_claims")
	if claimsRaw == nil {
		// No auth token configured / passed successfully (meaning auth is disabled)
		return true 
	}
	
	claims, ok := claimsRaw.(*auth.TokenClaims)
	if !ok {
		return false
	}
	
	switch action {
	case "read": return claims.CanRead(collection)
	case "write": return claims.CanWrite(collection)
	case "admin": return claims.CanAdmin(collection)
	case "global_admin": return claims.Permissions.GlobalAdmin
	}
	return false
}

// NewLimyeDBService creates a new LimyeDB service
func NewLimyeDBService(collections *collection.Manager, snapshots *snapshot.Manager) *LimyeDBService {
	return &LimyeDBService{
		collections: collections,
		snapshots:   snapshots,
	}
}

// Collection operations

func (s *LimyeDBService) CreateCollection(ctx context.Context, req *pb.CreateCollectionRequest) (*pb.CreateCollectionResponse, error) {
	if !checkPermission(ctx, req.Config.Name, "write") {
		return nil, status.Errorf(codes.PermissionDenied, "insufficient permissions to create collection")
	}

	cfg := &config.CollectionConfig{
		Name:      req.Config.Name,
		Dimension: int(req.Config.Dimension),
		Metric:    config.MetricType(req.Config.Metric),
		OnDisk:    req.Config.OnDisk,
	}

	if req.Config.Hnsw != nil {
		cfg.HNSW = config.HNSWConfig{
			M:              int(req.Config.Hnsw.M),
			EfConstruction: int(req.Config.Hnsw.EfConstruction),
			EfSearch:       int(req.Config.Hnsw.EfSearch),
		}
	}

	coll, err := s.collections.Create(cfg)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create collection: %v", err)
	}

	return &pb.CreateCollectionResponse{
		Success: true,
		Info:    collectionInfoToProto(coll.Info()),
	}, nil
}

func (s *LimyeDBService) GetCollection(ctx context.Context, req *pb.GetCollectionRequest) (*pb.GetCollectionResponse, error) {
	if !checkPermission(ctx, req.Name, "read") {
		return nil, status.Errorf(codes.PermissionDenied, "insufficient permissions to access collection")
	}

	coll, err := s.collections.Get(req.Name)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "collection not found: %v", err)
	}

	return &pb.GetCollectionResponse{
		Info: collectionInfoToProto(coll.Info()),
	}, nil
}

func (s *LimyeDBService) ListCollections(ctx context.Context, req *pb.ListCollectionsRequest) (*pb.ListCollectionsResponse, error) {
	infos := s.collections.ListInfo()
	protoInfos := make([]*pb.CollectionInfo, 0, len(infos))
	for _, info := range infos {
		if checkPermission(ctx, info.Name, "read") {
			protoInfos = append(protoInfos, collectionInfoToProto(info))
		}
	}

	return &pb.ListCollectionsResponse{
		Collections: protoInfos,
	}, nil
}

func (s *LimyeDBService) DeleteCollection(ctx context.Context, req *pb.DeleteCollectionRequest) (*pb.DeleteCollectionResponse, error) {
	if !checkPermission(ctx, req.Name, "admin") {
		return nil, status.Errorf(codes.PermissionDenied, "insufficient permissions to delete collection")
	}

	if err := s.collections.Delete(req.Name); err != nil {
		return nil, status.Errorf(codes.NotFound, "collection not found: %v", err)
	}

	return &pb.DeleteCollectionResponse{Success: true}, nil
}

// pb.Point operations

func (s *LimyeDBService) UpsertPoints(ctx context.Context, req *pb.UpsertPointsRequest) (*pb.UpsertPointsResponse, error) {
	if !checkPermission(ctx, req.Collection, "write") {
		return nil, status.Errorf(codes.PermissionDenied, "insufficient permissions to upsert points")
	}

	coll, err := s.collections.Get(req.Collection)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "collection not found: %v", err)
	}

	points := make([]*point.Point, len(req.Points))
	for i, p := range req.Points {
		points[i] = protoToPoint(p)
	}

	result, err := coll.InsertBatch(points)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to upsert points: %v", err)
	}

	errors := make([]string, len(result.Errors))
	for i, e := range result.Errors {
		errors[i] = e.Err.Error()
	}

	return &pb.UpsertPointsResponse{
		Success:  result.Failed == 0,
		Upserted: safeIntToInt32(result.Succeeded),
		Errors:   errors,
	}, nil
}

func (s *LimyeDBService) GetPoints(ctx context.Context, req *pb.GetPointsRequest) (*pb.GetPointsResponse, error) {
	if !checkPermission(ctx, req.Collection, "read") {
		return nil, status.Errorf(codes.PermissionDenied, "insufficient permissions to access collection")
	}

	coll, err := s.collections.Get(req.Collection)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "collection not found: %v", err)
	}

	points := make([]*pb.Point, 0, len(req.Ids))
	for _, id := range req.Ids {
		p, err := coll.Get(id)
		if err != nil {
			continue
		}
		points = append(points, pointToProto(p, req.WithVector, req.WithPayload))
	}

	return &pb.GetPointsResponse{Points: points}, nil
}

func (s *LimyeDBService) DeletePoints(ctx context.Context, req *pb.DeletePointsRequest) (*pb.DeletePointsResponse, error) {
	if !checkPermission(ctx, req.Collection, "write") {
		return nil, status.Errorf(codes.PermissionDenied, "insufficient permissions to delete points")
	}

	coll, err := s.collections.Get(req.Collection)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "collection not found: %v", err)
	}

	deleted := 0
	for _, id := range req.Ids {
		if err := coll.Delete(id); err == nil {
			deleted++
		}
	}

	return &pb.DeletePointsResponse{
		Success: true,
		Deleted: safeIntToInt32(deleted),
	}, nil
}

func (s *LimyeDBService) StreamUpsertPoints(stream pb.LimyeDB_StreamUpsertPointsServer) error {
	var totalUpserted int32
	var errors []string

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&pb.UpsertPointsResponse{
				Success:  len(errors) == 0,
				Upserted: totalUpserted,
				Errors:   errors,
			})
		}
		if err != nil {
			return err
		}

		coll, err := s.collections.Get(req.Collection)
		if err != nil {
			errors = append(errors, "collection not found: "+req.Collection)
			continue
		}

		for _, p := range req.Points {
			pt := protoToPoint(p)
			if err := coll.Insert(pt); err != nil {
				errors = append(errors, err.Error())
			} else {
				totalUpserted++
			}
		}
	}
}

// Search operations

func (s *LimyeDBService) Search(ctx context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
	if !checkPermission(ctx, req.Collection, "read") {
		return nil, status.Errorf(codes.PermissionDenied, "insufficient permissions to search collection")
	}

	coll, err := s.collections.Get(req.Collection)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "collection not found: %v", err)
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 10
	}

	params := &collection.SearchParams{
		K:           limit,
		Ef:          int(req.Ef),
		WithVector:  req.WithVector,
		WithPayload: req.WithPayload,
	}

	// Convert gRPC filter to payload filter
	if req.Filter != nil {
		params.Filter = protoFilterToPayload(req.Filter)
	}

	result, err := coll.SearchV2WithParams(req.Vector.Data, req.VectorName, params)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "search failed: %v", err)
	}

	scoredPoints := make([]*pb.ScoredPoint, len(result.Points))
	for i, sp := range result.Points {
		scoredPoints[i] = &pb.ScoredPoint{
			Id:    sp.ID,
			Score: sp.Score,
		}
		if req.WithVector {
			var vData []float32
			if req.VectorName != "" && req.VectorName != "default" {
				if vec, ok := sp.Vectors[req.VectorName]; ok {
					vData = vec
				}
			} else {
				vData = sp.Vector
			}
			scoredPoints[i].Vector = &pb.Vector{Data: vData}
		}
		if req.WithPayload {
			scoredPoints[i].Payload = payloadToProto(sp.Payload)
		}
	}

	return &pb.SearchResponse{
		Results: scoredPoints,
		TookMs:  result.TookMs,
	}, nil
}

func (s *LimyeDBService) SearchBatch(ctx context.Context, req *pb.SearchBatchRequest) (*pb.SearchBatchResponse, error) {
	if !checkPermission(ctx, req.Collection, "read") {
		return nil, status.Errorf(codes.PermissionDenied, "insufficient permissions to search collection")
	}

	coll, err := s.collections.Get(req.Collection)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "collection not found: %v", err)
	}

	// Convert filter once for all searches
	var filter *payload.Filter
	if req.Filter != nil {
		filter = protoFilterToPayload(req.Filter)
	}

	results := make([]*pb.SearchResponse, len(req.Vectors))
	for i, v := range req.Vectors {
		limit := int(req.Limit)
		if limit <= 0 {
			limit = 10
		}

		params := &collection.SearchParams{
			K:           limit,
			WithVector:  req.WithVector,
			WithPayload: req.WithPayload,
			Filter:      filter,
		}

		result, err := coll.SearchV2WithParams(v.Data, req.VectorName, params)
		if err != nil {
			continue
		}

		scoredPoints := make([]*pb.ScoredPoint, len(result.Points))
		for j, sp := range result.Points {
			scoredPoints[j] = &pb.ScoredPoint{
				Id:    sp.ID,
				Score: sp.Score,
			}
			if req.WithVector {
				var vData []float32
				if req.VectorName != "" && req.VectorName != "default" {
					if vec, ok := sp.Vectors[req.VectorName]; ok {
						vData = vec
					}
				} else {
					vData = sp.Vector
				}
				scoredPoints[j].Vector = &pb.Vector{Data: vData}
			}
			if req.WithPayload {
				scoredPoints[j].Payload = payloadToProto(sp.Payload)
			}
		}

		results[i] = &pb.SearchResponse{
			Results: scoredPoints,
			TookMs:  result.TookMs,
		}
	}

	return &pb.SearchBatchResponse{Results: results}, nil
}

func (s *LimyeDBService) Recommend(ctx context.Context, req *pb.RecommendRequest) (*pb.RecommendResponse, error) {
	if !checkPermission(ctx, req.Collection, "read") {
		return nil, status.Errorf(codes.PermissionDenied, "insufficient permissions to access collection")
	}

	coll, err := s.collections.Get(req.Collection)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "collection not found: %v", err)
	}

	if len(req.PositiveIds) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "at least one positive ID required")
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 10
	}

	result, err := coll.Recommend(req.PositiveIds[0], limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "recommend failed: %v", err)
	}

	scoredPoints := make([]*pb.ScoredPoint, len(result.Points))
	for i, sp := range result.Points {
		scoredPoints[i] = &pb.ScoredPoint{
			Id:    sp.ID,
			Score: sp.Score,
		}
	}

	return &pb.RecommendResponse{
		Results: scoredPoints,
		TookMs:  result.TookMs,
	}, nil
}

func (s *LimyeDBService) Discover(ctx context.Context, req *pb.DiscoverRequest) (*pb.DiscoverResponse, error) {
	if !checkPermission(ctx, req.Collection, "read") {
		return nil, status.Errorf(codes.PermissionDenied, "insufficient permissions to access collection")
	}

	coll, err := s.collections.Get(req.Collection)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "collection not found: %v", err)
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 10
	}

	// Prepare DiscParams for Collection
	discParams := &collection.DiscoveryParams{
		K:           limit,
		WithVector:  req.WithVector,
		WithPayload: req.WithPayload,
		Context:     &collection.DiscoveryContext{},
	}

	if req.Target != nil {
		discParams.Target = req.Target.Data
	}

	for _, pair := range req.Context {
		discParams.Context.Positive = append(discParams.Context.Positive, collection.ContextExample{ID: pair.PositiveId})
		discParams.Context.Negative = append(discParams.Context.Negative, collection.ContextExample{ID: pair.NegativeId})
	}

	if req.Filter != nil {
		discParams.Filter = protoFilterToPayload(req.Filter)
	}

	result, err := coll.Discover(discParams)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "discover failed: %v", err)
	}

	scoredPoints := make([]*pb.ScoredPoint, len(result.Points))
	for i, sp := range result.Points {
		scoredPoints[i] = &pb.ScoredPoint{
			Id:    sp.ID,
			Score: sp.Score,
		}
		if req.WithVector {
			scoredPoints[i].Vector = &pb.Vector{Data: sp.Vector}
		}
		if req.WithPayload {
			scoredPoints[i].Payload = payloadToProto(sp.Payload)
		}
	}

	return &pb.DiscoverResponse{
		Results: scoredPoints,
		TookMs:  result.TookMs,
	}, nil
}

// Snapshot operations

func (s *LimyeDBService) CreateSnapshot(ctx context.Context, req *pb.CreateSnapshotRequest) (*pb.CreateSnapshotResponse, error) {
	if s.snapshots == nil {
		return nil, status.Errorf(codes.Unavailable, "snapshots not configured")
	}

	snap, err := s.collections.CreateSnapshot(s.snapshots)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create snapshot: %v", err)
	}

	return &pb.CreateSnapshotResponse{
		Id:        snap.ID,
		Timestamp: snap.Timestamp.Unix(),
		Size:      snap.Size,
	}, nil
}

func (s *LimyeDBService) ListSnapshots(ctx context.Context, req *pb.ListSnapshotsRequest) (*pb.ListSnapshotsResponse, error) {
	if s.snapshots == nil {
		return nil, status.Errorf(codes.Unavailable, "snapshots not configured")
	}

	snaps, err := s.snapshots.List()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list snapshots: %v", err)
	}

	infos := make([]*pb.SnapshotInfo, len(snaps))
	for i, snap := range snaps {
		infos[i] = &pb.SnapshotInfo{
			Id:          snap.ID,
			Timestamp:   snap.Timestamp.Unix(),
			Size:        snap.Size,
			Collections: snap.Collections,
		}
	}

	return &pb.ListSnapshotsResponse{Snapshots: infos}, nil
}

func (s *LimyeDBService) RestoreSnapshot(ctx context.Context, req *pb.RestoreSnapshotRequest) (*pb.RestoreSnapshotResponse, error) {
	if s.snapshots == nil {
		return nil, status.Errorf(codes.Unavailable, "snapshots not configured")
	}

	if err := s.collections.RestoreSnapshot(s.snapshots, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to restore snapshot: %v", err)
	}

	return &pb.RestoreSnapshotResponse{Success: true}, nil
}

// Health

func (s *LimyeDBService) Health(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{
		Status:  "healthy",
		Version: "0.1.0",
	}, nil
}

// Helper functions

func collectionInfoToProto(info *collection.Info) *pb.CollectionInfo {
	return &pb.CollectionInfo{
		Name:      info.Name,
		Dimension: safeIntToInt32(info.Dimension),
		Metric:    info.Metric,
		Size:      info.Size,
		CreatedAt: info.CreatedAt.Unix(),
		UpdatedAt: info.UpdatedAt.Unix(),
	}
}

func protoToPoint(p *pb.Point) *point.Point {
	var vector point.Vector
	if p.Vector != nil {
		vector = p.Vector.Data
	}

	var payload map[string]interface{}
	if p.Payload != nil {
		payload = protoPayloadToMap(p.Payload)
	}

	return point.NewPointWithID(p.Id, vector, payload)
}

func pointToProto(p *point.Point, withVector, withPayload bool) *pb.Point {
	proto := &pb.Point{Id: p.ID}

	if withVector {
		proto.Vector = &pb.Vector{Data: p.Vector}
	}

	if withPayload {
		proto.Payload = payloadToProto(p.Payload)
	}

	return proto
}

func payloadToProto(payload map[string]interface{}) map[string]*pb.Value {
	if payload == nil {
		return nil
	}

	result := make(map[string]*pb.Value)
	for k, v := range payload {
		result[k] = valueToProto(v)
	}
	return result
}

func valueToProto(v interface{}) *pb.Value {
	switch val := v.(type) {
	case float64:
		return &pb.Value{Kind: &pb.Value_NumberValue{NumberValue: val}}
	case string:
		return &pb.Value{Kind: &pb.Value_StringValue{StringValue: val}}
	case bool:
		return &pb.Value{Kind: &pb.Value_BoolValue{BoolValue: val}}
	default:
		return &pb.Value{Kind: &pb.Value_StringValue{StringValue: ""}}
	}
}

func protoPayloadToMap(payload map[string]*pb.Value) map[string]interface{} {
	if payload == nil {
		return nil
	}

	result := make(map[string]interface{})
	for k, v := range payload {
		result[k] = protoValueToInterface(v)
	}
	return result
}

func protoValueToInterface(v *pb.Value) interface{} {
	if v == nil {
		return nil
	}

	switch kind := v.Kind.(type) {
	case *pb.Value_NumberValue:
		return kind.NumberValue
	case *pb.Value_StringValue:
		return kind.StringValue
	case *pb.Value_BoolValue:
		return kind.BoolValue
	default:
		return nil
	}
}

// protoFilterToPayload converts a gRPC Filter to a payload.Filter
func protoFilterToPayload(f *pb.Filter) *payload.Filter {
	if f == nil || f.Filter == nil {
		return nil
	}

	switch fv := f.Filter.(type) {
	case *pb.Filter_Must:
		var filters []*payload.Filter
		for _, cond := range fv.Must.Conditions {
			if pf := protoFilterToPayload(cond); pf != nil {
				filters = append(filters, pf)
			}
		}
		if len(filters) == 0 {
			return nil
		}
		if len(filters) == 1 {
			return filters[0]
		}
		return payload.And(filters...)

	case *pb.Filter_Should:
		var filters []*payload.Filter
		for _, cond := range fv.Should.Conditions {
			if pf := protoFilterToPayload(cond); pf != nil {
				filters = append(filters, pf)
			}
		}
		if len(filters) == 0 {
			return nil
		}
		if len(filters) == 1 {
			return filters[0]
		}
		return payload.Or(filters...)

	case *pb.Filter_MustNot:
		var filters []*payload.Filter
		for _, cond := range fv.MustNot.Conditions {
			if pf := protoFilterToPayload(cond); pf != nil {
				filters = append(filters, payload.Not(pf))
			}
		}
		if len(filters) == 0 {
			return nil
		}
		if len(filters) == 1 {
			return filters[0]
		}
		return payload.And(filters...)

	case *pb.Filter_Field:
		return conditionToPayloadFilter(fv.Field)
	}
	return nil
}

// conditionToPayloadFilter converts a single gRPC FieldCondition to a payload.Filter
func conditionToPayloadFilter(cond *pb.FieldCondition) *payload.Filter {
	if cond == nil || cond.Field == "" {
		return nil
	}

	switch cv := cond.Condition.(type) {
	case *pb.FieldCondition_Match:
		if cv.Match.Value != nil {
			return payload.Field(cond.Field, payload.Eq(protoValueToInterface(cv.Match.Value)))
		}
	case *pb.FieldCondition_Range:
		var rangeFilters []*payload.Filter
		if cv.Range.Gt != nil {
			rangeFilters = append(rangeFilters, payload.Field(cond.Field, payload.Gt(*cv.Range.Gt)))
		}
		if cv.Range.Gte != nil {
			rangeFilters = append(rangeFilters, payload.Field(cond.Field, payload.Gte(*cv.Range.Gte)))
		}
		if cv.Range.Lt != nil {
			rangeFilters = append(rangeFilters, payload.Field(cond.Field, payload.Lt(*cv.Range.Lt)))
		}
		if cv.Range.Lte != nil {
			rangeFilters = append(rangeFilters, payload.Field(cond.Field, payload.Lte(*cv.Range.Lte)))
		}
		if len(rangeFilters) == 1 {
			return rangeFilters[0]
		}
		if len(rangeFilters) > 1 {
			return payload.And(rangeFilters...)
		}
	case *pb.FieldCondition_Values:
		if len(cv.Values.Values) > 0 {
			var vals []interface{}
			for _, v := range cv.Values.Values {
				vals = append(vals, protoValueToInterface(v))
			}
			return payload.Field(cond.Field, payload.In(vals...))
		}
	}
	return nil
}
