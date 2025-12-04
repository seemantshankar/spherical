# Implementation Plan: Row-Level Chunking for Table Data

**Branch**: `004-row-level-chunking` | **Date**: 2025-12-04 | **Spec**: `specs/004-row-level-chunking/spec.md`
**Input**: Feature specification from `/specs/004-row-level-chunking/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Enhance the existing knowledge engine ingestion pipeline to implement row-level chunking for table data, converting each table row into an individual semantic chunk. This enables precise retrieval of specific table attributes (e.g., "car colors") instead of returning entire table chunks. The implementation modifies the parser to detect tables, extract row-level chunks with structured text format (key-value pairs from 5 columns: Parent Category, Sub-Category, Specification, Value, Additional metadata), generate content hashes for deduplication, link chunks to ParsedSpec records, and process embeddings in batches of 50-100 chunks. The query/retrieval system is enhanced to support hierarchical grouping by parent category and sub-category. The feature maintains backward compatibility by requiring re-ingestion of existing documents. All table rows are processed with error handling that stores incomplete chunks (failed embeddings) for later retry without blocking ingestion.

## Technical Context

**Language/Version**: Go 1.25 (module in `libs/knowledge-engine`)  
**Primary Dependencies**: Existing dependencies from `002-create-product-knowledge`: `pgx/v5` + `pgvector-go`, `sqlc`, `chi`, `gqlgen`, `connectrpc.com/connect`, `cobra`, `redis/go-redis`, `testcontainers-go`  
**Storage**: SQLite 3 (dev/test) + FAISS/in-memory ANN for local semantic search, Postgres 16 with PGVector 0.7 (prod) + Redis 7 for hot cache  
**Testing**: `go test ./...` with TDD-first unit suites, `testcontainers-go` backed integration tests  
**Target Platform**: Containerized Linux amd64/arm64 (prod) with macOS dev parity  
**Project Type**: Enhancement to existing Go library module (`libs/knowledge-engine`)  
**Performance Goals**: Ingestion of document with 200 table rows completes within 10 minutes, query response time remains within existing budgets (p50 ≤150 ms, p95 ≤350 ms) despite increased chunk count  
**Constraints**: Strict TDD cadence, CLI-first operability, library boundaries per module (<500 LOC), multi-tenant row-level security, no mocks in integration tests, backward compatibility (re-ingestion required)  
**Scale/Scope**: Handle tables with 100+ rows efficiently, batch embedding generation (50-100 chunks per batch), content hash-based deduplication across documents

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **Principle I – TDD**: Plan enforces test-first workflow (`go test` + integration suites) before implementation. Tests validate row-to-chunk mapping accuracy, content hash generation, batch embedding processing, and hierarchical grouping.
- **Principle II – Library First**: Changes are scoped to `libs/knowledge-engine` as a standalone module; CLI/API remain under `cmd/`.
- **Principle III – CLI First**: Ingestion operations continue via existing Cobra CLI; no new CLI commands required (re-ingestion uses existing ingest command).
- **Principle IV – Integration Testing Without Mocks**: Testcontainers harness validates table parsing, row chunk generation, batch embedding, and query grouping with real Postgres/PGVector.
- **Principle VI/VII/VIII – Go + Modular & Size Guardrails**: Implementation uses Go only, extends existing packages (`internal/ingest`, `internal/retrieval`) while maintaining <500 LOC per package boundaries.
- **Principle IX – Enterprise Security**: Multi-tenant tagging preserved, audit logs maintained, content hash enables secure deduplication.
- **Principle X – Documentation**: Spec, plan, research, data-model, quickstart included; updates to existing README/docs tracked.
- **Principle XI – Git Workflow**: Work continues on feature branch `004-row-level-chunking`.

*Post-Phase 1 re-check: PASS — design artifacts preserve all constitutional guarantees without additional exceptions.*

## Project Structure

### Documentation (this feature)

```text
specs/004-row-level-chunking/
├── plan.md              # This file (/speckit.plan output)
├── research.md          # Phase 0 decisions + clarifications
├── data-model.md        # Phase 1 entity + relationship design
├── quickstart.md        # Phase 1 developer enablement guide
└── tasks.md             # Created by /speckit.tasks in Phase 2
```

### Source Code (repository root)

```text
libs/knowledge-engine/
├── internal/
│   ├── ingest/
│   │   ├── parser.go              # MODIFY: Add row-level chunk generation
│   │   └── pipeline.go            # MODIFY: Add batch embedding processing
│   ├── retrieval/
│   │   └── router.go              # MODIFY: Add hierarchical grouping support
│   └── storage/
│       ├── models.go              # MODIFY: Add content hash, completion status to KnowledgeChunk
│       └── repositories.go        # MODIFY: Add content hash queries, retry logic
└── tests/
    └── integration/
        └── row_chunking_test.go   # NEW: Integration tests for row-level chunking
```

**Structure Decision**: Extends existing packages without creating new modules. Row-level chunking is integrated into the existing ingestion pipeline and retrieval system, maintaining backward compatibility.

## Complexity Tracking

Not required — all constitution gates satisfied without exceptions.

