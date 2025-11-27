# Tasks: Product Knowledge Engine Library

**Input**: Design documents from `/specs/002-create-product-knowledge/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Scaffold the Go library, commands, and configuration assets used across all user stories.

- [ ] T001 Create library skeleton per plan (`libs/knowledge-engine/{cmd,internal,db,migrations,api/proto,pkg,testdata}`) and add README stub.
- [ ] T002 Initialize `libs/knowledge-engine/go.mod` with Go 1.25 module path plus baseline dependencies (`pgx/v5`, `chi`, `cobra`, `sqlc`, `gqlgen`, `connectrpc.com/connect`).
- [ ] T003 [P] Add shared config artifacts (`configs/dev.yaml`, `.env.example`, `ops/dev/knowledge-engine-compose.yml`) following Quickstart defaults.
- [ ] T004 [P] Add developer tooling (Taskfile/Makefile, `golangci-lint.yml`, `buf.gen.yaml`) so `go test`, `schemathesis`, `buf generate`, and perf benchmarks run via one command.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure (schema, protocols, security, observability) that every story depends on.

- [ ] T005 Define initial migrations covering tenants/products/campaigns/spec tables in `libs/knowledge-engine/db/migrations/0001_init.sql` and register via `sqlc.yaml`.
- [ ] T006 [P] Implement row-level-security-aware repositories in `libs/knowledge-engine/internal/storage/repositories.go` using sqlc-generated code.
- [ ] T007 [P] Build centralized config loader + secret resolution (`libs/knowledge-engine/internal/config/config.go`) supporting SQLite, Postgres, Redis, S3, FAISS.
- [ ] T008 Establish logging + OpenTelemetry wiring in `libs/knowledge-engine/internal/observability/logger.go` and expose hooks for CLI/API binaries.
- [ ] T009 [P] Implement Redis cache client + TTL helpers in `libs/knowledge-engine/internal/cache/redis_client.go`.
- [ ] T010 Bootstrap CLI/API entrypoints (`cmd/knowledge-engine-cli/main.go`, `cmd/knowledge-engine-api/main.go`) so future commands/handlers share wiring.
- [ ] T011 [P] Scaffold GraphQL server with `gqlgen` config + base schema (`internal/api/graphql/schema.graphqls`, `resolvers.go`) mirroring REST contracts.
- [ ] T012 [P] Define Connect/gRPC proto files under `libs/knowledge-engine/api/proto/knowledgeengine/v1/service.proto` and generate Go stubs plus server wiring.
- [ ] T013 Implement FAISS/in-memory vector adapter (`internal/retrieval/vector_adapter_faiss.go`) and config toggles to swap between FAISS (dev) and PGVector (prod).
- [ ] T014 Implement OAuth2 client-credentials + mTLS middleware (`cmd/knowledge-engine-api/middleware/auth.go`) enforcing tenant-aware RBAC before requests reach business logic.

> **Checkpoint**: All shared infrastructure ready – user story work can start in parallel.

---

## Phase 3: User Story 1 – Tenant admin onboards a new campaign (Priority: P1) 🎯

**Goal**: Admins can ingest brochure Markdown, normalize specs/features/USPs, export/import data, and publish a campaign version without impacting other tenants.

**Independent Test**: Run CLI ingestion against Camry brochure, inspect staging DB for correctly scoped rows, export CSV/Parquet snapshot, then publish draft and verify other tenant data remains untouched.

### Tests – User Story 1 (write first, ensure fail)

- [ ] T015 [P] [US1] Add ingestion + publish contract test (`tests/contract/knowledge-engine/ingest_publish.http`) covering happy path, PDF-only inputs that auto-run `libs/pdf-extractor/cmd/pdf-extractor`, and conflict errors.
- [ ] T016 [P] [US1] Add integration test for ingestion pipeline in `tests/integration/knowledge-engine/ingest_pipeline_test.go` using testcontainers, ensuring the CLI shells out to the pdf-extractor binary when no Markdown path is provided.

### Implementation – User Story 1

- [ ] T017 [US1] Implement Markdown → domain parsing & validation service (`internal/ingest/parser.go`) honoring YAML metadata + 4-column tables.
- [ ] T018 [P] [US1] Build ingestion orchestrator with dedupe + doc-source linking in `internal/ingest/pipeline.go`.
- [ ] T019 [P] [US1] Persist structured specs/features/USPs using repositories in `internal/storage/spec_repository.go` and `feature_block_repository.go`.
- [ ] T020 [US1] Implement publish + rollback workflow in `internal/ingest/publisher.go`, updating campaign versions and effective ranges.
- [ ] T021 [US1] Wire CLI commands (`cmd/knowledge-engine-cli/ingest.go`, `publish.go`) with JSON output, Spec Kit progress events, and automatic invocation of `libs/pdf-extractor/cmd/pdf-extractor` when a PDF path is supplied (falling back to existing Markdown when provided).
- [ ] T022 [US1] Emit audit + lineage events during ingestion/publish via `internal/monitoring/audit_logger.go`.
- [ ] T023 [US1] Implement CSV/Parquet export + bulk import commands (`cmd/knowledge-engine-cli/export.go`, `import.go`) over `spec_view_latest`.
- [ ] T024 [US1] Add ingestion benchmark harness (`tests/perf/ingestion_benchmark.md` + Go driver) proving 20-page brochure publishes ≤15 min on reference hardware.

> **Checkpoint**: Ingestion + publish story independently testable; MVP candidate.

---

## Phase 4: User Story 2 – AI sales agent answers trim-specific questions (Priority: P2)

**Goal**: Retrieval tier serves deterministic specs + semantic context across REST, GraphQL, gRPC, and CLI surfaces in ≤150 ms p50 with cache + FAISS/PGVector parity.

**Independent Test**: Simulate trim-specific queries via REST/GraphQL/gRPC/CLI, confirm structured facts + semantic chunks returned with correct latency, fallback, and cache behavior.

### Tests – User Story 2

- [ ] T025 [P] [US2] Add retrieval contract tests to `tests/contract/knowledge-engine/retrieval.http` and GraphQL/gRPC equivalents covering spec lookup, semantic fallback, cache hits.
- [ ] T026 [P] [US2] Add integration test for hybrid router latency + FAISS/PGVector parity in `tests/integration/knowledge-engine/retrieval_router_test.go`.
- [ ] T027 [P] [US2] Add audit logging integration test ensuring retrieval requests emit events in `tests/integration/knowledge-engine/retrieval_audit_test.go`.

### Implementation – User Story 2

- [ ] T028 [US2] Implement intent classifier + routing strategy in `internal/retrieval/router.go` (structured-first, fallback to semantic).
- [ ] T029 [P] [US2] Create spec view/query layer with cache hints in `internal/storage/spec_view.go`.
- [ ] T030 [P] [US2] Implement vector search abstraction covering PGVector (prod) and FAISS (dev) in `internal/retrieval/vector_search.go`.
- [ ] T031 [US2] Build REST handler for `/retrieval/query` in `cmd/knowledge-engine-api/handlers/retrieval_rest.go`.
- [ ] T032 [US2] Add Redis-backed response cache + invalidation triggers in `internal/retrieval/cache.go`.
- [ ] T033 [US2] Publish Go SDK helper wrapping retrieval API in `pkg/engine/retrieval.go`.
- [ ] T034 [US2] Implement GraphQL schema/resolvers for retrieval (`internal/api/graphql/retrieval_resolvers.go`) mirroring REST contract.
- [ ] T035 [US2] Implement gRPC/Connect retrieval service (`internal/api/grpc/retrieval_service.go`) plus contract tests.
- [ ] T036 [US2] Handle edge cases (deleted campaigns, trim mismatches) by falling back to last published variant and surfacing policy-compliant responses in `internal/retrieval/fallbacks.go`.
- [ ] T037 [US2] Add CLI query command (`cmd/knowledge-engine-cli/query.go`) that streams retrieval responses with JSON output.
- [ ] T038 [US2] Wire audit logging into retrieval handlers/CLI (`internal/monitoring/audit_logger.go`) covering request metadata + response citations.

---

## Phase 5: User Story 3 – Comparative assistant responds to cross-make prompts (Priority: P3)

**Goal**: Comparison service safely combines benchmark + tenant data to answer “Camry vs Accord” without leaking restricted trims across REST/GraphQL/gRPC/CLI.

**Independent Test**: Precompute Camry vs Accord rows, hit `/comparisons/query` (REST/GraphQL/gRPC/CLI), ensure only shareable data is returned and restricted competitors are rejected.

### Tests – User Story 3

- [ ] T039 [P] [US3] Add comparison contract tests in `tests/contract/knowledge-engine/comparisons.http` (REST + GraphQL/gRPC variants).
- [ ] T040 [P] [US3] Add integration test for comparison materializer job (`tests/integration/knowledge-engine/comparison_job_test.go`).
- [ ] T041 [P] [US3] Add audit logging integration test for comparison requests in `tests/integration/knowledge-engine/comparison_audit_test.go`.

### Implementation – User Story 3

- [ ] T042 [US3] Implement comparison materializer + scheduler in `internal/comparison/materializer.go` and `cmd/knowledge-engine-api/scheduler.go`.
- [ ] T043 [P] [US3] Enforce shareability policies in `internal/comparison/access_policies.go` (benchmark/public/private logic + enforcement guards).
- [ ] T044 [US3] Build `/comparisons/query` handlers for REST/GraphQL/gRPC in `cmd/knowledge-engine-api/handlers/comparisons_{rest,graphql,grpc}.go`.
- [ ] T045 [US3] Backfill CLI/ADMIN triggers for recomputing comparisons in `cmd/knowledge-engine-cli/comparisons.go`.
- [ ] T046 [US3] Add CLI comparison query command (`cmd/knowledge-engine-cli/compare.go`) with JSON/markdown output for agents.
- [ ] T047 [US3] Wire audit logging into comparison handlers/CLI so every comparator response records provenance in `internal/monitoring/audit_logger.go`.

---

## Phase 6: User Story 4 – Data team audits lineage & drift (Priority: P4)

**Goal**: Compliance analysts can trace any fact back to its brochure source, detect embedding-version drift, honor retention SLAs, and receive alerts when campaigns age out via CLI + APIs (dashboard UI deferred).

**Independent Test**: Query `/lineage/{resource}` (REST/GraphQL/gRPC/CLI) for a chunk and confirm provenance, trigger drift + purge jobs, and verify alerts & purge completion SLAs.

### Tests – User Story 4

- [ ] T048 [P] [US4] Add lineage contract tests in `tests/contract/knowledge-engine/lineage.http` (REST + GraphQL/gRPC).
- [ ] T049 [P] [US4] Add integration test covering drift detection, purge flow, and embedding-version guardrails in `tests/integration/knowledge-engine/drift_monitor_test.go`.

### Implementation – User Story 4

- [ ] T050 [US4] Implement lineage writer hooking ingestion/retrieval events in `internal/monitoring/lineage_writer.go`.
- [ ] T051 [P] [US4] Implement drift detection runner comparing hashes/ages in `internal/monitoring/drift_runner.go`.
- [ ] T052 [US4] Expose `/lineage/{resourceType}/{resourceId}` handlers for REST/GraphQL/gRPC in `cmd/knowledge-engine-api/handlers/lineage_{rest,graphql,grpc}.go`.
- [ ] T053 [US4] Add CLI drift command + alert publishing (Redis channel) in `cmd/knowledge-engine-cli/drift.go`.
- [ ] T054 [US4] Implement retention/purge tooling (`cmd/knowledge-engine-cli/purge.go`) that deletes tenant data within 30 days and logs audit trails.
- [ ] T055 [US4] Detect embedding model version mismatches and queue re-embedding jobs (`internal/monitoring/embedding_guard.go`) so mixed vectors are never queried together.
- [ ] T056 [US4] Add REST/GraphQL/gRPC endpoints for listing drift alerts (`cmd/knowledge-engine-api/handlers/drift_alerts_{rest,graphql,grpc}.go`) consumed by dashboards.
- [ ] T057 [US4] Add CLI drift report command (`cmd/knowledge-engine-cli/drift_report.go`) summarizing open alerts for analysts.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [ ] T058 [P] Update Quickstart + README with verified commands (REST/GraphQL/gRPC, CLI queries, export/import, FAISS toggle) (`specs/002-create-product-knowledge/quickstart.md`, `libs/knowledge-engine/README.md`).
- [ ] T059 Run Schemathesis + load tests (200 RPS mixed workload) and capture results under `tests/perf/retrieval_load.md`.
- [ ] T060 [P] Harden security (OAuth2 scopes, mTLS verification, tenancy guards) in `cmd/knowledge-engine-api/middleware/auth.go`.
- [ ] T061 Finalize observability dashboards + alert rules and document in `docs/ops/monitoring.md`.

---

## Dependencies & Execution Order

- **Setup (Phase 1)** → **Foundational (Phase 2)** → user stories in priority order (US1 → US2 → US3 → US4) → **Polish**.
- User stories can proceed in parallel once Phase 2 completes, but P1 remains MVP gate.
- Within each story: tests → models/services → endpoints/CLI → integration.

### Parallel Opportunities

- Setup tasks T003–T004 can run concurrently.
- Foundational tasks T006–T010 touch different packages and can proceed in parallel after migrations.
- In each story, tasks marked `[P]` (tests, repositories, vector search, shareability checks) target separate files so multiple contributors can execute simultaneously.

### Independent Tests per Story

- **US1**: Execute ingestion CLI against Camry brochure, inspect DB + audit logs, publish draft, confirm scoping.
- **US2**: Call `/retrieval/query` (REST/GraphQL/gRPC/CLI), assert structured facts + semantic chunks + latency budgets and verify audit entries.
- **US3**: Seed benchmark data, call `/comparisons/query`, confirm shareability enforcement, CLI parity, and audit output.
- **US4**: Query lineage/drift endpoints + CLI reports, trigger drift job, verify purge + alert logging while noting dashboard UI deferral.

### MVP Scope

- Deliver through **User Story 1** to unlock brochure ingestion + campaign publishing; this enables downstream teams to populate the knowledge base while other stories proceed.

---

## Parallel Execution Example (User Story 2)

```bash
# Terminal 1 – contract + audit tests
go test ./tests/contract/knowledge-engine -run TestRetrievalContract && go test ./tests/integration/knowledge-engine -run TestRetrievalAudit

# Terminal 2 – router implementation
devbox run air --build.cmd "cd libs/knowledge-engine && go test ./internal/retrieval -run TestRouter && go test ./tests/integration/knowledge-engine -run TestRetrievalRouter"
```
