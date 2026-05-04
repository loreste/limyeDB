# LimyeDB - Open Source Vector Database for GenAI, RAG & LLMs

[![CI](https://github.com/loreste/limyeDB/actions/workflows/ci.yml/badge.svg)](https://github.com/loreste/limyeDB/actions/workflows/ci.yml)
[![CodeQL](https://github.com/loreste/limyeDB/actions/workflows/codeql.yml/badge.svg)](https://github.com/loreste/limyeDB/actions/workflows/codeql.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/limyedb/limyedb)](https://goreportcard.com/report/github.com/limyedb/limyedb)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)
[![Release](https://img.shields.io/github/v/release/loreste/limyeDB)](https://github.com/loreste/limyeDB/releases/latest)

**LimyeDB** is a fast, highly-available **open-source vector database** engineered specifically for the next generation of AI applications. Built entirely from scratch in Go, it is a semantic storage engine designed to power **Retrieval-Augmented Generation (RAG)**, large language model (LLM) memory arrays, and predictive similarity matching with low-millisecond retrieval latency.

LimyeDB distinguishes itself by natively supporting **Hybrid Search**—combining a memory-mapped **HNSW** (Hierarchical Navigable Small World) dense vector index with a fast local **BM25 inverted index** for sparse vectors. It fuses these multi-modal queries using **Reciprocal Rank Fusion (RRF)**. LimyeDB deploys as a single binary or as a Raft-replicated high-availability cluster.

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
- **Real-time subscriptions** for reactive user interfaces
- **Built-in embedding** to avoid separate ETL pipelines
- **Authentication and authorization** with per-collection access control

Getting all of this typically means stitching together multiple systems—each with its own failure modes, version conflicts, and operational overhead.

---

### Our Mission: Production-Grade AI Infrastructure for Everyone

**LimyeDB was created to prove that powerful doesn't have to mean complicated.**

We built the vector database we wished existed: one that a solo developer can run on a laptop for prototyping, and that replicates across a Raft-backed high-availability cluster when production calls for it. One that delivers production-grade capabilities without enterprise-grade complexity or cost.

---

### What Makes LimyeDB the Best Open Source Vector Database

#### 🚀 Single Binary Deployment — No Dependencies, No DevOps Nightmare

LimyeDB compiles to a **single statically-linked Go binary**. No JVM tuning. No external PostgreSQL, etcd, or Redis instances. No message queues. No container orchestration required for basic deployments.

```bash
# That's it. You now have a production-ready vector database.
./limyedb -data ./my-vectors -rest :8080
```

This isn't just convenience—it's **operational sanity**. Fewer moving parts means fewer failure modes, simpler debugging, and faster disaster recovery.

#### ⚡ mmap-Backed HNSW — Predictable Latency

Most vector databases written in Go or Java suffer from **garbage collection pauses** that spike P99 latencies unpredictably. LimyeDB's HNSW implementation reduces GC pressure via:

- **Memory-mapped graph storage** keeps the connection arrays off the Go heap
- **Pooled visited-set buffers** during search reduce per-query allocations
- **Concurrent reads** with a single build lock for inserts
- **SIMD-accelerated distance calculations** on ARM64 (NEON) and x86-64 (AVX2)

The result: low-millisecond P99 search latencies that stay stable under load (see [benchmarks](#benchmarks)).

#### 🔍 Native Hybrid Search — Semantic + Keyword in One Query

While other vector databases bolt on keyword search as an afterthought, **LimyeDB was architected from day one for hybrid retrieval**:

| Component | Technology | Purpose |
|-----------|------------|---------|
| Dense Vectors | HNSW (mmap-backed graph) | Semantic similarity search |
| Sparse Vectors | BM25 | Keyword and lexical matching |
| Fusion | Reciprocal Rank Fusion (RRF) | Mathematically optimal result merging |

This matters because **pure vector search fails on proper nouns, product codes, and exact phrases**. Hybrid search delivers better relevance for real-world queries without requiring multiple systems or post-processing.

```bash
# Single query combining semantic understanding with keyword precision
curl -X POST http://localhost:8080/collections/docs/search/v2 \
  -d '{
    "vector": [0.1, 0.2, ...],
    "sparse_vector": {"indices": [101, 403], "values": [2.4, 0.8]},
    "limit": 10
  }'
```

#### 🔐 JWT-Based RBAC with Per-Collection ACLs

LimyeDB ships with built-in authentication and authorization:

- **JWT bearer tokens** with configurable claims (global admin or per-collection ACL)
- **API key authentication** for service-to-service traffic
- **Per-collection access control** — read/write permissions scoped to specific collections via JWT claims
- **Constant-time token comparison** to prevent timing attacks

No call-home licensing, no paywalled enterprise edition for auth.

#### 🔒 Security-Hardened from the Ground Up

AI systems increasingly process sensitive data—customer conversations, proprietary documents, personal information. LimyeDB implements **defense-in-depth security**:

| Protection | Implementation |
|------------|----------------|
| Authentication | Bearer tokens, API keys, JWT with RBAC |
| Encryption | TLS 1.3 for client connections |
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

#### 🌐 Raft-Replicated High Availability

Run a 3-node cluster for HA via [Hashicorp Raft](https://github.com/hashicorp/raft):

- **Strongly consistent writes** through the Raft leader
- **Leader election and automatic failover** on node loss
- **FSM-replicated collection metadata and points** across nodes
- **Snapshot-based recovery** for fast catch-up on rejoin

A single binary deploys as either a standalone node or a member of a Raft cluster.

#### 📈 Production Observability Built-In

Monitor everything with native integrations:

- **Prometheus Metrics** at `/metrics` (latencies, throughput, index stats)
- **Structured JSON Logging** with configurable levels via `slog`
- **Health and Readiness Endpoints** for Kubernetes probes
- **Grafana Dashboard** included in the repository

#### 🆓 Truly Open Source — No Bait-and-Switch

LimyeDB is released under **GPL v3**. Everything is open:

- ✅ Raft-replicated high availability
- ✅ JWT-based RBAC and per-collection ACLs
- ✅ HNSW vector index (mmap-backed graph)
- ✅ Hybrid search (dense + BM25 sparse, RRF fusion)
- ✅ Auto-embedding via OpenAI / Cohere
- ✅ Server-side snapshots
- ✅ TLS and security hardening
- ✅ Prometheus metrics

No "enterprise edition" holding features hostage. No phone call required to get pricing. Fork it, modify it, self-host it.

---

### LimyeDB vs. Other Vector Databases

| Feature | LimyeDB | Pinecone | Qdrant | Milvus | Weaviate |
|---------|---------|----------|--------|--------|----------|
| **Open Source** | ✅ GPL v3 | ❌ Proprietary | ✅ Apache 2.0 | ✅ Apache 2.0 | ✅ BSD-3 |
| **Single Binary** | ✅ | N/A (SaaS) | ✅ | ❌ (etcd, MinIO, Pulsar) | ✅ |
| **Native Hybrid Search** | ✅ BM25 + Dense | ✅ | ⚠️ Sparse only | ✅ | ✅ |
| **mmap-Backed HNSW** | ✅ | N/A | ❌ | ❌ | ❌ |
| **Auto-Embedding** | ✅ OpenAI / Cohere | ❌ | ❌ | ❌ | ✅ |
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
| **Hybrid Search via RRF** | Fuse dense semantic queries with sparse BM25 scores using Reciprocal Rank Fusion |
| **mmap-Backed HNSW** | Memory-mapped graph storage with pooled visited-set buffers; SIMD distance kernels (NEON/AVX2) |
| **Embedder Orchestrator** | Server-side text-to-vector via OpenAI and Cohere |
| **SQLite Payload Indexing** | Persistent metadata indexed in SQLite with parameterized JSON-path queries |
| **Raft-Replicated HA** | Hashicorp Raft replicates collection metadata and points across a 3-node cluster |
| **JWT-Based RBAC** | Per-collection access control via JWT claims, enforced consistently across REST and gRPC |
| **Product & Binary Quantization** | Product, scalar, and 1-bit binary quantization for memory footprint reduction |
| **S3 Archive Storage** | Push collection snapshots to S3 for cold storage and restore on demand |
| **CDC Mutation Webhooks** | HTTP webhook delivery of insert/update/delete events with SSRF-validated URLs |
| **WAL Persistence** | Write-ahead log captures every mutation; replayed on startup for crash recovery |

### Technical Highlights

- **mmap-Backed HNSW Engine:** Reduces GC pressure by storing graph connections in a memory-mapped file; pooled visited-set buffers and SIMD distance kernels.
- **AST Payload Filtering:** JSON predicate AST compiled into parameterized SQLite queries for safe, complex `WHERE`-style filters.
- **Quantization:** Product, scalar, and 1-bit binary quantization for memory-footprint reduction.
- **Authentication:** JWT bearer tokens and API keys with constant-time comparison.
- **AWS SDK Integration:** Push collection snapshots to S3 buckets for archive and disaster recovery.
- **Prometheus Metrics:** Native `/metrics` endpoint for monitoring.
- **Security Hardened:** Constant-time token comparison, SSRF protection on webhooks, path traversal prevention, decompression-bomb limits, and strict file permissions.

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
pkg/index/payload/     SQLite-backed payload filtering
pkg/storage/mmap/      Memory-mapped vector and graph storage
pkg/storage/wal/       Write-ahead logging
pkg/storage/s3/        S3 tiered storage
pkg/cluster/           Hashicorp Raft consensus + gossip membership
pkg/collection/        Collection and shard management
pkg/index/sparse/      BM25 sparse index + RRF fusion
pkg/quantization/      Product, scalar, and binary quantization
pkg/embedder/          OpenAI, Cohere, Google embedding orchestration
pkg/auth/              JWT + per-collection RBAC
pkg/security/          API key generation, encryption
pkg/realtime/          WebSocket event streaming
pkg/webhook/           CDC webhook dispatch with SSRF protection
pkg/cache/             Semantic result caching
pkg/backup/            Tar-based backup and restore
pkg/metrics/           Prometheus metrics
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

# To stop gracefully, press Ctrl+C or send SIGTERM
# This ensures all data is safely persisted to disk
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
```

### Server Flags

`limyedb` is configured via command-line flags or a YAML config file (`-config <path>`).

| Flag | Description | Default |
|------|-------------|---------|
| `-config` | Path to YAML configuration file | (none) |
| `-data` | Data directory | `./data` |
| `-rest` | REST API listen address | `:8080` |
| `-grpc` | gRPC API listen address | `:50051` |
| `-auth-token` | Bearer token for API authentication (also used as JWT signing key) | (none, auth disabled) |
| `-tls-cert` | Path to TLS certificate file | (none) |
| `-tls-key` | Path to TLS private key file | (none) |
| `-raft-bind` | Raft TCP bind address (enables clustering) | (none) |
| `-raft-data` | Raft data directory | (none) |
| `-raft-node-id` | Raft node ID | `node0` |
| `-raft-bootstrap` | Bootstrap this node as the cluster leader | `false` |
| `-raft-join` | Address of an existing Raft node to join | (none) |

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
| `backup` | Trigger a server-side snapshot; prints the new snapshot ID |
| `restore <snapshot-id>` | Restore the server from an existing snapshot ID |
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

# Backup and restore (server-side snapshot)
limyedb-cli backup
# -> "Backup created: snap-1730000000 at 2025-...
limyedb-cli restore snap-1730000000
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
curl -X POST http://localhost:8080/collections/documents/search/v2 \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "vector": [0.1, 0.2, 0.3, ...],
    "sparse_vector": {
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

LimyeDB uses Hashicorp Raft for cluster coordination:

1. **Raft Consensus**: All writes flow through the leader and are committed once a quorum acknowledges
2. **Snapshot-based Recovery**: Followers catch up via Raft log + snapshot when rejoining
3. **Member Discovery**: Static peer list or `-raft-join` to bootstrap a follower against an existing leader

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

LimyeDB uses Hashicorp Raft for replication. Writes are accepted by the leader and committed once a quorum of followers acknowledges; reads currently hit the local FSM on the receiving node and may return slightly stale data on followers (no `VerifyLeader` barrier yet).

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

### Persistence & Recovery

LimyeDB provides crash-safe persistence with minimal data loss:

#### Data Flow

```
WRITE PATH:
  Client Insert → WAL.Write() → Collection.Insert() → HNSW.Insert()

STARTUP PATH:
  main() → LoadCollections() → LoadIndexMetadata() → ReplayWAL() → Ready

SHUTDOWN PATH:
  Signal → StopServers → SyncWAL → SaveIndexMetadata → Close
```

#### Graceful Shutdown

LimyeDB handles shutdown signals gracefully to ensure data integrity. Always use proper shutdown methods:

```bash
# Recommended: Send SIGTERM or SIGINT
kill -TERM <pid>

# Or press Ctrl+C if running in foreground
^C

# Docker
docker stop limyedb

# Docker Compose
docker-compose stop

# Kubernetes
kubectl delete pod limyedb-0
```

**What happens during graceful shutdown:**

1. **Stop accepting requests** - REST and gRPC servers stop accepting new connections
2. **Drain in-flight requests** - Existing requests complete (up to configured timeout)
3. **Sync WAL** - Write-ahead log is flushed to disk
4. **Save index metadata** - HNSW entry points, connections, and ID mappings are persisted
5. **Close resources** - File handles, mmap regions, and network connections are released

**Warning:** Avoid using `kill -9` (SIGKILL) as it prevents graceful shutdown and may result in data loss of recent writes.

#### Configuration

```yaml
storage:
  wal:
    enabled: true
    dir: "./data/wal"
    segment_size_mb: 64
    sync_on_write: true  # fsync after each write for maximum durability
```

#### Data Loss Analysis

| Scenario | Data Loss |
|----------|-----------|
| Graceful shutdown (Ctrl+C, SIGTERM) | None |
| Kill -9 / SIGKILL | Last ~1 second (unflushed WAL buffer) |
| With `sync_on_write: true` | Minimal (OS buffer only) |
| Power failure | Up to last WAL segment |

#### What Gets Persisted

| Component | Location | Contents |
|-----------|----------|----------|
| WAL Records | `./data/wal/` | Every Insert, Delete, and Upsert operation |
| Index Metadata | `./data/collections/{name}/index.meta` | Entry point, max level, node connections, deleted flags, ID-to-index mapping |
| Collection Config | `./data/collections/{name}/meta.json` | Dimension, metric, HNSW config |
| Graph Mmap | `./data/collections/{name}/graph.mmap` | Memory-mapped HNSW connections (if mmap enabled) |

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

#### JWT with Per-Collection RBAC

JWT claims encode either global admin status or a per-collection action list under the `limyedb_permissions` claim:

```json
{
  "sub": "service-account-1",
  "exp": 1735689600,
  "limyedb_permissions": {
    "global_admin": false,
    "collections": {
      "docs":       ["READ_ONLY"],
      "embeddings": ["COLLECTION_ADMIN"]
    }
  }
}
```

Tokens are signed with the same secret as `auth_token` (HS256). Requests against a collection not present in the `collections` map receive 403.

### TLS

```bash
./limyedb \
  -tls-cert /certs/server.crt \
  -tls-key /certs/server.key
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

### Performance Optimizations

LimyeDB includes several built-in performance optimizations:

#### Search Result Caching

Repeated searches with the same query vector are cached for 5 minutes (configurable), providing near-instant responses for hot queries:

- Cache capacity: 10,000 queries per collection
- TTL: 5 minutes (auto-invalidated on writes)
- Cache hit = ~0ms latency vs ~2-10ms for uncached

#### Batch WAL Writes

Batch inserts use optimized WAL writes with a single fsync per batch instead of per-record:

```bash
# Single insert: 1 fsync per insert (~1-5ms each)
# Batch of 1000: 1 fsync total (~5-10ms for entire batch)
```

#### Async WAL Mode (Optional)

For maximum write throughput with slightly higher data loss risk:

```yaml
storage:
  wal:
    enabled: true
    sync_on_write: false        # Disable per-write fsync
    async_enabled: true         # Enable async write queue
    async_batch_size: 100       # Batch up to 100 records
    async_interval_ms: 10       # Flush every 10ms max
```

| Mode | Write Latency | Data Loss Risk |
|------|---------------|----------------|
| sync_on_write: true | ~1-5ms | Minimal |
| sync_on_write: false | ~0.1ms | Up to 1 second |
| async_enabled: true | ~0.01ms | Up to async_interval_ms |

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
