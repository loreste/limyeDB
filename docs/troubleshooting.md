# LimyeDB Troubleshooting Guide

Common problems and what to do about them. Every command below uses real
flags and routes that exist in the current codebase.

## Table of Contents

1. [Server won't start](#server-wont-start)
2. [Authentication failures](#authentication-failures)
3. [Connection issues](#connection-issues)
4. [Search problems](#search-problems)
5. [Performance issues](#performance-issues)
6. [Cluster issues](#cluster-issues)
7. [Data recovery](#data-recovery)
8. [Logs](#logs)
9. [Metrics](#metrics)

---

## Server won't start

### "authentication is required: pass -auth-token … or -allow-anonymous"

LimyeDB refuses to start without an explicit auth decision. Either:

```bash
# Production: real secret
./limyedb -auth-token "$(openssl rand -hex 32)"

# Development / local-only: opt out
./limyedb -allow-anonymous
```

This is intentional and replaces the historical foot-gun where
`./limyedb` ran wide open by default.

### "address already in use"

Another process is on `:8080` or `:50051`. Either stop it or override:

```bash
./limyedb -auth-token "$AUTH" -rest :8081 -grpc :50052
```

### Permission errors on the data directory

LimyeDB creates `./data` (or whatever you pass to `-data`) with `0750`
permissions. If you point at a directory the user can't write, startup
will fail with `Failed to create data directory`. Fix the path or run as
a user with write access.

---

## Authentication failures

### "401 Unauthorized" on every request

You started with `-auth-token` but your client isn't sending the header.
The server expects:

```
Authorization: Bearer <your-auth-token>
```

```bash
curl -H "Authorization: Bearer $AUTH_TOKEN" http://localhost:8080/health
```

### "invalid token claims"

You supplied a JWT signed with the wrong secret. The JWT signing key is
the same value as `-auth-token`. Mint a JWT with HS256 against that
secret and it will be accepted.

### "permission denied for collection X"

JWTs that carry a `limyedb_permissions` claim are scoped to the
collections listed under `collections`. A token with
`{"docs": ["READ_ONLY"]}` cannot write to `docs` and cannot touch
`embeddings` at all. Mint a new JWT with the right claim, or use the
master `-auth-token` directly (it grants global admin).

---

## Connection issues

```bash
# Check the process is alive
ps aux | grep limyedb

# Confirm the listener
lsof -iTCP:8080 -sTCP:LISTEN
lsof -iTCP:50051 -sTCP:LISTEN

# Liveness check (no auth required)
curl http://localhost:8080/health
```

If `/health` returns `200` but your application calls fail, double-check
the protocol (REST vs gRPC), the auth header, and the URL path against
the actual REST routes — many old docs and sample apps reference paths
like `/cluster/snapshot`, `/admin/...`, or `/sql` that **do not exist**
in the current server. Use the routes listed in `api/rest/server.go` as
the source of truth.

---

## Search problems

### Empty result set

```bash
# 1. Confirm the points are there
curl -H "Authorization: Bearer $AUTH_TOKEN" \
  http://localhost:8080/collections/docs/points/<known-id>

# 2. Check the collection has points
curl -H "Authorization: Bearer $AUTH_TOKEN" \
  http://localhost:8080/collections/docs
```

If points are present but searches return nothing, the most common
causes are:

- **Vector dimension mismatch** — search vector length differs from the
  collection's `dimension`. Returns no error, just no matches.
- **Filter excludes everything** — if you pass `filter`, try the same
  query without it.

### Low recall

Increase `ef_search` per request:

```json
{"vector": [0.1, 0.2, ...], "limit": 10, "ef": 200}
```

Higher `ef` means more candidates explored at query time, costing
latency for recall.

### Stale read after a write

By default reads on a follower can be slightly behind the leader.
For read-after-write semantics on the same client, append
`?consistent=true`:

```bash
curl -X POST "http://localhost:8080/collections/docs/search?consistent=true" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"vector": [0.1, 0.2, ...], "limit": 5}'
```

The handler proxies the request to the current Raft leader, whose FSM
has all committed writes by definition.

---

## Performance issues

### Slow inserts

- Use the batch endpoint: `POST /collections/:name/points/batch` groups
  writes into a single WAL fsync.
- HNSW insert holds a global build lock today (single-threaded). For
  bulk loads, prefer offline ingestion.
- Make sure `wal.sync_on_write` is what you want: `true` is durable but
  forces an fsync per record; `false` is faster but loses the last
  segment on crash.

### Slow searches

- Check `ef_search`. Default 100 is a balance; reduce for speed,
  increase for recall.
- For heavily filtered queries, add a payload index on the filter field:
  ```bash
  curl -X POST http://localhost:8080/collections/docs/payload-indexes \
    -H "Authorization: Bearer $AUTH_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"field": "tenant_id", "type": "hash"}'
  ```

### High memory

HNSW is fully in-memory by design (the graph connections are
mmap-backed but the working set must fit). Estimate ~40 bytes/edge
plus the vectors. For very large datasets there is currently no
production-wired disk-resident index — the `pkg/index/diskann` package
is unwired.

---

## Cluster issues

### Followers can't see writes

The Raft replication is asynchronous; followers apply log entries after
the leader commits. If your client reads from a follower right after a
write to the leader, it may see stale data. Use `?consistent=true` or
target the leader directly.

### "not the leader" on writes

The handler reverse-proxies writes to the current leader automatically.
If you see this error directly, the receiving node could not find the
leader's REST address yet (the leader hasn't broadcast it). Wait for
leader election to settle or check that all nodes share the same
`-auth-token` so the broadcast can authenticate.

### Adding a node never converges

A new node joins via `-raft-join http://<leader>:8080`. The bootstrap
node must have been started with `-raft-bootstrap` and reachable on its
`-raft-bind` address from the joining node. There is **no** automatic
peer discovery or DNS-based join; the join URL is required.

---

## Data recovery

### After a crash

LimyeDB replays the WAL on startup. Recovery is automatic — there is no
`limyedb recover` subcommand. Look for log lines like `WAL initialized`
and `Loaded N collections` to confirm.

### Restoring from a snapshot

```bash
# List snapshots
curl -H "Authorization: Bearer $AUTH_TOKEN" \
  http://localhost:8080/snapshots

# Restore one
curl -X POST -H "Authorization: Bearer $AUTH_TOKEN" \
  http://localhost:8080/snapshots/<id>/restore
```

Snapshots are written via tmp+fsync+rename and the `.meta` file is the
publish point — a half-written snapshot cannot be served.

---

## Logs

LimyeDB logs JSON to stdout via Go's `slog`. Notable lines:

- `WAL initialized` — durable storage online.
- `Starting REST API server` / `Starting gRPC API server` — listeners up.
- `running with -allow-anonymous: REST and gRPC are unauthenticated` —
  reminder you started without auth.
- `CDC webhook delivery failed for ...` — a registered webhook target
  is unreachable; the dispatcher drops the event after a timeout.

There is no built-in log-level toggle endpoint; restart with a different
configuration if you need a different verbosity.

---

## Metrics

Prometheus metrics are exposed at `/metrics` (no auth):

```bash
curl http://localhost:8080/metrics
```

Notable series live in `pkg/metrics/metrics.go`:

- `limyedb_http_request_duration_seconds` — REST request latency.
- `limyedb_http_requests_total` — REST request count by route/status.

> Some dashboards floating around reference series like
> `limyedb_gc_pause_seconds` or `limyedb_pending_vectors`. Those are not
> defined in the current code; do not assume they exist without checking
> `pkg/metrics/metrics.go`.

For deeper introspection (heap profiles, goroutine counts), the binary
does not register `/debug/pprof/*` — you'd have to add it locally.
