# LimyeDB gRPC API Reference

This document provides a comprehensive reference for the LimyeDB gRPC API.

## Overview

LimyeDB provides a gRPC interface for high-performance vector operations. gRPC offers:

- **Lower latency** compared to REST
- **Streaming support** for large batch operations
- **Strong typing** via Protocol Buffers
- **Connection multiplexing** for high throughput

## Connection

### Default Port

```
localhost:50051
```

### Connection Example (Go)

```go
import (
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
    pb "github.com/limyedb/limyedb/api/grpc/proto"
)

conn, err := grpc.Dial(
    "localhost:50051",
    grpc.WithTransportCredentials(insecure.NewCredentials()),
)
if err != nil {
    log.Fatalf("Failed to connect: %v", err)
}
defer conn.Close()

client := pb.NewLimyeDBClient(conn)
```

### Connection with Authentication

```go
import (
    "google.golang.org/grpc"
    "google.golang.org/grpc/metadata"
)

// Create interceptor for authentication
authInterceptor := func(ctx context.Context, method string, req, reply interface{},
    cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
    ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+apiKey)
    return invoker(ctx, method, req, reply, cc, opts...)
}

conn, err := grpc.Dial(
    "localhost:50051",
    grpc.WithUnaryInterceptor(authInterceptor),
)
```

---

## Service Definition

```protobuf
service LimyeDB {
    // Collection operations
    rpc CreateCollection(CreateCollectionRequest) returns (CreateCollectionResponse);
    rpc GetCollection(GetCollectionRequest) returns (GetCollectionResponse);
    rpc ListCollections(ListCollectionsRequest) returns (ListCollectionsResponse);
    rpc DeleteCollection(DeleteCollectionRequest) returns (DeleteCollectionResponse);

    // Point operations
    rpc UpsertPoints(UpsertPointsRequest) returns (UpsertPointsResponse);
    rpc GetPoints(GetPointsRequest) returns (GetPointsResponse);
    rpc DeletePoints(DeletePointsRequest) returns (DeletePointsResponse);

    // Search operations
    rpc Search(SearchRequest) returns (SearchResponse);
    rpc BatchSearch(BatchSearchRequest) returns (BatchSearchResponse);
    rpc RangeSearch(RangeSearchRequest) returns (RangeSearchResponse);
    rpc Recommend(RecommendRequest) returns (RecommendResponse);

    // Streaming operations
    rpc SearchStream(SearchRequest) returns (stream SearchResult);
    rpc UpsertStream(stream UpsertPointsRequest) returns (UpsertPointsResponse);
}
```

---

## Collection Operations

### CreateCollection

Creates a new vector collection.

**Request:**
```protobuf
message CreateCollectionRequest {
    string name = 1;
    int32 dimension = 2;
    string metric = 3;  // "cosine", "euclidean", "dot_product"
    HNSWConfig hnsw_config = 4;
    bool on_disk = 5;
    map<string, VectorConfig> vectors = 6;
}

message HNSWConfig {
    int32 m = 1;
    int32 ef_construction = 2;
    int32 ef_search = 3;
    int32 max_elements = 4;
}
```

**Response:**
```protobuf
message CreateCollectionResponse {
    string name = 1;
    string status = 2;
}
```

**Example:**
```go
resp, err := client.CreateCollection(ctx, &pb.CreateCollectionRequest{
    Name:      "documents",
    Dimension: 1536,
    Metric:    "cosine",
    HnswConfig: &pb.HNSWConfig{
        M:              16,
        EfConstruction: 200,
        EfSearch:       100,
    },
})
```

### GetCollection

Retrieves collection information.

**Request:**
```protobuf
message GetCollectionRequest {
    string name = 1;
}
```

**Response:**
```protobuf
message GetCollectionResponse {
    string name = 1;
    int32 dimension = 2;
    string metric = 3;
    int64 points_count = 4;
    string status = 5;
}
```

### ListCollections

Lists all collections.

**Request:**
```protobuf
message ListCollectionsRequest {}
```

**Response:**
```protobuf
message ListCollectionsResponse {
    repeated CollectionInfo collections = 1;
}
```

### DeleteCollection

Deletes a collection.

**Request:**
```protobuf
message DeleteCollectionRequest {
    string name = 1;
}
```

**Response:**
```protobuf
message DeleteCollectionResponse {
    bool success = 1;
}
```

---

## Point Operations

### UpsertPoints

Inserts or updates points in a collection.

**Request:**
```protobuf
message UpsertPointsRequest {
    string collection_name = 1;
    repeated Point points = 2;
    bool wait = 3;
}

message Point {
    string id = 1;
    repeated float vector = 2;
    map<string, google.protobuf.Value> payload = 3;
    map<string, Vector> named_vectors = 4;
    SparseVector sparse = 5;
}

message SparseVector {
    repeated int32 indices = 1;
    repeated float values = 2;
}
```

**Response:**
```protobuf
message UpsertPointsResponse {
    int32 succeeded = 1;
    int32 failed = 2;
}
```

**Example:**
```go
resp, err := client.UpsertPoints(ctx, &pb.UpsertPointsRequest{
    CollectionName: "documents",
    Points: []*pb.Point{
        {
            Id:     "doc-1",
            Vector: []float32{0.1, 0.2, 0.3, ...},
            Payload: map[string]*structpb.Value{
                "title": structpb.NewStringValue("Hello World"),
                "count": structpb.NewNumberValue(42),
            },
        },
    },
    Wait: true,
})
```

### GetPoints

Retrieves points by IDs.

**Request:**
```protobuf
message GetPointsRequest {
    string collection_name = 1;
    repeated string ids = 2;
    bool with_vector = 3;
    bool with_payload = 4;
}
```

**Response:**
```protobuf
message GetPointsResponse {
    repeated Point points = 1;
}
```

### DeletePoints

Deletes points by IDs.

**Request:**
```protobuf
message DeletePointsRequest {
    string collection_name = 1;
    repeated string ids = 2;
}
```

**Response:**
```protobuf
message DeletePointsResponse {
    int32 deleted = 1;
}
```

---

## Search Operations

### Search

Performs k-NN search.

**Request:**
```protobuf
message SearchRequest {
    string collection_name = 1;
    repeated float vector = 2;
    int32 limit = 3;
    int32 ef = 4;
    Filter filter = 5;
    bool with_payload = 6;
    bool with_vector = 7;
    float score_threshold = 8;
    string vector_name = 9;
}

message Filter {
    repeated Condition must = 1;
    repeated Condition must_not = 2;
    repeated Condition should = 3;
}

message Condition {
    string key = 1;
    oneof condition {
        Match match = 2;
        Range range = 3;
    }
}

message Match {
    google.protobuf.Value value = 1;
}

message Range {
    optional double gt = 1;
    optional double gte = 2;
    optional double lt = 3;
    optional double lte = 4;
}
```

**Response:**
```protobuf
message SearchResponse {
    repeated SearchResult results = 1;
    int64 took_ms = 2;
}

message SearchResult {
    string id = 1;
    float score = 2;
    repeated float vector = 3;
    map<string, google.protobuf.Value> payload = 4;
}
```

**Example:**
```go
resp, err := client.Search(ctx, &pb.SearchRequest{
    CollectionName: "documents",
    Vector:         queryVector,
    Limit:          10,
    Ef:             100,
    WithPayload:    true,
    Filter: &pb.Filter{
        Must: []*pb.Condition{
            {
                Key: "category",
                Condition: &pb.Condition_Match{
                    Match: &pb.Match{
                        Value: structpb.NewStringValue("news"),
                    },
                },
            },
        },
    },
})
```

### BatchSearch

Performs batch k-NN search for multiple queries.

**Request:**
```protobuf
message BatchSearchRequest {
    string collection_name = 1;
    repeated SearchQuery queries = 2;
}

message SearchQuery {
    repeated float vector = 1;
    int32 limit = 2;
    Filter filter = 3;
    bool with_payload = 4;
}
```

**Response:**
```protobuf
message BatchSearchResponse {
    repeated SearchResponse results = 1;
}
```

### RangeSearch

Finds all points within a distance threshold.

**Request:**
```protobuf
message RangeSearchRequest {
    string collection_name = 1;
    repeated float vector = 2;
    float radius = 3;
    Filter filter = 4;
}
```

**Response:**
```protobuf
message RangeSearchResponse {
    repeated SearchResult results = 1;
}
```

### Recommend

Finds similar points to a given point.

**Request:**
```protobuf
message RecommendRequest {
    string collection_name = 1;
    string positive_id = 2;
    repeated string negative_ids = 3;
    int32 limit = 4;
    Filter filter = 5;
}
```

**Response:**
```protobuf
message RecommendResponse {
    repeated SearchResult results = 1;
}
```

---

## Streaming Operations

### SearchStream

Streams search results for low-latency progressive rendering.

```go
stream, err := client.SearchStream(ctx, &pb.SearchRequest{
    CollectionName: "documents",
    Vector:         queryVector,
    Limit:          100,
})
if err != nil {
    log.Fatal(err)
}

for {
    result, err := stream.Recv()
    if err == io.EOF {
        break
    }
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("ID: %s, Score: %.4f\n", result.Id, result.Score)
}
```

### UpsertStream

Streams points for bulk insertion.

```go
stream, err := client.UpsertStream(ctx)
if err != nil {
    log.Fatal(err)
}

for _, point := range points {
    err := stream.Send(&pb.UpsertPointsRequest{
        CollectionName: "documents",
        Points:         []*pb.Point{point},
    })
    if err != nil {
        log.Fatal(err)
    }
}

resp, err := stream.CloseAndRecv()
fmt.Printf("Inserted: %d\n", resp.Succeeded)
```

---

## Connection Pooling

For high-throughput applications, use connection pooling:

```go
import "google.golang.org/grpc/keepalive"

conn, err := grpc.Dial(
    "localhost:50051",
    grpc.WithKeepaliveParams(keepalive.ClientParameters{
        Time:                10 * time.Second,
        Timeout:             5 * time.Second,
        PermitWithoutStream: true,
    }),
    grpc.WithInitialWindowSize(1 << 20),    // 1MB
    grpc.WithInitialConnWindowSize(1 << 20), // 1MB
)
```

---

## Performance Comparison

| Operation     | REST (QPS) | gRPC (QPS) | Improvement |
|---------------|------------|------------|-------------|
| Single Search | 2,000      | 5,000      | 2.5x        |
| Batch Search  | 500        | 2,000      | 4x          |
| Upsert        | 3,000      | 8,000      | 2.7x        |
| Streaming     | N/A        | 10,000     | N/A         |

---

## Error Handling

gRPC uses standard status codes:

| Code | Description |
|------|-------------|
| `OK` | Success |
| `INVALID_ARGUMENT` | Bad request parameters |
| `NOT_FOUND` | Collection/point not found |
| `ALREADY_EXISTS` | Collection already exists |
| `UNAUTHENTICATED` | Invalid credentials |
| `PERMISSION_DENIED` | Insufficient permissions |
| `INTERNAL` | Server error |

**Error handling example:**
```go
resp, err := client.Search(ctx, req)
if err != nil {
    st, ok := status.FromError(err)
    if ok {
        switch st.Code() {
        case codes.NotFound:
            log.Printf("Collection not found: %s", st.Message())
        case codes.InvalidArgument:
            log.Printf("Invalid request: %s", st.Message())
        default:
            log.Printf("Error: %s", st.Message())
        }
    }
    return err
}
```

---

## Language-Specific Clients

### Python

```python
import grpc
from limyedb_pb2_grpc import LimyeDBStub

channel = grpc.insecure_channel('localhost:50051')
client = LimyeDBStub(channel)
```

### Java

```java
ManagedChannel channel = ManagedChannelBuilder
    .forAddress("localhost", 50051)
    .usePlaintext()
    .build();

LimyeDBGrpc.LimyeDBBlockingStub client =
    LimyeDBGrpc.newBlockingStub(channel);
```

### Rust

```rust
use tonic::transport::Channel;

let channel = Channel::from_static("http://localhost:50051")
    .connect()
    .await?;

let client = LimyeDbClient::new(channel);
```

---

## Best Practices

1. **Reuse connections** - Create client once, reuse for all requests
2. **Use batch operations** - Group multiple operations when possible
3. **Enable keepalive** - Prevents connection drops for idle connections
4. **Set appropriate deadlines** - Use context timeouts for all requests
5. **Handle backpressure** - Implement retry with exponential backoff
6. **Monitor metrics** - Track latency, error rates, and throughput
