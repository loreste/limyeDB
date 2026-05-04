# LimyeDB Clustering & High Availability

LimyeDB uses [Hashicorp Raft](https://github.com/hashicorp/raft) to
replicate collection metadata and points across a 3+ node cluster. There
is no external coordinator (no etcd/ZooKeeper/Consul) — Raft state lives
inside each node alongside the data.

## What's actually shipped

- **Single Raft group** across all cluster members. All writes flow
  through the leader and are committed once a quorum acknowledges.
- **Manual peer discovery.** A new node points at the current leader's
  REST address with `-raft-join http://<leader>:8080`. There is no
  built-in DNS, gossip, or Kubernetes peer discovery yet.
- **Strong-consistency reads via opt-in.** Reads default to the local
  FSM (eventually consistent on followers). Append `?consistent=true` to
  a read URL and the request is reverse-proxied to the current leader.
- **Snapshot-based recovery.** New followers catch up via the Raft log
  plus an FSM snapshot when they rejoin.

> Sharding-style horizontal partitioning is **not** implemented. The
> `pkg/cluster.{Coordinator,HashRing,ShardManager}` types exist in the
> source tree but are never instantiated from `cmd/limyedb` — production
> deployments are a single replicated state machine.

---

## Bootstrap a 3-node cluster

All nodes must share the same `-auth-token`; it is also the JWT signing
key, so JWTs minted on one node are accepted by the others.

```bash
# Node 1 (Bootstrap leader)
./limyedb \
  -raft-node-id node1 \
  -raft-bind 192.168.1.1:7000 \
  -raft-data /data/node1/raft \
  -raft-bootstrap \
  -rest 192.168.1.1:8080 \
  -data /data/node1 \
  -auth-token "$CLUSTER_SECRET"

# Node 2 (joins existing cluster)
./limyedb \
  -raft-node-id node2 \
  -raft-bind 192.168.1.2:7000 \
  -raft-data /data/node2/raft \
  -raft-join http://192.168.1.1:8080 \
  -rest 192.168.1.2:8080 \
  -data /data/node2 \
  -auth-token "$CLUSTER_SECRET"

# Node 3 (joins existing cluster)
./limyedb \
  -raft-node-id node3 \
  -raft-bind 192.168.1.3:7000 \
  -raft-data /data/node3/raft \
  -raft-join http://192.168.1.1:8080 \
  -rest 192.168.1.3:8080 \
  -data /data/node3 \
  -auth-token "$CLUSTER_SECRET"
```

## Read paths

- **Default (eventually consistent):** the receiving node serves reads
  from its local FSM, which can lag the leader by a small amount on
  followers.
- **Strong consistency:** append `?consistent=true` to any of the read
  endpoints (`POST /collections/:name/search`, `/search/v2`, `/recommend`,
  `GET /collections`, `GET /collections/:name`,
  `GET /collections/:name/points/:id`). The handler proxies to the
  current leader, whose FSM has all committed writes by definition.

## Failover

If the leader is lost, Raft elects a new leader from the surviving voters.
Writes against a follower error with the leader's address; the REST
handler reverse-proxies to the leader automatically.

## Snapshots & log compaction

Raft triggers periodic snapshots of the FSM into `pkg/storage/snapshot`,
which are written via tmp+fsync+rename so a crash mid-snapshot cannot
leave a partially-published `.snap`/`.meta` pair. The Raft log is
truncated against the snapshot index after each successful publish.
