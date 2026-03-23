package grpc

import (
	"context"
	"io"
	"math"

	"github.com/limyedb/limyedb/pkg/collection"
	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/index/payload"
	"github.com/limyedb/limyedb/pkg/point"
	"github.com/limyedb/limyedb/pkg/storage/snapshot"
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
	UnimplementedLimyeDBServer
}

// UnimplementedLimyeDBServer provides default implementations
type UnimplementedLimyeDBServer struct{}

func (UnimplementedLimyeDBServer) CreateCollection(context.Context, *CreateCollectionRequest) (*CreateCollectionResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateCollection not implemented")
}
func (UnimplementedLimyeDBServer) GetCollection(context.Context, *GetCollectionRequest) (*GetCollectionResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetCollection not implemented")
}
func (UnimplementedLimyeDBServer) ListCollections(context.Context, *ListCollectionsRequest) (*ListCollectionsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListCollections not implemented")
}
func (UnimplementedLimyeDBServer) DeleteCollection(context.Context, *DeleteCollectionRequest) (*DeleteCollectionResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeleteCollection not implemented")
}
func (UnimplementedLimyeDBServer) UpsertPoints(context.Context, *UpsertPointsRequest) (*UpsertPointsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UpsertPoints not implemented")
}
func (UnimplementedLimyeDBServer) GetPoints(context.Context, *GetPointsRequest) (*GetPointsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetPoints not implemented")
}
func (UnimplementedLimyeDBServer) DeletePoints(context.Context, *DeletePointsRequest) (*DeletePointsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeletePoints not implemented")
}
func (UnimplementedLimyeDBServer) StreamUpsertPoints(LimyeDB_StreamUpsertPointsServer) error {
	return status.Errorf(codes.Unimplemented, "method StreamUpsertPoints not implemented")
}
func (UnimplementedLimyeDBServer) Search(context.Context, *SearchRequest) (*SearchResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Search not implemented")
}
func (UnimplementedLimyeDBServer) SearchBatch(context.Context, *SearchBatchRequest) (*SearchBatchResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SearchBatch not implemented")
}
func (UnimplementedLimyeDBServer) Recommend(context.Context, *RecommendRequest) (*RecommendResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Recommend not implemented")
}
func (UnimplementedLimyeDBServer) Discover(context.Context, *DiscoverRequest) (*DiscoverResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Discover not implemented")
}
func (UnimplementedLimyeDBServer) CreateSnapshot(context.Context, *CreateSnapshotRequest) (*CreateSnapshotResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateSnapshot not implemented")
}
func (UnimplementedLimyeDBServer) ListSnapshots(context.Context, *ListSnapshotsRequest) (*ListSnapshotsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListSnapshots not implemented")
}
func (UnimplementedLimyeDBServer) RestoreSnapshot(context.Context, *RestoreSnapshotRequest) (*RestoreSnapshotResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RestoreSnapshot not implemented")
}
func (UnimplementedLimyeDBServer) Health(context.Context, *HealthRequest) (*HealthResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Health not implemented")
}
func (UnimplementedLimyeDBServer) mustEmbedUnimplementedLimyeDBServer() {}

// NewLimyeDBService creates a new LimyeDB service
func NewLimyeDBService(collections *collection.Manager, snapshots *snapshot.Manager) *LimyeDBService {
	return &LimyeDBService{
		collections: collections,
		snapshots:   snapshots,
	}
}

// Collection operations

func (s *LimyeDBService) CreateCollection(ctx context.Context, req *CreateCollectionRequest) (*CreateCollectionResponse, error) {
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

	return &CreateCollectionResponse{
		Success: true,
		Info:    collectionInfoToProto(coll.Info()),
	}, nil
}

func (s *LimyeDBService) GetCollection(ctx context.Context, req *GetCollectionRequest) (*GetCollectionResponse, error) {
	coll, err := s.collections.Get(req.Name)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "collection not found: %v", err)
	}

	return &GetCollectionResponse{
		Info: collectionInfoToProto(coll.Info()),
	}, nil
}

func (s *LimyeDBService) ListCollections(ctx context.Context, req *ListCollectionsRequest) (*ListCollectionsResponse, error) {
	infos := s.collections.ListInfo()
	protoInfos := make([]*CollectionInfo, len(infos))
	for i, info := range infos {
		protoInfos[i] = collectionInfoToProto(info)
	}

	return &ListCollectionsResponse{
		Collections: protoInfos,
	}, nil
}

func (s *LimyeDBService) DeleteCollection(ctx context.Context, req *DeleteCollectionRequest) (*DeleteCollectionResponse, error) {
	if err := s.collections.Delete(req.Name); err != nil {
		return nil, status.Errorf(codes.NotFound, "collection not found: %v", err)
	}

	return &DeleteCollectionResponse{Success: true}, nil
}

// Point operations

func (s *LimyeDBService) UpsertPoints(ctx context.Context, req *UpsertPointsRequest) (*UpsertPointsResponse, error) {
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

	return &UpsertPointsResponse{
		Success:  result.Failed == 0,
		Upserted: safeIntToInt32(result.Succeeded),
		Errors:   errors,
	}, nil
}

func (s *LimyeDBService) GetPoints(ctx context.Context, req *GetPointsRequest) (*GetPointsResponse, error) {
	coll, err := s.collections.Get(req.Collection)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "collection not found: %v", err)
	}

	points := make([]*Point, 0, len(req.Ids))
	for _, id := range req.Ids {
		p, err := coll.Get(id)
		if err != nil {
			continue
		}
		points = append(points, pointToProto(p, req.WithVector, req.WithPayload))
	}

	return &GetPointsResponse{Points: points}, nil
}

func (s *LimyeDBService) DeletePoints(ctx context.Context, req *DeletePointsRequest) (*DeletePointsResponse, error) {
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

	return &DeletePointsResponse{
		Success: true,
		Deleted: safeIntToInt32(deleted),
	}, nil
}

func (s *LimyeDBService) StreamUpsertPoints(stream LimyeDB_StreamUpsertPointsServer) error {
	var totalUpserted int32
	var errors []string

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&UpsertPointsResponse{
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

func (s *LimyeDBService) Search(ctx context.Context, req *SearchRequest) (*SearchResponse, error) {
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

	result, err := coll.SearchWithParams(req.Vector.Data, params)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "search failed: %v", err)
	}

	scoredPoints := make([]*ScoredPoint, len(result.Points))
	for i, sp := range result.Points {
		scoredPoints[i] = &ScoredPoint{
			Id:    sp.ID,
			Score: sp.Score,
		}
		if req.WithVector {
			scoredPoints[i].Vector = &Vector{Data: sp.Vector}
		}
		if req.WithPayload {
			scoredPoints[i].Payload = payloadToProto(sp.Payload)
		}
	}

	return &SearchResponse{
		Results: scoredPoints,
		TookMs:  result.TookMs,
	}, nil
}

func (s *LimyeDBService) SearchBatch(ctx context.Context, req *SearchBatchRequest) (*SearchBatchResponse, error) {
	coll, err := s.collections.Get(req.Collection)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "collection not found: %v", err)
	}

	// Convert filter once for all searches
	var filter *payload.Filter
	if req.Filter != nil {
		filter = protoFilterToPayload(req.Filter)
	}

	results := make([]*SearchResponse, len(req.Vectors))
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

		result, err := coll.SearchWithParams(v.Data, params)
		if err != nil {
			continue
		}

		scoredPoints := make([]*ScoredPoint, len(result.Points))
		for j, sp := range result.Points {
			scoredPoints[j] = &ScoredPoint{
				Id:    sp.ID,
				Score: sp.Score,
			}
		}

		results[i] = &SearchResponse{
			Results: scoredPoints,
			TookMs:  result.TookMs,
		}
	}

	return &SearchBatchResponse{Results: results}, nil
}

func (s *LimyeDBService) Recommend(ctx context.Context, req *RecommendRequest) (*RecommendResponse, error) {
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

	scoredPoints := make([]*ScoredPoint, len(result.Points))
	for i, sp := range result.Points {
		scoredPoints[i] = &ScoredPoint{
			Id:    sp.ID,
			Score: sp.Score,
		}
	}

	return &RecommendResponse{
		Results: scoredPoints,
		TookMs:  result.TookMs,
	}, nil
}

func (s *LimyeDBService) Discover(ctx context.Context, req *DiscoverRequest) (*DiscoverResponse, error) {
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

	scoredPoints := make([]*ScoredPoint, len(result.Points))
	for i, sp := range result.Points {
		scoredPoints[i] = &ScoredPoint{
			Id:    sp.ID,
			Score: sp.Score,
		}
		if req.WithVector {
			scoredPoints[i].Vector = &Vector{Data: sp.Vector}
		}
		if req.WithPayload {
			scoredPoints[i].Payload = payloadToProto(sp.Payload)
		}
	}

	return &DiscoverResponse{
		Results: scoredPoints,
		TookMs:  result.TookMs,
	}, nil
}

// Snapshot operations

func (s *LimyeDBService) CreateSnapshot(ctx context.Context, req *CreateSnapshotRequest) (*CreateSnapshotResponse, error) {
	if s.snapshots == nil {
		return nil, status.Errorf(codes.Unavailable, "snapshots not configured")
	}

	snap, err := s.collections.CreateSnapshot(s.snapshots)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create snapshot: %v", err)
	}

	return &CreateSnapshotResponse{
		Id:        snap.ID,
		Timestamp: snap.Timestamp.Unix(),
		Size:      snap.Size,
	}, nil
}

func (s *LimyeDBService) ListSnapshots(ctx context.Context, req *ListSnapshotsRequest) (*ListSnapshotsResponse, error) {
	if s.snapshots == nil {
		return nil, status.Errorf(codes.Unavailable, "snapshots not configured")
	}

	snaps, err := s.snapshots.List()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list snapshots: %v", err)
	}

	infos := make([]*SnapshotInfo, len(snaps))
	for i, snap := range snaps {
		infos[i] = &SnapshotInfo{
			Id:          snap.ID,
			Timestamp:   snap.Timestamp.Unix(),
			Size:        snap.Size,
			Collections: snap.Collections,
		}
	}

	return &ListSnapshotsResponse{Snapshots: infos}, nil
}

func (s *LimyeDBService) RestoreSnapshot(ctx context.Context, req *RestoreSnapshotRequest) (*RestoreSnapshotResponse, error) {
	if s.snapshots == nil {
		return nil, status.Errorf(codes.Unavailable, "snapshots not configured")
	}

	if err := s.collections.RestoreSnapshot(s.snapshots, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to restore snapshot: %v", err)
	}

	return &RestoreSnapshotResponse{Success: true}, nil
}

// Health

func (s *LimyeDBService) Health(ctx context.Context, req *HealthRequest) (*HealthResponse, error) {
	return &HealthResponse{
		Status:  "healthy",
		Version: "0.1.0",
	}, nil
}

// Helper functions

func collectionInfoToProto(info *collection.Info) *CollectionInfo {
	return &CollectionInfo{
		Name:      info.Name,
		Dimension: safeIntToInt32(info.Dimension),
		Metric:    info.Metric,
		Size:      info.Size,
		CreatedAt: info.CreatedAt.Unix(),
		UpdatedAt: info.UpdatedAt.Unix(),
	}
}

func protoToPoint(p *Point) *point.Point {
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

func pointToProto(p *point.Point, withVector, withPayload bool) *Point {
	proto := &Point{Id: p.ID}

	if withVector {
		proto.Vector = &Vector{Data: p.Vector}
	}

	if withPayload {
		proto.Payload = payloadToProto(p.Payload)
	}

	return proto
}

func payloadToProto(payload map[string]interface{}) map[string]*Value {
	if payload == nil {
		return nil
	}

	result := make(map[string]*Value)
	for k, v := range payload {
		result[k] = valueToProto(v)
	}
	return result
}

func valueToProto(v interface{}) *Value {
	switch val := v.(type) {
	case float64:
		return &Value{Kind: &Value_NumberValue{NumberValue: val}}
	case string:
		return &Value{Kind: &Value_StringValue{StringValue: val}}
	case bool:
		return &Value{Kind: &Value_BoolValue{BoolValue: val}}
	default:
		return &Value{Kind: &Value_StringValue{StringValue: ""}}
	}
}

func protoPayloadToMap(payload map[string]*Value) map[string]interface{} {
	if payload == nil {
		return nil
	}

	result := make(map[string]interface{})
	for k, v := range payload {
		result[k] = protoValueToInterface(v)
	}
	return result
}

func protoValueToInterface(v *Value) interface{} {
	if v == nil {
		return nil
	}

	switch kind := v.Kind.(type) {
	case *Value_NumberValue:
		return kind.NumberValue
	case *Value_StringValue:
		return kind.StringValue
	case *Value_BoolValue:
		return kind.BoolValue
	default:
		return nil
	}
}

// protoFilterToPayload converts a gRPC Filter to a payload.Filter
func protoFilterToPayload(f *Filter) *payload.Filter {
	if f == nil {
		return nil
	}

	var filters []*payload.Filter

	// Process Must conditions (AND)
	for _, cond := range f.Must {
		if pf := conditionToPayloadFilter(cond); pf != nil {
			filters = append(filters, pf)
		}
	}

	// Process MustNot conditions (NOT)
	for _, cond := range f.MustNot {
		if pf := conditionToPayloadFilter(cond); pf != nil {
			filters = append(filters, payload.Not(pf))
		}
	}

	// Process Should conditions (OR)
	var shouldFilters []*payload.Filter
	for _, cond := range f.Should {
		if pf := conditionToPayloadFilter(cond); pf != nil {
			shouldFilters = append(shouldFilters, pf)
		}
	}

	if len(shouldFilters) > 0 {
		filters = append(filters, payload.Or(shouldFilters...))
	}

	if len(filters) == 0 {
		return nil
	}
	if len(filters) == 1 {
		return filters[0]
	}
	return payload.And(filters...)
}

// conditionToPayloadFilter converts a single gRPC Condition to a payload.Filter
func conditionToPayloadFilter(cond *Condition) *payload.Filter {
	if cond == nil || cond.Field == "" {
		return nil
	}

	// Handle Match conditions
	if cond.Match != nil {
		if len(cond.Match.Values) > 0 {
			return payload.Field(cond.Field, payload.In(cond.Match.Values...))
		}
		if cond.Match.Value != nil {
			return payload.Field(cond.Field, payload.Eq(cond.Match.Value))
		}
	}

	// Handle Range conditions
	if cond.Range != nil {
		var rangeFilters []*payload.Filter

		if cond.Range.Gt != nil {
			rangeFilters = append(rangeFilters, payload.Field(cond.Field, payload.Gt(cond.Range.Gt)))
		}
		if cond.Range.Gte != nil {
			rangeFilters = append(rangeFilters, payload.Field(cond.Field, payload.Gte(cond.Range.Gte)))
		}
		if cond.Range.Lt != nil {
			rangeFilters = append(rangeFilters, payload.Field(cond.Field, payload.Lt(cond.Range.Lt)))
		}
		if cond.Range.Lte != nil {
			rangeFilters = append(rangeFilters, payload.Field(cond.Field, payload.Lte(cond.Range.Lte)))
		}

		if len(rangeFilters) == 1 {
			return rangeFilters[0]
		}
		if len(rangeFilters) > 1 {
			return payload.And(rangeFilters...)
		}
	}

	return nil
}

// Message types (these would normally be generated by protoc)

type CreateCollectionRequest struct {
	Config *CollectionConfig
}

type CreateCollectionResponse struct {
	Success bool
	Info    *CollectionInfo
}

type GetCollectionRequest struct {
	Name string
}

type GetCollectionResponse struct {
	Info *CollectionInfo
}

type ListCollectionsRequest struct{}

type ListCollectionsResponse struct {
	Collections []*CollectionInfo
}

type DeleteCollectionRequest struct {
	Name string
}

type DeleteCollectionResponse struct {
	Success bool
}

type UpsertPointsRequest struct {
	Collection string
	Points     []*Point
	Wait       bool
}

type UpsertPointsResponse struct {
	Success  bool
	Upserted int32
	Errors   []string
}

type GetPointsRequest struct {
	Collection  string
	Ids         []string
	WithVector  bool
	WithPayload bool
}

type GetPointsResponse struct {
	Points []*Point
}

type DeletePointsRequest struct {
	Collection string
	Ids        []string
	Filter     *Filter
}

type DeletePointsResponse struct {
	Success bool
	Deleted int32
}

type SearchRequest struct {
	Collection     string
	Vector         *Vector
	Limit          int32
	Offset         int32
	Filter         *Filter
	WithVector     bool
	WithPayload    bool
	Ef             int32
	ScoreThreshold float32
}

type SearchResponse struct {
	Results []*ScoredPoint
	TookMs  int64
}

type SearchBatchRequest struct {
	Collection  string
	Vectors     []*Vector
	Limit       int32
	Filter      *Filter
	WithVector  bool
	WithPayload bool
}

type SearchBatchResponse struct {
	Results []*SearchResponse
}

type RecommendRequest struct {
	Collection  string
	PositiveIds []string
	NegativeIds []string
	Limit       int32
	Filter      *Filter
	WithVector  bool
	WithPayload bool
}

type RecommendResponse struct {
	Results []*ScoredPoint
	TookMs  int64
}

type ContextPair struct {
	PositiveId string
	NegativeId string
}

type DiscoverRequest struct {
	Collection  string
	Target      *Vector
	Context     []*ContextPair
	Limit       int32
	Offset      int32
	Filter      *Filter
	WithVector  bool
	WithPayload bool
}

type DiscoverResponse struct {
	Results []*ScoredPoint
	TookMs  int64
}

type CreateSnapshotRequest struct {
	Collections []string
}

type CreateSnapshotResponse struct {
	Id        string
	Timestamp int64
	Size      int64
}

type ListSnapshotsRequest struct{}

type ListSnapshotsResponse struct {
	Snapshots []*SnapshotInfo
}

type RestoreSnapshotRequest struct {
	Id string
}

type RestoreSnapshotResponse struct {
	Success bool
}

type HealthRequest struct{}

type HealthResponse struct {
	Status  string
	Version string
}

// Proto message types

type Vector struct {
	Data []float32
}

type Point struct {
	Id      string
	Vector  *Vector
	Payload map[string]*Value
}

type Value struct {
	Kind isValue_Kind
}

type isValue_Kind interface {
	isValue_Kind()
}

type Value_NumberValue struct {
	NumberValue float64
}

type Value_StringValue struct {
	StringValue string
}

type Value_BoolValue struct {
	BoolValue bool
}

func (*Value_NumberValue) isValue_Kind() {}
func (*Value_StringValue) isValue_Kind() {}
func (*Value_BoolValue) isValue_Kind()   {}

type ScoredPoint struct {
	Id      string
	Score   float32
	Vector  *Vector
	Payload map[string]*Value
}

type HNSWConfig struct {
	M              int32
	EfConstruction int32
	EfSearch       int32
}

type CollectionConfig struct {
	Name      string
	Dimension int32
	Metric    string
	OnDisk    bool
	Hnsw      *HNSWConfig
}

type CollectionInfo struct {
	Name      string
	Dimension int32
	Metric    string
	Size      int64
	Config    *CollectionConfig
	CreatedAt int64
	UpdatedAt int64
}

// Filter represents a filter expression for gRPC
type Filter struct {
	Must    []*Condition `json:"must,omitempty"`
	Should  []*Condition `json:"should,omitempty"`
	MustNot []*Condition `json:"must_not,omitempty"`
}

// Condition represents a field condition for gRPC
type Condition struct {
	Field string
	Match *Match
	Range *Range
}

// Match represents equality matching
type Match struct {
	Value  interface{}
	Values []interface{}
}

// Range represents range conditions
type Range struct {
	Gt  interface{}
	Gte interface{}
	Lt  interface{}
	Lte interface{}
}

type SnapshotInfo struct {
	Id          string
	Timestamp   int64
	Size        int64
	Collections []string
}
