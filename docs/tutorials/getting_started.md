# Getting Started with LimyeDB

Welcome to LimyeDB, the **high-performance open-source vector database** designed for AI applications, RAG pipelines, and semantic search. This tutorial walks you through setting up LimyeDB and performing your first vector operations.

## Why LimyeDB for Your AI Application?

Before diving in, here's what makes LimyeDB the ideal choice for your vector search needs:

- **Zero-Configuration Start**: Single binary, no dependencies—run `./limyedb` and you're ready
- **Sub-Millisecond Latency**: Zero-allocation HNSW delivers consistent P99 performance
- **Native Hybrid Search**: Combine semantic (dense) and keyword (sparse/BM25) search in one query
- **Production-Ready Security**: JWT auth, RBAC, TLS, and multi-tenancy built-in
- **Scale as You Grow**: From laptop prototype to billion-vector cluster without changing your code

---

## Table of Contents

1. [Installation](#installation)
2. [Starting the Server](#starting-the-server)
3. [Creating Your First Collection](#creating-your-first-collection)
4. [Inserting Vectors](#inserting-vectors)
5. [Searching Vectors](#searching-vectors)
6. [Filtering Results](#filtering-results)
7. [Next Steps](#next-steps)

---

## Installation

### Download Binary

```bash
# macOS (Apple Silicon)
curl -LO https://github.com/limyedb/limyedb/releases/latest/download/limyedb-darwin-arm64.tar.gz
tar xzf limyedb-darwin-arm64.tar.gz

# macOS (Intel)
curl -LO https://github.com/limyedb/limyedb/releases/latest/download/limyedb-darwin-amd64.tar.gz
tar xzf limyedb-darwin-amd64.tar.gz

# Linux (x86_64)
curl -LO https://github.com/limyedb/limyedb/releases/latest/download/limyedb-linux-amd64.tar.gz
tar xzf limyedb-linux-amd64.tar.gz
```

### Build from Source

```bash
git clone https://github.com/limyedb/limyedb.git
cd limyedb
go build -o limyedb ./cmd/limyedb
```

### Docker

```bash
docker pull limyedb/limyedb:latest
docker run -p 8080:8080 -p 50051:50051 -v limyedb-data:/data limyedb/limyedb
```

---

## Starting the Server

### Basic Startup

```bash
./limyedb serve
```

The server starts with default settings:
- REST API: `http://localhost:8080`
- gRPC API: `localhost:50051`
- Data directory: `./data`

### Custom Configuration

Create a `config.yaml` file:

```yaml
server:
  rest_address: ":8080"
  grpc_address: ":50051"

storage:
  data_dir: "/var/lib/limyedb"
  wal_dir: "/var/lib/limyedb/wal"

logging:
  level: info
  format: json
```

Start with configuration:

```bash
./limyedb serve --config config.yaml
```

### Verify Server is Running

```bash
curl http://localhost:8080/health
```

Response:
```json
{
  "status": "healthy",
  "version": "1.0.0"
}
```

---

## Creating Your First Collection

A collection is a container for vectors with the same dimension.

### Using REST API

```bash
curl -X POST http://localhost:8080/collections \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my_documents",
    "dimension": 384,
    "metric": "cosine"
  }'
```

### Using Python SDK

```python
from limyedb import LimyeDBClient

client = LimyeDBClient(host="http://localhost:8080")

client.create_collection(
    name="my_documents",
    dimension=384,
    metric="cosine"
)
```

### Using JavaScript SDK

```javascript
const { LimyeDBClient } = require("limyedb");

const client = new LimyeDBClient({ host: "http://localhost:8080" });

await client.createCollection({
  name: "my_documents",
  dimension: 384,
  metric: "cosine"
});
```

### Collection Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `name` | Unique collection name | Required |
| `dimension` | Vector dimension | Required |
| `metric` | Distance metric (`cosine`, `euclidean`, `dot_product`) | `cosine` |
| `hnsw.m` | Max connections per node | 16 |
| `hnsw.ef_construction` | Build quality | 200 |

---

## Inserting Vectors

### Generate Embeddings

First, generate embeddings using your preferred embedding model:

```python
from sentence_transformers import SentenceTransformer

model = SentenceTransformer('all-MiniLM-L6-v2')

documents = [
    "The quick brown fox jumps over the lazy dog",
    "Machine learning is transforming industries",
    "Vector databases enable similarity search",
    "Python is a popular programming language"
]

embeddings = model.encode(documents)
```

### Insert Points

```python
from limyedb import Point

points = [
    Point.create(
        id=f"doc-{i}",
        vector=embeddings[i].tolist(),
        payload={"text": doc, "index": i}
    )
    for i, doc in enumerate(documents)
]

client.upsert("my_documents", points)
```

### Batch Insert

For large datasets, use batch operations:

```python
# Insert in batches of 100
batch_size = 100
for i in range(0, len(all_points), batch_size):
    batch = all_points[i:i+batch_size]
    client.upsert("my_documents", batch)
```

### Using REST API

```bash
curl -X POST http://localhost:8080/collections/my_documents/points \
  -H "Content-Type: application/json" \
  -d '{
    "points": [
      {
        "id": "doc-0",
        "vector": [0.1, 0.2, 0.3, ...],
        "payload": {"text": "The quick brown fox...", "index": 0}
      }
    ]
  }'
```

---

## Searching Vectors

### Basic Search

Find the most similar vectors to a query:

```python
# Generate query embedding
query = "What are vector databases?"
query_vector = model.encode(query).tolist()

# Search
results = client.search(
    collection_name="my_documents",
    query_vector=query_vector,
    limit=5
)

for result in results:
    print(f"ID: {result.id}, Score: {result.score:.4f}")
    print(f"  Text: {result.payload['text']}")
```

### Search with Payload

Include payload in results:

```python
results = client.search(
    collection_name="my_documents",
    query_vector=query_vector,
    limit=5,
    with_payload=True,
    with_vector=False  # Don't return vectors to save bandwidth
)
```

### Using REST API

```bash
curl -X POST http://localhost:8080/collections/my_documents/search \
  -H "Content-Type: application/json" \
  -d '{
    "vector": [0.1, 0.2, 0.3, ...],
    "limit": 5,
    "with_payload": true
  }'
```

### Response

```json
{
  "results": [
    {
      "id": "doc-2",
      "score": 0.8542,
      "payload": {
        "text": "Vector databases enable similarity search",
        "index": 2
      }
    },
    {
      "id": "doc-1",
      "score": 0.7821,
      "payload": {
        "text": "Machine learning is transforming industries",
        "index": 1
      }
    }
  ],
  "took_ms": 2
}
```

---

## Filtering Results

Combine vector search with metadata filters.

### Simple Filter

```python
# Find similar documents with index > 1
results = client.search(
    collection_name="my_documents",
    query_vector=query_vector,
    limit=5,
    filter={
        "must": [
            {"key": "index", "range": {"gt": 1}}
        ]
    }
)
```

### Multiple Conditions

```python
# Find similar documents with specific criteria
results = client.search(
    collection_name="my_documents",
    query_vector=query_vector,
    limit=5,
    filter={
        "must": [
            {"key": "category", "match": {"value": "technology"}},
            {"key": "published", "match": {"value": True}}
        ],
        "must_not": [
            {"key": "status", "match": {"value": "deleted"}}
        ]
    }
)
```

### Using REST API

```bash
curl -X POST http://localhost:8080/collections/my_documents/search \
  -H "Content-Type: application/json" \
  -d '{
    "vector": [0.1, 0.2, 0.3, ...],
    "limit": 5,
    "filter": {
      "must": [
        {"key": "category", "match": {"value": "technology"}}
      ]
    }
  }'
```

---

## Complete Example

Here's a complete working example:

```python
from limyedb import LimyeDBClient, Point
from sentence_transformers import SentenceTransformer

# Initialize
client = LimyeDBClient(host="http://localhost:8080")
model = SentenceTransformer('all-MiniLM-L6-v2')

# Create collection
client.create_collection(
    name="articles",
    dimension=384,
    metric="cosine"
)

# Sample data
articles = [
    {"title": "Introduction to Python", "category": "programming"},
    {"title": "Machine Learning Basics", "category": "ai"},
    {"title": "Database Design Patterns", "category": "databases"},
    {"title": "Neural Networks Explained", "category": "ai"},
    {"title": "REST API Best Practices", "category": "programming"},
]

# Generate embeddings and insert
points = []
for i, article in enumerate(articles):
    embedding = model.encode(article["title"]).tolist()
    points.append(Point.create(
        id=f"article-{i}",
        vector=embedding,
        payload=article
    ))

client.upsert("articles", points)

# Search
query = "How do neural networks work?"
query_vector = model.encode(query).tolist()

results = client.search(
    collection_name="articles",
    query_vector=query_vector,
    limit=3,
    filter={
        "must": [
            {"key": "category", "match": {"value": "ai"}}
        ]
    }
)

print(f"Query: {query}")
print("Results:")
for r in results:
    print(f"  - {r.payload['title']} (score: {r.score:.4f})")
```

Output:
```
Query: How do neural networks work?
Results:
  - Neural Networks Explained (score: 0.8234)
  - Machine Learning Basics (score: 0.7156)
```

---

## Next Steps

Now that you've learned the basics, explore these advanced topics:

1. **[Building RAG Applications](rag_application.md)** - Create retrieval-augmented generation systems
2. **[Hybrid Search Deep Dive](hybrid_search_deep_dive.md)** - Combine dense and sparse vectors
3. **[Scaling to Millions](scaling_to_millions.md)** - Handle large-scale deployments
4. **[Performance Tuning](../performance_tuning.md)** - Optimize for your workload
5. **[Advanced Filtering](../advanced_filtering.md)** - Complex query patterns

### Additional Resources

- [REST API Reference](../rest_api.md)
- [gRPC API Reference](../grpc_api.md)
- [Configuration Guide](../configuration.md)
- [Troubleshooting](../troubleshooting.md)

