# Implementation Plan: Semantic spec facts with explanations

**Branch**: `005-semantic-spec-facts` | **Date**: 2025-12-06 | **Spec**: [spec.md](./spec.md)  
**Input**: Feature specification from `/specs/005-semantic-spec-facts/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Enrich spec ingestion to generate per-row explanations and semantic spec_fact chunks, embed them for retrieval, and update fallback retrieval to surface structured facts with single-line explanations when keyword confidence is low. Guardrails enforce single-sentence, <=160-char explanations and sanitize on read.

## Technical Context

**Language/Version**: Go 1.25.0  
**Primary Dependencies**: connectrpc, go-chi router, zerolog, Cobra CLI, Postgres/SQLite drivers, Redis, pgvector-backed embedding store (existing knowledge-engine stack), LLM provider for explanations.  
**Storage**: Postgres (primary), pgvector for embeddings, SQLite for local/dev, Redis for caching/queues.  
**Testing**: go test with testify, integration via testcontainers (Postgres/Redis), contract/integration suites in `libs/knowledge-engine/tests`.  
**Target Platform**: Containerized Linux services (knowledge-engine API/ingest/retrieval).  
**Project Type**: Backend service + CLI (library-first, CLI-first).  
**Performance Goals**: Preserve current search latency with measurable targets: end-to-end retrieval p95 ≤ baseline + 100ms (absolute p95 ≤ 900ms) and p99 ≤ 1.2s when semantic fallback triggers. Validated via `TestStructuredRetrieval_RealisticPerformance`.  
**Constraints**: Single-line explanation rendering; semantic fallback only on low-confidence keyword results (configurable threshold + minimum-result check); avoid hallucinations; keep ingest non-blocking on LLM failure; deduplicate spec_fact embeddings on normalized keys; sanitize legacy explanations on read.  
**Scale/Scope**: Current and near-term vehicle spec volume (tens of thousands of rows) with semantic embedding growth; handle per-ingest full sheet runs and search QPS consistent with existing retrieval service.

## Constitution Check

- TDD required: add tests first for ingest, storage, retrieval, and rendering checks.  
- Library-first & CLI-first: keep changes inside knowledge-engine libraries and expose via existing CLI/ingest commands.  
- Integration tests without mocks: use testcontainers/real DB+vector store in integration coverage.  
- Go mandated: align with Go codebase.  
- Modular architecture & <500 LOC per module: keep additions scoped (e.g., new models/repo methods, retriever changes).  
- Security & docs: ensure inputs validated/sanitized; document prompts and data flows in quickstart.  
- Git best practices/worktrees: already on feature branch; maintain clean branch.

Gate status: PASS (no violations anticipated; monitor module size and integration coverage).

## Project Structure

### Documentation (this feature)
```text
specs/005-semantic-spec-facts/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
└── tasks.md (created by /speckit.tasks)
```

### Source Code (repository root)
```text
libs/knowledge-engine/
├── cmd/
│   ├── knowledge-engine-api/              # API/GraphQL/Connect handlers
│   └── orchestrator/                      # ingest orchestration (new)
├── internal/
│   ├── ingest/                            # parser, pipeline, LLM generation
│   ├── retrieval/                         # router, availability detector, semantic fallback
│   ├── storage/                           # models, repositories, spec view
│   └── api/                               # graphql/grpc contracts
├── db/migrations/                         # Postgres/SQLite migrations
├── tests/                                 # integration suites
└── pkg/engine/                            # retrieval engine

libs/pdf-extractor/                        # existing extractor dependency
```

**Structure Decision**: Single backend library/service with CLI + API; work is centered in `libs/knowledge-engine` (ingest, retrieval, storage, migrations) with docs under `specs/005-semantic-spec-facts`.

## Complexity Tracking

_No constitution violations; table not required._

## Migration & Rollout Notes

- Apply migrations through existing Taskfile (`task migrate` for SQLite) to ensure explanation/spec_fact columns exist.
- Rebuild FAISS indexes per campaign (vector sync) to load spec_fact embeddings with enriched metadata (explanations, provenance).
- Deploy retrieval with explanation sanitization (first sentence, <=160 chars) and semantic fallback filters; restart API/ingest so new config is loaded.
- Post-deploy checks: run `go test ./libs/knowledge-engine/tests/integration -run TestStructuredRetrieval_RealisticPerformance` and spot-check retrieval responses for single-line explanations.

## Phase 0: Research

See `research.md` for resolved unknowns (LLM prompt enforcement, embedding storage, fallback strategy, failure handling).

## Phase 1: Design & Contracts

See `data-model.md`, `contracts/`, and `quickstart.md` for design outputs; agent context updated per constitution.

## Phase 2: (Reserved for /speckit.tasks)

Implementation tasks will be generated by `/speckit.tasks`.
