# LimyeDB Installation Guide

LimyeDB is built natively in Go and compiles to a single, statically linked binary. It can be executed globally without any external dependencies safely.

## 1. Docker (Recommended)

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
