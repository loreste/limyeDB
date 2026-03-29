# LimyeDB - Open Source Vector Database for GenAI, RAG & LLMs

[![CI](https://github.com/loreste/limyeDB/actions/workflows/ci.yml/badge.svg)](https://github.com/loreste/limyeDB/actions/workflows/ci.yml)
[![CodeQL](https://github.com/loreste/limyeDB/actions/workflows/codeql.yml/badge.svg)](https://github.com/loreste/limyeDB/actions/workflows/codeql.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/limyedb/limyedb)](https://goreportcard.com/report/github.com/limyedb/limyedb)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)
[![Release](https://img.shields.io/github/v/release/loreste/limyeDB)](https://github.com/loreste/limyeDB/releases/latest)

**LimyeDB** is a lightning-fast, highly-available **open-source vector database** engineered specifically for the next generation of AI applications. Built entirely from scratch in Go, it is the ultimate semantic storage engine designed to power **Retrieval-Augmented Generation (RAG)**, large language model (LLM) memory arrays, and predictive similarity matching with sub-millisecond retrieval latency.

LimyeDB distinguishes itself by natively supporting **Hybrid Search**—combining zero-allocation memory mapped **NVMe HNSW** (Hierarchical Navigable Small World) Dense vector indexing alongside a blisteringly fast local **BM25/SPLADE Inverted Index** for Sparse vectors. It mathematically fuses these Multi-Modal queries using industry-standard **Reciprocal Rank Fusion (RRF)**. LimyeDB scales effortlessly from lightweight embedded binaries to universally distributed, fault-tolerant clusters.

---

## Why LimyeDB? The Vector Database Built for Production AI

### The Challenge with Existing Vector Databases

The explosive growth of **Large Language Models (LLMs)**, **Retrieval-Augmented Generation (RAG)**, and **AI-powered search** has created unprecedented demand for vector databases. Yet teams building production AI systems consistently face the same frustrations:

#### Vendor Lock-in and Unpredictable Costs
Proprietary vector database services like **Pinecone** charge based on vector storage and queries, leading to bills that scale unpredictably as your AI application grows. Open-source alternatives often follow an "open core" model where critical features—clustering, security, enterprise support—are paywalled. Organizations building mission-critical AI infrastructure deserve better than hoping their vendor's pricing stays reasonable.

#### Operational Complexity
Solutions like **Milvus** require deploying etcd, MinIO, Pulsar, and multiple coordinator services before you can store a single vector. **Weaviate** and **Qdrant** simplify deployment but still demand careful tuning for production workloads. Teams end up spending more time managing infrastructure than building AI features.

#### The Performance Trade-off Trap
Vector databases force painful compromises:
- **In-memory indexes** (like standard HNSW) deliver sub-millisecond latency but limit dataset size to available RAM
- **Disk-based solutions** scale larger but introduce 10-100x latency penalties
- **Managed services** abstract complexity but add network round-trips and throttling
- **JVM-based systems** suffer garbage collection pauses that destroy P99 latencies

#### Missing Features for Real-World AI Applications
Production AI systems need more than just vector search:
- **Hybrid search** combining semantic similarity with keyword matching for better relevance
- **Multi-tenancy** for SaaS products serving multiple customers
- **Real-time subscriptions** for reactive user interfaces
- **Built-in embedding** to avoid separate ETL pipelines
- **Enterprise security** including RBAC, encryption, and audit logging

Getting all of this typically means stitching together multiple systems—each with its own failure modes, version conflicts, and operational overhead.

---

### Our Mission: Production-Grade AI Infrastructure for Everyone

**LimyeDB was created to prove that powerful doesn't have to mean complicated.**

We built the vector database we wished existed: one that a solo developer can run on a laptop for prototyping, yet scales horizontally to handle billions of vectors across a globally distributed cluster. One that delivers enterprise-grade capabilities without enterprise-grade complexity or cost.

---

### What Makes LimyeDB the Best Open Source Vector Database

#### 🚀 Single Binary Deployment — No Dependencies, No DevOps Nightmare

LimyeDB compiles to a **single statically-linked Go binary**. No JVM tuning. No external PostgreSQL, etcd, or Redis instances. No message queues. No container orchestration required for basic deployments.

```bash
# That's it. You now have a production-ready vector database.
./limyedb -data ./my-vectors -rest :8080
```

This isn't just convenience—it's **operational sanity**. Fewer moving parts means fewer failure modes, simpler debugging, and faster disaster recovery.

#### ⚡ Zero-Allocation HNSW — Consistent Sub-Millisecond Latency

Most vector databases written in Go or Java suffer from **garbage collection pauses** that spike P99 latencies from 2ms to 200ms+ unpredictably. LimyeDB's HNSW implementation takes a radically different approach:

- **Memory-mapped NVMe storage** bypasses Go's heap entirely
- **Zero-allocation graph traversal** means no GC pressure during searches
- **Lock-free concurrent reads** enable massive query parallelism
- **SIMD-accelerated distance calculations** on ARM64 (NEON) and x86-64 (AVX2)

The result: **consistent sub-millisecond P99 latencies** that don't degrade under load. Your real-time AI applications stay responsive.

#### 🔍 Native Hybrid Search — Semantic + Keyword in One Query

While other vector databases bolt on keyword search as an afterthought, **LimyeDB was architected from day one for hybrid retrieval**:

| Component | Technology | Purpose |
|-----------|------------|---------|
| Dense Vectors | HNSW / DiskANN / IVF / ScaNN | Semantic similarity search |
| Sparse Vectors | BM25 / SPLADE | Keyword and lexical matching |
| Fusion | Reciprocal Rank Fusion (RRF) | Mathematically optimal result merging |

This matters because **pure vector search fails on proper nouns, product codes, and exact phrases**. Hybrid search delivers better relevance for real-world queries without requiring multiple systems or post-processing.

```bash
# Single query combining semantic understanding with keyword precision
curl -X POST http://localhost:8080/collections/docs/search \
  -d '{
    "vector": [0.1, 0.2, ...],
    "sparse_query": {"indices": [101, 403], "values": [2.4, 0.8]},
    "limit": 10
  }'
```

#### 📊 Billion-Scale Vector Search with DiskANN

Not every organization can afford to keep billions of vectors in RAM. LimyeDB's **DiskANN Vamana implementation** enables:

- **SSD-resident graph indexes** that search billion-vector datasets
- **10-100x lower infrastructure costs** compared to in-memory solutions
- **Configurable memory/latency trade-offs** for your specific requirements
- **Seamless tiering** between hot (RAM) and warm (SSD) data

Run the same vector operations on a $50/month VPS that would require a $5,000/month high-memory instance with RAM-only solutions.

#### 🏢 Enterprise-Ready Multi-Tenancy and RBAC

Building a multi-tenant AI SaaS? LimyeDB provides **first-class tenant isolation**:

- **Tenant-scoped collections** with complete data separation
- **Role-Based Access Control (RBAC)** with granular permissions
- **Resource quotas** per tenant (vectors, storage, query rate)
- **JWT authentication** with configurable claims
- **API key management** with automatic rotation

No need to deploy separate database instances per customer or build isolation logic in your application layer.

#### 🔒 Security-Hardened from the Ground Up

AI systems increasingly process sensitive data—customer conversations, proprietary documents, personal information. LimyeDB implements **defense-in-depth security**:

| Protection | Implementation |
|------------|----------------|
| Authentication | Bearer tokens, API keys, JWT with RBAC |
| Encryption | TLS 1.3, mTLS for inter-node communication |
| Timing Attack Prevention | Constant-time token comparison (`crypto/subtle`) |
| SSRF Protection | Webhook URL validation against private IP ranges |
| SQL Injection Prevention | Parameterized queries with escaped LIKE patterns |
| Path Traversal Prevention | Sanitized paths with Zip Slip protection |
| Decompression Bombs | Size limits on archive extraction |
| Cryptographic Randomness | `crypto/rand` for all security-sensitive operations |

#### 🤖 Automatic Embedding Orchestration

Skip the ETL pipeline. LimyeDB integrates directly with embedding providers:

```bash
# Send text, receive indexed vectors
curl -X POST http://localhost:8080/collections/docs/auto-embed \
  -d '{
    "provider": "openai",
    "model": "text-embedding-3-small",
    "api_key": "sk-...",
    "points": [
      {"id": "doc1", "payload": {"content": "Your text here"}}
    ]
  }'
```

**Supported Providers:**
- OpenAI (text-embedding-3-small, text-embedding-3-large, ada-002)
- Cohere (embed-english-v3.0, embed-multilingual-v3.0)
- Google Vertex AI
- Local models via HTTP endpoints

#### 🗄️ SQL-Like Query Interface

No proprietary DSL to learn. Query your vectors with familiar SQL syntax:

```sql
SELECT * FROM documents
NEAREST TO [0.1, 0.2, 0.3, ...]
WHERE category = "technology" AND price < 100
LIMIT 10
```

#### 🌐 Distributed Clustering for High Availability

Scale horizontally with LimyeDB's **hybrid clustering architecture**:

- **Raft Consensus** for strongly consistent metadata operations
- **SWIM Gossip Protocol** for efficient failure detection
- **Consistent Hashing** for data distribution across nodes
- **Automatic Rebalancing** when nodes join or leave
- **Configurable Replication** for durability guarantees

Deploy a 3-node cluster for high availability or scale to dozens of nodes for massive throughput.

#### 📈 Production Observability Built-In

Monitor everything with native integrations:

- **Prometheus Metrics** at `/metrics` (latencies, throughput, index stats)
- **OpenTelemetry Tracing** for distributed request tracking
- **Structured JSON Logging** with configurable levels
- **Health and Readiness Endpoints** for Kubernetes probes
- **Grafana Dashboard** included in the repository

#### 🆓 Truly Open Source — No Bait-and-Switch

LimyeDB is released under **GPL v3**. Everything is open:

- ✅ Clustering and high availability
- ✅ Multi-tenancy and RBAC
- ✅ All index types (HNSW, DiskANN, IVF, ScaNN)
- ✅ Hybrid search with BM25
- ✅ Auto-embedding orchestration
- ✅ Backup and restore
- ✅ TLS and security features
- ✅ Observability integrations

No "enterprise edition" holding features hostage. No phone call required to get pricing. Fork it, modify it, self-host it.

---

### LimyeDB vs. Other Vector Databases

| Feature | LimyeDB | Pinecone | Qdrant | Milvus | Weaviate |
|---------|---------|----------|--------|--------|----------|
| **Open Source** | ✅ GPL v3 | ❌ Proprietary | ✅ Apache 2.0 | ✅ Apache 2.0 | ✅ BSD-3 |
| **Single Binary** | ✅ | N/A (SaaS) | ✅ | ❌ (etcd, MinIO, Pulsar) | ✅ |
| **Native Hybrid Search** | ✅ BM25 + Dense | ✅ | ⚠️ Sparse only | ✅ | ✅ |
| **DiskANN (Billion-scale SSD)** | ✅ | ❌ | ❌ | ✅ | ❌ |
| **Zero-GC HNSW** | ✅ mmap | N/A | ❌ | ❌ | ❌ |
| **Built-in Multi-Tenancy** | ✅ | ✅ | ⚠️ Limited | ✅ | ✅ |
| **Auto-Embedding** | ✅ | ❌ | ❌ | ❌ | ✅ |
| **SQL Interface** | ✅ | ❌ | ❌ | ✅ | ✅ GraphQL |
| **ColBERT MaxSim** | ✅ | ❌ | ✅ | ❌ | ❌ |
| **Self-Hosted** | ✅ | ❌ | ✅ | ✅ | ✅ |
| **Predictable Pricing** | ✅ Free | ❌ | ✅ | ✅ | ⚠️ |

---

### Who Should Use LimyeDB?

#### AI/ML Engineers Building RAG Pipelines
You need reliable, low-latency vector retrieval without becoming a database administrator. LimyeDB's single-binary deployment and automatic embedding let you focus on your AI application, not infrastructure.

#### Startups Shipping AI Products
You can't afford a dedicated DevOps team or unpredictable SaaS bills. LimyeDB gives you production-grade vector search that runs on a single VPS today and scales to a cluster tomorrow.

#### Enterprises Requiring Vendor Independence
You need to audit your infrastructure, comply with data residency requirements, and avoid lock-in. LimyeDB is fully self-hostable with no call-home telemetry.

#### Platform Teams Building Multi-Tenant AI SaaS
You need tenant isolation, usage quotas, and RBAC without building it yourself. LimyeDB's native multi-tenancy handles the hard parts.

#### Researchers and Educators
You want a performant vector database you can understand, modify, and extend. LimyeDB's clean Go codebase and comprehensive documentation make it hackable.

---

### The Bottom Line

LimyeDB exists because **the foundational infrastructure for AI should be accessible to everyone**—not just organizations with deep pockets, large ops teams, or willingness to accept vendor lock-in.

We built the vector database we wished existed. Now it's yours to use.

---

## Table of Contents

- [Key Features](#key-features)
- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [Installation](#installation)
- [Configuration](#configuration)
- [CLI Tool](#cli-tool)
- [API Reference](#api-reference)
- [Client SDKs](#client-sdks)
- [Clustering](#clustering)
- [Advanced Features](#advanced-features)
- [Observability](#observability)
- [Security](#security)
- [Performance](#performance)
- [Integrations](#integrations)
- [Contributing](#contributing)
- [License](#license)

---

## Key Features

### Core Capabilities

| Feature | Description |
|---------|-------------|
| **Hybrid Search via RRF** | Natively fuse dense semantic queries with sparse token frequencies (BM25) via Reciprocal Rank Fusion |
| **High-Performance HNSW** | O(1) Zero-Allocation Graph-Native memory pools bypassing Go garbage collection for extreme retrieval speeds |
| **Embedder Orchestrator** | Automated direct text-to-vector integration scaling seamlessly across OpenAI, Cohere, and local models natively |
| **ColBERT MaxSim Engine** | Advanced Late-Interaction similarity algorithms evaluating precise dot-product MultiVector matrices directly |
| **SQLite Payload Indexing**| Persistent embedded metadata structures mapping Document Nodes directly into disk-backed B-Trees securely |
| **Distributed Clustering** | Native Raft consensus and SWIM gossip protocol for masterless high availability and global persistence |
| **Multi-Tenancy & RBAC** | Strict granular isolation schemas featuring dynamic JSON Web Token (JWT) Authorization across REST and gRPC pipelines |
| **Vector SQL Interface** | Powerful and familiar declarative SQL-like query interfaces explicitly controlling embedded semantic properties |
| **Product & Binary Quantization** | Sub-space clustering and Hamming distance arrays reducing RAM bloat mathematically by over 32x natively |
| **Serverless S3 Tiering** | Offload memory-mapped clustered vectors intelligently separating persistent storage from internal compute |
| **CDC Mutation Webhooks** | Dispatcher pipeline publishing raw Insert/Delete events cleanly across decoupled HTTP REST topologies |
| **DiskANN Vamana Topologies** | Establish highly-connected pure SSD single-layer routing graphs dropping HNSW hierarchical scaling limits perfectly |
| **Event-Driven Mutators** | Live WebSocket data-streams reacting to vector insertion and clustering algorithms dynamically |
| **Real-Time Auto-Tuning** | Adapts index parameters dynamically ensuring top 99P recall guarantees continuously in production |

### Technical Highlights

- **Zero-Allocation HNSW Engine & DiskANN:** Eliminates GC pauses with raw memory-mapped NVMe bypass mechanisms and pure multi-terabyte SSD graphs natively handling billions of items.
- **Advanced AST Payload Filtering:** Execute complex JSON constraints securely backed natively by embedded SQLite B-Tree metadata mappings.
- **Generative Quantization Protocols:** Compresses multi-modal vector inputs structurally utilizing Subspace Product clustering and 1-bit BQ.
- **Enterprise Security:** JWT Bearer tokens alongside mTLS verification protocols for deep cross-cluster defense.
- **Serverless AWS SDK Integration:** Flush compute layers cleanly into Cold Storage AWS Object architectures completely asynchronously.
- **Prometheus Metrics:** Native `/metrics` endpoint for monitoring
- **OpenTelemetry Tracing:** Distributed tracing support
- **Security Hardened:** Constant-time token comparison, SSRF protection, path traversal prevention, decompression bomb limits, and strict file permissions

---

## Architecture

LimyeDB ships as two binaries:

| Binary | Description |
|--------|-------------|
| `limyedb` | The database server — REST + gRPC APIs, clustering, storage engine |
| `limyedb-cli` | Management CLI — import/export, backup/restore, collection management |

### Internal Package Structure

```
cmd/limyedb/          Server entry point
cmd/limyedb-cli/      CLI entry point
api/rest/              REST API (Gin) with middleware, auth, CORS
api/grpc/              gRPC API with streaming support
pkg/index/hnsw/        HNSW index (concurrent, mmap-backed)
pkg/index/ivf/         IVF (Inverted File) index
pkg/index/scann/       ScaNN anisotropic quantization index
pkg/index/diskann/     DiskANN Vamana graph index
pkg/index/payload/     SQLite-backed payload filtering
pkg/storage/mmap/      Memory-mapped vector and graph storage
pkg/storage/wal/       Write-ahead logging
pkg/storage/s3/        S3 tiered storage
pkg/cluster/           Raft consensus + SWIM gossip + consistent hashing
pkg/collection/        Collection management and sharding
pkg/hybrid/            BM25 + dense RRF fusion
pkg/quantization/      Product, scalar, and binary quantization
pkg/embedder/          OpenAI, Cohere, Google embedding orchestration
pkg/security/          API key generation, JWT, encryption
pkg/tenancy/           Multi-tenant RBAC isolation
pkg/realtime/          WebSocket event streaming
pkg/webhook/           CDC webhook dispatch with SSRF protection
pkg/cache/             Semantic result caching
pkg/ratelimit/         Token bucket rate limiting
pkg/backup/            Tar-based backup and restore
pkg/observability/     OpenTelemetry tracing
pkg/metrics/           Prometheus metrics
internal/raft/         Standalone Raft implementation
internal/pool/         Worker pool for parallel operations
```

---

## Quick Start

### Using Docker (Recommended)

```bash
# Pull and run LimyeDB
docker run -d \
  --name limyedb \
  -p 8080:8080 \
  -p 50051:50051 \
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
# Download latest release (Linux amd64)
curl -LO https://github.com/loreste/limyeDB/releases/latest/download/limyedb_$(curl -s https://api.github.com/repos/loreste/limyeDB/releases/latest | grep tag_name | cut -d'"' -f4 | sed 's/v//')_linux_amd64.tar.gz

# Extract and run
tar xzf limyedb_*_linux_amd64.tar.gz
./limyedb -rest :8080
```

Available platforms: `linux_amd64`, `linux_arm64`, `darwin_amd64`, `darwin_arm64`

---

## Installation

### Prerequisites

- Go 1.26+ (for building from source)
- Docker 20.10+ (for containerized deployment)
- Make (optional, for build automation)
- protoc (optional, for regenerating gRPC stubs)

### Build from Source

```bash
# Clone repository
git clone https://github.com/loreste/limyeDB.git
cd limyeDB

# Build both binaries
make build

# Run tests with race detection
make test

# The binaries will be at ./bin/
./bin/limyedb -help
./bin/limyedb-cli -help
```

### Makefile Targets

| Target | Description |
|--------|-------------|
| `make build` | Build `limyedb` and `limyedb-cli` into `bin/` |
| `make test` | Run all tests with `-race` |
| `make bench` | Run benchmarks on core packages |
| `make lint` | Run golangci-lint |
| `make fmt` | Format code with gofmt and goimports |
| `make proto` | Regenerate protobuf Go files |
| `make docker` | Build Docker image |
| `make clean` | Remove build artifacts |
| `make help` | Show all available targets |

---

## Configuration

### Server Flags

```bash
./limyedb \
  -config config.json \              # Path to configuration file
  -data ./data \                     # Data directory (default "./data")
  -rest :8080 \                      # REST API address (default ":8080")
  -grpc :50051 \                     # gRPC API address (default ":50051")
  -auth-token SECRET \               # Master bearer token for auth
  -tls-cert ./certs/server.crt \     # TLS certificate path
  -tls-key ./certs/server.key \      # TLS private key path
  -raft-node-id node1 \             # Raft node ID (default "node0")
  -raft-bind 0.0.0.0:7000 \         # Raft TCP bind address
  -raft-data ./raft-data \           # Raft data directory
  -raft-bootstrap \                  # Bootstrap as first cluster leader
  -raft-join http://node1:8080 \     # Join existing Raft cluster
  -version                           # Print version and exit
```

### Configuration File

See [`config.example.yaml`](config.example.yaml) for a complete annotated example. Summary:

```yaml
server:
  rest_addr: "0.0.0.0:8080"
  grpc_addr: "0.0.0.0:50051"
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
| `LIMYEDB_GRPC_ADDR` | gRPC API address | `0.0.0.0:50051` |
| `LIMYEDB_DATA_DIR` | Data directory | `./data` |
| `LIMYEDB_AUTH_TOKEN` | Authentication token | (none) |
| `LIMYEDB_LOG_LEVEL` | Log level | `info` |

---

## CLI Tool

LimyeDB includes `limyedb-cli` for managing collections, importing/exporting data, and performing backups from the command line.

### Usage

```bash
limyedb-cli [options] <command> [arguments]
```

### Options

| Flag | Description | Default |
|------|-------------|---------|
| `-host` | LimyeDB server URL | `http://localhost:8080` |
| `-api-key` | API key for authentication | (none) |
| `-timeout` | Request timeout | `30s` |

### Commands

| Command | Description |
|---------|-------------|
| `import <collection> <file>` | Import points from a JSON file (batched in groups of 100) |
| `export <collection> <file>` | Export all points from a collection to JSON |
| `collections` | List all collections |
| `create <name> <dimension>` | Create a new collection with cosine metric |
| `delete <name>` | Delete a collection |
| `info <name>` | Get collection details (point count, config) |
| `health` | Check server health status |
| `backup <output>` | Create a snapshot backup |
| `restore <input>` | Restore from a snapshot backup |
| `version` | Print CLI version |

### Examples

```bash
# Create a collection
limyedb-cli create my_collection 1536

# Import data from a JSON file
limyedb-cli import my_collection data.json

# Export collection to file
limyedb-cli export my_collection backup.json

# List all collections
limyedb-cli -host https://db.example.com -api-key secret collections

# Check server health
limyedb-cli health

# Backup and restore
limyedb-cli backup /tmp/backup.snapshot
limyedb-cli restore /tmp/backup.snapshot
```

### Import File Format

The JSON file for import should follow this structure:

```json
{
  "points": [
    {
      "id": "doc1",
      "vector": [0.1, 0.2, 0.3],
      "payload": {"title": "Example", "category": "test"}
    }
  ]
}
```

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

#### Semantic Hybrid Search (Multi-Modal Sparse + Dense via RRF)

Accelerate RAG retrieval pipelines by fusing keyword frequency with dense contextual embeddings gracefully:

```bash
curl -X POST http://localhost:8080/collections/documents/search  \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "vector": [0.1, 0.2, 0.3, ...],
    "sparse_query": { 
         "indices": [101, 403, 11200], 
         "values": [2.4, 0.8, 4.1] 
    },
    "limit": 10,
    "with_payload": true
  }'
```

#### Auto-Embedding Orchestration

Automatically convert raw text contexts into dense semantic matrices natively on the server without client-side parsing pipelines via OpenAI or Cohere:

```bash
curl -X POST http://localhost:8080/collections/documents/auto-embed \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "provider": "openai",
    "model": "text-embedding-3-small",
    "api_key": "YOUR_OPENAI_KEY",
    "source_fields": ["context", "title"],
    "points": [
      {
        "id": "doc1",
        "payload": {
          "title": "Machine Learning",
          "context": "AI algorithms scaling horizontally."
        }
      }
    ]
  }'
```

#### ColBERT Late-Interaction Search (MaxSim)

Evaluate query chunks across entire sequences of documents precisely using native MaxSim Dot-Product evaluations directly inside the HNSW indexes seamlessly parsing `MultiVectors`:

```bash
curl -X POST http://localhost:8080/collections/documents/search \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "vector": [0.1, ...],
    "multi_vector": [[0.1, 0.2], [0.3, 0.4]],
    "vector_name": "colbert",
    "limit": 10
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

### Health & Readiness

```bash
# Health check with component status
curl http://localhost:8080/health
# {"status":"healthy","version":"0.2.0","uptime":"2h15m","components":{"storage":"healthy","collections":{"count":5,"status":"healthy"}}}

# Readiness probe (for Kubernetes)
curl http://localhost:8080/readiness
# {"status":"ready"}
```

### Request Tracing

Every request receives a unique `X-Request-Id` header for end-to-end tracing:

```bash
curl -v http://localhost:8080/health 2>&1 | grep X-Request-Id
# < X-Request-Id: 7f3a8b2c-4d5e-6f7a-8b9c-0d1e2f3a4b5c

# Pass your own request ID for correlation
curl -H "X-Request-Id: my-trace-123" http://localhost:8080/collections
```

### Error Responses

All API errors return a consistent structured format:

```json
{
  "error": {
    "code": "NOT_FOUND",
    "message": "collection 'missing' not found",
    "request_id": "7f3a8b2c-4d5e-6f7a-8b9c-0d1e2f3a4b5c"
  }
}
```

Common error codes: `NOT_FOUND`, `ALREADY_EXISTS`, `INVALID_REQUEST`, `INTERNAL_ERROR`

### Rate Limiting

Per-endpoint rate limiting is available (opt-in via server configuration):

| Endpoint Pattern | Default Rate |
|-----------------|-------------|
| Search / Recommend / Discover | 100 req/s per IP |
| Read operations | 1000 req/s per IP |
| All other endpoints | 500 req/s per IP |

Health, readiness, and metrics endpoints are exempt from rate limiting.

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
  -raft-node-id node1 \
  -raft-bind 192.168.1.1:7000 \
  -raft-data /data/node1/raft \
  -raft-bootstrap \
  -rest 192.168.1.1:8080 \
  -data /data/node1
```

#### Node 2 (Join Cluster)

```bash
./limyedb \
  -raft-node-id node2 \
  -raft-bind 192.168.1.2:7000 \
  -raft-data /data/node2/raft \
  -raft-join http://192.168.1.1:8080 \
  -rest 192.168.1.2:8080 \
  -data /data/node2
```

#### Node 3 (Join Cluster)

```bash
./limyedb \
  -raft-node-id node3 \
  -raft-bind 192.168.1.3:7000 \
  -raft-data /data/node3/raft \
  -raft-join http://192.168.1.1:8080 \
  -rest 192.168.1.3:8080 \
  -data /data/node3
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

| Metric | Type | Description |
|--------|------|-------------|
| `limyedb_request_duration_seconds` | Histogram | Request latency by method, path, and status code |
| `limyedb_request_total` | Counter | Total requests by method, path, and status code |
| `limyedb_search_latency_seconds` | Histogram | Search operation latency |
| `limyedb_search_total` | Counter | Total search requests |
| `limyedb_insert_total` | Counter | Total insert operations |
| `limyedb_vectors_total` | Gauge | Total vectors stored |
| `limyedb_collections_total` | Gauge | Number of collections |
| `limyedb_raft_state` | Gauge | Raft cluster state |
| `limyedb_gossip_members` | Gauge | Active gossip members |

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

LimyeDB uses Go's `slog` structured logging with JSON output by default. Log level is controlled via configuration.

---

## Security

### Authentication

#### API Key Authentication

```bash
./limyedb -auth-token YOUR_SECRET_TOKEN

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
  -tls-cert /certs/server.crt \
  -tls-key /certs/server.key
```

#### mTLS (Mutual TLS)

```bash
./limyedb \
  -tls-cert /certs/server.crt \
  -tls-key /certs/server.key \
  -tls-client-ca /certs/ca.crt
```

### Security Hardening

LimyeDB includes multiple layers of security hardening:

| Protection | Description |
|-----------|-------------|
| **Constant-time token comparison** | API keys and bearer tokens validated with `crypto/subtle.ConstantTimeCompare` to prevent timing attacks |
| **SSRF protection** | Webhook URLs validated against private IP ranges (127.0.0.0/8, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16), localhost, and non-HTTP schemes |
| **Path traversal prevention** | All file paths sanitized with `filepath.Clean` and validated against base directories |
| **Decompression bomb limits** | Backup metadata reads capped at 10MB; file extraction limited to prevent resource exhaustion |
| **Cryptographic RNG** | All security-sensitive randomness uses `crypto/rand` (API keys, webhook IDs, cluster protocols) |
| **Strict file permissions** | Data files created with 0600, directories with 0750 |
| **Parameterized SQL** | All SQLite payload queries use parameterized arguments |
| **CORS origin validation** | Configurable allowed origins (no wildcard in production) |
| **Integer overflow protection** | Safe conversion helpers with bounds checking for all unsafe casts |
| **Zip Slip protection** | Archive extraction validated with `filepath.Rel` to prevent directory escape |
| **HTTP client timeouts** | All outbound HTTP clients configured with explicit timeouts |
| **Rate limiting** | Token bucket rate limiter with configurable limits per endpoint |

### Network Security

- Use private networks for cluster communication
- Encrypt inter-node traffic with TLS
- Configure `AllowedOrigins` for CORS in production deployments

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

A Helm chart is included in the repository:

```bash
# Install from local chart
helm install limyedb ./deploy/helm/limyedb \
  --namespace limyedb \
  --create-namespace \
  --set persistence.size=100Gi

# With custom values
helm install limyedb ./deploy/helm/limyedb \
  --namespace limyedb \
  --create-namespace \
  -f my-values.yaml
```

See [`deploy/helm/limyedb/values.yaml`](deploy/helm/limyedb/values.yaml) for all configurable options including auth, TLS, persistence, and resource limits.

---

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

### Development Setup

```bash
# Clone
git clone https://github.com/loreste/limyeDB.git
cd limyeDB

# Build both binaries
make build

# Run tests with race detection
make test

# Run linter
make lint

# Format code
make fmt
```

### Pre-Commit Hooks

Install pre-commit hooks to run gofmt, golangci-lint, and go vet before each commit:

```bash
pip install pre-commit
pre-commit install
```

### Testing

```bash
# Unit tests with race detection
make test

# Benchmarks
make bench

# Integration tests (starts real cluster nodes)
LIMYEDB_INTEGRATION=1 go test -race -timeout=10m ./pkg/...

# Goroutine leak tests
go test -race ./pkg/webhook/... ./pkg/cluster/... ./pkg/cache/... ./pkg/ratelimit/... -run Leak

# Race condition stress tests
go test -race ./pkg/collection/... ./pkg/index/hnsw/... ./pkg/cache/... ./pkg/cluster/... -run Race
```

### Code Standards

- Run `make lint` and `make fmt` before submitting PRs
- Add tests for new features (target >70% coverage for new packages)
- Use `errors.Is()` for error comparisons, `%w` for error wrapping
- Use `crypto/rand` for any security-sensitive randomness
- Use `filepath.Clean` and validate paths for any file operations

---

## License

LimyeDB is licensed under the [GNU General Public License v3.0](LICENSE).

---

## Support

- **GitHub Issues**: [https://github.com/loreste/limyeDB/issues](https://github.com/loreste/limyeDB/issues)
- **Security Issues**: See [SECURITY.md](SECURITY.md) for responsible disclosure
- **Changelog**: See [CHANGELOG.md](CHANGELOG.md) for release history
- **Releases**: [https://github.com/loreste/limyeDB/releases](https://github.com/loreste/limyeDB/releases)
