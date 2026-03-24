# LimyeDB Performance Tricks & Best Practices

To get the absolute maximum Queries-Per-Second (QPS) and memory resilience from LimyeDB, consider the following optimization strategies.

## 1. Optimizing HNSW Index Parameters (M and Ef)

The HNSW graph relies on several constants which dictate speed vs precision:

- **`M` (Edges per Node)**: The default is tightly optimized for 768d or 1536d floats. If using tiny datasets, increase `M` slightly to bound closer. If scaling massive collections, decrease `M` to save memory.
- **`ef_construction`**: The search effort during graph building. Keep this high (e.g. 100-200) for a highly polished dense graph layout.
- **`ef_search`**: The dynamically configurable parameter mapping search quality limit per query.

### Trick: Dynamic Ef
When hitting `POST /search`, you can adjust `ef_search` on the fly to maximize speed natively for non-critical queries.
```json
{
  "vector": [1.0, 0.4],
  "limit": 10,
  "params": {"ef_search": 64}
}
```

## 2. Leverage Zero-Allocation Pooling Natively

LimyeDB heavily leans on `sync.Pool` under the hood. For maximum ingest throughput:
- Send **Batch Insertions** instead of single points. Uploading 1000 points per request prevents HTTP overhead and drastically accelerates internal graph connections!
- Ensure clients implement internal back-off retries rather than overloading raw concurrent connections continuously.

## 3. Reverse Proxies & Leadership Traversal

When implementing a multi-node Raft deployment:
- Writes must go to the **Leader**.
- LimyeDB automatically hijacks cluster calls and dynamically executes `httputil.ReverseProxy` to tunnel followers silently into the active leader. 
- **Trick**: If your application is insanely read-heavy, spread traffic seamlessly across Followers statically. If it is purely write-heavy, point your Application Load Balancers (like NGINX/ALB) strictly to the `Leader` metrics endpoint so you bypass HTTP redirect latencies entirely.
