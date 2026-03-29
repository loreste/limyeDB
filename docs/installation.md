# LimyeDB Installation Guide

LimyeDB is the **open-source vector database** built for production AI applications. Written entirely in Go, it compiles to a single statically-linked binary with zero external dependencies—no JVM, no Python runtime, no external databases required.

## Why Choose LimyeDB?

| Feature | Benefit |
|---------|---------|
| **Single Binary** | Download and run—no complex deployment topology |
| **Zero-GC HNSW** | Sub-millisecond P99 latency without garbage collection pauses |
| **Native Hybrid Search** | BM25 + dense vectors with Reciprocal Rank Fusion built-in |
| **DiskANN Support** | Billion-scale vector search directly from SSD |
| **Multi-Tenancy** | RBAC, tenant isolation, and quotas out of the box |
| **Auto-Embedding** | Direct integration with OpenAI, Cohere, and local models |
| **Production Security** | mTLS, JWT auth, SSRF protection, parameterized queries |

---

## Quick Start

```bash
# Download and run in 30 seconds
curl -LO https://github.com/loreste/limyeDB/releases/latest/download/limyedb_linux_amd64.tar.gz
tar xzf limyedb_linux_amd64.tar.gz
./limyedb -rest :8080

# Verify it's running
curl http://localhost:8080/health
```

---

## Installation Methods

### 1. Docker (Recommended for Production)

The most resilient way to scale LimyeDB in production is via Docker. The image mounts a local persistence volume to protect HNSW indexes.

```bash
docker pull limyedb/limyedb:latest

docker run -d \
  --name limyedb_core \
  -p 8080:8080 \
  -p 50051:50051 \
  -v limyedb_data:/data \
  limyedb/limyedb:latest
```

## 2. Compile From Source

If you require custom forks or architecture optimizations (e.g., Apple Silicon M4 / AVX-512 extensions), cloning and compiling natively is optimal:

```bash
git clone https://github.com/loreste/limyeDB.git
cd limyeDB

# Build both binaries
make build

# Or build manually
go build -o bin/limyedb ./cmd/limyedb
go build -o bin/limyedb-cli ./cmd/limyedb-cli

# Run
./bin/limyedb -rest :8080 -grpc :50051
```

## 3. Kubernetes Deployment (Helm)

For distributed HNSW meshes, deploy using Kubernetes.

Create a `values.yaml`:
```yaml
storage:
  size: 50Gi
resources:
  requests:
    memory: "16Gi"
    cpu: "4"
```

*Note: LimyeDB nodes automatically discover each other in K8s using our built-in Consul/K8s DNS resolver on port 7946.*

## 4. Production Security (API, JWT & TLS)

LimyeDB Phase 2 introduced Granular RBAC. You can secure the instance instantly using runtime flags and JSON Web Tokens:
```bash
./limyedb \
    --auth-token="<GLOBAL_ADMIN_JWT_OR_STATIC_SECRET>" \
    --tls-cert="/etc/ssl/limyedb.crt" \
    --tls-key="/etc/ssl/limyedb.key"
```

*Note: For multi-tenant clusters, requests must pass an `Authorization: Bearer <TOKEN>` header where the JWT contains a `limyedb_permissions` claim mapping strings like `READ_ONLY` or `COLLECTION_ADMIN`.*

---

## 5. Platform-Specific Downloads

| Platform | Architecture | Download |
|----------|--------------|----------|
| Linux | x86_64 (amd64) | `limyedb_linux_amd64.tar.gz` |
| Linux | ARM64 | `limyedb_linux_arm64.tar.gz` |
| macOS | Apple Silicon (M1/M2/M3/M4) | `limyedb_darwin_arm64.tar.gz` |
| macOS | Intel | `limyedb_darwin_amd64.tar.gz` |
| Windows | x86_64 | `limyedb_windows_amd64.zip` |

All binaries are available at: https://github.com/loreste/limyeDB/releases

---

## 6. System Requirements

### Minimum Requirements
- **CPU**: 2 cores
- **RAM**: 4GB (for small datasets <100K vectors)
- **Disk**: 10GB SSD

### Recommended for Production
- **CPU**: 8+ cores (HNSW index building is CPU-intensive)
- **RAM**: 32GB+ (or use DiskANN for larger-than-RAM datasets)
- **Disk**: NVMe SSD (significantly improves mmap performance)
- **Network**: Low-latency connection for cluster deployments

### Memory Estimation

| Vectors | Dimensions | Estimated RAM (HNSW) | With DiskANN |
|---------|------------|---------------------|--------------|
| 100K | 384 | ~500MB | ~50MB |
| 1M | 384 | ~5GB | ~500MB |
| 10M | 384 | ~50GB | ~5GB |
| 100M | 384 | ~500GB | ~50GB |
| 1B | 384 | N/A (use DiskANN) | ~500GB SSD |

---

## 7. Configuration Options

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `LIMYEDB_REST_ADDR` | REST API bind address | `0.0.0.0:8080` |
| `LIMYEDB_GRPC_ADDR` | gRPC API bind address | `0.0.0.0:50051` |
| `LIMYEDB_DATA_DIR` | Data storage directory | `./data` |
| `LIMYEDB_AUTH_TOKEN` | Master authentication token | (none) |
| `LIMYEDB_LOG_LEVEL` | Logging level (debug/info/warn/error) | `info` |
| `LIMYEDB_METRICS_ENABLED` | Enable Prometheus metrics | `true` |

### Command-Line Flags

```bash
./limyedb \
  -rest :8080 \                    # REST API address
  -grpc :50051 \                   # gRPC API address
  -data ./data \                   # Data directory
  -auth-token SECRET \             # Authentication token
  -tls-cert ./cert.pem \           # TLS certificate
  -tls-key ./key.pem \             # TLS private key
  -raft-node-id node1 \            # Raft cluster node ID
  -raft-bind :7000 \               # Raft bind address
  -raft-bootstrap                  # Bootstrap as cluster leader
```

---

## 8. Verifying Installation

### Health Check

```bash
curl http://localhost:8080/health
```

Expected response:
```json
{
  "status": "healthy",
  "version": "1.0.0",
  "uptime": "5m32s"
}
```

### Create a Test Collection

```bash
curl -X POST http://localhost:8080/collections \
  -H "Content-Type: application/json" \
  -d '{"name": "test", "dimension": 128, "metric": "cosine"}'
```

### Run a Test Search

```bash
# Insert a vector
curl -X PUT http://localhost:8080/collections/test/points \
  -H "Content-Type: application/json" \
  -d '{"points": [{"id": "1", "vector": [0.1, 0.2, ...]}]}'

# Search
curl -X POST http://localhost:8080/collections/test/search \
  -H "Content-Type: application/json" \
  -d '{"vector": [0.1, 0.2, ...], "limit": 5}'
```

---

## Next Steps

- [Getting Started Tutorial](tutorials/getting_started.md) - Build your first vector search application
- [RAG Application Guide](tutorials/rag_application.md) - Create retrieval-augmented generation systems
- [Clustering Guide](clustering.md) - Deploy a high-availability cluster
- [Performance Tuning](performance_tuning.md) - Optimize for your workload
