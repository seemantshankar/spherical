# Knowledge Engine

Multi-tenant product knowledge library powering the AI sales agent with hybrid retrieval (structured specs + semantic chunks), ingestion pipelines, comparison caching, and lineage tracking.

## Features

- **Multi-tenant ingestion** – Parse brochure Markdown, normalize specs/features/USPs, dedupe, and publish campaigns
- **Hybrid retrieval** – Intent-based routing to SQL (structured) or PGVector/FAISS (semantic) with caching
- **Comparison service** – Pre-computed cross-product deltas with shareability enforcement
- **Lineage & drift** – Full audit trail plus freshness monitoring and purge tooling
- **Embedding support** – OpenRouter API integration with google/gemini-embedding-001 model

## Quick Start

```bash
# Prerequisites: Go 1.23+, Docker, SQLite 3

# Start dev databases
docker compose -f ../../ops/dev/knowledge-engine-compose.yml up -d

# Run migrations
go run ./cmd/knowledge-engine-cli migrate --sqlite

# Ingest a brochure
go run ./cmd/knowledge-engine-cli ingest \
  --tenant toyota \
  --product camry-2025 \
  --campaign camry-2025-hybrid-india \
  --markdown ../../artifacts/camry-output-v3.md \
  --publish-draft

# Start API server
go run ./cmd/knowledge-engine-api --config ./configs/dev.yaml
```

## Project Structure

```
libs/knowledge-engine/
├── cmd/
│   ├── knowledge-engine-cli/   # Ingestion, publish, drift commands
│   └── knowledge-engine-api/   # REST/GraphQL/gRPC entrypoints
├── internal/
│   ├── ingest/                 # Markdown parsing, normalization, publish
│   ├── storage/                # sqlc repositories with RLS
│   ├── retrieval/              # Intent router, vector search, cache
│   ├── comparison/             # Materializer, access policies
│   ├── monitoring/             # Drift, lineage, audit logging
│   ├── config/                 # Unified config loader
│   ├── cache/                  # Memory and Redis client helpers
│   ├── embedding/              # OpenRouter embedding client
│   ├── observability/          # OpenTelemetry, structured logging
│   └── api/
│       ├── graphql/            # gqlgen schema/resolvers
│       └── grpc/               # Connect/gRPC handlers
├── db/migrations/              # SQL migration files
├── api/proto/                  # Protocol buffer definitions
├── pkg/engine/                 # Public Go SDK
├── tests/integration/          # Testcontainers-based integration tests
├── testdata/                   # Golden files, fixtures
└── configs/                    # Environment configs
```

## Configuration

The engine supports **dual database modes**:
- **Development**: SQLite + FAISS (in-memory vectors) + Memory cache
- **Production**: Postgres + PGVector + Redis cache

```yaml
# Development configuration (default)
database:
  driver: sqlite
  sqlite:
    path: /tmp/knowledge-engine.db
    journal_mode: WAL

vector:
  adapter: faiss
  faiss:
    dimension: 768

cache:
  driver: memory
  ttl: 5m
```

```yaml
# Production configuration
database:
  driver: postgres
  postgres:
    dsn: postgres://user:pass@localhost:5432/knowledge
    max_open_conns: 25

vector:
  adapter: pgvector
  pgvector:
    index_type: ivfflat
    lists: 100

cache:
  driver: redis
  redis:
    addr: localhost:6379

embedding:
  model: google/gemini-embedding-001
  dimension: 768
  batch_size: 100
```

### Database Migrations

SQLite and Postgres have separate migration files:

```bash
# SQLite (development)
go run ./cmd/knowledge-engine-cli migrate --sqlite

# Postgres (production)
go run ./cmd/knowledge-engine-cli migrate --postgres
```

## Embedding Configuration

The Knowledge Engine uses OpenRouter for generating embeddings:

```go
import "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/embedding"

client, err := embedding.NewClient(embedding.Config{
    APIKey:    os.Getenv("OPENROUTER_API_KEY"),
    Model:     "google/gemini-embedding-001",
    Dimension: 768,
})

// Generate embeddings
embeddings, err := client.Embed(ctx, []string{
    "What is the fuel efficiency?",
    "How much horsepower?",
})

// Or for a single text
vec, err := client.EmbedSingle(ctx, "What is the mileage?")
```

Set the `OPENROUTER_API_KEY` environment variable with your OpenRouter API key.

## CLI Commands

```bash
# Ingest brochure
knowledge-engine-cli ingest --tenant <id> --product <id> --campaign <id> --markdown <file>

# Publish campaign
knowledge-engine-cli publish --tenant <id> --campaign <id>

# Query knowledge
knowledge-engine-cli query --tenant <id> --products <ids> --question "What is the fuel efficiency?"

# Compare products
knowledge-engine-cli compare --tenant <id> --primary <id> --secondary <id>

# Check for drift
knowledge-engine-cli drift --tenant <id> --check

# Export data
knowledge-engine-cli export --tenant <id> --output data.csv

# Import data
knowledge-engine-cli import --tenant <id> --input data.csv
```

## API Endpoints

### REST API (Port 8085)

- `POST /api/v1/retrieval/query` - Hybrid knowledge retrieval
- `POST /api/v1/tenants/{tenantId}/products/{productId}/campaigns/{campaignId}/ingest` - Ingest brochure
- `POST /api/v1/tenants/{tenantId}/campaigns/{campaignId}/publish` - Publish campaign
- `POST /api/v1/comparisons/query` - Compare products
- `GET /api/v1/lineage/{resourceType}/{resourceId}` - Query lineage
- `GET /api/v1/drift/alerts` - List drift alerts
- `POST /api/v1/drift/check` - Trigger drift check

### Health Checks

- `GET /health` - Service health status
- `GET /ready` - Readiness probe

## Testing

```bash
# Unit tests
go test ./internal/... ./pkg/...

# Integration tests (requires Docker)
go test ./tests/integration/... -v

# All tests
go test ./...

# Contract tests (requires schemathesis)
schemathesis run ../../specs/002-create-product-knowledge/contracts/knowledge-engine.openapi.yaml
```

## Development

```bash
# Install dependencies
go mod download

# Build
go build ./...

# Run linter
golangci-lint run

# Generate protobuf (if modified)
buf generate

# Generate sqlc (if migrations modified)
sqlc generate

# Generate GraphQL (if schema modified)
go run github.com/99designs/gqlgen generate
```

## License

Proprietary – Spherical AI Inc.
