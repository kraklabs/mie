# Changelog

All notable changes to MIE (Memory Intelligence Engine) will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.3.6] - 2026-03-12

### Fixed

- **Fork bomb in daemon start**: `forkDaemonBackground` and `connectOrStartDaemon` spawned `mie daemon start` without `--foreground`, so each child re-forked indefinitely (~1 fork/500ms). The forked child now runs with `--foreground` to enter the actual daemon serve loop

## [1.3.5] - 2026-03-12

### Fixed

- Daemon child processes left as zombies: `runDaemonStart` and `connectOrStartDaemon` now call `cmd.Wait()` in a background goroutine after `cmd.Start()` to reap the forked daemon process
- Zombie accumulation when daemon runs as PID 1 in a container without tini: a `SIGCHLD` handler with `Wait4(WNOHANG)` now reaps orphaned child processes automatically

## [1.3.4] - 2026-02-15

### Fixed

- `--foreground` flag ignored in `mie daemon start --foreground`: the parent `runDaemon` FlagSet had `UnknownFlags = true` which silently consumed `--foreground` before it reached `runDaemonStart` — the daemon would fork into background mode and exit immediately, causing systemd services to fail with a restart loop

## [1.3.3] - 2026-02-15

### Changed

- `mie daemon start` now runs in background by default; use `--foreground` for debugging (replaces `--background`)
- `mie status` connects to the daemon via socket first, falling back to direct RocksDB only if the daemon is not running — fixes `LOCK: Resource temporarily unavailable` when daemon is active

## [1.3.2] - 2026-02-15

### Fixed

- Broken pipe error on daemon close: `handleConn` now returns immediately on `MethodClose` without attempting to write a response to the already-closed client connection

## [1.3.1] - 2026-02-14

### Added

- Auto-start daemon on `mie init`: the daemon is started after config creation (and interview if used), so the MCP server is immediately ready when the IDE connects

## [1.3.0] - 2026-02-14

### Added

- **Daemon multi-instance architecture**: a single `mie daemon` process holds the exclusive RocksDB lock, multiple MCP clients connect via Unix domain socket — eliminates "database locked" errors when agents run concurrently
- `mie daemon start|stop|status` CLI commands for managing the daemon lifecycle
- `--background` flag for `mie daemon start` to run the daemon as a detached process
- Auto-start daemon: `mie --mcp` automatically starts the daemon if not running, with retry/backoff connection logic
- `MetaBackend` interface abstracting `EmbeddedBackend` and `SocketBackend` for transparent local/remote storage
- `SocketBackend`: client that forwards Datalog queries over Unix domain socket using newline-delimited JSON protocol
- `Daemon` server: accepts socket connections, dispatches queries to the embedded CozoDB backend
- `NewClientWithBackend` constructor for connecting `memory.Client` to a pre-existing backend (e.g. daemon socket)
- Ping-based liveness check: `SocketBackend.Ping()` verifies daemon is alive after connecting
- PID file locking with `flock(LOCK_EX|LOCK_NB)` to prevent concurrent daemon starts
- Daemon connection tracking: active connections are stored in a map and closed on shutdown for prompt cleanup
- Socket file permissions restricted to owner-only (`0600`) to prevent local privilege escalation
- Embedding dimension validation: daemon stores dimensions in `mie_meta`, clients verify on connect to catch mismatches early
- 13 new socket/daemon tests + 2 dimension validation tests in `pkg/memory`

### Fixed

- **Deadlock in `SocketBackend.Close()`** (CRITICAL): `Close()` now releases the mutex before closing the connection, unblocking any goroutine stuck in `ReadBytes`
- **Silent fallback to embedded mode** (CRITICAL): `mie --mcp` no longer silently falls back to embedded storage when the daemon is unreachable — fails with a clear error and hint message instead
- **PID file race condition** (CRITICAL): daemon start now uses `flock()` for atomic ownership instead of write-then-check, preventing two daemons from starting simultaneously
- **Stale socket without live process** (CRITICAL): `connectOrStartDaemon` now pings after connecting and removes stale sockets from crashed daemons
- Scanner errors in daemon `handleConn` are now logged instead of silently ignored
- Daemon `handleConn` receives the parent context for proper cancellation propagation
- Background daemon startup verifies the process is still alive via `Signal(0)` after a brief startup period
- `SocketBackend.send()` marks backend as closed on I/O errors so subsequent calls fail fast
- `SocketBackend.send()` validates response ID matches request ID to detect protocol desync
- `SocketBackend.Close()` sends `MethodClose` to daemon for clean disconnect before closing the connection

### Changed

- MCP server version bumped to 1.3.0
- `mie --mcp` requires a running daemon (auto-started or manual) instead of opening CozoDB directly
- Daemon shutdown waits up to 5 seconds for active handlers to complete before forcing exit
- `runDaemonStop` handles stale PID files gracefully (removes file if process not found)

## [1.2.0] - 2026-02-09

### Added

- `mie_repair` tool: rebuilds HNSW indexes and cleans orphaned embeddings when semantic search encounters corruption
- Embedding backfill: `BackfillEmbeddings` generates vectors for nodes created before embeddings were enabled
- Semantic search result boosting: content containing the query text gets its distance halved
- Semantic search quality filter: results with cosine distance > 0.6 (< 40% similarity) are excluded
- Field length validation: content (50,000 chars), titles (500), descriptions (2,000), names (200)
- `mie_bulk_store` pre-validation: validates all items (required fields, enums, JSON arrays, date formats) before storing any, ensuring atomic-or-nothing behavior
- Entity upsert detection: `mie_store` now shows "Stored new entity" vs "Updated existing entity"
- `topic` added to MCP schema `node_types` enum across all tools
- Fact invalidation without replacement: `replacement_id` is now optional in `mie_update`; omitting it marks the fact invalid without creating a replacement chain
- Edge existence check: `mie_delete remove_relationship` now verifies the edge exists before attempting removal
- Invalidation target pre-validation: `mie_store` with `invalidates` verifies the target fact exists before storing the new fact
- JSON validation for `alternatives` field in decisions (rejects non-JSON-array values)
- Warning for empty `role` on `decision_entity` edges
- Embedding coverage stats per node type in `mie_status` (e.g., "Facts: 47/49")
- Decision status breakdown in `mie_status` (e.g., "9 active, 1 superseded")
- Orphaned edge cleanup during node deletion

### Fixed

- **Datalog export syntax** (HIGH): now generates valid CozoDB `:put` format with proper key/value separation — previously produced unparseable output
- **`mie_analyze` conflict display**: shows the existing fact (with ID) instead of the empty proposed fact
- **`mie_query` filter pass-through**: `category`/`kind` filters no longer exclude Decision and Event types that don't have those fields
- **Embedding deletion**: errors in `DeleteNode` now log warnings instead of being silently swallowed, preventing orphaned HNSW entries
- **Async embedding timeout**: embedding generation now uses a 30-second timeout instead of unbounded `context.Background()`
- **`mie_export` embeddings message**: only shown when actual embedding count > 0
- **`MockEmbeddingProvider`**: uses word-level hashing so texts sharing words produce similar vectors, fixing `TestIntegrationSemanticSearch`

### Changed

- Edge schema refactored to support explicit key/value column separation for CozoDB `:put` syntax
- Sorting and filtering logic improved in list and query operations with better error handling
- CI workflow updated to golangci-lint-action v7
- MCP server version bumped to 1.2.0

## [0.1.9] - 2026-02-08

### Fixed

- Search queries now return `created_at` column, enabling date filters (`created_after`/`created_before`) to work correctly
- ExactSearch no longer hardcodes `valid = true` in Datalog, allowing `valid_only=false` to return invalidated facts
- `parseSearchResult` now populates Confidence, Status, EventDate, and CreatedAt in metadata for all node types
- Invalid fact category now returns an error instead of silently falling back to "general"
- Out-of-range confidence now returns an error instead of silently resetting to 0.8
- Edge target nodes are validated before creating relationships; dangling edges are skipped with a warning
- `target_ref` out-of-bounds errors in `mie_bulk_store` now report the item index and batch size
- Datalog export uses single-quoted CozoDB literals with proper escaping instead of Go `%q` double-quoting
- Status health section now shows "Embeddings: active" / "not configured" instead of the misleading "Embeddings enabled (provider not configured)"
- Export edge filtering: only edges whose both endpoint types are in the requested `node_types` are exported
- `mie_decision_entity` edge table now accepts the `role` field

### Added

- Topic filter support for `mie_list`: filter nodes by topic name via the `topic` parameter
- "Valid" column in fact list table output showing "yes"/"no"
- Correct pluralization in `mie_bulk_store` output: "1 entity" instead of "1 entitys"
- Comprehensive E2E test suite: 19 Go tests + bash script with 127 MCP integration assertions

### Changed

- `mie_status` internally uses `strings.Builder` instead of string concatenation

## [0.1.8] - 2026-02-08

### Added

- Topic graph traversals: `facts_about_topic`, `decisions_about_topic`, `entities_about_topic` for navigating topic relationships
- Tiered conflict recommendations based on similarity score: high (>=90%), medium (>=75%), and low similarity produce distinct guidance
- Type-aware sort validation in `mie_list` with "name" alias mapped per node type

### Fixed

- `escapeDatalog` now escapes newlines, carriage returns, tabs, and null bytes to prevent CozoDB query issues with multiline strings
- Embeddings status messages clarified: "provider active" vs "provider not configured"
- MCP schema for `mie_query` traversal enum now includes topic traversal types

## [0.1.7] - 2026-02-08

### Fixed

- Makefile `lint` target missing `--build-tags cozodb` and CGO environment variables, causing it to analyze different code than CI

### Changed

- Refactored `runInterview` to extract `collectInterviewAnswers`, reducing cognitive complexity below threshold

### Added

- ExactSearch integration test covering both `mem` and `rocksdb` storage engines

## [0.1.6] - 2026-02-08

### Added

- `mie_delete` tool with cascade delete (node + embedding + all associated edges) and relationship removal
- `created_after` and `created_before` time-range filters on `mie_list` and `mie_query`
- Human-readable UTC timestamps in `mie_get` and `mie_list` output (replaces raw unix timestamps)

## [0.1.5] - 2026-02-08

### Added

- `mie_get` tool for retrieving a single memory node by its ID with full details
- `category`, `kind`, and `valid_only` post-search filters wired into `mie_query` for both semantic and exact modes

### Fixed

- ExactSearch returning 0 facts: `parseSearchResult` did not set `Valid=true` on Fact metadata, causing the `valid_only` filter to drop all results

### Removed

- Dead `content_type` parameter from `mie_analyze` (was accepted but never used)

## [0.1.4] - 2026-02-08

### Fixed

- `confidence=0` silently replaced with default 0.8 due to dual-layer validation bug in `writer.go` (`<= 0` instead of `< 0`)
- MCP schema for confidence parameter declared `"default": 0.8`, causing some clients to omit zero values
- Confidence display format `%.1f` rounded low values like 0.01 to "0.0" — now uses `%g` for full precision

## [0.1.3] - 2026-02-08

### Fixed

- Conflict detection threshold mismatch: similarity value (0.85) was incorrectly used as cosine distance, causing massive false positives
- Category filter ignored in HNSW neighbor search during conflict detection
- CozoDB injection via unvalidated `sort_by` parameter in `mie_list` (added allowlist validation)
- `confidence=0` rejected as invalid in `mie_store` — zero is a valid confidence value
- `mie_export` omitting all relationship edges from output
- Replaced O(n²) bubble sort with `sort.Slice` in conflict detection

## [0.1.2] - 2026-02-06

### Added

- `mie_bulk_store` tool for batch storage of up to 50 nodes per call, with cross-batch references via `target_ref` (0-based index into the items array)
- MCP `initialize` response now includes `instructions` field guiding agents on proactive memory capture, query-first behavior, and self-import workflows
- MCP resource `mie://context/recent` for preflight context injection (latest facts, decisions, and entities)
- `mie import` CLI command supporting JSON and Datalog formats (inverse of `mie export`)
- `mie init --interview` flag for interactive project bootstrapping (asks about stack, team, and pre-populates entities/topics)
- Usage counters in `mie_status`: total queries, total stores, and last query/store timestamps tracked in `mie_meta`
- Self-import instructions for markdown/ADRs and git history extraction (agent-driven, no external LLM needed)

### Removed

- `LLMConfig` from configuration — the connected AI agent is the LLM, MIE stays as a pure storage/retrieval engine

## [0.1.0] - 2026-02-05

### Added

- Core memory graph with five node types: facts, decisions, entities, events, and topics
- Six relationship edge types: `fact_entity`, `fact_topic`, `decision_topic`, `decision_entity`, `event_decision`, `entity_topic`
- MCP server (JSON-RPC 2.0 over stdio) with 8 tools: `mie_analyze`, `mie_store`, `mie_query`, `mie_update`, `mie_list`, `mie_conflicts`, `mie_export`, `mie_status`
- Three query modes: semantic (embedding similarity), exact (substring match), graph (relationship traversal)
- Fact invalidation chains with replacement tracking
- CozoDB storage backend with RocksDB, SQLite, and in-memory engines
- Embedding support via Ollama, OpenAI, and Nomic providers
- `mie init` for project initialization with `.mie/config.yaml`
- `mie export` in JSON and Datalog formats
- Configuration via YAML file with environment variable overrides
- Conflict detection for semantically similar but potentially contradicting facts

[1.3.3]: https://github.com/kraklabs/mie/compare/v1.3.2...v1.3.3
[1.3.2]: https://github.com/kraklabs/mie/compare/v1.3.1...v1.3.2
[1.3.1]: https://github.com/kraklabs/mie/compare/v1.3.0...v1.3.1
[1.3.0]: https://github.com/kraklabs/mie/compare/v1.2.0...v1.3.0
[1.2.0]: https://github.com/kraklabs/mie/compare/v0.1.9...v1.2.0
[0.1.9]: https://github.com/kraklabs/mie/compare/v0.1.8...v0.1.9
[0.1.8]: https://github.com/kraklabs/mie/compare/v0.1.7...v0.1.8
[0.1.7]: https://github.com/kraklabs/mie/compare/v0.1.6...v0.1.7
[0.1.6]: https://github.com/kraklabs/mie/compare/v0.1.5...v0.1.6
[0.1.5]: https://github.com/kraklabs/mie/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/kraklabs/mie/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/kraklabs/mie/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/kraklabs/mie/compare/v0.1.0...v0.1.2
[0.1.0]: https://github.com/kraklabs/mie/releases/tag/v0.1.0