# Changelog

All notable changes to LimyeDB are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **HNSW Algorithm 4 neighbor selection** (Malkov & Yashunin §4.2) for the
  diversity-aware heuristic; previous code was naive top-M. `Mmax = M`
  on upper layers, `Mmax0 = 2*M` at layer 0, matching the paper.
- `?consistent=true` query parameter on read endpoints (`/search`,
  `/search/v2`, `/recommend`, `GET /collections`,
  `GET /collections/:name`, `GET /collections/:name/points/:id`). When
  set, the request is reverse-proxied to the current Raft leader for
  read-after-write semantics. Default behavior unchanged.
- `InsertBatch` (Raft path) responses now include real `succeeded` /
  `failed` counts plus an `errors[]` array of `{id, error}` entries.
  Previously the Raft path always reported `failed: 0` regardless of
  per-point validation outcomes. The shape is additive, so old clients
  reading only `succeeded` / `failed` still parse the same fields.
- Recall regression test for HNSW (`TestHNSWRecallSynthetic`) measuring
  recall@10 against a brute-force scan that uses the same
  `distance.Calculator` as HNSW. Previous baseline was 94.0%; now 100%.
- Cross-architecture SIMD correctness tests
  (`pkg/distance/simd_correctness_test.go`) so the same property holds
  on amd64 and arm64. Previously `simd_test.go` was gated to
  `//go:build amd64`, hiding the arm64 NEON bug for years.

### Changed (BREAKING)
- **Authentication is now required by default.** `./limyedb` refuses to
  start unless either `-auth-token <secret>` is supplied (preferred) or
  `-allow-anonymous` is passed (development only; logs a loud warning).
  Previously, omitting `-auth-token` silently ran an unauthenticated
  REST + gRPC server. Existing deployments must add one of the two
  flags to their launch command.

### Fixed
- **Distance kernel correctness on arm64.** The hand-written NEON
  assembly in `pkg/distance/simd_arm64.s` produced wrong values for
  `cosineDistanceNEON`, `euclideanDistanceNEON`, and `dotProductNEON`
  on non-trivial inputs. Disabled NEON until the assembly is fixed;
  scalar fallback is now used. HNSW recall@10 in our synthetic test
  jumps from 95.6% (with broken cosine) to 100% as a result.
- **WAL `Truncate` and seq-number persistence.** `loadSegments`
  populated `segmentInfo` with size and path only — `lastSeqNum` stayed
  at zero, so `Truncate(beforeSeqNum)` either deleted everything or
  nothing, and new writes after restart reused seq numbers from
  already-persisted records. `loadSegments` now scans each segment for
  its highest seq number, and `w.lastSeqNum` is seeded from the max
  across segments.
- **mmap durability.** `Storage.Sync()` now calls
  `unix.Msync(MS_SYNC)` before `file.Sync()` so dirty pages are
  flushed to disk; previously only the file descriptor was synced
  while pages still sat in the kernel cache. The allocator `.meta`
  file is now written via `tmp + fsync + rename`.
- **Snapshot atomic publish.** `pkg/storage/snapshot` writes
  `id.snap.tmp`, fsyncs, renames to `id.snap`, then writes
  `id.snap.meta.tmp`, fsyncs, renames to `id.snap.meta`. `ListSnapshots`
  filters on `.meta`, so the snapshot becomes visible only after the
  body is durably on disk. A crash mid-publish leaves at most an
  orphan body that can be swept on startup.
- **SQLite payload index per-shard isolation.** `payload.NewIndex("")`
  used `file::memory:?cache=shared`, which caused every in-memory
  payload index in a single process to share state. Each shard's
  payload index had access to other shards' filter results. Each
  instance now gets a uniquely-named in-memory database.
- **Filter `OpNotIn` ("nin") was missing from SQL generation** and
  silently returned no results. Wired into `buildWhereClause` mirroring
  `OpIn`.
- **CDC webhook subscriptions are SSRF-validated.** `Subscribe` now
  rejects URLs targeting private/loopback IP space, localhost variants,
  and non-HTTP(S) schemes before the subscription is recorded. This
  was previously enforced only by the unused `pkg/webhook` package.
- **gRPC context key.** `context.WithValue(ctx, "token_claims", claims)`
  used a built-in string key, which trips `go vet` and risks collisions
  with keys defined in other packages. Replaced with typed
  `auth.WithClaims` / `auth.ClaimsFromContext` helpers.
- **`handleUpdateCollection` no longer panics on race.** The handler
  used `coll, _ := s.collections.Get(name)` after `UpdateConfig` and
  immediately called `coll.Info()`; a concurrent `Delete` between the
  two left `coll == nil` and crashed the request goroutine.

### Removed
- **`internal/raft/`** (-651 LoC): standalone Raft skeleton superseded
  by Hashicorp Raft via `pkg/cluster`.
- **`pkg/tenancy/`** (-1029 LoC, two files): duplicate RBAC
  implementation that was never instantiated. Production auth lives in
  `pkg/auth`.
- **`pkg/observability/`** (-574 LoC): parallel metrics/OTel
  definitions never imported. Production metrics live in `pkg/metrics`.
- **`pkg/ratelimit/`** (-510 LoC): duplicate token bucket; production
  rate limiting is in `api/rest/middleware.go`.
- **`pkg/hybrid/`** (-2155 LoC): unused BM25/hybrid implementation.
  Production hybrid path uses `pkg/index/sparse.BM25Index` plus
  `sparse.FuseResults` from `pkg/collection/collection.go`.
- **`pkg/webhook/`** (-1020 LoC): unused webhook Manager. The wired
  webhook path goes through `pkg/cdc.Dispatcher`, which now has its own
  SSRF validation.
- **`test/surpass_qdrant_test.go`** (-535 LoC): only consumer of the
  dead packages above.

### Documentation
- Two README credibility passes plus a 14-file docs review removed
  ~2,500 lines of fabricated content covering features that didn't
  exist (multi-tenancy, mTLS, OpenTelemetry tracing, SPLADE,
  DiskANN/IVF/ScaNN as selectable indexes, `/sql`, `/autotune`,
  `/tenants`, `/admin/roles`, `/admin/users`, `LIMYEDB_*` env vars,
  per-collection replication factor, `cluster.token`, K8s auto-peer
  discovery, et al.) and corrected curl examples that referenced
  non-existent routes.

## [0.3.0] - 2026-04-06

### Added
- **Full Database Persistence**: LimyeDB now survives reboots with minimal data loss
  - WAL (Write-Ahead Log) integration with Insert, Delete, and Upsert operations
  - HNSW index metadata persistence (entry point, connections, deleted flags, id-to-index mapping)
  - Automatic recovery on startup: loads collections, restores index state, replays WAL
  - Graceful shutdown: syncs WAL, saves index metadata before exit
- New `pkg/index/hnsw/metadata.go` with `SaveMetadata()` and `LoadMetadata()` methods
- New `pkg/collection/recovery.go` with `Recover()` and `replayWAL()` methods
- Internal methods `insertInternal()`, `deleteInternal()`, `upsertInternal()` for WAL replay
- Point serialization helpers for binary WAL records
- Recovery tests for persistence verification

### Fixed
- HNSW `Delete()` now checks if point is already deleted to prevent double-counting during recovery
- WAL replay correctly handles idempotent operations

### Changed
- `Manager` struct now accepts optional WAL instance via `ManagerConfig`
- `Collection` struct now holds WAL reference for write operations
- Startup flow: recovery runs before servers start accepting requests
- Shutdown flow: WAL sync and index metadata save before closing

## [0.2.0] - 2026-03-28

### Added
- GoReleaser configuration with cross-platform binary builds (linux/darwin, amd64/arm64)
- GitHub Actions release workflow triggered on version tags
- Helm chart for Kubernetes deployment (`deploy/helm/limyedb/`)
- Example configuration file (`config.example.yaml`)
- Generated protobuf Go files for gRPC API (full binary now compiles)
- Benchmark workflow for pull requests
- Nightly full test workflow with integration tests
- Issue templates (bug report, feature request)
- Pull request template with checklist
- Dependabot configuration for Go modules and GitHub Actions
- SECURITY.md vulnerability disclosure policy
- CODEOWNERS file
- Pre-commit hooks configuration
- `Close()` method on payload index for SQLite connection cleanup
- `Stop()` method on RateLimiterStore to terminate cleanup goroutine
- `Shutdown()` method on RaftNode for graceful goroutine cleanup
- Semaphore to limit concurrent CDC webhook goroutines
- WaitGroup for gossip protocol goroutine lifecycle management
- SSRF protection for webhook URLs (rejects private IPs, localhost)
- Decompression bomb protection (10MB metadata limit in backup restore)

### Fixed
- Replaced `math/rand` with `crypto/rand` in all security-sensitive paths
- Constant-time comparison for API keys and bearer tokens (prevents timing attacks)
- Path traversal prevention with `filepath.Clean` and base directory validation
- Integer overflow protection with safe conversion helpers
- Zip Slip protection in backup archive extraction
- Parameterized SQL queries in payload index (prevents SQL injection)
- Restricted file permissions to 0600 and directory permissions to 0750
- HTTP client timeouts on OpenAI and Cohere embedders (30s)
- Wildcard CORS replaced with configurable allowed origins
- Race conditions in cluster coordinator (channel send under mutex, nested locks)
- Goroutine leaks in CDC dispatcher, gossip protocol, raft leadership, worker pool
- Thread-safe shard access in `GetState()` and `GetPrimaryNode()`
- HNSW Insert uses `defer unlock` to prevent lock leaks
- SemanticCache `FindSimilar` avoids nested lock acquisition

### Changed
- Dockerfile updated to Go 1.26 with CGO enabled and proto generation
- gRPC default port corrected from 6334 to 50051
- CI pipeline updated for Go 1.26.x with golangci-lint v2
- golangci-lint config rewritten for v2 format
- All Go source files formatted with `gofmt` and `goimports`
- Makefile simplified with updated targets matching actual project structure
- Raft integration test gated behind `LIMYEDB_INTEGRATION` environment variable
- Docker Publish workflow disabled (pending Docker Hub credentials)

## [0.1.0] - 2026-03-27

### Added
- Core vector database engine with HNSW indexing
- REST API with Gin framework
- gRPC API with streaming support
- Hybrid search (dense + sparse BM25 via Reciprocal Rank Fusion) at `/search/v2`
- BM25 inverted index for sparse vector search (clients supply `{indices, values}`; LimyeDB does not tokenize text server-side)
- SQLite-backed payload filtering with JSON extraction
- Product, scalar, and binary quantization
- Memory-mapped vector and graph storage
- Write-ahead logging (WAL)
- S3 archive storage (collection-level snapshot upload/download; not hot/warm/cold tiering)
- Raft consensus clustering via Hashicorp Raft (single Raft group)
- JWT authentication with per-collection RBAC
- Auto-embedding orchestration (OpenAI, Cohere)
- CDC mutation webhooks
- Semantic result caching
- Backup and restore (tar-based; in `pkg/backup`, not yet wired into the CLI)
- Server-side snapshot management (`POST /snapshots`)
- Collection aliases
- Faceted search
- Query explanation/planning
- Prometheus metrics
- CLI tool (`limyedb-cli`) for management and data import/export
- Docker and Docker Compose support

> **Note**: earlier versions of this changelog claimed
> IVF/ScaNN/DiskANN as selectable index types, multi-tenancy as a
> first-class primitive, OpenTelemetry tracing, SWIM gossip, consistent
> hash sharding, WebSocket event streaming on mutations, and a Google
> Vertex AI embedder. Those features had package-level code in the
> source tree but no production wiring (or, in the multi-tenancy case,
> were duplicates of `pkg/auth`). They have been removed from the
> 0.1.0 list to avoid implying behavior the binary did not exhibit.

[0.3.0]: https://github.com/loreste/limyeDB/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/loreste/limyeDB/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/loreste/limyeDB/releases/tag/v0.1.0
