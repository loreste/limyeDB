# LimyeDB Installation Guide

LimyeDB is built natively in Go and compiles to a single, statically linked binary. It can be executed globally without any external dependencies safely.

## 1. Docker (Recommended)

The most resilient way to scale LimyeDB in production is via Docker. The image mounts a local persistence volume to protect HNSW indexes.

```bash
docker pull loreste3201/limyedb:latest

docker run -d \
  --name limyedb_core \
  -p 8080:8080 \
  -p 6334:6334 \
  -v limyedb_data:/data \
  loreste3201/limyedb:latest
```

## 2. Compile From Source

If you require custom forks or architecture optimizations (e.g., Apple Silicon M4 / AVX-512 extensions), cloning and compiling natively is optimal:

```bash
git clone https://github.com/loreste/limyeDB.git
cd limyeDB

# Build the generic CLI
go build -o limyedb ./cmd/limyedb
./limyedb --port 8080 --metrics --grpc-port 6334
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

## 4. Production Security (API & TLS)

You can secure the instance instantly using runtime flags:
```bash
./limyedb \
    --api-key="super_secret_token" \
    --tls-cert="/etc/ssl/limyedb.crt" \
    --tls-key="/etc/ssl/limyedb.key"
```
