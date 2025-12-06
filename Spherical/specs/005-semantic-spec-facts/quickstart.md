# Quickstart

1) Install deps & env
- Ensure Go 1.25.0, Docker for testcontainers, and env vars for SQLite/Redis/LLM provider.
- Copy `libs/knowledge-engine/.env.example` → `.env` and set:
  - `DATABASE_URL=sqlite:/tmp/knowledge-engine.db`
  - `VECTOR_ADAPTER=faiss` and `FAISS_INDEX_PATH=/tmp/knowledge-engine.faiss`
  - `OPENROUTER_API_KEY=<your-key>` (required for real embeddings; mocks otherwise)
  - Optional: `REDIS_URL=redis://localhost:6380` (cache), OTEL endpoints

2) Run migrations
- `task migrate` (SQLite) to apply the explanation + spec_fact chunk migrations; FAISS index file is created/updated during ingest/vector sync.

3) Ingest sample specs with explanations + guardrails
- `go run ./libs/knowledge-engine/cmd/orchestrator --input <spec-file>` (or existing ingest CLI) to parse rows, generate explanations, and store spec_fact chunks + embeddings.
- Verify ingest logs show either a single-sentence explanation or `explanation_failed=true` for rows that violate guardrails (too long/multi-sentence).

4) Validate retrieval fallback
- Call API/GraphQL retrieval with low-keyword queries (e.g., "phone integration", "USB charging") and confirm facts + single-line explanations (sanitized to first sentence, <=160 chars) are returned from FAISS/SQLite.
- Confirm keyword-first behavior remains for high-confidence queries; semantic fallback only triggers on low-confidence keyword results or empty hits.

5) Tests (TDD)
- Add/verify unit tests for explanation formatting and chunk construction.
- Add/verify integration tests (testcontainers) for ingest→store→retrieve with semantic fallback.
- Performance/regression: `go test ./libs/knowledge-engine/tests/integration -run TestStructuredRetrieval_RealisticPerformance`.
- Run `go test ./libs/knowledge-engine/...` (ensure containers reachable).

6) UI/format check
- Render retrieval results in current block layout; ensure explanation shows on a single line (no wrapping/truncation issues).

7) Retry/ops checks
- Simulate LLM timeout/error; confirm failure marker stored and ingest continues; optional retry path works.
