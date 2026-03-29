# LimyeDB Clustering & High Availability

LimyeDB provides **production-grade distributed clustering** without external dependencies. While other vector databases require etcd, ZooKeeper, or Consul for coordination, LimyeDB includes everything you need in a single binary.

## Why LimyeDB Clustering is Different

| Feature | LimyeDB | Alternatives |
|---------|---------|--------------|
| **Self-contained** | No external coordinators needed | Requires etcd, ZooKeeper, or Consul |
| **Raft Consensus** | Built-in strong consistency | Often requires external setup |
| **SWIM Gossip** | Automatic failure detection | Manual health checks |
| **Auto-Discovery** | Native Kubernetes DNS integration | Sidecar or operator required |
| **Replication** | Configurable per collection | Global or none |
| **Deployment** | Single binary per node | Multiple services to manage |

LimyeDB utilizes a hybrid clustering architecture combining Raft for strong consistency and SWIM gossip for efficient failure detection, achieving highly available, fault-tolerant vector storage with minimal operational overhead.

---

## The Consensus Model

Under the hood, LimyeDB nodes form a Replicated State Machine (FSM). 
- **Leader Node**: Only one node serves as the Leader. All cluster writes (Create Collection, Upsert Points) stream exclusively through the Leader.
- **Follower Nodes**: Adhere to the Heartbeats broadcasted by the Leader. LimyeDB followers automatically intercept write payload requests locally and seamlessly proxy (Reverse Proxy) them to the active Leader.

### Node Bootstrapping (Kubernetes Auto-Discovery)

LimyeDB automatically leverages native Kubernetes DNS headless services for remote peer discovery, dropping the legacy requirement for Consul KV or external orchestrators.

```bash
# Node 1 (Bootstrap Node starting the RAFT ring)
./limyedb --bootstrap --bind-addr=0.0.0.0:7946

# Node 2 (Follower routing dynamically via K8s DNS)
./limyedb --seed-nodes=limyedb-0.limyedb-headless.default.svc.cluster.local:7946 --bind-addr=0.0.0.0:7946
```

## Fault Tolerance & Log Compaction
LimyeDB automatically invokes periodic `createSnapshot` internals mapping active Index Memory graphs into the Raft store asynchronously when thresholds exceed limit, guarding against unbounded log memory traces.
