# Getting Started with LimyeDB

LimyeDB is an open-source vector database written in Go that ships as a
single binary. This tutorial walks through bringing it up locally and
running your first vector operations.

## What you get out of the box

- **Single-binary deployment** — `cmd/limyedb` and `cmd/limyedb-cli`.
- **mmap-backed HNSW** — low-millisecond p99 search; see the [benchmarks in the README](../../README.md#benchmarks) for the numbers from a c6i.8xlarge run.
- **Hybrid search** — dense HNSW + sparse BM25 fused via Reciprocal Rank Fusion at `/collections/:name/search/v2`.
- **JWT auth + per-collection RBAC** — required by default; opt out with `-allow-anonymous` for local development.
- **Hashicorp Raft** for high availability — single Raft group across nodes; manual peer discovery via `-raft-join`.

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
curl -LO https://github.com/loreste/limyeDB/releases/latest/download/limyedb_darwin_arm64.tar.gz
tar xzf limyedb_darwin_arm64.tar.gz

# Linux (x86_64)
curl -LO https://github.com/loreste/limyeDB/releases/latest/download/limyedb_linux_amd64.tar.gz
tar xzf limyedb_linux_amd64.tar.gz
```

### Build from Source

```bash
git clone https://github.com/loreste/limyeDB.git
cd limyeDB
go build -o bin/limyedb ./cmd/limyedb
```

### Docker

```bash
docker pull limyedb/limyedb:latest
TOKEN="$(openssl rand -hex 32)"
docker run -p 8080:8080 -p 50051:50051 \
  -v limyedb-data:/data \
  limyedb/limyedb -auth-token "$TOKEN"
```

---

## Starting the Server

LimyeDB refuses to start without an explicit auth decision. Either supply
`-auth-token <secret>` (preferred — every request must then carry
`Authorization: Bearer <token>`) or `-allow-anonymous` (development only,
all endpoints become unauthenticated).

For this tutorial we'll use `-allow-anonymous` so the curl examples are
short. **Do not do this on a host reachable from an untrusted network.**

### Basic Startup

```bash
./limyedb -allow-anonymous
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
```

Start with the configuration file (auth flag is still required):

```bash
./limyedb -config config.yaml -allow-anonymous
```

### Verify Server is Running

The `/health` endpoint does not require authentication:

```bash
curl http://localhost:8080/health
```

---

## Creating Your First Collection

A collection is a container for vectors with the same dimension and
distance metric.

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

### Collection Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `name` | Unique collection name | required |
| `dimension` | Vector dimension | required |
| `metric` | Distance metric: `cosine`, `euclidean`, `dot_product` | `cosine` |
| `hnsw.m` | Max graph connections per node | `16` |
| `hnsw.ef_construction` | Index build quality | `200` |

### Using a client SDK

The repo contains client SDKs in `clients/` for Go, Python, JavaScript,
TypeScript, Rust, and C#. They wrap the REST API and have not yet been
published to language-specific package registries; install them locally
from the directories under `clients/`.

---

## Inserting Vectors

### Generate Embeddings

Generate embeddings client-side with your preferred model — for example
`sentence-transformers`:

```python
from sentence_transformers import SentenceTransformer

model = SentenceTransformer('all-MiniLM-L6-v2')

documents = [
    "The quick brown fox jumps over the lazy dog",
    "Machine learning is transforming industries",
    "Vector databases enable similarity search",
    "Python is a popular programming language",
]
embeddings = model.encode(documents)
```

Alternatively, use the server-side `auto-embed` endpoint to call OpenAI or
Cohere directly:

```bash
curl -X POST http://localhost:8080/collections/my_documents/auto-embed \
  -H "Content-Type: application/json" \
  -d '{
    "provider": "openai",
    "model": "text-embedding-3-small",
    "api_key": "sk-...",
    "source_fields": ["text"],
    "points": [
      {"id": "doc-0", "payload": {"text": "Vector databases enable similarity search"}}
    ]
  }'
```

### Insert points (REST)

```bash
curl -X PUT http://localhost:8080/collections/my_documents/points \
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

### Batch upsert

`POST /collections/:name/points/batch` accepts up to a few thousand points
per call and is the recommended path for bulk loads:

```bash
curl -X POST http://localhost:8080/collections/my_documents/points/batch \
  -H "Content-Type: application/json" \
  -d '{
    "points": [
      {"id": "doc-1", "vector": [0.1, 0.2, ...]},
      {"id": "doc-2", "vector": [0.3, 0.4, ...]}
    ]
  }'
```

The response includes per-point success/failure counts and an `errors[]`
array if any points were rejected (dimension mismatch, missing fields).

---

## Searching Vectors

### Basic Search

```bash
curl -X POST http://localhost:8080/collections/my_documents/search \
  -H "Content-Type: application/json" \
  -d '{
    "vector": [0.1, 0.2, 0.3, ...],
    "limit": 5,
    "with_payload": true
  }'
```

### Response shape

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
  ]
}
```

### Strong-consistency reads

By default reads hit the local Raft FSM, which means a follower can return
data slightly behind the leader. For read-after-write semantics, append
`?consistent=true` and the request will be proxied to the current leader:

```bash
curl -X POST "http://localhost:8080/collections/my_documents/search?consistent=true" \
  -H "Content-Type: application/json" \
  -d '{"vector": [0.1, 0.2, 0.3, ...], "limit": 5}'
```

---

## Filtering Results

LimyeDB supports payload filters that compile into SQLite JSON queries.
Filters can be combined with `must`, `should`, and `must_not`.

### Simple Filter

```bash
curl -X POST http://localhost:8080/collections/my_documents/search \
  -H "Content-Type: application/json" \
  -d '{
    "vector": [0.1, 0.2, 0.3, ...],
    "limit": 5,
    "filter": {
      "must": [
        {"key": "index", "range": {"gt": 1}}
      ]
    }
  }'
```

### Multiple Conditions

```bash
curl -X POST http://localhost:8080/collections/my_documents/search \
  -H "Content-Type: application/json" \
  -d '{
    "vector": [0.1, 0.2, 0.3, ...],
    "limit": 5,
    "filter": {
      "must": [
        {"key": "category", "match": {"value": "technology"}}
      ],
      "must_not": [
        {"key": "status", "match": {"value": "deleted"}}
      ]
    }
  }'
```

See [Advanced Filtering](../advanced_filtering.md) for the full operator
list and how to add a payload index for selective fields.

---

## Next Steps

1. **[Hybrid Search Deep Dive](hybrid_search_deep_dive.md)** — combine dense and sparse vectors with RRF.
2. **[RAG Application Guide](rag_application.md)** — wire LimyeDB into a retrieval-augmented generation pipeline.
3. **[Performance Tuning](../performance_tuning.md)** — pick HNSW parameters for your dataset.
4. **[Clustering Guide](../clustering.md)** — run a 3-node Raft-replicated deployment.

### API references

- [gRPC API](../grpc_api.md)
- [Troubleshooting](../troubleshooting.md)
