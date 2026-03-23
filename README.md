# LimyeDB - Enterprise Distributed Vector Database for AI & RAG

[![Go Reference](https://pkg.go.dev/badge/github.com/limyedb/limyedb.svg)](https://pkg.go.dev/github.com/limyedb/limyedb)
[![Go Report Card](https://goreportcard.com/badge/github.com/limyedb/limyedb)](https://goreportcard.com/report/github.com/limyedb/limyedb)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)
[![Docker Pulls](https://img.shields.io/docker/pulls/limyedb/limyedb)](https://hub.docker.com/r/limyedb/limyedb)

**LimyeDB** is a lightning-fast, highly-available open-source **vector database** engineered for the next generation of AI applications. Built entirely in Go, it is designed from the ground up to power **Retrieval-Augmented Generation (RAG)**, Large Language Model (LLM) memory, and advanced semantic search use-cases with sub-millisecond latency.

LimyeDB uniquely combines Graph-Native HNSW traversal with Zero-Allocation memory pools, unlocking massive Queries-Per-Second (QPS) at a fraction of hardware costs. LimyeDB scales transparently from lightweight embedded binaries to globally distributed, fault-tolerant clusters.

---

## Table of Contents

- [Key Features](#-key-features)
- [Quick Start](#-quick-start)
- [Installation](#-installation)
- [Configuration](#-configuration)
- [API Reference](#-api-reference)
- [Client SDKs](#-client-sdks)
- [Clustering](#-clustering)
- [Advanced Features](#-advanced-features)
- [Observability](#-observability)
- [Security](#-security)
- [Performance](#-performance)
- [Integrations](#-integrations)
- [Contributing](#-contributing)
- [License](#-license)

---

## Key Features

### Core Capabilities

| Feature | Description |
|---------|-------------|
| **HNSW Vector Index** | State-of-the-art Hierarchical Navigable Small World graph for sub-millisecond nearest neighbor search |
| **Hybrid Search** | Combine dense vectors (HNSW) with sparse vectors (BM25) using Reciprocal Rank Fusion |
| **Distributed Clustering** | Native Raft consensus + SWIM gossip protocol for high availability |
| **Multi-Tenancy** | Full tenant isolation with resource quotas and RBAC |
| **SQL Interface** | Familiar SQL-like query syntax for vector operations |
| **Real-Time Updates** | WebSocket subscriptions for live data streaming |
| **Auto-Tuning** | Self-optimizing index parameters based on workload |
| **Auto-Embedding** | Built-in vectorization with OpenAI, Cohere, HuggingFace |

### Technical Highlights

- **Zero-Allocation HNSW Engine:** Eliminates GC pauses with O(1) generational memory pools
- **Advanced AST Payload Filtering:** Execute complex JSON constraints during index traversal
- **Enterprise Security:** TLS/mTLS, API key authentication, Bearer tokens
- **Prometheus Metrics:** Native `/metrics` endpoint for monitoring
- **OpenTelemetry Tracing:** Distributed tracing support

---

## Quick Start

### Using Docker (Recommended)

```bash
# Pull and run LimyeDB
docker run -d \
  --name limyedb \
  -p 8080:8080 \
  -p 6334:6334 \
  -v limyedb_data:/data \
  limyedb/limyedb:latest

# Verify it's running
curl http://localhost:8080/health
```

### Using Docker Compose

```bash
# Clone the repository
git clone https://github.com/loreste/limyeDB.git
cd limyeDB

# Start with docker-compose
docker-compose up -d

# Check logs
docker-compose logs -f
```

### From Binary

```bash
# Download latest release
curl -LO https://github.com/loreste/limyeDB/releases/latest/download/limyedb-linux-amd64
chmod +x limyedb-linux-amd64

# Run
./limyedb-linux-amd64 --rest-addr=0.0.0.0:8080
```

---

## Installation

### Prerequisites

- Go 1.21+ (for building from source)
- Docker 20.10+ (for containerized deployment)
- Make (optional, for build automation)

### Build from Source

```bash
# Clone repository
git clone https://github.com/loreste/limyeDB.git
cd limyeDB

# Install dependencies
make deps

# Build
make build

# Run tests
make test

# The binary will be at ./bin/limyedb
./bin/limyedb --help
```

### Makefile Targets

| Target | Description |
|--------|-------------|
| `make build` | Build the limyedb binary |
| `make test` | Run all tests |
| `make bench` | Run benchmarks |
| `make lint` | Run linters |
| `make proto` | Generate protobuf files |
| `make docker` | Build Docker image |
| `make clean` | Clean build artifacts |

---

## Configuration

### Command Line Options

```bash
./limyedb \
  --rest-addr=0.0.0.0:8080 \       # REST API address
  --grpc-addr=0.0.0.0:6334 \       # gRPC API address
  --data-dir=./data \              # Data directory
  --auth-token=SECRET \            # Authentication token
  --tls-cert=./certs/server.crt \  # TLS certificate
  --tls-key=./certs/server.key \   # TLS private key
  --raft-node-id=node1 \           # Raft node ID
  --raft-bind=0.0.0.0:7000 \       # Raft bind address
  --raft-bootstrap=true \          # Bootstrap new cluster
  --log-level=info                 # Log level (debug/info/warn/error)
```

### Configuration File (config.yaml)

```yaml
server:
  rest_addr: "0.0.0.0:8080"
  grpc_addr: "0.0.0.0:6334"
  data_dir: "./data"

security:
  auth_token: "${AUTH_TOKEN}"
  tls:
    enabled: true
    cert_file: "./certs/server.crt"
    key_file: "./certs/server.key"
    client_ca_file: "./certs/ca.crt"  # For mTLS

cluster:
  node_id: "node1"
  raft:
    bind_addr: "0.0.0.0:7000"
    bootstrap: true
  gossip:
    bind_addr: "0.0.0.0:7001"
    seeds: ["node2:7001", "node3:7001"]

hnsw:
  default_m: 16
  default_ef_construction: 200
  default_ef_search: 100

storage:
  wal:
    sync_writes: true
    segment_size: 67108864  # 64MB
  mmap:
    enabled: true
  snapshot:
    interval: "1h"
    retention: 5

observability:
  metrics:
    enabled: true
    path: "/metrics"
  tracing:
    enabled: true
    endpoint: "http://jaeger:14268/api/traces"
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `LIMYEDB_REST_ADDR` | REST API address | `0.0.0.0:8080` |
| `LIMYEDB_GRPC_ADDR` | gRPC API address | `0.0.0.0:6334` |
| `LIMYEDB_DATA_DIR` | Data directory | `./data` |
| `LIMYEDB_AUTH_TOKEN` | Authentication token | (none) |
| `LIMYEDB_LOG_LEVEL` | Log level | `info` |

---

## API Reference

### Collections

#### Create Collection

```bash
curl -X POST http://localhost:8080/collections \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "name": "documents",
    "dimension": 1536,
    "metric": "cosine",
    "hnsw_config": {
      "m": 16,
      "ef_construction": 200
    },
    "payload_schema": {
      "title": "keyword",
      "category": "keyword",
      "price": "float",
      "in_stock": "bool"
    }
  }'
```

#### List Collections

```bash
curl http://localhost:8080/collections \
  -H "Authorization: Bearer YOUR_TOKEN"
```

#### Get Collection Info

```bash
curl http://localhost:8080/collections/documents \
  -H "Authorization: Bearer YOUR_TOKEN"
```

#### Delete Collection

```bash
curl -X DELETE http://localhost:8080/collections/documents \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### Points (Vectors)

#### Upsert Points

```bash
curl -X PUT http://localhost:8080/collections/documents/points \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "points": [
      {
        "id": "doc1",
        "vector": [0.1, 0.2, 0.3, ...],
        "payload": {
          "title": "Introduction to AI",
          "category": "technology",
          "price": 29.99,
          "in_stock": true
        }
      },
      {
        "id": "doc2",
        "vector": [0.4, 0.5, 0.6, ...],
        "payload": {
          "title": "Machine Learning Basics",
          "category": "technology",
          "price": 39.99,
          "in_stock": false
        }
      }
    ]
  }'
```

#### Get Point by ID

```bash
curl http://localhost:8080/collections/documents/points/doc1 \
  -H "Authorization: Bearer YOUR_TOKEN"
```

#### Delete Points

```bash
curl -X POST http://localhost:8080/collections/documents/points/delete \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "points": ["doc1", "doc2"]
  }'
```

### Search

#### Vector Search

```bash
curl -X POST http://localhost:8080/collections/documents/search \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "vector": [0.1, 0.2, 0.3, ...],
    "limit": 10,
    "with_payload": true,
    "with_vector": false
  }'
```

#### Filtered Search

```bash
curl -X POST http://localhost:8080/collections/documents/search \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "vector": [0.1, 0.2, 0.3, ...],
    "limit": 10,
    "filter": {
      "must": [
        {"key": "category", "match": {"value": "technology"}},
        {"key": "in_stock", "match": {"value": true}}
      ],
      "must_not": [
        {"key": "price", "range": {"gt": 50.0}}
      ]
    }
  }'
```

#### Hybrid Search (Dense + Sparse)

```bash
curl -X POST http://localhost:8080/collections/documents/search/hybrid \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "dense_vector": [0.1, 0.2, 0.3, ...],
    "sparse_query": "machine learning introduction",
    "limit": 10,
    "fusion": {
      "method": "rrf",
      "k": 60
    }
  }'
```

### SQL Interface

LimyeDB supports a SQL-like query interface:

```bash
curl -X POST http://localhost:8080/sql \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "query": "SELECT * FROM documents NEAREST TO [0.1, 0.2, 0.3, ...] LIMIT 10 WHERE category = \"technology\""
  }'
```

Supported SQL operations:
- `CREATE TABLE collection_name (dimension INT, metric STRING)`
- `DROP TABLE collection_name`
- `DESCRIBE collection_name`
- `SHOW TABLES`
- `SELECT * FROM collection NEAREST TO [vector] LIMIT n WHERE conditions`

### Batch Operations

#### Batch Import

```bash
curl -X POST http://localhost:8080/collections/documents/points/batch \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "batch_size": 1000,
    "points": [...]
  }'
```

#### Scroll (Pagination)

```bash
curl -X POST http://localhost:8080/collections/documents/points/scroll \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "limit": 100,
    "offset": 0,
    "with_payload": true,
    "filter": {...}
  }'
```

---

## Client SDKs

### Python

```bash
pip install limyedb
```

```python
from limyedb import LimyeDBClient

# Connect to LimyeDB
client = LimyeDBClient(
    host="localhost",
    port=8080,
    api_key="YOUR_API_KEY"
)

# Create a collection
client.create_collection(
    name="documents",
    dimension=1536,
    metric="cosine"
)

# Insert vectors
client.upsert(
    collection="documents",
    points=[
        {
            "id": "doc1",
            "vector": [0.1, 0.2, ...],
            "payload": {"title": "Introduction to AI"}
        }
    ]
)

# Search
results = client.search(
    collection="documents",
    vector=[0.1, 0.2, ...],
    limit=10,
    filter={"category": "technology"}
)

for result in results:
    print(f"ID: {result.id}, Score: {result.score}")
```

### JavaScript/TypeScript

```bash
npm install limyedb
```

```typescript
import { LimyeDBClient } from 'limyedb';

// Connect to LimyeDB
const client = new LimyeDBClient({
  host: 'localhost',
  port: 8080,
  apiKey: 'YOUR_API_KEY'
});

// Create a collection
await client.createCollection({
  name: 'documents',
  dimension: 1536,
  metric: 'cosine'
});

// Insert vectors
await client.upsert('documents', [
  {
    id: 'doc1',
    vector: [0.1, 0.2, ...],
    payload: { title: 'Introduction to AI' }
  }
]);

// Search
const results = await client.search('documents', {
  vector: [0.1, 0.2, ...],
  limit: 10,
  filter: { category: 'technology' }
});

results.forEach(result => {
  console.log(`ID: ${result.id}, Score: ${result.score}`);
});
```

### Go

```go
import "github.com/loreste/limyeDB/clients/go/limyedb"

// Connect to LimyeDB
client := limyedb.NewClient("http://localhost:8080", "YOUR_API_KEY")

// Create a collection
err := client.CreateCollection(context.Background(), &limyedb.CreateCollectionRequest{
    Name:      "documents",
    Dimension: 1536,
    Metric:    "cosine",
})

// Insert vectors
err = client.Upsert(context.Background(), "documents", []limyedb.Point{
    {
        ID:      "doc1",
        Vector:  []float32{0.1, 0.2, ...},
        Payload: map[string]interface{}{"title": "Introduction to AI"},
    },
})

// Search
results, err := client.Search(context.Background(), "documents", &limyedb.SearchRequest{
    Vector: []float32{0.1, 0.2, ...},
    Limit:  10,
})
```

---

## Clustering

### Architecture

LimyeDB uses a hybrid clustering approach:

1. **Raft Consensus**: For strong consistency on metadata and critical operations
2. **SWIM Gossip**: For efficient failure detection and membership management
3. **Consistent Hashing**: For data distribution across nodes

```
┌─────────────────────────────────────────────────────────────────┐
│                     LimyeDB Cluster                              │
│                                                                  │
│  ┌─────────┐     ┌─────────┐     ┌─────────┐     ┌─────────┐   │
│  │ Node 1  │────▶│ Node 2  │────▶│ Node 3  │────▶│ Node N  │   │
│  │ (Leader)│◀────│(Follower│◀────│(Follower│◀────│(Follower│   │
│  └─────────┘     └─────────┘     └─────────┘     └─────────┘   │
│       │              │               │               │          │
│       └──────────────┴───────────────┴───────────────┘          │
│                         Gossip Ring                              │
└─────────────────────────────────────────────────────────────────┘
```

### Bootstrap a Cluster

#### Node 1 (Bootstrap Leader)

```bash
./limyedb \
  --raft-node-id=node1 \
  --raft-bind=192.168.1.1:7000 \
  --raft-bootstrap=true \
  --gossip-bind=192.168.1.1:7001 \
  --rest-addr=192.168.1.1:8080 \
  --data-dir=/data/node1
```

#### Node 2 (Join Cluster)

```bash
./limyedb \
  --raft-node-id=node2 \
  --raft-bind=192.168.1.2:7000 \
  --raft-join=http://192.168.1.1:8080 \
  --gossip-bind=192.168.1.2:7001 \
  --gossip-seeds=192.168.1.1:7001 \
  --rest-addr=192.168.1.2:8080 \
  --data-dir=/data/node2
```

#### Node 3 (Join Cluster)

```bash
./limyedb \
  --raft-node-id=node3 \
  --raft-bind=192.168.1.3:7000 \
  --raft-join=http://192.168.1.1:8080 \
  --gossip-bind=192.168.1.3:7001 \
  --gossip-seeds=192.168.1.1:7001,192.168.1.2:7001 \
  --rest-addr=192.168.1.3:8080 \
  --data-dir=/data/node3
```

### Node Discovery

LimyeDB supports multiple discovery mechanisms:

- **Static**: Configure seed nodes manually
- **DNS SRV**: Discover nodes via DNS service records
- **Consul**: Integrate with HashiCorp Consul
- **Kubernetes**: Use Kubernetes service discovery

```yaml
cluster:
  discovery:
    type: kubernetes
    kubernetes:
      namespace: limyedb
      service: limyedb-headless
      port_name: gossip
```

### Replication & Consistency

- **Replication Factor**: Configure per-collection (`replication_factor: 3`)
- **Write Concern**: `one`, `majority`, `all`
- **Read Concern**: `local`, `majority`

---

## Advanced Features

### Auto-Embedding

Automatically convert text to vectors on insert:

```bash
curl -X POST http://localhost:8080/collections/documents/auto-embed \
  -H "Content-Type: application/json" \
  -d '{
    "provider": "openai",
    "model": "text-embedding-3-small",
    "api_key": "sk-...",
    "source_fields": ["title", "content"],
    "template": "{{.title}}: {{.content}}"
  }'
```

Then insert text directly:

```bash
curl -X PUT http://localhost:8080/collections/documents/points \
  -H "Content-Type: application/json" \
  -d '{
    "points": [
      {
        "id": "doc1",
        "payload": {
          "title": "Introduction to AI",
          "content": "AI is transforming industries..."
        }
      }
    ]
  }'
```

### Multi-Tenancy

Create isolated tenants with resource quotas:

```bash
curl -X POST http://localhost:8080/tenants \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "customer_123",
    "plan": "professional",
    "quotas": {
      "max_collections": 100,
      "max_vectors": 10000000,
      "max_storage_gb": 500
    }
  }'
```

### Real-Time Subscriptions

Connect via WebSocket to receive live updates:

```javascript
const ws = new WebSocket('ws://localhost:8080/ws');

ws.onopen = () => {
  ws.send(JSON.stringify({
    type: 'subscribe',
    collection: 'documents',
    events: ['point.insert', 'point.update', 'point.delete']
  }));
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Event:', data.type, data.point);
};
```

### Auto-Tuning

Enable automatic parameter optimization:

```bash
curl -X POST http://localhost:8080/collections/documents/autotune \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "goal": "balanced",
    "constraints": {
      "min_recall": 0.95,
      "max_latency_ms": 50
    }
  }'
```

---

## Observability

### Prometheus Metrics

Access metrics at `/metrics`:

```bash
curl http://localhost:8080/metrics
```

Key metrics:

| Metric | Description |
|--------|-------------|
| `limyedb_search_latency_seconds` | Search latency histogram |
| `limyedb_search_total` | Total search requests |
| `limyedb_insert_total` | Total insert operations |
| `limyedb_vectors_total` | Total vectors stored |
| `limyedb_collections_total` | Number of collections |
| `limyedb_raft_state` | Raft cluster state |
| `limyedb_gossip_members` | Active gossip members |

### Grafana Dashboard

Import the included Grafana dashboard:

```bash
curl -X POST http://grafana:3000/api/dashboards/db \
  -H "Content-Type: application/json" \
  -d @deploy/grafana/limyedb-dashboard.json
```

### OpenTelemetry Tracing

Configure tracing in your config:

```yaml
observability:
  tracing:
    enabled: true
    exporter: otlp
    endpoint: "http://jaeger:4318/v1/traces"
    service_name: limyedb
    sample_rate: 0.1
```

### Logging

JSON-structured logging:

```bash
./limyedb --log-format=json --log-level=debug
```

---

## Security

### Authentication

#### API Key Authentication

```bash
./limyedb --auth-token=YOUR_SECRET_TOKEN

# All requests must include the token
curl -H "Authorization: Bearer YOUR_SECRET_TOKEN" ...
```

#### Multi-Tenant RBAC

Create roles and users:

```bash
# Create a role
curl -X POST http://localhost:8080/admin/roles \
  -H "Authorization: Bearer ADMIN_TOKEN" \
  -d '{
    "name": "reader",
    "permissions": ["collection.read", "point.read", "search"]
  }'

# Create a user with role
curl -X POST http://localhost:8080/admin/users \
  -H "Authorization: Bearer ADMIN_TOKEN" \
  -d '{
    "username": "readonly_user",
    "api_key": "user_api_key",
    "roles": ["reader"],
    "tenant_id": "customer_123"
  }'
```

### TLS/mTLS

#### TLS (Server Certificate)

```bash
./limyedb \
  --tls-cert=/certs/server.crt \
  --tls-key=/certs/server.key
```

#### mTLS (Mutual TLS)

```bash
./limyedb \
  --tls-cert=/certs/server.crt \
  --tls-key=/certs/server.key \
  --tls-client-ca=/certs/ca.crt \
  --tls-require-client-cert=true
```

### Network Security

- Enable IP allowlisting
- Use private networks for cluster communication
- Encrypt inter-node traffic with TLS

---

## Performance

### Benchmarks

Tested on AWS c6i.8xlarge (32 vCPU, 64GB RAM):

| Operation | Latency (p99) | Throughput |
|-----------|---------------|------------|
| Search (1M vectors) | 2.1ms | 15,000 QPS |
| Search (10M vectors) | 8.3ms | 8,500 QPS |
| Insert (batch 1000) | 45ms | 22,000 vectors/s |
| Filtered search | 4.2ms | 10,000 QPS |

### Tuning Tips

1. **HNSW Parameters**:
   - Higher `M` = better recall, more memory
   - Higher `ef_construction` = better index quality, slower build
   - Higher `ef_search` = better recall, slower search

2. **Memory**:
   - Enable mmap for larger-than-RAM datasets
   - Use scalar quantization for 4x memory reduction

3. **Concurrency**:
   - Tune `GOMAXPROCS` for CPU-bound workloads
   - Use batch operations for bulk inserts

---

## Integrations

### LangChain

```python
from langchain_community.vectorstores import LimyeDB
from langchain_openai import OpenAIEmbeddings

embeddings = OpenAIEmbeddings()

vectorstore = LimyeDB(
    url="http://localhost:8080",
    api_key="YOUR_API_KEY",
    collection_name="documents",
    embedding=embeddings
)

# Add documents
vectorstore.add_documents(documents)

# Search
results = vectorstore.similarity_search("query", k=5)
```

### LlamaIndex

```python
from llama_index.vector_stores.limyedb import LimyeDBVectorStore
from llama_index.core import VectorStoreIndex

vector_store = LimyeDBVectorStore(
    url="http://localhost:8080",
    api_key="YOUR_API_KEY",
    collection_name="documents"
)

index = VectorStoreIndex.from_vector_store(vector_store)
query_engine = index.as_query_engine()
response = query_engine.query("What is AI?")
```

### Kubernetes Deployment

```bash
# Add Helm repo
helm repo add limyedb https://charts.limyedb.io
helm repo update

# Install
helm install limyedb limyedb/limyedb \
  --namespace limyedb \
  --create-namespace \
  --set replicaCount=3 \
  --set persistence.size=100Gi
```

---

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

### Development Setup

```bash
# Clone
git clone https://github.com/loreste/limyeDB.git
cd limyeDB

# Install dependencies
make deps

# Run tests
make test

# Build
make build
```

### Code Style

- Follow Go best practices
- Run `make lint` before submitting PRs
- Add tests for new features

---

## License

LimyeDB is licensed under the [GNU General Public License v3.0](LICENSE).

---

## Support

- **Documentation**: [https://docs.limyedb.io](https://docs.limyedb.io)
- **GitHub Issues**: [https://github.com/loreste/limyeDB/issues](https://github.com/loreste/limyeDB/issues)
- **Discord**: [https://discord.gg/limyedb](https://discord.gg/limyedb)
- **Twitter**: [@LimyeDB](https://twitter.com/limyedb)

---

Built with passion for the AI community.
