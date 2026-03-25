# LimyeDB Quantization Guide

This guide explains vector quantization in LimyeDB, helping you choose the right technique to balance memory usage, search speed, and accuracy.

## Table of Contents

1. [What is Quantization?](#what-is-quantization)
2. [Quantization Types](#quantization-types)
3. [When to Use Quantization](#when-to-use-quantization)
4. [Configuration Examples](#configuration-examples)
5. [Training Requirements](#training-requirements)
6. [Compression vs. Recall Trade-offs](#compression-vs-recall-trade-offs)
7. [Rescoring Strategies](#rescoring-strategies)
8. [Best Practices](#best-practices)

---

## What is Quantization?

Quantization reduces the memory footprint of vectors by representing them with fewer bits. Instead of storing full 32-bit floating-point values, quantized vectors use compressed representations.

### Benefits

- **Reduced Memory**: 4x to 32x smaller vector storage
- **Faster Search**: Smaller data fits in CPU cache, distance calculations are faster
- **Lower Cost**: More vectors per GB of RAM
- **Higher Throughput**: More concurrent searches

### Trade-offs

- **Reduced Accuracy**: Some precision loss in distance calculations
- **Training Overhead**: Some methods require a training phase
- **Configuration Complexity**: Need to tune parameters for your data

---

## Quantization Types

LimyeDB supports several quantization methods:

### Scalar Quantization (SQ)

Converts each float32 component to int8, achieving **4x compression**.

| Property | Value |
|----------|-------|
| Compression | 4x |
| Recall Loss | 1-3% |
| Training Required | No |
| Best For | General use, first choice |

**How it works:**
1. Find min/max values across all vectors
2. Map each float to 0-255 range
3. Store as uint8

```
Original:  [0.15, -0.42, 0.78, 0.03]  (16 bytes)
Quantized: [128, 45, 220, 115]        (4 bytes)
```

### Binary Quantization (BQ)

Converts each component to a single bit, achieving **32x compression**.

| Property | Value |
|----------|-------|
| Compression | 32x |
| Recall Loss | 5-15% |
| Training Required | No |
| Best For | Similarity detection, first-pass filtering |

**How it works:**
1. Compare each component to threshold (usually 0)
2. Store as 1 if positive, 0 if negative
3. Use Hamming distance for search

```
Original:  [0.15, -0.42, 0.78, 0.03, -0.11, 0.55, -0.22, 0.09]
Quantized: [1, 0, 1, 1, 0, 1, 0, 1] = 0xB5 (1 byte for 8 dims)
```

### Product Quantization (PQ)

Divides vectors into segments, each quantized independently using learned codebooks.

| Property | Value |
|----------|-------|
| Compression | 8x - 64x (configurable) |
| Recall Loss | 3-10% |
| Training Required | Yes (k-means per segment) |
| Best For | Large datasets, memory-constrained |

**How it works:**
1. Split vector into M segments (e.g., 8 segments of 192 dims for 1536-dim vectors)
2. Train k-means codebook per segment (256 centroids typical)
3. Store centroid index (1 byte) per segment

```
Original:  1536 floats = 6144 bytes
PQ (8 seg): 8 bytes (8 centroid indices)
Compression: 768x
```

### Anisotropic Quantization (AQ)

Advanced quantization that preserves directional importance, used in ScaNN.

| Property | Value |
|----------|-------|
| Compression | 8x - 32x |
| Recall Loss | 2-5% |
| Training Required | Yes (covariance + eigenvectors) |
| Best For | High-accuracy requirements, ScaNN index |

**How it works:**
1. Compute covariance matrix of training vectors
2. Find principal components (eigenvectors)
3. Project and quantize in importance-weighted space

---

## When to Use Quantization

### Decision Matrix

| Scenario | Recommendation |
|----------|----------------|
| < 100K vectors | Usually not needed |
| 100K - 1M vectors | Scalar Quantization |
| 1M - 10M vectors | Scalar or Product Quantization |
| > 10M vectors | Product Quantization + On-disk |
| Memory critical | Binary or Product Quantization |
| Accuracy critical | Scalar with rescoring |
| First-pass filtering | Binary Quantization |

### Memory Calculation

**Without quantization (1536-dim vectors):**
```
1M vectors × 1536 dims × 4 bytes = 6.14 GB
```

**With scalar quantization:**
```
1M vectors × 1536 dims × 1 byte = 1.54 GB (4x reduction)
```

**With binary quantization:**
```
1M vectors × 1536 dims / 8 = 192 MB (32x reduction)
```

**With product quantization (8 segments):**
```
1M vectors × 8 bytes = 8 MB (768x reduction)
```

---

## Configuration Examples

### Scalar Quantization

```json
{
  "name": "documents",
  "dimension": 1536,
  "metric": "cosine",
  "quantization": {
    "type": "scalar"
  }
}
```

**With rescoring:**
```json
{
  "quantization": {
    "type": "scalar",
    "rescore": true,
    "rescore_limit": 100
  }
}
```

### Binary Quantization

```json
{
  "name": "similarity_check",
  "dimension": 768,
  "metric": "cosine",
  "quantization": {
    "type": "binary",
    "threshold": 0.0
  }
}
```

### Product Quantization

```json
{
  "name": "large_collection",
  "dimension": 1536,
  "metric": "euclidean",
  "quantization": {
    "type": "pq",
    "pq_segments": 8,
    "pq_centroids": 256,
    "training_samples": 100000
  }
}
```

### Anisotropic Quantization (ScaNN)

```json
{
  "name": "scann_collection",
  "dimension": 768,
  "index_type": "scann",
  "scann": {
    "num_leaves": 1000,
    "num_rerank": 100,
    "quantization_dims": 64,
    "anisotropic_threshold": 0.2
  }
}
```

---

## Training Requirements

### No Training Required
- **Scalar Quantization**: Uses data statistics (min/max)
- **Binary Quantization**: Uses fixed threshold

### Training Required
- **Product Quantization**: Needs representative samples
- **Anisotropic Quantization**: Needs representative samples

### Training Best Practices

1. **Sample Size**: Use 10,000 - 100,000 representative vectors
2. **Distribution**: Samples should match production data distribution
3. **Timing**: Train before bulk insert for best results
4. **Re-training**: Consider re-training if data distribution changes

### Training API

```python
# Python - Train quantization on samples
client.train_quantization(
    collection_name="documents",
    samples=training_vectors,  # List of vectors
    quantization_type="pq",
    pq_segments=8
)
```

```bash
# REST API
curl -X POST http://localhost:8080/collections/documents/quantization/train \
  -H "Content-Type: application/json" \
  -d '{
    "vectors": [...],
    "type": "pq",
    "pq_segments": 8,
    "pq_centroids": 256
  }'
```

---

## Compression vs. Recall Trade-offs

### Benchmark Results (1M vectors, 1536 dimensions)

| Quantization | Memory | Search Latency | Recall@10 |
|--------------|--------|----------------|-----------|
| None         | 6.14 GB | 5ms           | 100%      |
| Scalar       | 1.54 GB | 3ms           | 98%       |
| Scalar + Rescore | 1.54 GB | 4ms       | 99.5%     |
| Binary       | 192 MB | 1ms            | 88%       |
| Binary + Rescore | 192 MB | 3ms        | 95%       |
| PQ (8 seg)   | 8 MB   | 2ms            | 92%       |
| PQ + Rescore | 8 MB   | 4ms            | 97%       |

### Recall vs. Compression Chart

```
Recall
  ^
100|  * None
   |    * Scalar+Rescore
 98|      * Scalar
   |
 95|          * Binary+Rescore
   |            * PQ+Rescore
 92|              * PQ
   |
 88|                * Binary
   |
   +---------------------------------> Compression
     1x   4x    8x   16x   32x
```

### Choosing Based on Requirements

**If recall > 98% is required:**
- Use Scalar Quantization with rescoring
- Or no quantization

**If recall > 95% is acceptable:**
- Use Scalar Quantization (no rescoring)
- Or PQ with rescoring

**If recall > 90% is acceptable:**
- Use Product Quantization
- Or Binary with rescoring

**If recall > 85% is acceptable:**
- Use Binary Quantization

---

## Rescoring Strategies

Rescoring improves recall by re-computing exact distances for top candidates.

### How Rescoring Works

1. **Phase 1**: Search with quantized vectors, get top N candidates
2. **Phase 2**: Re-compute exact distances for top N using original vectors
3. **Return**: Re-ranked top K results

### Configuring Rescoring

```json
{
  "quantization": {
    "type": "scalar",
    "rescore": true,
    "rescore_limit": 100
  }
}
```

**Parameters:**
- `rescore`: Enable/disable rescoring
- `rescore_limit`: Number of candidates to rescore (default: 2 × limit)

### Query-Time Override

```python
# Override rescoring at query time
results = client.search(
    collection_name="documents",
    query_vector=query,
    limit=10,
    params={
        "rescore": True,
        "rescore_limit": 200
    }
)
```

### Rescoring Guidelines

| rescore_limit | Use Case |
|---------------|----------|
| 2 × limit | Good default |
| 5 × limit | High accuracy needs |
| 10 × limit | Critical accuracy |
| 1 × limit | Speed priority |

---

## Best Practices

### 1. Start with Scalar Quantization

Scalar quantization offers the best balance of simplicity, compression, and accuracy:

```json
{
  "quantization": {
    "type": "scalar",
    "rescore": true
  }
}
```

### 2. Enable Rescoring for Production

Always use rescoring in production unless latency is critical:

```json
{
  "quantization": {
    "type": "scalar",
    "rescore": true,
    "rescore_limit": 100
  }
}
```

### 3. Test on Your Data

Run benchmarks with your actual data distribution:

```python
# Measure recall at different configurations
configs = [
    {"type": "none"},
    {"type": "scalar"},
    {"type": "scalar", "rescore": True},
    {"type": "pq", "segments": 8},
]

for config in configs:
    recall = measure_recall(test_queries, ground_truth, config)
    print(f"{config}: Recall@10 = {recall:.2%}")
```

### 4. Consider Hybrid Approaches

Combine quantization with other optimizations:

```json
{
  "name": "optimized_collection",
  "dimension": 1536,
  "hnsw": {
    "m": 16,
    "ef_construction": 200
  },
  "quantization": {
    "type": "scalar",
    "rescore": true
  },
  "on_disk": false
}
```

### 5. Monitor Recall in Production

Track recall metrics to detect degradation:

```yaml
# Prometheus alert
- alert: LowSearchRecall
  expr: limyedb_search_recall < 0.95
  for: 5m
  labels:
    severity: warning
```

### 6. Plan for Growth

Choose quantization based on projected scale:

| Current Size | Projected Size | Recommendation |
|--------------|----------------|----------------|
| 100K | 1M | Scalar now, no changes needed |
| 1M | 10M | Scalar + on-disk storage |
| 10M | 100M | PQ + sharding + on-disk |

---

## Quantization Comparison Summary

| Feature | Scalar | Binary | PQ | Anisotropic |
|---------|--------|--------|-----|-------------|
| Compression | 4x | 32x | 8-64x | 8-32x |
| Recall | High | Low | Medium | High |
| Training | No | No | Yes | Yes |
| Complexity | Low | Low | Medium | High |
| Best For | General | Filtering | Large scale | ScaNN |

---

## Troubleshooting

### Low Recall After Quantization

1. **Enable rescoring** with higher `rescore_limit`
2. **Increase ef_search** at query time
3. **Try scalar** instead of binary/PQ
4. **Check training data** distribution for PQ

### High Memory Despite Quantization

1. Verify quantization is enabled in collection config
2. Check if original vectors are also stored (for rescoring)
3. Consider binary quantization for maximum compression

### Slow Training

1. Reduce `training_samples` count
2. Use sampling instead of full dataset
3. Run training during off-peak hours

### Inconsistent Results

1. Ensure training data matches production distribution
2. Re-train after significant data distribution changes
3. Use deterministic seed for reproducibility

