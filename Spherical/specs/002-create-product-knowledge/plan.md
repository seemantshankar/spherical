# Implementation Plan: Product Knowledge Engine Library

**Branch**: `002-create-product-knowledge` | **Date**: 2025-11-27 | **Spec**: `specs/002-create-product-knowledge/spec.md`
**Input**: Feature specification from `/specs/002-create-product-knowledge/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Build a reusable Go library plus CLI/API/GraphQL/gRPC surface (`libs/knowledge-engine`) that ingests brochure-derived Markdown, normalizes it into multi-tenant relational tables, stores semantic chunks in a vector index, and exposes low-latency retrieval/comparison endpoints for the AI sales agent. Phase 1 delivers the ingestion pipeline (SQLite dev with FAISS adapter, Postgres + PGVector prod), hybrid retrieval service with intent routing, comparison cache, lineage auditing, drift monitoring hooks, CSV/Parquet export tooling, and ingestion benchmarks so OEM admins can publish campaigns confidently while the voice agent receives grounded answers in <150 ms p50. The ingestion CLI shells out to the existing `libs/pdf-extractor/cmd/pdf-extractor` binary (feature 001) whenever a PDF path is supplied, capturing the Markdown output automatically before normalization. The interactive CLI demo (`knowledge-demo`) must use the production Router directly (not a simplified version) to ensure consistent behavior and production parity. The Router implements keyword-first routing with confidence-based fallback: simple queries (confidence ≥0.8) return immediately from keyword search (<25 ms), while complex queries trigger vector search fallback. Only vector search results are cached (5-10 min TTL) to optimize latency for complex queries while keeping simple queries fast. The Router tracks metrics for path distribution, latency per path, and confidence scores. Every runtime capability (ingestion, retrieval, comparison, drift) ships with CLI commands so we remain CLI-first even when REST/GraphQL/gRPC surfaces are available. Compliance insights are exposed through CLI + APIs; a bespoke dashboard UI is explicitly deferred for a later release.

## Technical Context

<!--
  ACTION REQUIRED: Replace the content in this section with the technical details
  for the project. The structure here is presented in advisory capacity to guide
  the iteration process.
-->

**Language/Version**: Go 1.25 (module in `libs/knowledge-engine`)  
**Primary Dependencies**: `pgx/v5` + `pgvector-go` for Postgres/PGVector, `sqlc` for typed queries, `chi` for REST, `gqlgen` for GraphQL schema/resolvers, `connectrpc.com/connect` (gRPC/Connect), `cobra` for CLI, `redis/go-redis` for cache, FAISS C bindings for dev vector adapter, `testcontainers-go` for integration harness  
**Storage**: SQLite 3 (dev/test) + FAISS/in-memory ANN for local semantic search, Postgres 16 with PGVector 0.7 (prod) + Redis 7 for hot cache, S3-compatible object store for brochure artifacts  
**Testing**: `go test ./...` with TDD-first unit suites, `testcontainers-go` backed integration tests, Schemathesis/Newman contract tests across REST + GraphQL + gRPC  
**Target Platform**: Containerized Linux amd64/arm64 (prod) with macOS dev parity; deployable as Go library, CLI, REST, GraphQL, or gRPC service  
**Project Type**: Library-first Go module with CLI subcommand(s) and multi-protocol façade (REST/GraphQL/gRPC)  
**Performance Goals**: Retrieval API ≤150 ms p50 / ≤350 ms p95, ingestion of 20-page brochure ≤15 min, comparison queries ≤250 ms p95, cache hit ratio ≥70% on hot specs, simple keyword queries (confidence ≥0.8) ≤25 ms p50 by skipping vector search
**Constraints**: Strict TDD cadence, CLI-first operability, OAuth2 client-credentials + mTLS between services, 5-year data retention with purge tooling, library boundaries per module (<500 LOC), multi-tenant row-level security, no mocks in integration tests, <500 MB RSS for ingestion runs  
**Scale/Scope**: Target 10 OEM tenants, 200 products, 1k campaign variants, 20 RPS sustained retrieval load, embeddings up to 5 M chunks (approx. 4 GB vectors)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **Principle I – TDD**: Plan enforces test-first workflow (`go test` + contract suites) before implementation and defines integration validations for ingestion/routing.
- **Principle II – Library First**: Scope centers on `libs/knowledge-engine` as a standalone module with documented interfaces; apps (CLI/API) sit under `cmd/`.
- **Principle III – CLI First**: Operations (ingest, publish, drift audit) exposed via Cobra CLI with JSON output in addition to service endpoints.
- **Principle IV – Integration Testing Without Mocks**: Testcontainers harness spins real Postgres/Redis/PGVector to validate hybrid retrieval and ingestion dedupe.
- **Principle VI/VII/VIII – Go + Modular & Size Guardrails**: Design keeps Go as the only implementation language and splits packages (`internal/ingest`, `internal/retrieval`, `internal/storage`) to stay <500 LOC each.
- **Principle IX – Enterprise Security**: Multi-tenant tagging, audit logs, encryption at rest (Postgres/Redis TLS), and signed ingestion artifacts baked into requirements.
- **Principle X – Documentation**: Spec, plan, research, data-model, quickstart, and API contracts included; CLI help and README updates tracked in deliverables.
- **Principle XI – Git Workflow**: Work continues on feature branch `002-create-product-knowledge`; plan documents need for worktrees when parallelizing ingestion vs retrieval streams.

*Post-Phase 1 re-check: PASS — design artifacts preserve all constitutional guarantees without additional exceptions.*

## Project Structure

### Documentation (this feature)

```text
specs/002-create-product-knowledge/
├── plan.md              # This file (/speckit.plan output)
├── research.md          # Phase 0 decisions + clarifications
├── data-model.md        # Phase 1 entity + relationship design
├── quickstart.md        # Phase 1 developer enablement guide
├── contracts/
│   └── knowledge-engine.openapi.yaml
└── tasks.md             # Created by /speckit.tasks in Phase 2
```

### Source Code (repository root)

```text
libs/
├── knowledge-engine/
│   ├── go.mod
│   ├── cmd/
│   │   ├── knowledge-engine-cli/        # ingestion + admin CLI
│   │   └── knowledge-engine-api/        # REST/GraphQL/gRPC entrypoints
│   ├── internal/
│   │   ├── ingest/                      # brochure parsing + normalization
│   │   ├── storage/                     # repositories, sqlc queries, RLS helpers
│   │   ├── retrieval/                   # intent routing, hybrid search
│   │   ├── comparison/                  # cross-product cache + diff logic
│   │   ├── monitoring/                  # drift detection, lineage emitters
│   │   └── api/
│   │       ├── graphql/                 # gqlgen schema + resolvers
│   │       └── grpc/                    # Connect/gRPC handlers generated from proto
│   ├── pkg/engine/                      # public Go API for embedding in other services
│   ├── api/proto/                       # knowledgeengine/v1/*.proto
│   └── testdata/                        # golden Markdown + fixtures
├── pdf-extractor/                       # upstream brochure -> Markdown producer (existing)
└── markdown-tools/                      # cleanup utilities (existing)

tests/
├── integration/knowledge-engine/        # Testcontainers Postgres/PGVector/Redis suites
├── contract/knowledge-engine/           # Newman/Postman or dredd runs vs OpenAPI
└── unit/knowledge-engine/               # TDD suites per package
```

**Structure Decision**: Single Go module inside `libs/knowledge-engine` keeps the Library-First requirement intact while letting CLI/API binaries live under `cmd/`. Shared integration + contract tests stay under `tests/` at repo root to exercise the published interfaces end-to-end.

## Complexity Tracking

Not required — all constitution gates satisfied without exceptions.
