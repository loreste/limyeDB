# LimyeDB Troubleshooting Guide

This guide helps diagnose and resolve common issues with LimyeDB.

## Table of Contents

1. [Connection Issues](#connection-issues)
2. [Search Problems](#search-problems)
3. [Performance Issues](#performance-issues)
4. [Memory Problems](#memory-problems)
5. [Cluster Issues](#cluster-issues)
6. [Data Recovery](#data-recovery)
7. [Log Interpretation](#log-interpretation)
8. [Monitoring Metrics](#monitoring-metrics)

---

## Connection Issues

### Cannot Connect to Server

**Symptoms:**
- `connection refused` errors
- Timeouts when connecting

**Diagnosis:**
```bash
# Check if server is running
ps aux | grep limyedb

# Check port availability
lsof -i :8080
lsof -i :50051

# Test connectivity
curl http://localhost:8080/health
```

**Solutions:**

1. **Server not running:**
   ```bash
   ./limyedb serve --config config.yaml
   ```

2. **Port conflict:**
   ```yaml
   # Change ports in config.yaml
   server:
     rest_address: ":8081"
     grpc_address: ":50052"
   ```

3. **Firewall blocking:**
   ```bash
   # Linux
   sudo ufw allow 8080
   sudo ufw allow 50051

   # macOS
   sudo pfctl -d  # Disable firewall temporarily for testing
   ```

### Authentication Failures

**Symptoms:**
- `401 Unauthorized` responses
- `invalid API key` errors

**Diagnosis:**
```bash
# Test with curl
curl -H "Authorization: Bearer YOUR_KEY" http://localhost:8080/health
```

**Solutions:**

1. **Verify API key format:**
   ```python
   # Python - correct format
   client = LimyeDBClient(host="http://localhost:8080", api_key="your-key")

   # Not auth_token
   ```

2. **Check key in server config:**
   ```yaml
   auth:
     enabled: true
     keys:
       - "your-api-key"
   ```

3. **Regenerate API key** if compromised

---

## Search Problems

### Low Recall / Missing Results

**Symptoms:**
- Known similar documents not returned
- Recall lower than expected

**Diagnosis:**
```python
# Check if points exist
point = client.get_point("collection", "expected-id")
print(point)

# Search with higher ef
results = client.search("collection", query, limit=10, ef=500)
```

**Solutions:**

1. **Increase ef_search:**
   ```python
   # At query time
   results = client.search(collection, query, limit=10, ef=200)
   ```

2. **Check vector normalization:**
   ```python
   # For cosine similarity, normalize vectors
   import numpy as np
   query = query / np.linalg.norm(query)
   ```

3. **Verify dimension matches:**
   ```python
   collection_info = client.get_collection("my_collection")
   print(f"Collection dimension: {collection_info['dimension']}")
   print(f"Query dimension: {len(query)}")
   ```

4. **Rebuild index with higher ef_construction:**
   ```json
   {
     "hnsw": {
       "ef_construction": 400
     }
   }
   ```

### Slow Searches

**Symptoms:**
- Search latency >100ms
- Timeouts on search requests

**Diagnosis:**
```python
import time

start = time.time()
results = client.search(collection, query, limit=10)
print(f"Search took: {(time.time() - start) * 1000:.2f}ms")
```

**Solutions:**

1. **Reduce ef_search:**
   ```python
   results = client.search(collection, query, limit=10, ef=50)
   ```

2. **Enable quantization:**
   ```json
   {
     "quantization": {
       "type": "scalar",
       "rescore": true
     }
   }
   ```

3. **Optimize filters:**
   - Index frequently filtered fields
   - Use most selective conditions first

4. **Check server resources:**
   ```bash
   top -p $(pgrep limyedb)
   ```

### Filter Not Working

**Symptoms:**
- Filter conditions ignored
- Wrong results returned

**Diagnosis:**
```python
# Verify payload exists
point = client.get_point("collection", "point-id")
print(point.payload)

# Check filter syntax
filter = {
    "must": [
        {"key": "category", "match": {"value": "news"}}
    ]
}
```

**Solutions:**

1. **Check field name case:**
   ```python
   # Payload field names are case-sensitive
   filter = {"must": [{"key": "Category", ...}]}  # Matches "Category"
   filter = {"must": [{"key": "category", ...}]}  # Matches "category"
   ```

2. **Check value types:**
   ```python
   # String vs number
   filter = {"must": [{"key": "count", "match": {"value": 42}}]}   # number
   filter = {"must": [{"key": "count", "match": {"value": "42"}}]} # string
   ```

3. **Use correct range syntax:**
   ```python
   filter = {
       "must": [
           {
               "key": "price",
               "range": {"gte": 10, "lte": 100}  # Not "gt_eq"
           }
       ]
   }
   ```

---

## Performance Issues

### High CPU Usage

**Symptoms:**
- CPU at 100%
- Slow response times
- Server unresponsive

**Diagnosis:**
```bash
# Check CPU profile
top -p $(pgrep limyedb)

# Generate CPU profile
curl http://localhost:8080/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof cpu.prof
```

**Solutions:**

1. **Reduce concurrent operations:**
   ```yaml
   server:
     max_concurrent_requests: 100
   ```

2. **Enable rate limiting:**
   ```yaml
   rate_limit:
     enabled: true
     requests_per_second: 1000
   ```

3. **Scale horizontally** - Add more nodes to cluster

### High Memory Usage

See [Memory Problems](#memory-problems) section.

### Slow Insertions

**Symptoms:**
- Insert throughput <1000/s
- Long wait times for upsert

**Diagnosis:**
```python
import time

start = time.time()
client.upsert("collection", points[:100])
print(f"100 points took: {(time.time() - start) * 1000:.2f}ms")
```

**Solutions:**

1. **Use batch upserts:**
   ```python
   # Instead of one at a time
   client.upsert_batch("collection", points, batch_size=100)
   ```

2. **Disable wait:**
   ```python
   client.upsert("collection", points, wait=False)
   ```

3. **Reduce ef_construction** for faster builds (trade-off with quality)

---

## Memory Problems

### Out of Memory (OOM)

**Symptoms:**
- Server crashes with OOM
- `killed` by OS
- Memory usage at limit

**Diagnosis:**
```bash
# Check memory usage
free -h

# Check container limits
docker stats limyedb

# Check Go memory stats
curl http://localhost:8080/debug/pprof/heap > heap.prof
go tool pprof heap.prof
```

**Solutions:**

1. **Enable on-disk storage:**
   ```json
   {
     "name": "large_collection",
     "on_disk": true
   }
   ```

2. **Enable quantization:**
   ```json
   {
     "quantization": {
       "type": "scalar"  // 4x memory reduction
     }
   }
   ```

3. **Increase system memory** or use larger instances

4. **Shard data across nodes**

### Memory Leak Detection

**Symptoms:**
- Memory grows over time
- Memory not released after deletes

**Diagnosis:**
```bash
# Monitor over time
watch -n 5 'ps -o rss,vsz -p $(pgrep limyedb)'

# Generate heap profiles at intervals
for i in 1 2 3; do
  curl http://localhost:8080/debug/pprof/heap > heap_$i.prof
  sleep 60
done

# Compare profiles
go tool pprof -diff_base=heap_1.prof heap_3.prof
```

**Solutions:**

1. **Compact deleted points:**
   ```bash
   curl -X POST http://localhost:8080/collections/name/compact
   ```

2. **Restart server** to clear fragmentation

3. **Report issue** with heap profile if leak persists

---

## Cluster Issues

### Node Not Joining Cluster

**Symptoms:**
- `failed to join cluster` errors
- Node shows as unhealthy

**Diagnosis:**
```bash
# Check cluster status
curl http://localhost:8080/cluster/status

# Check network connectivity
ping other-node-ip

# Check Raft logs
grep "raft" /var/log/limyedb/limyedb.log
```

**Solutions:**

1. **Verify network connectivity:**
   ```bash
   telnet other-node 7000  # Raft port
   ```

2. **Check node IDs are unique:**
   ```yaml
   cluster:
     node_id: "node-1"  # Must be unique
   ```

3. **Ensure same cluster token:**
   ```yaml
   cluster:
     token: "same-token-on-all-nodes"
   ```

### Split Brain / Data Inconsistency

**Symptoms:**
- Different data on different nodes
- Write conflicts

**Solutions:**

1. **Check leader status:**
   ```bash
   curl http://localhost:8080/cluster/leader
   ```

2. **Force leader election** (careful!):
   ```bash
   curl -X POST http://localhost:8080/cluster/step-down
   ```

3. **Restore from snapshot** if data is corrupted

---

## Data Recovery

### Restoring from Snapshot

```bash
# List available snapshots
ls /data/snapshots/

# Stop server
systemctl stop limyedb

# Restore snapshot
limyedb restore --snapshot /data/snapshots/snapshot-20240101.tar.gz

# Start server
systemctl start limyedb
```

### Recovering from WAL

```bash
# If snapshot is corrupted but WAL is intact
limyedb recover --wal-dir /data/wal/
```

### Exporting Data

```python
# Export collection to file
import json

all_points = []
offset = None

while True:
    result = client.scroll("collection", limit=1000, offset=offset)
    all_points.extend(result["points"])
    offset = result.get("next_offset")
    if not offset:
        break

with open("backup.json", "w") as f:
    json.dump(all_points, f)
```

---

## Log Interpretation

### Log Levels

| Level | Meaning |
|-------|---------|
| DEBUG | Detailed debugging info |
| INFO  | Normal operations |
| WARN  | Potential issues |
| ERROR | Operation failures |
| FATAL | Server crash |

### Common Log Patterns

**Successful search:**
```
INFO  search completed collection=docs k=10 ef=100 took_ms=5
```

**Slow query warning:**
```
WARN  slow query detected collection=docs took_ms=150 threshold_ms=100
```

**Memory pressure:**
```
WARN  memory pressure high usage_percent=85 threshold=80
```

**Connection error:**
```
ERROR failed to connect to peer addr=node2:7000 error="connection refused"
```

### Enabling Debug Logs

```yaml
logging:
  level: debug
  format: json
```

Or at runtime:
```bash
curl -X POST http://localhost:8080/admin/log-level?level=debug
```

---

## Monitoring Metrics

### Key Metrics to Watch

| Metric | Normal | Warning | Critical |
|--------|--------|---------|----------|
| `search_latency_p99` | <50ms | <200ms | >500ms |
| `memory_usage_percent` | <70% | <85% | >90% |
| `cpu_usage_percent` | <60% | <80% | >95% |
| `wal_lag_bytes` | <1MB | <10MB | >100MB |
| `error_rate` | <0.1% | <1% | >5% |

### Prometheus Metrics Endpoint

```bash
curl http://localhost:8080/metrics
```

### Key Metrics

```prometheus
# Search performance
limyedb_search_duration_seconds{quantile="0.99"}
limyedb_search_total{status="success"}
limyedb_search_total{status="error"}

# Memory
limyedb_memory_usage_bytes
limyedb_gc_pause_seconds

# Collections
limyedb_collection_points_total{collection="docs"}
limyedb_collection_vectors_bytes{collection="docs"}

# Cluster
limyedb_cluster_nodes_total
limyedb_raft_leader
```

### Alerting Rules (Prometheus)

```yaml
groups:
  - name: limyedb
    rules:
      - alert: HighSearchLatency
        expr: limyedb_search_duration_seconds{quantile="0.99"} > 0.5
        for: 5m
        labels:
          severity: warning

      - alert: HighMemoryUsage
        expr: limyedb_memory_usage_bytes / limyedb_memory_limit_bytes > 0.9
        for: 5m
        labels:
          severity: critical

      - alert: HighErrorRate
        expr: rate(limyedb_search_total{status="error"}[5m]) / rate(limyedb_search_total[5m]) > 0.05
        for: 5m
        labels:
          severity: critical
```

---

## Getting Help

If you cannot resolve an issue:

1. **Search existing issues:** https://github.com/limyedb/limyedb/issues
2. **Check documentation:** https://docs.limyedb.io
3. **Open new issue** with:
   - LimyeDB version
   - OS and hardware specs
   - Relevant logs
   - Steps to reproduce
   - Expected vs actual behavior
