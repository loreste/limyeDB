# LimyeDB Performance Tricks & Best Practices

A handful of operational patterns worth knowing.

## 1. HNSW parameters (M, ef_construction, ef_search)

- **`M`** — number of bidirectional connections per node. Defaults to 16.
  Larger M improves recall at the cost of memory and index-build time.
  Useful range: 8–48.
- **`ef_construction`** — search width during index build. Defaults to
  200. Higher values give a more diverse graph at the cost of longer
  build time. 100–400 is a typical range.
- **`ef_search`** — search width at query time. Higher values improve
  recall at the cost of latency. Defaults to 100; can be overridden
  per-request with the `ef` field.

```json
{
  "vector": [1.0, 0.4, ...],
  "limit": 10,
  "ef": 64
}
```

## 2. Batch your writes

Each `PUT /collections/:name/points` call is a separate round-trip and a
separate WAL fsync. Use `POST /collections/:name/points/batch` for bulk
ingest — it groups records into a single WAL append with one fsync per
batch. Typical batch sizes are 100–1000 points.

The batch endpoint returns real per-point success/failure counts plus an
`errors[]` array describing rejected points (dimension mismatch, missing
fields, etc.) — check it; the response shape is additive vs the legacy
`{succeeded, failed}` payload, so existing clients still parse the same
fields.

## 3. Read scaling: route to followers, opt into consistency when needed

In a Raft cluster:

- **Writes** must reach the leader. A write sent to a follower returns
  the leader's address in the error message; the REST handler also
  reverse-proxies the request automatically.
- **Reads** default to the local FSM, which means a follower may briefly
  return data behind the leader. For most read-heavy similarity-search
  workloads this is fine.
- For **read-after-write** semantics on the same client (write to leader,
  read your own write), append `?consistent=true` to the read URL. The
  handler proxies that single request to the leader, whose FSM has all
  committed writes by definition. There is currently no server-side
  fence beyond leader-routing.

A typical deployment puts a load balancer in front of all nodes for
reads and lets the auto-proxy handle write routing.

## 4. Pre-build a payload index for selective filters

Filtering on a high-cardinality payload field (e.g. `tenant_id`,
`category`) without an index forces a full SQLite JSON scan after the
HNSW search. Create a payload index on the field once at collection-init
time so the SQL planner can short-circuit:

```bash
curl -X POST http://localhost:8080/collections/docs/payload-indexes \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"field": "tenant_id", "type": "hash"}'
```

See [Advanced Filtering](advanced_filtering.md) for the full operator and
index-type list.
