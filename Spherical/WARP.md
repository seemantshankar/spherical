# WARP.md

This file provides guidance to WARP (warp.dev) when working with code in this repository.

## Project Overview

Spherical is a platform for extracting, storing, and retrieving structured product knowledge from specification documents. The codebase is organized as a monorepo with self-contained libraries.

**Core Components:**
- **pdf-extractor**: Go-based CLI/library for extracting product specifications from PDF brochures using vision LLMs
- **knowledge-engine**: Go-based semantic knowledge engine with vector search, deduplication, and structured retrieval
- **markdown-tools**: Python/Shell scripts for cleaning and formatting Markdown files

## Directory Structure

```
.
├── libs/                  # Self-contained code libraries (ALL code lives here)
│   ├── pdf-extractor/     # Go: PDF → structured Markdown extraction
│   ├── knowledge-engine/  # Go: Ingestion, storage, semantic retrieval
│   └── markdown-tools/    # Python/Shell: Markdown utilities
├── specs/                 # Feature specifications (Spec Kit workflow)
├── tests/                 # Cross-library integration/performance tests
├── ops/                   # Docker Compose for dev dependencies
└── CONVENTIONS.md         # Architectural guidelines (READ THIS)
```

**Critical Rule:** All functional code MUST reside in `libs/`. Do not create top-level directories for code.

## Common Commands

### pdf-extractor (Go)

```bash
cd libs/pdf-extractor

# Build
go build -o pdf-extractor cmd/pdf-extractor/main.go

# Run (requires OPENROUTER_API_KEY in .env)
./pdf-extractor brochure.pdf
./pdf-extractor --verbose --output specs.md brochure.pdf

# Test
go test ./...                      # All tests
go test -short ./...               # Fast tests only
go test ./tests/integration/       # Integration tests (requires API key)
go test -cover ./...               # With coverage
```

**Requirements:**
- Go 1.25+
- CGO enabled
- MuPDF 1.24.9+ (`brew install mupdf` on macOS)
- `OPENROUTER_API_KEY` environment variable

### knowledge-engine (Go + Task)

The knowledge-engine uses [Task](https://taskfile.dev) as a task runner (defined in `Taskfile.yml`).

```bash
cd libs/knowledge-engine

# Install Task (if not available)
brew install go-task/tap/go-task  # macOS
# or: go install github.com/go-task/task/v3/cmd/task@latest

# View all available tasks
task

# Build
task build                         # Build CLI + API
task build:cli                     # Build CLI only
task build:api                     # Build API only

# Test
task test                          # All tests with coverage
task test:unit                     # Fast unit tests only
task test:integration              # Integration tests (requires Docker)
task test:coverage                 # Generate HTML coverage report

# Lint
task lint                          # Run golangci-lint
task lint:fix                      # Auto-fix issues
task fmt                           # Format code (gofmt + gofumpt)

# Development
task dev:deps                      # Start Postgres/Redis/MinIO via Docker Compose
task dev:deps:down                 # Stop dependencies
task dev                           # Run API server with hot reload

# Database
task migrate                       # Run migrations (SQLite)
task migrate:postgres              # Run migrations (Postgres)

# CLI shortcuts
task ingest MARKDOWN=path/to/file.md TENANT=dev PRODUCT=camry CAMPAIGN=2024

# Code generation (after schema changes)
task generate:sqlc                 # Generate typed SQL queries
task generate:proto                # Generate Connect RPC code
task generate:graphql              # Generate GraphQL resolvers
```

**Requirements:**
- Go 1.24+
- Docker (for integration tests and dev dependencies)
- Tools (install with `task tools`):
  - `golangci-lint` (linting)
  - `sqlc` (SQL code generation)
  - `buf` (protobuf generation)
  - `gqlgen` (GraphQL generation)

### markdown-tools (Python/Shell)

```bash
cd libs/markdown-tools

# Run scripts directly (executable, no build needed)
./clean-markdown.sh input.md
./fix-markdown-tables.py input.md
./enhance_usps.py input.md

# Install Python dependencies (if any)
pip install -r requirements.txt   # If file exists
```

## Library Development

### Creating New Libraries

Follow conventions from `CONVENTIONS.md`:

1. **Naming:** Use kebab-case (e.g., `libs/image-processor`)
2. **Go libraries:** Include `go.mod` at library root, use standard layout (`cmd/`, `pkg/`, `internal/`)
3. **Run commands from library directory**, not project root
4. **One feature = One library** (generally)

### Go Project Layout (Shared Pattern)

Both Go libraries follow standard Go conventions:

```
libs/<library>/
├── cmd/                   # Main applications/binaries
│   └── <app>/main.go
├── pkg/                   # Public library code (importable)
├── internal/              # Private application code
│   ├── domain/           # Core business logic, models, interfaces
│   ├── <feature>/        # Feature-specific implementations
│   └── ...
├── tests/
│   └── integration/      # Integration tests (use testcontainers for deps)
├── go.mod                # Must be at library root
└── README.md
```

**Architecture Philosophy:**
- **Domain-driven:** Core business logic in `internal/domain` (models, interfaces, errors)
- **Clean boundaries:** Use interfaces to decouple components
- **Testability:** `internal/` packages are testable via integration tests or through public `pkg/` API

### Testing Strategy (TDD)

**CRITICAL:** This project follows Test-Driven Development (TDD):

1. **Write tests FIRST** before implementing features
2. **Integration tests preferred:** Use real dependencies (testcontainers) over mocks
3. **Do NOT mark tasks done** until ALL of the following pass:
   - `go test ./...` (all tests passing)
   - `go build ./...` (no compilation errors)
   - `golangci-lint run ./...` (or `task lint`) — no linter errors

**knowledge-engine Testing:**
- Uses `testcontainers-go` for real Postgres/Redis instances in tests
- Fast tests: `task test:unit` (no Docker)
- Full tests: `task test:integration` (requires Docker)
- Integration tests use test fixtures in `testdata/`

**pdf-extractor Testing:**
- Integration tests require valid `OPENROUTER_API_KEY`
- Use `-short` flag to skip slow/API-dependent tests during development
- Test data in `testdata/` directory

### Linting Configuration

knowledge-engine includes comprehensive `golangci-lint` configuration (`.golangci.yml`):
- 40+ enabled linters (errcheck, gosec, govet, staticcheck, etc.)
- Line length: 140 characters
- Test files have relaxed rules (allowed: dupl, forcetypeassert, gosec, lll)

Always run linter before committing:
```bash
task lint        # knowledge-engine
golangci-lint run ./...  # pdf-extractor
```

## Specifications Workflow

Features are developed using the [Spec Kit](https://github.com/specify-dev/speckit) workflow:

```bash
# Specs live in specs/<feature-id>-<name>/
specs/
├── 001-pdf-spec-extractor/
├── 002-create-product-knowledge/
├── 003-fix-pdf-extractor/
└── 004-row-level-chunking/

# Typical spec structure
<spec>/
├── plan.md              # Implementation plan
├── tasks.md             # Task breakdown
├── quickstart.md        # Usage guide
└── contracts/           # API contracts (OpenAPI, etc.)
```

**Use Cursor's `/speckit-*` commands** (found in `.cursor/commands/`) to generate/update specs.

## Environment Setup

### Required Environment Variables

**pdf-extractor:**
```bash
OPENROUTER_API_KEY=sk-or-your-key-here
LLM_MODEL=google/gemini-2.5-flash-preview-09-2025  # Optional override
```

**knowledge-engine:**
- Configuration via YAML files in `configs/` (e.g., `configs/dev.yaml`)
- Environment can override config (see `internal/config/`)

### Development Dependencies (knowledge-engine)

```bash
cd libs/knowledge-engine

# Start Postgres, Redis, MinIO
task dev:deps

# Stop dependencies
task dev:deps:down

# Remove volumes (clean slate)
task dev:deps:clean
```

Compose file: `ops/dev/knowledge-engine-compose.yml`

## Cross-Library Integration

### knowledge-engine + pdf-extractor Pipeline

```bash
# 1. Extract PDF to Markdown
cd libs/pdf-extractor
./pdf-extractor brochure.pdf  # Outputs brochure-specs.md

# 2. Ingest Markdown into knowledge engine
cd ../knowledge-engine
task ingest MARKDOWN=../pdf-extractor/brochure-specs.md TENANT=dev PRODUCT=camry CAMPAIGN=2024

# 3. Query the knowledge
task cli -- query --tenant dev --product camry --question "What colors are available?"
```

### Output Format Contract

pdf-extractor produces Markdown with:
- **YAML frontmatter** (domain, make, model, country, year, condition)
- **5-column specification tables** (Parent Category | Sub-Category | Specification | Value | Variant Availability)
- **Key Features** and **USPs** sections

knowledge-engine expects this format and parses:
- Table rows → individual semantic chunks (row-level chunking)
- Content hashing for deduplication across documents
- Hierarchical grouping by category in retrieval

## Key Architectural Patterns

### pdf-extractor

**Event Streaming:** The library streams processing events for real-time progress:
- `EventPageProcessing`: Page started
- `EventLLMStreaming`: Text chunk from LLM
- `EventComplete`: Processing done (includes metadata + stats)

**Error Handling:** Domain errors with automatic retry for rate limits (exponential backoff)

### knowledge-engine

**Repository Pattern:** All data access through repository interfaces in `internal/storage`

**Retrieval Architecture:**
- **Router** → **Strategy** → **Ranker** → **Grouper**
- Hybrid search: vector (semantic) + keyword (BM25-like)
- Row-level chunking: Each table row is a separate searchable chunk
- Hierarchical grouping: Results grouped by Parent Category → Sub-Category

**Deduplication:** SHA-256 content hashes map duplicate specs across documents to the same chunk

**Observability:** Structured logging via `zerolog`, metrics via `internal/monitoring`

## Tech Stack Summary

| Library | Language | Key Dependencies |
|---------|----------|------------------|
| pdf-extractor | Go 1.25 | go-fitz (MuPDF), OpenRouter API |
| knowledge-engine | Go 1.24 | pgx/v5, pgvector-go, redis, chi, cobra, testcontainers, sqlc |
| markdown-tools | Python/Shell | (Standalone scripts) |

## Common Pitfalls

1. **Running Go commands from project root:** Always `cd libs/<library>` first
2. **Missing CGO/MuPDF:** pdf-extractor requires CGO + MuPDF libraries installed
3. **Forgetting to run linter:** Always `task lint` or `golangci-lint run` before committing
4. **Skipping integration tests:** Use `task test` (not just unit tests) before marking work complete
5. **Not following TDD:** Tests MUST be written first and ALL must pass (build, test, lint)
6. **Creating top-level code directories:** All code goes in `libs/`
