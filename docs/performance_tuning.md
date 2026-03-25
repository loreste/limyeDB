# LimyeDB Performance Tuning Guide

This guide covers best practices for optimizing LimyeDB performance across different workloads and deployment scenarios.

## Table of Contents

1. [HNSW Parameter Tuning](#hnsw-parameter-tuning)
2. [Index Type Selection](#index-type-selection)
3. [Quantization Trade-offs](#quantization-trade-offs)
4. [Batch Operations](#batch-operations)
5. [Memory vs. Disk Trade-offs](#memory-vs-disk-trade-offs)
6. [Filter Optimization](#filter-optimization)
7. [Hardware Considerations](#hardware-considerations)

---

## HNSW Parameter Tuning

HNSW (Hierarchical Navigable Small World) is the default index type in LimyeDB. Understanding its parameters is crucial for optimal performance.

### Key Parameters

#### M (Max Connections)

The `M` parameter controls the maximum number of bi-directional links per node.

| M Value | Memory Usage | Search Speed | Recall |
|---------|--------------|--------------|--------|
| 8       | Low          | Fast         | Lower  |
| 16      | Medium       | Balanced     | Good   |
| 32      | Higher       | Slower       | Better |
| 64      | High         | Slowest      | Best   |

**Recommendations:**
- Start with `M=16` (default)
- Increase to `M=32` for datasets requiring high recall
- Use `M=8` for memory-constrained environments

```json
{
  "name": "my_collection",
  "dimension": 1536,
  "hnsw": {
    "m": 16
  }
}
```

#### ef_construction (Build Quality)

Controls the size of the dynamic candidate list during index construction.

| ef_construction | Build Time | Index Quality |
|-----------------|------------|---------------|
| 100             | Fast       | Lower         |
| 200             | Medium     | Good          |
| 400             | Slow       | Better        |
| 800             | Very Slow  | Best          |

**Recommendations:**
- Use `ef_construction=200` for most use cases
- Increase to `400-800` for production indexes where build time is less critical
- Can be reduced for rapid prototyping

#### ef_search (Query Quality)

Controls the size of the candidate list during search. Can be adjusted at query time.

| ef_search | Latency | Recall@10 |
|-----------|---------|-----------|
| 50        | ~1ms    | ~90%      |
| 100       | ~2ms    | ~95%      |
| 200       | ~4ms    | ~98%      |
| 500       | ~10ms   | ~99%      |

**Recommendations:**
- Default `ef_search=100` provides good balance
- Dynamically increase for queries requiring higher recall
- Use lower values for latency-sensitive applications

```bash
# Override ef at query time
curl -X POST http://localhost:8080/collections/docs/search \
  -d '{"vector": [...], "limit": 10, "ef": 200}'
```

### Parameter Interaction Chart

```
                    Recall
                      ^
                      |
            ef=500  * |           * ef=200, M=32
                      |
            ef=200  * |     * ef=100, M=16 (default)
                      |
            ef=50   * |
                      |
                      +-----------------------> Latency
```

---

## Index Type Selection

LimyeDB supports multiple index types. Choose based on your requirements:

### HNSW (Default)

**Best for:**
- General-purpose vector search
- Balanced recall and latency
- Collections up to 10M vectors

**Trade-offs:**
- Higher memory usage
- Slower build time
- Excellent search performance

### IVF (Inverted File Index)

**Best for:**
- Very large datasets (10M+ vectors)
- Memory-constrained environments
- Batch-heavy workloads

**Trade-offs:**
- Lower recall at same latency
- Requires training phase
- Good for approximate results

```json
{
  "name": "large_collection",
  "dimension": 768,
  "index_type": "ivf",
  "ivf": {
    "num_clusters": 1000,
    "nprobe": 50
  }
}
```

**Tuning `nprobe`:**
| nprobe | Recall | Latency |
|--------|--------|---------|
| 10     | ~70%   | Fast    |
| 50     | ~90%   | Medium  |
| 100    | ~95%   | Slower  |

### ScaNN

**Best for:**
- Ultra-low latency requirements
- Very large scale (100M+ vectors)
- When accuracy can be traded for speed

**Trade-offs:**
- Requires training
- Two-phase search (approximate + rerank)
- Best with anisotropic quantization

```json
{
  "name": "huge_collection",
  "dimension": 768,
  "index_type": "scann",
  "scann": {
    "num_leaves": 2000,
    "num_rerank": 100
  }
}
```

### DiskANN

**Best for:**
- Datasets larger than available RAM
- Cost-sensitive deployments
- SSD/NVMe storage available

**Trade-offs:**
- Depends on disk I/O performance
- Higher latency than in-memory indexes
- Excellent price/performance ratio

---

## Quantization Trade-offs

Quantization reduces memory usage at the cost of some accuracy.

### Scalar Quantization (4x compression)

Converts float32 to int8, reducing memory by 4x.

| Metric        | Full Precision | Scalar Quantized |
|---------------|----------------|------------------|
| Memory/vector | 4B × dim       | 1B × dim         |
| Recall@10     | 100%           | ~98%             |
| Speed         | Baseline       | ~1.2x faster     |

```json
{
  "quantization": {
    "type": "scalar",
    "rescore": true,
    "rescore_limit": 100
  }
}
```

### Binary Quantization (32x compression)

Converts to 1-bit representation.

| Metric        | Full Precision | Binary Quantized |
|---------------|----------------|------------------|
| Memory/vector | 4B × dim       | 1b × dim         |
| Recall@10     | 100%           | ~85-95%          |
| Speed         | Baseline       | ~5x faster       |

**Best for:**
- Similarity detection (not exact ranking)
- First-pass filtering
- Extremely large datasets

### Product Quantization (PQ)

Divides vectors into segments, each quantized independently.

```json
{
  "quantization": {
    "type": "pq",
    "pq_segments": 8,
    "pq_centroids": 256
  }
}
```

**Compression varies based on configuration:**
- 8 segments, 256 centroids: ~32x compression
- Requires training data
- Good balance of compression and accuracy

### Quantization Decision Tree

```
Need >90% recall?
├── Yes → Use Scalar Quantization with rescoring
└── No → Need >80% recall?
    ├── Yes → Use Product Quantization
    └── No → Use Binary Quantization
```

---

## Batch Operations

### Optimal Batch Sizes

| Operation | Recommended Batch Size | Max Throughput |
|-----------|------------------------|----------------|
| Upsert    | 100-500 points         | ~10K/s         |
| Search    | 10-50 queries          | ~5K/s          |
| Delete    | 100-1000 IDs           | ~50K/s         |

### Batch Upsert Example

```python
# Python
batch_size = 100
for i in range(0, len(points), batch_size):
    batch = points[i:i+batch_size]
    client.upsert(collection_name, batch)
```

### Parallel Batch Processing

For maximum throughput, process batches in parallel:

```python
from concurrent.futures import ThreadPoolExecutor

def upsert_batch(batch):
    return client.upsert(collection_name, batch)

with ThreadPoolExecutor(max_workers=4) as executor:
    batches = [points[i:i+100] for i in range(0, len(points), 100)]
    results = list(executor.map(upsert_batch, batches))
```

---

## Memory vs. Disk Trade-offs

### In-Memory (Default)

```json
{
  "name": "fast_collection",
  "on_disk": false
}
```

| Aspect      | In-Memory |
|-------------|-----------|
| Latency     | <5ms      |
| Cost        | Higher    |
| Capacity    | RAM-bound |

### On-Disk (Mmap)

```json
{
  "name": "large_collection",
  "on_disk": true
}
```

| Aspect      | On-Disk   |
|-------------|-----------|
| Latency     | 10-50ms   |
| Cost        | Lower     |
| Capacity    | Disk-bound|

### Hybrid Configuration

Store vectors on disk, keep graph in memory:

```json
{
  "name": "hybrid_collection",
  "hnsw": {
    "on_disk_payload": false,
    "on_disk_vectors": true
  }
}
```

---

## Filter Optimization

### Filter Selectivity Impact

| Selectivity | Strategy | Latency Impact |
|-------------|----------|----------------|
| >50%        | Post-filter | 2-3x slower |
| 10-50%      | Adaptive   | 1.5-2x slower |
| <10%        | Pre-filter | May need full scan |

### Indexing Payload Fields

Create payload indexes for frequently filtered fields:

```bash
curl -X PUT http://localhost:8080/collections/docs/index \
  -d '{"field_name": "category", "field_schema": "keyword"}'
```

### Supported Index Types

| Field Type | Index Type | Use Case |
|------------|------------|----------|
| String     | keyword    | Exact match |
| Integer    | integer    | Range queries |
| Float      | float      | Range queries |
| Boolean    | bool       | Flag filtering |

### Filter Best Practices

1. **Order conditions by selectivity** - most selective first
2. **Use indexed fields** in `must` clauses
3. **Avoid `should` with many conditions** - expensive OR
4. **Combine with vector pre-filtering** when possible

---

## Hardware Considerations

### CPU

- AVX2/AVX-512 significantly improves distance calculations
- ARM NEON supported on Apple Silicon and ARM servers
- More cores = better batch processing throughput

### Memory

| Dataset Size | Recommended RAM |
|--------------|-----------------|
| 1M vectors   | 8-16 GB         |
| 10M vectors  | 64-128 GB       |
| 100M vectors | 512 GB+         |

*Note: Depends on dimension and quantization*

### Storage

For on-disk indexes:
- NVMe SSD recommended
- SATA SSD acceptable
- HDD not recommended

### Network

- Use gRPC for high-throughput batch operations
- REST API suitable for <1000 QPS
- Co-locate clients with servers to minimize latency

---

## Performance Benchmarking

### Running Benchmarks

```bash
# Index benchmarks
go test ./pkg/index/hnsw/... -bench=. -benchmem

# Full system benchmarks
go test ./test/benchmark/... -bench=. -cpuprofile=cpu.prof
```

### Key Metrics to Monitor

| Metric | Target | Concern Threshold |
|--------|--------|-------------------|
| P50 Latency | <10ms | >50ms |
| P99 Latency | <100ms | >500ms |
| Recall@10 | >95% | <90% |
| Throughput | >1000 QPS | <100 QPS |
| Memory Usage | <80% | >90% |

### Profiling

```bash
# CPU profiling
go tool pprof cpu.prof

# Memory profiling
go tool pprof -alloc_space mem.prof
```

---

## Quick Reference Card

### For Low Latency (<5ms P99)
- Use HNSW with `M=16`, `ef_search=50-100`
- Enable scalar quantization with rescoring
- Keep data in memory
- Use gRPC client

### For High Recall (>99%)
- Use HNSW with `M=32`, `ef_search=500`
- No quantization or scalar with rescoring
- Increase `ef` at query time for critical searches

### For Large Scale (>10M vectors)
- Consider IVF or ScaNN
- Use on-disk storage
- Enable quantization
- Shard across multiple nodes

### For Cost Optimization
- Use on-disk mode with NVMe
- Enable binary or PQ quantization
- Use lower `M` values
- Consider DiskANN index type
