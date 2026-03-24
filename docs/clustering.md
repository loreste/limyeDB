# LimyeDB Clustering & Distributed Consensus

LimyeDB utilizes HashiCorp's Raft implementation to achieve strongly-consistent replication across a network of vector database nodes. It provides highly available, fault-tolerant vector storage natively.

## The Consensus Model

Under the hood, LimyeDB nodes form a Replicated State Machine (FSM). 
- **Leader Node**: Only one node serves as the Leader. All cluster writes (Create Collection, Upsert Points) stream exclusively through the Leader.
- **Follower Nodes**: Adhere to the Heartbeats broadcasted by the Leader. LimyeDB followers automatically intercept write payload requests locally and seamlessly proxy (Reverse Proxy) them to the active Leader.

### Node Bootstrapping

LimyeDB automatically leverages HTTP HashiCorp Consul APIs for remote peer discovery. Simply set up a Consul KV or point directly to seed IPs.

```bash
# Node 1 (Bootstrap Node)
./limyedb --bootstrap --bind-addr=192.168.1.10:7946

# Node 2 (Follower)
./limyedb --seed-nodes=192.168.1.10:7946 --bind-addr=192.168.1.11:7946
```

## Fault Tolerance & Log Compaction
LimyeDB automatically invokes periodic `createSnapshot` internals mapping active Index Memory graphs into the Raft store asynchronously when thresholds exceed limit, guarding against unbounded log memory traces.
