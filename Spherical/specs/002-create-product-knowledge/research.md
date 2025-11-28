# Research – Product Knowledge Engine Library

## Decision 1: Storage + Vector Stack
- **Decision**: Use SQLite 3 for developer parity and migrations, Postgres 16 + PGVector 0.7 for production, and Redis 7 for hot caches.
- **Rationale**: SQLite keeps onboarding trivial (single file, zero infra) while matching SQL semantics with Postgres, so schema migrations and sqlc queries stay identical. PGVector embeds directly inside the relational store, eliminating cross-service hops and letting us filter by tenant/product via SQL. Redis handles low-latency cache of structured spec responses and intent classifications without overloading Postgres.
- **Alternatives considered**: Qdrant/Milvus (rejected: extra infra, separate ACLs for tenants), DuckDB (rejected: not ideal for long-running service writes, lacks PG-compatible vector indexes), DynamoDB + Kendra (rejected: higher cost and slower cross-region latency, complicates on-prem installs).

## Decision 2: Data Access Pattern
- **Decision**: Adopt `sqlc`-generated repositories layered under `internal/storage` with explicit row-level security enforcement helpers.
- **Rationale**: sqlc keeps Go structs tightly coupled to SQL while allowing fine-grained queries for specs, feature blocks, and comparison rows. It also plays nicely with SQLite and Postgres simultaneously and keeps packages under the 500 LOC threshold by pushing SQL into `.sql` files. Explicit helper functions enforce tenant/product filters before any query executes, satisfying the enterprise security mandate.
- **Alternatives considered**: GORM/Ent (rejected: ORM reflection overhead, harder to guarantee SQL parity between SQLite and Postgres, more difficult to reason about PGVector-specific syntax), plain `pgx` everywhere (rejected: more boilerplate + error-prone scanning).

## Decision 3: Hybrid Retrieval Router
- **Decision**: Implement a dedicated intent classifier + router inside `internal/retrieval/router` that first attempts deterministic spec lookups, then falls back to semantic or hybrid search with reranking only when necessary.
- **Rationale**: Most questions (~70% per discovery) are strict spec lookups; routing them directly to SQL keeps latency down and costs near-zero. The router can still issue PGVector similarity queries when classification confidence drops or when the prompt explicitly requests qualitative info/comparisons. This matches the performance targets (≤150 ms p50) and avoids burning token budget on unnecessary rerankers.
- **Alternatives considered**: Always-on hybrid search (rejected: higher latency and cost), using an external LLM router service (rejected: introduces network dependency + new failure modes), building a rule-only router (rejected: too rigid for marketing questions).

## Decision 4: Comparison Cache
- **Decision**: Materialize `comparison_rows` via background jobs that compute deltas for allowed product pairs and store them alongside provenance metadata.
- **Rationale**: Comparisons are expensive if calculated on-demand (two spec queries + semantic context). Precomputing keeps the conversational thread responsive and ensures only shareable data escapes tenant boundaries. Metadata (source doc + timestamp) doubles as a governance audit artifact.
- **Alternatives considered**: Computing comparisons on the fly (rejected: slower and harder to audit), storing comparisons in Redis only (rejected: loses durability/history needed for audits).

## Decision 5: Drift + Lineage Tracking
- **Decision**: Emit lineage events per chunk/spec into `internal/monitoring` that write to the primary DB plus optional OpenTelemetry logs, and schedule a daily drift job comparing brochure hashes + age thresholds.
- **Rationale**: OEM compliance teams demanded traceability; storing lineage alongside spec/USP rows ensures every retrieval response can cite a brochure page + ingestion run. Drift scanning prevents stale campaigns by reusing the same metadata; hooking into OTEL also lets the AI ops team visualize adoption across tenants.
- **Alternatives considered**: External data catalog (rejected: would delay MVP, adds another system to secure), manual spreadsheet tracking (rejected: not scalable, violates real-time task updates principle).

## Decision 6: Keyword Confidence-Based Routing
- **Decision**: Router calculates keyword search confidence based on exact matches (category/name/value, +0.3 each), partial matches (+0.1 each), query complexity (simple queries ≤2 keywords get +0.2 bonus, complex >4 keywords get -0.1 penalty), and result count (diminishing returns, max +0.3). When confidence ≥0.8 (configurable `KeywordConfidenceThreshold`), router returns immediately without vector search. Only vector search results are cached (5-10 min TTL), not keyword-only results. The `knowledge-demo` CLI uses the production Router directly to ensure consistent behavior and production parity.
- **Rationale**: Simple queries like "weight" achieve <25 ms latency by skipping expensive vector search (~400-600 ms). Complex queries like "safety features for children" trigger vector fallback when keyword confidence is low. Caching only slow paths (vector search) optimizes latency for complex queries while keeping simple queries fast. Using production Router in demo ensures consistent behavior and easier maintenance.
- **Alternatives considered**: Always running vector search (rejected: unnecessary latency for simple queries), caching all results (rejected: keyword-only results are already fast, caching adds overhead), implementing a simplified router for demo (rejected: inconsistent behavior, harder to maintain).

