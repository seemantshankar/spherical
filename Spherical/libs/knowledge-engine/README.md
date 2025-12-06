# Knowledge Engine

A semantic knowledge engine for extracting, storing, and retrieving structured information from product specification documents.

## Features

- **Document Ingestion**: Parse markdown documents and extract specifications, features, and USPs
- **Semantic Search**: Vector-based semantic retrieval with hybrid keyword/vector search
- **Row-Level Chunking**: Convert table rows into individual semantic chunks for precise retrieval
- **Content Deduplication**: Content hash-based deduplication across documents
- **Hierarchical Grouping**: Group query results by category hierarchy
- **Batch Processing**: Efficient batch embedding generation for large tables
- **Guardrails & Explanations**: Single-sentence, sanitized explanations with fallback markers; semantic fallback triggers on low keyword confidence.

## Row-Level Chunking

### Overview

Row-level chunking converts each table row into an individual semantic chunk, enabling precise retrieval of specific table attributes (e.g., "car colors") instead of returning entire table chunks.

### How It Works

1. **Table Detection**: Automatically detects markdown tables with 3, 4, or 5 columns
2. **Row Extraction**: Each table row becomes one chunk with structured text format
3. **Content Hashing**: SHA-256 hash generated for deduplication
4. **Metadata Extraction**: Extracts parent category, sub-category, specification type, and value
5. **Batch Embedding**: Processes embeddings in batches of 50-100 chunks
6. **Hierarchical Grouping**: Query results grouped by parent category → sub-category

### Table Format Support

#### 5-Column Tables (Recommended)
```
| Parent Category | Sub-Category | Specification | Value | Additional Metadata |
|-----------------|--------------|---------------|-------|---------------------|
| Exterior | Colors | Color | Red | Standard |
| Interior | Seats | Material | Leather | Premium Package |
```

#### 4-Column Tables
```
| Category | Specification | Value | Unit |
|----------|---------------|-------|------|
| Engine | Power | 176 | hp |
```

#### 3-Column Tables
```
| Category | Specification | Value |
|----------|---------------|-------|
| Engine | Type | Hybrid |
```

### Structured Text Format

Each row chunk is formatted as structured text (key-value pairs):

```
Category: Exterior
Sub-Category: Colors
Specification: Color
Value: Red
Additional Metadata: Standard
```

### Content Hash Deduplication

- Same row content across different documents maps to the same chunk
- Content hash: SHA-256 of normalized structured text (64-character hex)
- Deduplication: Links multiple document sources to the same chunk via `parsed_spec_ids` array

### Batch Embedding

- **Batch Size**: Configurable (default: 75 chunks per batch)
- **Range**: 50-100 chunks per batch for optimal performance
- **Error Handling**: Individual chunk failures don't block ingestion
- **Incomplete Chunks**: Failed embeddings stored with `completion_status='incomplete'` for retry

### Hierarchical Grouping

Query results are automatically grouped:

```
Exterior
  Colors
    - Red
    - Blue
  Materials
    - Steel
Interior
  Seats
    - Leather
    - Fabric
```

### Usage Examples

#### Ingesting Documents with Tables

```go
pipeline := ingest.NewPipeline(
    logger,
    ingest.PipelineConfig{
        EmbeddingBatchSize: 75, // Configure batch size
    },
    repos,
    embedder,
    vectorAdapter,
    lineageWriter,
)

result, err := pipeline.Ingest(ctx, ingest.IngestionRequest{
    TenantID:     tenantID,
    ProductID:    productID,
    CampaignID:   campaignID,
    MarkdownPath: "specs.md",
    Operator:     "user@example.com",
})
```

#### Querying with Specification Type Filter

```go
response, err := router.Query(ctx, retrieval.RetrievalRequest{
    TenantID:   tenantID,
    ProductIDs: []uuid.UUID{productID},
    Question:   "What colors are available?",
    Filters: retrieval.RetrievalFilters{
        SpecificationType: func() *string { s := "Color"; return &s }(), // Filter by specification type
    },
    MaxChunks: 20,
})
```

#### Retrying Incomplete Chunks

```go
incompleteChunks, err := repos.KnowledgeChunks.FindIncompleteChunks(ctx, tenantID, 100)
for _, chunk := range incompleteChunks {
    // Retry embedding generation
    embedding, err := embedder.EmbedSingle(ctx, chunk.Text)
    if err == nil {
        chunk.EmbeddingVector = embedding
        chunk.CompletionStatus = "complete"
        repos.KnowledgeChunks.Update(ctx, chunk)
    }
}
```

### Configuration

#### Pipeline Configuration

```go
type PipelineConfig struct {
    EmbeddingBatchSize int // Batch size for embedding generation (default: 75)
    // ... other config
}
```

#### Retrieval Filters

```go
type RetrievalFilters struct {
    Categories        []string           // Filter by categories
    ChunkTypes        []storage.ChunkType // Filter by chunk types
    SpecificationType *string            // Filter by specification_type for row chunks
}
```

### Database Schema

#### New Columns

- `content_hash VARCHAR(64)`: SHA-256 hash for deduplication
- `completion_status VARCHAR(20)`: 'complete', 'incomplete', or 'retry-needed'

#### Indexes

- `idx_chunks_content_hash`: Unique index for fast deduplication lookups
- `idx_chunks_completion_status`: Partial index for retry queue queries

#### Metadata JSON Structure

```json
{
  "parent_category": "Exterior",
  "sub_category": "Colors",
  "specification_type": "Color",
  "value": "Red",
  "additional_metadata": "Standard",
  "parsed_spec_ids": ["doc-uuid-1", "doc-uuid-2"],
  "table_column_1": "Exterior",
  "table_column_2": "Colors",
  "table_column_3": "Color",
  "table_column_4": "Red",
  "table_column_5": "Standard"
}
```

### Performance

- **Ingestion**: 200 table rows processed within 10 minutes
- **Batch Size**: 50-100 chunks per batch (configurable, default: 75)
- **Query Performance**: p50 ≤150 ms, p95 ≤350 ms (maintained despite increased chunk count)
- **Deduplication**: O(log n) lookup via unique index

### Testing

Run unit tests:
```bash
go test ./internal/ingest -v -run TestGenerateRowChunks
```

Run integration tests:
```bash
go test ./tests/integration -v -run TestRowChunking -short=false
```

### Migration

Apply database migration:
```bash
# Postgres
psql -d your_db -f db/migrations/0002_add_row_chunking_fields.sql

# SQLite
sqlite3 your_db.db < db/migrations/0002_add_row_chunking_fields_sqlite.sql
```

### Migration & Rollout (Semantic Spec Facts)

1. Apply the semantic spec_fact migrations (`task migrate` for SQLite) to add explanation/spec_fact chunk storage.
2. Rebuild/sync FAISS indexes per campaign so spec_fact embeddings (with explanations/provenance) load into the vector store.
3. Deploy API/retrieval; explanations are sanitized on read (first sentence, <=160 chars) and semantic fallback is gated by keyword confidence.
4. Smoke tests:
   - Ingest a sample sheet; verify `explanation_failed` is set for rows that violate guardrails.
   - Call retrieval with low-keyword queries and confirm single-line explanations.
   - Performance/regression: `go test ./libs/knowledge-engine/tests/integration -run TestStructuredRetrieval_RealisticPerformance`.

### See Also

- [Row-Level Chunking Testing Guide](./ROW_LEVEL_CHUNKING_TESTING.md)
- [Row-Level Chunking Test Results](./ROW_LEVEL_CHUNKING_TEST_RESULTS.md)
- [Implementation Plan](./ROW_LEVEL_CHUNKING_PLAN.md)
