# Scaling LimyeDB

This tutorial covers practical patterns for running LimyeDB on workloads
in the multi-million-vector range. It is intentionally honest about the
limits of what's wired today.

> **Headline limit**: a LimyeDB cluster is a single Hashicorp Raft group
> that fully replicates collection metadata and points across nodes.
> There is **no** sharding / horizontal partitioning yet. Every node
> stores the entire dataset; you scale reads by adding followers and
> writes by giving the leader more CPU and IOPS. The
> `pkg/cluster.Coordinator/HashRing/ShardManager` types in the source
> tree are not instantiated from `cmd/limyedb` and `index_type:
> diskann|ivf|scann` is not honored — collections use mmap-backed HNSW.

---

## Capacity planning

### Memory

HNSW is in-memory. The graph connections live in a memory-mapped file
so the OS can page them, but the working set must reasonably fit in RAM
or you'll thrash. Estimate:

```
mem_per_vector ≈ dim * 4 bytes  + M * 8 bytes  +  payload bytes
```

For 1M vectors at dim=384, M=16, ~200B payload:

```
1_000_000 * (384*4 + 16*8 + 200) ≈ 1.85 GB
```

| Vectors | Dim | Estimated RAM (HNSW) |
|---------|-----|----------------------|
| 100K    | 384 | ~250 MB |
| 1M      | 384 | ~2 GB |
| 10M     | 384 | ~20 GB |
| 1M      | 1536 | ~7 GB |
| 10M     | 1536 | ~70 GB |

### Disk

Vectors and payloads are persisted via WAL plus periodic snapshots.
Plan for ~1.5–2× the in-memory size for the WAL + snapshot rotation.
NVMe is recommended; the mmap graph read path benefits significantly
from low random-access latency.

---

## HNSW tuning at scale

| Parameter | Effect |
|-----------|--------|
| `M` (default 16) | Higher M improves recall and adds memory; range 8–48. |
| `ef_construction` (default 200) | Higher = better graph at the cost of build time; range 100–400. |
| `ef_search` (default 100) | Higher = better recall at the cost of latency; tunable per request via the `ef` field. |

For collections in the 10M+ range, expect:

- Insert throughput drops as the graph grows; bulk-load with the batch
  endpoint (`POST /collections/:name/points/batch`) for one fsync per
  batch instead of per record.
- Search latency stays roughly stable thanks to HNSW's logarithmic
  layer structure, provided you have enough RAM.

---

## Quantization for memory savings

`pkg/quantization` implements scalar (4×), binary (32×), and product
quantization. Configure it at collection-create time:

```bash
curl -X POST http://localhost:8080/collections \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "docs",
    "dimension": 1536,
    "metric": "cosine",
    "quantization": {"type": "scalar", "rescore": true}
  }'
```

Rescoring (`rescore: true`) re-runs the top candidates against the
full-precision vectors, which gives back most of the recall lost to
quantization at minimal latency cost.

Anisotropic / ScaNN-style quantization is not currently selectable; see
[the quantization guide](../quantization.md) for details.

---

## High-availability cluster

A 3-node cluster gives you fault tolerance and read-replica capacity.
Every node holds the full dataset.

### Bootstrap

```bash
# Leader
./limyedb \
  -raft-node-id node1 \
  -raft-bind 192.168.1.1:7000 \
  -raft-data /data/node1/raft \
  -raft-bootstrap \
  -rest 192.168.1.1:8080 \
  -data /data/node1 \
  -auth-token "$CLUSTER_SECRET"

# Followers
./limyedb \
  -raft-node-id node2 \
  -raft-bind 192.168.1.2:7000 \
  -raft-data /data/node2/raft \
  -raft-join http://192.168.1.1:8080 \
  -rest 192.168.1.2:8080 \
  -data /data/node2 \
  -auth-token "$CLUSTER_SECRET"
```

There is **no** automatic peer discovery. Every follower's `-raft-join`
must point at a current cluster member.

### Read scaling

By default reads hit the local FSM on whichever node receives them.
That's effectively eventual consistency — followers can lag the leader
by a small amount, which is usually fine for similarity search.

For read-after-write semantics (write to leader, read your own write),
append `?consistent=true` to the read URL; the request is proxied to
the current leader. Use this sparingly: it costs an extra network hop.

### Write scaling

All writes go through the Raft leader. To scale writes:

- Give the leader the most CPU and the fastest disk in the cluster.
- Batch your inserts (`POST /collections/:name/points/batch`).
- Set `wal.sync_on_write: false` if your durability budget allows the
  loss of the last segment on crash; this drops the per-record fsync.

There is no horizontal write partitioning today.

---

## Bulk loading

For an initial multi-million-vector load:

1. Pre-create the collection with the final HNSW parameters (`M`,
   `ef_construction`) — changing them later requires a full rebuild.
2. Pre-create payload indexes on fields you'll filter on:
   ```bash
   curl -X POST http://localhost:8080/collections/docs/payload-indexes \
     -H "Authorization: Bearer $AUTH_TOKEN" \
     -d '{"field_name": "tenant_id", "field_type": "keyword"}'
   ```
3. Stream in batches of 500–2000 points via
   `POST /collections/:name/points/batch`. The response includes real
   per-point success/failure counts and an `errors[]` array for any
   rejected points.
4. Take a snapshot once the load completes:
   ```bash
   curl -X POST http://localhost:8080/snapshots \
     -H "Authorization: Bearer $AUTH_TOKEN"
   ```

---

## Monitoring

`/metrics` exposes Prometheus series defined in
`pkg/metrics/metrics.go`. The series that exist today center on REST
request latencies and counts; do not assume Grafana dashboards from
the wider ecosystem will work without checking the actual series names.

The `/health` endpoint is unauthenticated and is a safe Kubernetes
liveness probe.

---

## What's not (yet) supported

These are honest about the gaps you may have seen elsewhere in the
docs or in the README's competitor matrix:

- **Sharding / horizontal partitioning** — a cluster is a single Raft
  group; every node stores the full dataset.
- **DiskANN / IVF / ScaNN selectable indexes** — packages exist but are
  not wired into the collection manager.
- **WebSocket push of insert/update/delete events** — the `/stream`
  route exists but `Hub.Publish` is never called from collection
  mutations, so subscribers receive nothing.
- **Auto-tuning** — there is no `/autotune` endpoint.
- **In-server text→sparse encoding** — clients compute their own sparse
  vectors and send them to `/search/v2` as `{indices, values}`.

If you need any of these, file an issue or be aware that you'll have
to build it yourself.
