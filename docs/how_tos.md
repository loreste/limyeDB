# LimyeDB How-Tos

Learn how to utilize LimyeDB's core distributed features safely for your upcoming RAG application!

## How To: Perform Hybrid Search (BM25 + Dense)

LimyeDB boasts built-in Reciprocal Rank Fusion (RRF) allowing you to execute sparse keyword indexing merged instantly with dense semantic spaces.

```json
POST /collections/products/search
{
    "vector": [1.44, 0.22, 0.99],
    "sparse_vector": {"1": 0.5, "44": 1.2},
    "limit": 10
}
```
*The engine calculates HNSW margins and aligns them alongside BM25 rankings autonomously!*

## How To: Stream Realtime Updates (WebSocket)

LimyeDB allows you to connect natively to the REST engine via WebSockets. Any insertions, deletions, or index updates executed by other users will trigger an immediate push event back to your web-client seamlessly.

```javascript
// Connect to the LimyeDB stream engine automatically via browser API
const ws = new WebSocket("ws://localhost:8080/stream");

ws.onmessage = (event) => {
    const payload = JSON.parse(event.data);
    console.log("Vector DB Event:", payload.event_type);
    
    if (payload.event_type === "UpsertPoints") {
       console.log("New User embedded:", payload.points);
    }
};
```

## How To: Backup & Snapshot High-Availability State

Raft inherently triggers automatic logs, but you can explicitly trigger a cold backup onto disk if migrating LimyeDB instances globally.

```bash
curl -X POST http://localhost:8080/cluster/snapshot -H "Authorization: Bearer <API_KEY>"
```
This forces the Replicated State Machine to compress all active `.wal` logs into an instant `.snap` file stored securely in `/data/snapshots/`.
