# LimyeDB How-Tos

Short recipes for common tasks. Endpoints assume auth is enabled — add
`-H "Authorization: Bearer $AUTH_TOKEN"` to each example.

## Hybrid Search (Dense + BM25 via RRF)

The `/search/v2` endpoint accepts an optional `sparse_vector` alongside
the dense vector and fuses the two ranked lists with Reciprocal Rank
Fusion. The sparse vector is a pre-tokenized index→weight map; LimyeDB
does **not** tokenize text server-side, so callers compute their own
sparse representation (e.g. with `pyserini`, a BM25 tokenizer, or a
trained model).

```bash
curl -X POST http://localhost:8080/collections/products/search/v2 \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "vector": [1.44, 0.22, 0.99],
    "sparse_vector": {"indices": [1, 44], "values": [0.5, 1.2]},
    "limit": 10
  }'
```

## Trigger a server-side snapshot

```bash
curl -X POST http://localhost:8080/snapshots \
  -H "Authorization: Bearer $AUTH_TOKEN"
```

Response:

```json
{"id": "snap_1730000000000", "created_at": "2026-05-04T..."}
```

The snapshot lives under `<data-dir>/snapshots/<id>.snap` with a sidecar
`<id>.snap.meta` published atomically. Restore via:

```bash
curl -X POST http://localhost:8080/snapshots/snap_1730000000000/restore \
  -H "Authorization: Bearer $AUTH_TOKEN"
```

## CDC webhooks

`POST /collections/:name/webhooks` registers an HTTPS callback that
receives insert/update/delete events. URLs are SSRF-validated (private
IP space, localhost variants, and non-HTTP(S) schemes are rejected).

```bash
curl -X POST http://localhost:8080/collections/products/webhooks \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"url": "https://hooks.example.com/limyedb"}'
```

## Read-after-write consistency

By default, reads on a follower can return slightly stale data. To force
the read to be served from the current Raft leader, add
`?consistent=true`:

```bash
curl -X POST "http://localhost:8080/collections/products/search?consistent=true" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"vector": [0.1, 0.2, ...], "limit": 5}'
```
