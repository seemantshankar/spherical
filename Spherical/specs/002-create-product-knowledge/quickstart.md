# Quickstart – Product Knowledge Engine Library

## 1. Prerequisites
- Go 1.25+
- Docker Desktop (for Postgres 16 + Redis + PGVector via Testcontainers/dev compose)
- SQLite 3 CLI (`brew install sqlite`)
- `sqlc` and `golangci-lint` installed locally
- OpenAI/OpenRouter API key (required for vector search in `knowledge-demo`)

## 2. Environment Setup
```bash
cd /Users/seemant/Documents/Projects/spherical/Spherical

# Ensure feature env variable helps Spec Kit scripts
export SPECIFY_FEATURE=002-create-product-knowledge

# Create the new library scaffold
mkdir -p libs/knowledge-engine/{cmd,internal,pkg,testdata}
cp libs/pdf-extractor/go.mod libs/knowledge-engine/go.mod  # update module path afterward

# Bootstrap local SQLite database
sqlite3 /tmp/knowledge-engine.db ".databases"
```

Create a `.env.local` at repo root:
```
DATABASE_URL=sqlite:///tmp/knowledge-engine.db
POSTGRES_URL=postgres://postgres:postgres@localhost:5433/knowledge_engine?sslmode=disable
REDIS_URL=redis://localhost:6380
OPENROUTER_API_KEY=sk-or-v1-...
```

## 3. Run Dev Databases
```bash
docker compose -f ops/dev/knowledge-engine-compose.yml up -d
# Provides Postgres 16 + PGVector, Redis 7, and MinIO (object storage)
```

## 4. CLI Workflows

### Interactive Demo (Production Router)
```bash
cd libs/knowledge-engine
# Run the interactive demo using production Router
# The demo uses the same Router as production for consistent behavior
# Simple queries (confidence ≥0.8) return in <25 ms, complex queries trigger vector search
# Make sure OPENROUTER_API_KEY is set in your environment
go run ./cmd/knowledge-demo
```

### Production CLI
```bash
cd libs/knowledge-engine

# Ingest brochure Markdown exported from pdf-extractor
go run ./cmd/knowledge-engine-cli ingest \
  --tenant toyota \
  --product camry-2025 \
  --campaign camry-2025-hybrid-india \
  --markdown ../../artifacts/camry-output-v3.md \
  --source-file ../../libs/pdf-extractor/camry.pdf \
  --publish-draft

# Publish a draft campaign once QA signs off
go run ./cmd/knowledge-engine-cli publish \
  --tenant toyota \
  --campaign camry-2025-hybrid-india \
  --version 3

# Trigger drift audit manually
go run ./cmd/knowledge-engine-cli drift --tenant toyota --campaign camry-2025-hybrid-india
```

All CLI commands must support `--format json` for automation per Constitution Principle III.

## 5. Retrieval API
```bash
# Start HTTP service with SQLite (dev)
go run ./cmd/knowledge-engine-api \
  --config ../../configs/dev.yaml \
  --sqlite-path /tmp/knowledge-engine.db

# Sample query
curl -X POST http://localhost:8085/retrieval/query \
  -H 'Content-Type: application/json' \
  -d '{
        "tenantId":"toyota",
        "productIds":["camry-2025-hybrid"],
        "question":"What is the fuel efficiency?",
        "conversationContext":[],
        "maxChunks":6
      }'
```

## 6. Testing
```bash
cd libs/knowledge-engine

# Unit tests (TDD)
go test ./internal/... ./pkg/...

# Integration tests spin containers for Postgres/Redis/PGVector
KNOWLEDGE_ENGINE_ENV=ci go test ./tests/integration/knowledge-engine -run TestHybridRetrieval -v

# Contract tests (after API server is running on localhost:8085)
pip install --upgrade schemathesis
schemathesis run ../../specs/002-create-product-knowledge/contracts/knowledge-engine.openapi.yaml \
  --base-url=http://localhost:8085
```

> Follow Red-Green-Refactor: write failing test, implement, refactor, commit only after all suites pass.

## 7. Schema Migration Flow
1. Update `.sql` files under `libs/knowledge-engine/db/migrations`.
2. Regenerate typed queries: `sqlc generate`.
3. Apply to SQLite: `go run ./cmd/knowledge-engine-cli migrate --sqlite`.
4. Apply to Postgres (local compose): `go run ./cmd/knowledge-engine-cli migrate --postgres`.
5. Capture before/after snapshots for audit.

## 8. Observability
- Service exports OTEL traces via `OTEL_EXPORTER_OTLP_ENDPOINT`.
- Audit logs streamed to stdout in JSON (`zap` logger) and persisted in `lineage_events`.
- Drift notifications published to Redis channel `drift.alerts` for the admin UI.

## 9. Cleanup
```bash
docker compose -f ops/dev/knowledge-engine-compose.yml down -v
rm /tmp/knowledge-engine.db
```

The Quickstart ensures every engineer can ingest a brochure, publish a campaign, and invoke retrieval in <15 minutes while remaining compliant with the constitution.

