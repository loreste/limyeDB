# LimyeDB Clustering & Distributed Consensus

LimyeDB utilizes HashiCorp's Raft implementation to achieve strongly-consistent replication across a network of vector database nodes. It provides highly available, fault-tolerant vector storage natively.

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
