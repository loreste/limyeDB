# LimyeDB Installation Guide

LimyeDB is an open-source vector database written in Go. It compiles to a single statically-linked binary with no external runtime dependencies.

## What ships today

| Feature | Status |
|---------|--------|
| **Single binary** | ✅ — `cmd/limyedb` and `cmd/limyedb-cli` |
| **HNSW vector index** | ✅ — mmap-backed graph, low-millisecond p99 (see [benchmarks](../README.md#benchmarks)) |
| **Hybrid search** | ✅ — dense HNSW + sparse BM25 fused via Reciprocal Rank Fusion at `/collections/:name/search/v2` |
| **JWT auth + per-collection RBAC** | ✅ — required by default; see [Authentication](#authentication) |
| **TLS for REST/gRPC** | ✅ — pass `-tls-cert` and `-tls-key` |
| **Auto-embedding** | ✅ — OpenAI and Cohere providers wired in `pkg/embedder` |
| **Raft replication** | ✅ — Hashicorp Raft, single Raft group across nodes |
| **WAL + crash-safe persistence** | ✅ — replay on startup, atomic snapshot publish |
| **Prometheus metrics** | ✅ — exposed at `/metrics` |
| **Server-side snapshots** | ✅ — `POST /snapshots`, restore via `POST /snapshots/:id/restore` |
| **CDC webhooks** | ✅ — SSRF-validated; `POST /collections/:name/webhooks` |

> **Note on index type**: collections use HNSW today. The repo also contains
> `pkg/index/{diskann,ivf,scann}` packages, but they have no production
> wiring — `index_type: diskann|ivf|scann` is not honored by the collection
> manager. Treat them as experimental, in-tree, not yet selectable.

---

## Quick Start

```bash
# Download and run. Auth is required by default; pick a real secret.
curl -LO https://github.com/loreste/limyeDB/releases/latest/download/limyedb_linux_amd64.tar.gz
tar xzf limyedb_linux_amd64.tar.gz
TOKEN="$(openssl rand -hex 32)"
./limyedb -rest :8080 -auth-token "$TOKEN"

# Verify
curl http://localhost:8080/health
```

To run without authentication (development/local-only — **NOT** for any host
reachable from an untrusted network), pass `-allow-anonymous` instead:

```bash
./limyedb -rest :8080 -allow-anonymous
```

---

## Installation Methods

### 1. Docker

```bash
docker pull limyedb/limyedb:latest

docker run -d \
  --name limyedb \
  -p 8080:8080 \
  -p 50051:50051 \
  -v limyedb_data:/data \
  -e AUTH_TOKEN \
  limyedb/limyedb:latest \
  -auth-token "$AUTH_TOKEN"
```

### 2. Compile From Source

```bash
git clone https://github.com/loreste/limyeDB.git
cd limyeDB

# Build both binaries
make build

# Or build manually
go build -o bin/limyedb ./cmd/limyedb
go build -o bin/limyedb-cli ./cmd/limyedb-cli

# Run
./bin/limyedb -rest :8080 -grpc :50051 -auth-token "$(openssl rand -hex 32)"
```

### 3. Kubernetes (Helm)

A Helm chart is provided in `deploy/helm/`. Each pod must receive an
`-auth-token` value (typically from a Kubernetes secret). Cluster
membership is configured manually via `-raft-bootstrap` on the first
node and `-raft-join http://<leader>:8080` on subsequent nodes — there
is **no** automatic peer discovery.

```yaml
storage:
  size: 50Gi
resources:
  requests:
    memory: "16Gi"
    cpu: "4"
auth:
  tokenSecretName: limyedb-auth
```

---

## Platform Downloads

| Platform | Architecture | Download |
|----------|--------------|----------|
| Linux | x86_64 (amd64) | `limyedb_linux_amd64.tar.gz` |
| Linux | ARM64 | `limyedb_linux_arm64.tar.gz` |
| macOS | Apple Silicon | `limyedb_darwin_arm64.tar.gz` |
| macOS | Intel | `limyedb_darwin_amd64.tar.gz` |

> ARM64 builds run with the SIMD distance kernels disabled (the hand-written
> NEON assembly produced incorrect results and is currently routed through
> the scalar fallback). x86 amd64 uses AVX2-accelerated kernels.

All binaries: https://github.com/loreste/limyeDB/releases

---

## System Requirements

### Minimum
- **CPU**: 2 cores
- **RAM**: 4 GB (small datasets, <100K vectors)
- **Disk**: 10 GB SSD

### Recommended for production
- **CPU**: 8+ cores (HNSW index build is CPU-intensive)
- **RAM**: large enough for the dense graph (HNSW is in-memory)
- **Disk**: NVMe SSD (mmap-backed graph benefits significantly)

### Memory estimation (HNSW, all in-memory)

| Vectors | Dimensions | Estimated RAM |
|---------|------------|---------------|
| 100K | 384 | ~500 MB |
| 1M | 384 | ~5 GB |
| 10M | 384 | ~50 GB |

---

## Server flags

`limyedb` is configured via flags or a YAML config file (`-config <path>`).

| Flag | Description | Default |
|------|-------------|---------|
| `-config` | Path to YAML configuration file | (none) |
| `-data` | Data directory | `./data` |
| `-rest` | REST API listen address | `:8080` |
| `-grpc` | gRPC API listen address | `:50051` |
| `-auth-token` | Bearer token for API auth (also JWT signing key). Required unless `-allow-anonymous`. | (none) |
| `-allow-anonymous` | Run without authentication. NOT recommended. | `false` |
| `-tls-cert` | TLS certificate path | (none) |
| `-tls-key` | TLS private key path | (none) |
| `-raft-bind` | Raft TCP bind address (enables clustering) | (none) |
| `-raft-data` | Raft data directory | (none) |
| `-raft-node-id` | Raft node ID | `node0` |
| `-raft-bootstrap` | Bootstrap this node as the cluster leader | `false` |
| `-raft-join` | Address of an existing Raft node to join | (none) |
| `-version` | Print version and exit | `false` |

There is no environment-variable layer; every option above goes through a
flag or the YAML config file.

---

## Authentication

LimyeDB **refuses to start without an explicit auth decision**. You must
pass `-auth-token <secret>` to enable JWT/Bearer auth, or `-allow-anonymous`
to opt out (development only).

The same `auth-token` value serves two purposes:

1. **Static bearer token** — `Authorization: Bearer $AUTH_TOKEN` is accepted
   and grants global admin permissions.
2. **JWT signing key** — JWTs you mint (HS256) with this secret will be
   accepted; their `limyedb_permissions` claim governs per-collection
   read/write access. See the [Security section of the README](../README.md#security)
   for the claim shape.

For TLS:

```bash
./limyedb \
  -tls-cert /etc/ssl/limyedb.crt \
  -tls-key /etc/ssl/limyedb.key \
  -auth-token "$AUTH_TOKEN"
```

---

## Verifying installation

### Health check (no auth required)

```bash
curl http://localhost:8080/health
```

### Create a test collection

```bash
curl -X POST http://localhost:8080/collections \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "test", "dimension": 128, "metric": "cosine"}'
```

### Insert and search

```bash
# Insert
curl -X PUT http://localhost:8080/collections/test/points \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"points": [{"id": "1", "vector": [0.1, 0.2, ...]}]}'

# Search
curl -X POST http://localhost:8080/collections/test/search \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"vector": [0.1, 0.2, ...], "limit": 5}'

# Strong-consistency read (route to Raft leader)
curl -X POST "http://localhost:8080/collections/test/search?consistent=true" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"vector": [0.1, 0.2, ...], "limit": 5}'
```

---

## Next steps

- [Getting Started Tutorial](tutorials/getting_started.md)
- [Clustering Guide](clustering.md)
- [Performance Tuning](performance_tuning.md)
