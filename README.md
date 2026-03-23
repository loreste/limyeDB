# LimyeDB - Enterprise Distributed Vector Database for AI & RAG

[![Go Reference](https://pkg.go.dev/badge/github.com/limyedb/limyedb.svg)](https://pkg.go.dev/github.com/limyedb/limyedb)
[![Go Report Card](https://goreportcard.com/badge/github.com/limyedb/limyedb)](https://goreportcard.com/report/github.com/limyedb/limyedb)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)

**LimyeDB** is a lightning-fast, highly-available open-source **vector database** engineered for the next generation of AI applications. Built entirely in Go, it is designed from the ground up to power **Retrieval-Augmented Generation (RAG)**, Large Language Model (LLM) memory, and advanced semantic search use-cases with sub-millisecond latency. 

LimyeDB uniquely combines Graph-Native HNSW traversal with Zero-Allocation memory pools, unlocking massive Queries-Per-Second (QPS) at a fraction of hardware costs. LimyeDB scales transparently from lightweight embedded binaries to globally distributed, fault-tolerant clusters.

---

## 🚀 Key Features for AI Developers

*   **Distributed High Availability (Raft):** LimyeDB features a native HashiCorp Raft implementation. Clusters autonomously elect leaders, replicate logs across geographical regions with sub-second latency, and automatically trigger bounded snapshots, ensuring impenetrable data persistence and zero-downtime scaling.
*   **Zero-Allocation HNSW Engine:** Written to eliminate Golang Garbage Collection (GC) pauses entirely. High-throughput trackers rely on $O(1)$ Generational memory pools, enabling massive query concurrency without allocation bloat.
*   **Advanced AST Payload Filtering:** Execute complex JSON property constraints (`$and`, `$or`, `$not`, `$in`, `$gte`) graph-natively. LimyeDB evaluates nested logical search boundaries exactly during index traversal, massively outperforming post-processing filters.
*   **Enterprise Security & Observability:** Secure clusters natively using strict `--auth-token` Bearer interceptors alongside `--tls-cert` HTTPS proxy bindings. Monitor live traffic directly through the native Prometheus `/metrics` endpoint.
*   **Reciprocal Rank Fusion (Hybrid Search):** Seamlessly combine the semantic depth of dense embeddings (HNSW) with crisp keyword precision (Sparse BM25).

---

## 📦 Official API SDKs

LimyeDB exposes comprehensive official SDK wrappers across all major AI application ecosystems:

- **🐍 Python & LangChain** (`limyedb`): Native Python drivers seamlessly powering LangChain's `VectorStore` module out of the box.
- **🌐 Node.js / TypeScript** (`limyedb`): Strictly typed JS bindings natively surfacing HTTP-forwarded proxy routing for web frameworks.
- **🐹 Golang** (`limyedb-go`): Blazing fast gRPC/HTTP protocol bindings for advanced microservices.

---

## 🛠 Installation & Cluster Bootstrapping

LimyeDB compiles cleanly into a single high-performance binary with absolutely zero external dependencies.

```bash
# 1. Download database dependencies
make deps

# 2. Compile the database
make build
```

### Starting a Standalone Node
Run LimyeDB securely over TLS with Prometheus monitoring enabled:

```bash
./bin/limyedb \
    --rest-addr=0.0.0.0:8080 \
    --auth-token="secret_master_key" \
    --tls-cert="./certs/server.crt" \
    --tls-key="./certs/server.key"
```

### Bootstrapping a Distributed Cluster
Launch the first node natively enabling distributed Raft consensus:

```bash
./bin/limyedb --raft-node-id="node1" --raft-bind="127.0.0.1:7201" --raft-data="./data/node1" --raft-bootstrap=true --rest-addr="0.0.0.0:8081"
```

Join infinite follower nodes instantly to dramatically multiply read-throughput. Mutator requests (POST/PUT/DELETE) are transparently reverse-proxied back to the active leader using native zero-config HTTP forwarding:

```bash
./bin/limyedb --raft-node-id="node2" --raft-bind="127.0.0.1:7202" --raft-data="./data/node2" --raft-join="http://127.0.0.1:8081" --rest-addr="0.0.0.0:8082"
```

---

## ⚡ Architecture Layers

1.  **Transport Layer:** Native Raft JSON APIs & Multi-plexed gRPC streams resolving Leader proxies automatically.
2.  **Collection Engine:** Handles precise namespace multi-tenancy formatting dynamic sharding over recursive AST configurations.
3.  **HNSW Vector Graph:** A highly sophisticated, pure-Go Hierarchical Navigable Small World implementation utilizing Scalar Quantization and memory-mapped (mmap) persistent segment handling for instantaneous sub-millisecond restarts.

### Testing & Contribution
LimyeDB maintains a comprehensive distributed cluster regression test and localized memory benchmarking suite to guarantee total system structural integrity. Run `go test ./...` to verify the Raft synchronization layer.
