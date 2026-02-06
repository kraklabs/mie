# Changelog

All notable changes to MIE (Memory Intelligence Engine) will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

[0.1.2]: https://github.com/kraklabs/mie/compare/v0.1.0...v0.1.2
[0.1.0]: https://github.com/kraklabs/mie/releases/tag/v0.1.0