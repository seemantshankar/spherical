# Quickstart: Row-Level Chunking

**Feature**: `004-row-level-chunking`  
**Date**: 2025-12-04

## Overview

Row-level chunking converts each table row in specification documents into an individual semantic chunk, enabling precise retrieval of specific attributes (e.g., "car colors") instead of returning entire table chunks.

## Prerequisites

- Go 1.25+
- Knowledge Engine library (`libs/knowledge-engine`)
- Database: SQLite (dev) or Postgres+PGVector (prod)
- Embedding service configured (OpenRouter API or mock)

## Setup

### 1. Database Migration

Apply the migration to add new columns:

```bash
cd libs/knowledge-engine
psql $DATABASE_URL < db/migrations/0002_add_row_chunking_fields.sql
```

Or for SQLite:
```bash
sqlite3 knowledge_engine.db < db/migrations/0002_add_row_chunking_fields.sql
```

### 2. Verify Configuration

Ensure your embedding service is configured in `configs/dev.yaml`:

```yaml
embedding:
  api_key: "your-api-key"
  model: "google/gemini-embedding-001"
  base_url: "https://openrouter.ai/api/v1"
  dimension: 768
```

## Usage

### Ingest Document with Row-Level Chunking

The row-level chunking is automatically enabled for all table rows during ingestion:

```bash
go run ./cmd/knowledge-engine-cli ingest \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --product wagon-r-2025 \
  --markdown testdata/camry-sample.md
```

**What Happens**:
1. Parser detects tables in markdown
2. Each table row is converted to a structured text chunk
3. Content hash is computed for deduplication
4. Chunks are stored with metadata (parent_category, sub_category, etc.)
5. Embeddings are generated in batches of 50-100 chunks
6. Failed embeddings are marked as incomplete for retry

### Query Row-Level Chunks

Query for specific table attributes:

```bash
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --product wagon-r-2025 \
  --question "What colors does this car come in?"
```

**Expected Output**:
```
Exterior:
  Colors:
    - Color: Solid White
    - Color: Metallic Silky Silver
    - Color: Metallic Magma Grey
    ...
```

Results are automatically grouped hierarchically by parent category, then sub-category.

## Table Format

Row-level chunking works with 5-column tables in markdown:

```markdown
| Parent Category | Sub-Category | Specification | Value | Additional Metadata |
|----------------|-------------|---------------|-------|-------------------|
| Exterior       | Colors      | Color         | Solid White | Standard |
| Exterior       | Colors      | Color         | Metallic Silky Silver | Standard |
```

**Column Mapping**:
- Column 1: Parent Category
- Column 2: Sub-Category  
- Column 3: Specification Type
- Column 4: Value
- Column 5: Additional Metadata

## Structured Text Format

Each row chunk is stored as structured text:

```
Category: Exterior
Sub-Category: Colors
Specification: Color
Value: Solid White
Additional Metadata: Standard
```

This format improves semantic search accuracy compared to raw table row text.

## Content Hash Deduplication

Chunks with identical content (same structured text) are automatically deduplicated:

- **Same content in multiple documents**: Maps to single chunk
- **Multiple ParsedSpec references**: Stored in `metadata.parsed_spec_ids` array
- **Storage efficiency**: Reduces duplicate embeddings

## Error Handling

### Incomplete Chunks

If embedding generation fails for a chunk:

1. Chunk is stored with `completion_status = 'incomplete'`
2. `embedding_vector` is NULL
3. Error is logged
4. Ingestion continues for other chunks

### Retry Failed Chunks

Query for incomplete chunks:

```sql
SELECT id, text, metadata 
FROM knowledge_chunks 
WHERE completion_status = 'incomplete';
```

Retry embedding generation (CLI command TBD in future phase).

## API Usage

### Programmatic Ingestion

```go
import (
    "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/ingest"
)

pipeline := ingest.NewPipeline(...)
req := ingest.IngestionRequest{
    TenantID:   tenantID,
    ProductID:  productID,
    CampaignID: campaignID,
    MarkdownPath: "path/to/spec.md",
}

result, err := pipeline.Ingest(ctx, req)
// Row-level chunks are automatically created for table rows
```

### Query with Grouping

```go
import (
    "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
)

router := retrieval.NewRouter(...)
req := retrieval.RetrievalRequest{
    TenantID:  tenantID,
    ProductIDs: []uuid.UUID{productID},
    Question:  "What colors does this car come in?",
}

resp, err := router.Retrieve(ctx, req)
// resp.SemanticChunks are grouped by category
```

## Testing

### Unit Tests

```bash
cd libs/knowledge-engine
go test ./internal/ingest/... -v
```

### Integration Tests

```bash
go test ./tests/integration/... -v -tags=integration
```

### Test Table Format

Create test markdown with tables:

```markdown
## Specifications

| Parent Category | Sub-Category | Specification | Value | Additional Metadata |
|----------------|-------------|---------------|-------|-------------------|
| Exterior       | Colors      | Color         | Red   | Standard |
| Exterior       | Colors      | Color         | Blue  | Standard |
```

## Troubleshooting

### Chunks Not Created

- **Check**: Table format matches 5-column structure
- **Check**: Parser logs for table detection
- **Verify**: Markdown contains valid table syntax

### Embeddings Not Generated

- **Check**: Embedding service configuration
- **Check**: API key is valid
- **Review**: Incomplete chunks in database (`completion_status = 'incomplete'`)

### Query Results Not Grouped

- **Verify**: Chunks have `chunk_type = 'spec_row'`
- **Check**: Metadata contains `parent_category` and `sub_category`
- **Review**: Router grouping logic

### Deduplication Not Working

- **Verify**: `content_hash` column is populated
- **Check**: Unique index exists on `content_hash`
- **Review**: Hash computation (should be deterministic)

## Performance

### Batch Size

Default batch size: 50-100 chunks. Adjust in pipeline config:

```go
config := ingest.PipelineConfig{
    // ... other config
    EmbeddingBatchSize: 75, // Adjust as needed
}
```

### Ingestion Time

- **Target**: 200 rows in <10 minutes
- **Factors**: Embedding API latency, batch size, network
- **Optimization**: Increase batch size (up to 100) if API allows

## Next Steps

- See `spec.md` for full specification
- See `data-model.md` for database schema details
- See `research.md` for technical decisions
- See `plan.md` for implementation plan

## Support

For issues or questions:
1. Check logs: `logs/knowledge-engine.log`
2. Review database: Query `knowledge_chunks` table
3. Verify configuration: Check `configs/dev.yaml`

