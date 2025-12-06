# Tasks: Semantic spec facts with explanations

**Input**: Design documents from `/specs/005-semantic-spec-facts/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: TDD per constitution; test tasks included.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Ensure env and tooling ready for semantic spec facts work.

- [X] T001 Update env example with LLM + SQLite/FAISS settings in `libs/knowledge-engine/.env.example`
- [X] T002 Validate testcontainers/DB/Redis connectivity tasks for SQLite/FAISS in `libs/knowledge-engine/Taskfile.yml` (migrate --sqlite, dev:deps, test:integration)
- [X] T003 [P] Add developer quickstart note for semantic facts to `specs/005-semantic-spec-facts/quickstart.md`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Schema and storage updates required before user stories.

- [X] T004 Create SQLite migration for explanation column and spec_fact chunk storage in `libs/knowledge-engine/db/migrations/0004_add_explanation_and_spec_fact_chunks_sqlite.sql`
- [X] T005 [P] Update FAISS vector index/build path to store spec_fact embeddings (no pgvector) in `libs/knowledge-engine/internal/retrieval/vector_adapter.go` and related startup wiring
- [X] T006 Update storage models/entities for explanation + chunk structs in `libs/knowledge-engine/internal/storage/models.go`
- [X] T007 Update spec view to expose explanation in `libs/knowledge-engine/internal/storage/spec_view.go`
- [X] T008 Update repositories to persist explanation and chunk_text/linkage in `libs/knowledge-engine/internal/storage/repositories.go`
- [X] T009 Wire SQLite migration into schema/indexing and FAISS loading in `libs/knowledge-engine/db/migrations/`, `libs/knowledge-engine/internal/storage/spec_view.go`, and FAISS init/vector sync paths

**Checkpoint**: Schema, models, and repos support explanation + chunk storage.

---

## Phase 3: User Story 1 - Enrich specs with semantic facts (Priority: P1) 🎯 MVP

**Goal**: Ingest rows with explanations and enriched spec_fact chunks, embed them.
**Independent Test**: Run ingest on sample sheet; each row stores explanation + chunk + embedding or clear failure marker.

### Tests for User Story 1 (TDD)
- [X] T010 [P] [US1] Add unit tests for explanation formatting and single-sentence enforcement in `libs/knowledge-engine/internal/ingest/pipeline_test.go`
- [X] T011 [US1] Add integration ingest test covering explanation + embedding persistence in `libs/knowledge-engine/tests/integration/semantic_spec_facts_ingest_test.go`

### Implementation for User Story 1
- [X] T012 [US1] Implement LLM prompt enforcement and explanation generation with failure markers in `libs/knowledge-engine/internal/ingest/pipeline.go`
- [X] T013 [P] [US1] Build spec_fact chunk_text (category > name, value/unit, key features, availability, gloss) in `libs/knowledge-engine/internal/ingest/parser.go`
- [X] T014 [US1] Persist explanation + chunk and store embeddings via repositories in `libs/knowledge-engine/internal/storage/repositories.go`
- [X] T015 [P] [US1] Wire orchestrator/CLI to pass new explanation/chunk through ingest flow in `libs/knowledge-engine/cmd/orchestrator/main.go`

**Checkpoint**: Ingest produces explanations + embedded chunks with retries/failure markers.

---

## Phase 4: User Story 2 - Retrieve facts when keywords fail (Priority: P2)

**Goal**: Fallback to semantic spec_fact embeddings on low keyword confidence and return structured facts with explanations.
**Independent Test**: Low-confidence queries return relevant facts with explanations; high-confidence queries stay on keyword path.

### Tests for User Story 2 (TDD)
- [X] T016 [US2] Add integration test for semantic fallback retrieval in `libs/knowledge-engine/tests/integration/structured_retrieval_semantic_fallback_test.go`
- [X] T017 [P] [US2] Add router threshold unit test for keyword confidence gating in `libs/knowledge-engine/internal/retrieval/router_test.go`

### Implementation for User Story 2
- [X] T018 [US2] Implement fallback to spec_fact embeddings when keyword confidence is low/empty in `libs/knowledge-engine/internal/retrieval/router.go` and `libs/knowledge-engine/pkg/engine/retrieval.go`
- [X] T019 [P] [US2] Include explanation and provenance in API responses (GraphQL/Connect) in `libs/knowledge-engine/internal/api/graphql/retrieval_resolvers.go` and `libs/knowledge-engine/internal/api/grpc/retrieval_service.go`
- [X] T020 [US2] Update HTTP handler to surface explanation single-line in block layout response in `libs/knowledge-engine/cmd/knowledge-engine-api/handlers/retrieval.go`
- [X] T021 [P] [US2] Ensure spec_fact embedding queries respect variant filters and dedup in `libs/knowledge-engine/internal/retrieval/availability_detector.go`

**Checkpoint**: Semantic fallback returns structured facts with explanations; API outputs include explanation + provenance.

---

## Phase 5: User Story 3 - Guardrails on LLM-generated text (Priority: P3)

**Goal**: Enforce safety/field-bounded outputs and audit failures.
**Independent Test**: Explanations use only provided fields; refusals/failure markers logged; no hallucinations observed in sampled rows.

### Tests for User Story 3 (TDD)
- [X] T022 [US3] Add negative/guardrail tests for hallucination/refusal behavior in `libs/knowledge-engine/internal/ingest/pipeline_test.go`
- [X] T023 [P] [US3] Add audit/logging assertions for failure markers in `libs/knowledge-engine/tests/integration/semantic_spec_facts_ingest_test.go`

### Implementation for User Story 3
- [X] T024 [US3] Enforce one-sentence, field-bounded validation and refusal handling in `libs/knowledge-engine/internal/ingest/pipeline.go`
- [X] T025 [P] [US3] Add sanitization/guardrails before returning explanations in retrieval responses in `libs/knowledge-engine/internal/retrieval/router.go`
- [X] T026 [US3] Add metrics/logs for explanation failures and semantic fallback usage in `libs/knowledge-engine/internal/ingest/pipeline.go` and `libs/knowledge-engine/internal/retrieval/router.go`

**Checkpoint**: Safety guardrails validated; monitoring in place.

---

## Phase 6: Polish & Cross-Cutting

- [X] T027 Update quickstart and contract docs with final behavior in `specs/005-semantic-spec-facts/quickstart.md` and `specs/005-semantic-spec-facts/contracts/retrieval.graphql`
- [X] T028 [P] Run semantic fallback performance/regression checks in `libs/knowledge-engine/tests/integration/structured_retrieval_performance_test.go`
- [X] T029 Document migration + rollout steps in `libs/knowledge-engine/README.md` and `specs/005-semantic-spec-facts/plan.md`

---

## Dependencies
- Phase 1 → Phase 2 → Phase 3 (US1) → Phase 4 (US2) → Phase 5 (US3) → Phase 6 (Polish)

## Parallel Execution Examples
- Schema migrations (T004/T005) and model/repo updates (T006–T009) can run in parallel once file ownership is clear.  
- In US1, chunk building (T013) and CLI wiring (T015) can proceed in parallel after LLM prompt design (T012).  
- In US2, API response wiring (T019) and variant-aware query tuning (T021) can run in parallel after fallback logic scaffold (T018).

## Implementation Strategy
- MVP: Complete US1 (Phase 3) to deliver ingestion with explanations + embeddings.  
- Then ship US2 for semantic fallback; follow with US3 guardrails and Polish.  
- Maintain TDD: write/adjust tests per phase before implementation tasks.
