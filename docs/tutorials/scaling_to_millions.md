# Scaling LimyeDB to Millions of Vectors

This guide covers strategies for scaling LimyeDB to handle millions (or billions) of vectors in production environments.

## Table of Contents

1. [Capacity Planning](#capacity-planning)
2. [Index Configuration](#index-configuration)
3. [Quantization](#quantization)
4. [Sharding](#sharding)
5. [Clustering](#clustering)
6. [On-Disk Storage](#on-disk-storage)
7. [Performance Optimization](#performance-optimization)
8. [Monitoring at Scale](#monitoring-at-scale)
9. [Case Studies](#case-studies)

---

## Capacity Planning

### Memory Estimation

Calculate memory requirements based on your data:

```python
def estimate_memory(
    num_vectors: int,
    dimension: int,
    quantization: str = None,
    index_overhead: float = 1.5  # HNSW overhead factor
) -> dict:
    """Estimate memory requirements."""
    # Vector storage
    bytes_per_float = 4  # float32

    if quantization == "scalar":
        bytes_per_component = 1  # int8
        full_vectors_for_rescore = num_vectors * dimension * bytes_per_float
        quantized_vectors = num_vectors * dimension * bytes_per_component
        vector_memory = quantized_vectors + full_vectors_for_rescore
    elif quantization == "binary":
        bits_per_component = 1
        vector_memory = num_vectors * dimension // 8
    elif quantization == "pq":
        pq_segments = 8
        vector_memory = num_vectors * pq_segments
    else:
        vector_memory = num_vectors * dimension * bytes_per_float

    # Index overhead (HNSW graph structure)
    index_memory = vector_memory * (index_overhead - 1)

    # Metadata/payload (estimate 500 bytes per point average)
    payload_memory = num_vectors * 500

    total = vector_memory + index_memory + payload_memory

    return {
        "vectors_gb": vector_memory / (1024**3),
        "index_gb": index_memory / (1024**3),
        "payload_gb": payload_memory / (1024**3),
        "total_gb": total / (1024**3),
        "recommended_ram_gb": total / (1024**3) * 1.3  # 30% headroom
    }


# Examples
print("1M vectors (1536d, no quantization):")
print(estimate_memory(1_000_000, 1536))

print("\n10M vectors (1536d, scalar quantization):")
print(estimate_memory(10_000_000, 1536, "scalar"))

print("\n100M vectors (768d, PQ):")
print(estimate_memory(100_000_000, 768, "pq"))
```

### Memory Requirements Table

| Vectors | Dimension | Quantization | RAM Required |
|---------|-----------|--------------|--------------|
| 1M | 1536 | None | ~10 GB |
| 1M | 1536 | Scalar | ~4 GB |
| 10M | 1536 | None | ~100 GB |
| 10M | 1536 | Scalar | ~35 GB |
| 10M | 768 | Scalar | ~18 GB |
| 100M | 768 | PQ | ~8 GB |

### Disk Space

For on-disk storage:
```
disk_space = 2 × memory_estimate  # WAL + snapshots
```

---

## Index Configuration

### HNSW for Large Scale

```yaml
# config.yaml for 10M+ vectors
collections:
  large_collection:
    dimension: 1536
    metric: cosine

    hnsw:
      m: 16                    # Balance memory/quality
      ef_construction: 200     # Higher for production
      ef_search: 100           # Adjustable at query time
      max_elements: 15000000   # Pre-allocate for growth

    quantization:
      type: scalar
      rescore: true
      rescore_limit: 100
```

### IVF for Very Large Scale

For 100M+ vectors:

```yaml
collections:
  huge_collection:
    dimension: 768
    index_type: ivf

    ivf:
      num_clusters: 10000      # sqrt(N) rule of thumb
      nprobe: 100              # Search clusters
      training_samples: 100000

    quantization:
      type: pq
      pq_segments: 8
      pq_centroids: 256
```

### ScaNN Configuration

```yaml
collections:
  scann_collection:
    dimension: 768
    index_type: scann

    scann:
      num_leaves: 5000
      num_rerank: 200
      quantization_dims: 64
      anisotropic_threshold: 0.2
```

---

## Quantization

### Scalar Quantization (Recommended Start)

4x memory reduction with minimal recall loss:

```python
client.create_collection(
    name="large_collection",
    dimension=1536,
    metric="cosine",
    quantization={
        "type": "scalar",
        "rescore": True,
        "rescore_limit": 100
    }
)
```

### Product Quantization (Maximum Compression)

For extreme scale with acceptable recall trade-off:

```python
client.create_collection(
    name="huge_collection",
    dimension=768,
    quantization={
        "type": "pq",
        "pq_segments": 8,       # 768/8 = 96 dims per segment
        "pq_centroids": 256,    # 8 bits per segment
        "training_samples": 100000
    }
)

# Train quantizer with representative data
client.train_quantization(
    collection_name="huge_collection",
    vectors=training_vectors[:100000]
)
```

### Quantization Comparison

| Method | Compression | Memory (10M, 768d) | Recall@10 |
|--------|-------------|-------------------|-----------|
| None | 1x | 30.7 GB | 100% |
| Scalar | 4x | 7.7 GB | 98% |
| Scalar+Rescore | 4x | 7.7 GB | 99.5% |
| PQ (8 seg) | 48x | 640 MB | 92% |
| PQ+Rescore | 48x | 8 GB* | 97% |

*Includes original vectors for rescoring

---

## Sharding

### Horizontal Sharding

Distribute data across multiple shards:

```python
# Create sharded collection
client.create_collection(
    name="sharded_collection",
    dimension=1536,
    shard_count=4,          # Number of shards
    replication_factor=2    # Replicas per shard
)
```

### Shard Key Strategies

```python
# 1. Hash-based (default) - uniform distribution
shard_key = hash(point_id) % num_shards

# 2. Range-based - for range queries
shard_key = get_shard_for_timestamp(created_at)

# 3. Custom - for data locality
shard_key = tenant_id  # Co-locate tenant data
```

### Query Routing

```python
# Search specific shards
results = client.search(
    collection_name="sharded_collection",
    query_vector=query,
    limit=10,
    shard_key="tenant-123"  # Route to specific shard
)

# Search all shards (default)
results = client.search(
    collection_name="sharded_collection",
    query_vector=query,
    limit=10
    # Queries all shards, merges results
)
```

---

## Clustering

### Multi-Node Setup

```yaml
# Node 1 config
cluster:
  enabled: true
  node_id: "node-1"
  listen_address: "0.0.0.0:7000"
  seed_nodes:
    - "node-2.limyedb.local:7000"
    - "node-3.limyedb.local:7000"

# Node 2 config
cluster:
  enabled: true
  node_id: "node-2"
  listen_address: "0.0.0.0:7000"
  seed_nodes:
    - "node-1.limyedb.local:7000"
    - "node-3.limyedb.local:7000"
```

### Kubernetes Deployment

```yaml
# limyedb-statefulset.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: limyedb
spec:
  serviceName: limyedb
  replicas: 3
  selector:
    matchLabels:
      app: limyedb
  template:
    metadata:
      labels:
        app: limyedb
    spec:
      containers:
        - name: limyedb
          image: limyedb/limyedb:latest
          ports:
            - containerPort: 8080
              name: http
            - containerPort: 50051
              name: grpc
            - containerPort: 7000
              name: raft
          env:
            - name: LIMYEDB_NODE_ID
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: LIMYEDB_SEED_NODES
              value: "limyedb-0.limyedb:7000,limyedb-1.limyedb:7000,limyedb-2.limyedb:7000"
          volumeMounts:
            - name: data
              mountPath: /data
          resources:
            requests:
              memory: "32Gi"
              cpu: "8"
            limits:
              memory: "64Gi"
              cpu: "16"
  volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        accessModes: ["ReadWriteOnce"]
        resources:
          requests:
            storage: 500Gi
        storageClassName: fast-ssd
```

### Load Balancing

```yaml
# limyedb-service.yaml
apiVersion: v1
kind: Service
metadata:
  name: limyedb-lb
spec:
  type: LoadBalancer
  selector:
    app: limyedb
  ports:
    - name: http
      port: 8080
      targetPort: 8080
    - name: grpc
      port: 50051
      targetPort: 50051
```

---

## On-Disk Storage

### Enable On-Disk Mode

For datasets larger than RAM:

```python
client.create_collection(
    name="disk_collection",
    dimension=1536,
    on_disk=True,
    hnsw={
        "on_disk_vectors": True,
        "on_disk_payload": True,
        # Keep graph in memory for fast traversal
        "on_disk_graph": False
    }
)
```

### Hybrid Memory/Disk

```yaml
# Optimal for large scale
collections:
  hybrid:
    dimension: 1536

    hnsw:
      # Graph in memory (fast traversal)
      on_disk_graph: false

      # Vectors on disk with mmap
      on_disk_vectors: true

      # Payload on disk
      on_disk_payload: true

    quantization:
      # Quantized vectors in memory
      type: scalar
      on_disk: false
```

### Storage Requirements

| Component | In-Memory | On-Disk |
|-----------|-----------|---------|
| Graph | Yes (default) | Optional |
| Vectors | Optional | Yes (mmap) |
| Quantized | Yes (default) | Optional |
| Payload | Optional | Yes |

### Disk Performance

- **NVMe SSD**: Recommended for production
- **SATA SSD**: Acceptable, ~2x latency
- **HDD**: Not recommended

```yaml
storage:
  # Use fast storage
  data_dir: /mnt/nvme/limyedb/data
  wal_dir: /mnt/nvme/limyedb/wal

  # Tune I/O
  mmap_threshold: 1048576  # 1MB
  read_buffer_size: 65536  # 64KB
```

---

## Performance Optimization

### Batch Operations

```python
# Insert in optimal batch sizes
BATCH_SIZE = 100  # Tune based on your setup

def bulk_insert(points: list, collection_name: str):
    for i in range(0, len(points), BATCH_SIZE):
        batch = points[i:i+BATCH_SIZE]
        client.upsert(collection_name, batch, wait=False)

    # Wait for final consistency
    client.wait_for_collection(collection_name)
```

### Parallel Search

```python
import asyncio
from concurrent.futures import ThreadPoolExecutor

async def parallel_search(queries: list, collection_name: str, limit: int = 10):
    """Execute searches in parallel."""
    loop = asyncio.get_event_loop()

    with ThreadPoolExecutor(max_workers=10) as executor:
        futures = [
            loop.run_in_executor(
                executor,
                lambda q=q: client.search(collection_name, q, limit=limit)
            )
            for q in queries
        ]

        results = await asyncio.gather(*futures)

    return results


# Usage
queries = [query1, query2, query3, ...]
results = asyncio.run(parallel_search(queries, "collection"))
```

### Connection Pooling

```python
from limyedb import LimyeDBClient

# Create client with connection pool
client = LimyeDBClient(
    host="http://localhost:8080",
    pool_size=20,
    pool_timeout=30
)
```

### Caching

```python
from functools import lru_cache
import hashlib

@lru_cache(maxsize=10000)
def cached_search(query_hash: str, collection: str, limit: int):
    """Cache search results."""
    # Retrieve from actual storage
    query_vec = get_cached_embedding(query_hash)
    return client.search(collection, query_vec, limit=limit)

def search_with_cache(query: str, collection: str, limit: int = 10):
    query_hash = hashlib.md5(query.encode()).hexdigest()
    return cached_search(query_hash, collection, limit)
```

---

## Monitoring at Scale

### Key Metrics

```yaml
# Prometheus alerting rules
groups:
  - name: limyedb-scale
    rules:
      # Memory pressure
      - alert: HighMemoryUsage
        expr: limyedb_memory_usage_bytes / limyedb_memory_limit_bytes > 0.85
        for: 5m
        labels:
          severity: warning

      # Search latency degradation
      - alert: HighSearchLatency
        expr: histogram_quantile(0.99, limyedb_search_duration_seconds_bucket) > 0.5
        for: 5m
        labels:
          severity: warning

      # Index build lag
      - alert: IndexBuildLag
        expr: limyedb_pending_vectors > 10000
        for: 10m
        labels:
          severity: warning

      # Disk space
      - alert: LowDiskSpace
        expr: limyedb_disk_usage_bytes / limyedb_disk_total_bytes > 0.9
        for: 5m
        labels:
          severity: critical

      # Cluster health
      - alert: ClusterNodeDown
        expr: limyedb_cluster_nodes_healthy < limyedb_cluster_nodes_total
        for: 2m
        labels:
          severity: critical
```

### Grafana Dashboard

```json
{
  "title": "LimyeDB Scale Metrics",
  "panels": [
    {
      "title": "Vectors Per Collection",
      "type": "stat",
      "targets": [
        {"expr": "sum(limyedb_collection_vectors_total) by (collection)"}
      ]
    },
    {
      "title": "Search Latency P99",
      "type": "graph",
      "targets": [
        {"expr": "histogram_quantile(0.99, sum(rate(limyedb_search_duration_seconds_bucket[5m])) by (le))"}
      ]
    },
    {
      "title": "Memory Usage",
      "type": "gauge",
      "targets": [
        {"expr": "limyedb_memory_usage_bytes / limyedb_memory_limit_bytes * 100"}
      ]
    },
    {
      "title": "Throughput (QPS)",
      "type": "graph",
      "targets": [
        {"expr": "sum(rate(limyedb_search_total[1m]))"}
      ]
    }
  ]
}
```

### Capacity Monitoring

```python
def check_capacity():
    """Monitor and alert on capacity."""
    info = client.get_cluster_info()

    for node in info["nodes"]:
        memory_pct = node["memory_used"] / node["memory_total"]
        disk_pct = node["disk_used"] / node["disk_total"]

        if memory_pct > 0.85:
            alert(f"Node {node['id']} memory at {memory_pct:.1%}")

        if disk_pct > 0.90:
            alert(f"Node {node['id']} disk at {disk_pct:.1%}")

    for collection in info["collections"]:
        growth_rate = calculate_growth_rate(collection["name"])
        days_until_full = estimate_days_until_full(collection, growth_rate)

        if days_until_full < 30:
            alert(f"Collection {collection['name']} will be full in {days_until_full} days")
```

---

## Case Studies

### Case 1: E-commerce Product Search (10M products)

**Requirements:**
- 10M product vectors (768d)
- <50ms P99 latency
- High recall for product discovery

**Solution:**
```yaml
collections:
  products:
    dimension: 768
    metric: cosine
    shard_count: 2

    hnsw:
      m: 16
      ef_construction: 200
      ef_search: 100

    quantization:
      type: scalar
      rescore: true
      rescore_limit: 100
```

**Infrastructure:**
- 3 nodes × 64GB RAM
- NVMe SSDs
- Kubernetes deployment

**Results:**
- P99 latency: 35ms
- Recall@10: 98.5%
- Throughput: 2,500 QPS

### Case 2: Document Retrieval (100M documents)

**Requirements:**
- 100M document chunks (1536d from OpenAI)
- Cost optimization priority
- Acceptable latency <200ms

**Solution:**
```yaml
collections:
  documents:
    dimension: 1536
    index_type: ivf
    shard_count: 4

    ivf:
      num_clusters: 10000
      nprobe: 100

    quantization:
      type: pq
      pq_segments: 16
      rescore: true

    storage:
      on_disk: true
```

**Infrastructure:**
- 4 nodes × 128GB RAM
- 2TB NVMe per node
- On-disk vectors with in-memory graph

**Results:**
- P99 latency: 120ms
- Recall@10: 95%
- Cost: 70% reduction vs full in-memory

### Case 3: Real-time Recommendations (1B vectors)

**Requirements:**
- 1B user-item vectors (256d)
- <10ms P99 for real-time serving
- Multi-region deployment

**Solution:**
```yaml
collections:
  recommendations:
    dimension: 256
    index_type: scann
    shard_count: 16
    replication_factor: 3

    scann:
      num_leaves: 50000
      num_rerank: 100

    quantization:
      type: binary
      rescore: true
      rescore_limit: 500
```

**Infrastructure:**
- 48 nodes across 3 regions
- 256GB RAM per node
- Dedicated network fabric

**Results:**
- P99 latency: 8ms
- Throughput: 100K QPS
- 99.99% availability

---

## Checklist for Scaling

### Before Scaling

- [ ] Benchmark current performance
- [ ] Identify bottlenecks (CPU, memory, I/O)
- [ ] Estimate growth rate
- [ ] Define SLOs (latency, recall, availability)

### Scaling Steps

1. [ ] Enable quantization (easiest win)
2. [ ] Optimize HNSW parameters
3. [ ] Add sharding if needed
4. [ ] Enable on-disk storage for large data
5. [ ] Deploy cluster for high availability
6. [ ] Set up monitoring and alerting

### Post-Scaling

- [ ] Validate recall metrics
- [ ] Monitor latency percentiles
- [ ] Test failover scenarios
- [ ] Document configuration

---

## Next Steps

- [Performance Tuning](../performance_tuning.md) - Fine-tune for your workload
- [Quantization Guide](../quantization.md) - Detailed quantization options
- [Troubleshooting](../troubleshooting.md) - Debug issues at scale

