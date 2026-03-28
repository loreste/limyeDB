# Changelog

All notable changes to LimyeDB are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
- Hybrid search (dense + sparse via Reciprocal Rank Fusion)
- BM25 inverted index for sparse vector search
- SQLite-backed payload filtering with JSON extraction
- IVF (Inverted File) index
- ScaNN anisotropic quantization index
- DiskANN Vamana graph index
- Product, scalar, and binary quantization
- Memory-mapped vector and graph storage
- Write-ahead logging (WAL)
- S3 tiered storage
- Raft consensus clustering
- SWIM gossip protocol for failure detection
- Consistent hash ring for data distribution
- Multi-tenancy with RBAC and JWT authentication
- Auto-embedding orchestration (OpenAI, Cohere, Google)
- WebSocket real-time event streaming
- CDC mutation webhooks
- Semantic result caching
- Token bucket rate limiting
- Backup and restore (tar-based)
- Snapshot management
- Collection aliases
- Faceted search
- Query explanation/planning
- Prometheus metrics
- OpenTelemetry tracing
- CLI tool (`limyedb-cli`) for management and data import/export
- Docker and Docker Compose support

[0.2.0]: https://github.com/loreste/limyeDB/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/loreste/limyeDB/releases/tag/v0.1.0
