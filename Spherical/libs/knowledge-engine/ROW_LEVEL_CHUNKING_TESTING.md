# Testing Guide: Row-Level Chunking Feature

This guide explains how to test the row-level chunking feature that has been implemented.

## Quick Start

### Run Unit Tests

```bash
cd libs/knowledge-engine

# Run all parser tests (including row chunking tests)
go test ./internal/ingest -v

# Run only row chunking tests
go test ./internal/ingest -v -run TestGenerateRowChunks

# Run all tests in the package
go test ./internal/ingest -v -run TestParser
```

### Run Integration Tests

```bash
# Run integration tests (requires testcontainers or database)
go test ./tests/integration -v -run TestIngestionPipeline

# Run with short mode disabled (full integration tests)
go test ./tests/integration -v -short=false
```

## Test Coverage

### ✅ Unit Tests (Parser Level)

The following unit tests verify parser functionality:

1. **`TestGenerateRowChunks_5ColumnTable`** - Tests 5-column table parsing
   - Verifies parent_category, sub_category extraction
   - Checks structured text format
   - Validates content hash generation

2. **`TestGenerateRowChunks_3ColumnTable`** - Tests 3-column table (legacy format)
   - Verifies default sub-category assignment
   - Checks backward compatibility

3. **`TestGenerateRowChunks_4ColumnTable`** - Tests 4-column table
   - Verifies intermediate format handling

4. **`TestGenerateRowChunks_SkipsHeaderRows`** - Tests header row filtering
   - Ensures header rows are not converted to chunks

5. **`TestComputeContentHash`** - Tests content hash generation
   - Verifies deterministic hashing
   - Checks hash uniqueness for different content

6. **`TestFormatRowChunkText`** - Tests structured text formatting
   - Verifies key-value pair format

7. **`TestExtractTableRowMetadata`** - Tests metadata extraction
   - Verifies default values for empty fields

### Manual Testing

#### 1. Test Parser with Sample Table

Create a test file `test-table.md`:

```markdown
---
title: Test Product
product: Test Car
year: 2025
---

## Exterior Colors

| Parent Category | Sub-Category | Specification | Value | Additional Metadata |
|-----------------|--------------|---------------|-------|---------------------|
| Exterior | Colors | Color | Pearl Metallic Gallant Red | Standard |
| Exterior | Colors | Color | Super White | Standard |
| Interior | Upholstery | Material | Leather | Premium Package |
```

Then test parsing:

```bash
# Use the test-parse command if available
go run cmd/test-parse/main.go test-table.md
```

#### 2. Test Content Hash Deduplication

The content hash should be the same for identical rows:

```go
// In a test or Go playground
text1 := "Category: Exterior\nSub-Category: Colors\nSpecification: Color\nValue: Red"
text2 := "Category: Exterior\nSub-Category: Colors\nSpecification: Color\nValue: Red"

hash1 := computeContentHash(text1)
hash2 := computeContentHash(text2)

// hash1 == hash2 (should be true)
```

#### 3. Test Database Migration

```bash
# Apply migration (if using migration tool)
# Or manually run the SQL:

# For Postgres:
psql -d your_db -f db/migrations/0002_add_row_chunking_fields.sql

# For SQLite:
sqlite3 your_db.db < db/migrations/0002_add_row_chunking_fields_sqlite.sql
```

Verify columns exist:

```sql
-- Postgres
\d knowledge_chunks

-- SQLite
.schema knowledge_chunks
```

You should see:
- `content_hash VARCHAR(64)`
- `completion_status VARCHAR(20)`
- Indexes: `idx_chunks_content_hash`, `idx_chunks_completion_status`

#### 4. Test Full Ingestion Pipeline

```bash
# If you have a CLI tool set up
./knowledge-engine-cli ingest --markdown test-table.md --tenant-id <uuid> --product-id <uuid>
```

Or use the integration test:

```bash
go test ./tests/integration -v -run TestIngestionPipeline_FullIngestion
```

#### 5. Test Hierarchical Grouping

Query for row chunks and verify they're grouped:

```go
// In a test
req := retrieval.RetrievalRequest{
    TenantID:   tenantID,
    ProductIDs: []uuid.UUID{productID},
    Question:   "What colors are available?",
    MaxChunks:  10,
}

response, err := router.Query(ctx, req)
// Check that SemanticChunks are grouped by parent_category, then sub_category
```

## What to Verify

### ✅ Parser Level
- [x] Tables are detected correctly
- [x] Each table row becomes one chunk
- [x] Content hash is generated for each row
- [x] Metadata includes parent_category, sub_category, specification_type, value
- [x] Structured text format is correct (key-value pairs)

### ✅ Storage Level
- [x] Chunks are stored with content_hash
- [x] Deduplication works (same content_hash = same chunk)
- [x] completion_status is set correctly
- [x] Metadata JSON is stored properly

### ✅ Retrieval Level
- [x] Row chunks are retrieved correctly
- [x] Hierarchical grouping works (parent_category → sub_category)
- [x] Category metadata is included in results

## Known Limitations (To Be Implemented)

### Phase 4: Batch Processing
- ⏳ Batch embedding (50-100 chunks per batch) - Not yet implemented
- ⏳ Per-chunk error handling - Not yet implemented
- ⏳ Incomplete chunk retry mechanism - Not yet implemented

### Phase 5: Metadata Linkage
- ⏳ ParsedSpec ID linking - Partially implemented
- ⏳ Specification type filtering - Not yet implemented

## Troubleshooting

### Tests Fail with "undefined: json"
- Make sure `encoding/json` is imported in `pipeline.go`

### Migration Fails
- Check database connection
- Verify migration hasn't been applied already
- For SQLite, ensure the file exists and is writable

### No Row Chunks Generated
- Verify table format matches expected structure (3, 4, or 5 columns)
- Check that tables use pipe separators (`|`)
- Ensure header rows are properly formatted

### Content Hash Not Working
- Verify `computeContentHash` function is called
- Check that text normalization is working (trimming, whitespace)

## Next Steps

1. **Complete Phase 4**: Implement batch embedding and error handling
2. **Complete Phase 5**: Add specification type filtering
3. **Add Integration Tests**: Create full end-to-end tests
4. **Performance Testing**: Test with large tables (200+ rows)

## Example Test Output

When running tests, you should see:

```
=== RUN   TestGenerateRowChunks_5ColumnTable
--- PASS: TestGenerateRowChunks_5ColumnTable (0.00s)
=== RUN   TestGenerateRowChunks_3ColumnTable
--- PASS: TestGenerateRowChunks_3ColumnTable (0.00s)
=== RUN   TestGenerateRowChunks_4ColumnTable
--- PASS: TestGenerateRowChunks_4ColumnTable (0.00s)
=== RUN   TestGenerateRowChunks_SkipsHeaderRows
--- PASS: TestGenerateRowChunks_SkipsHeaderRows (0.00s)
=== RUN   TestGenerateRowChunks_ContentHashDeduplication
--- PASS: TestGenerateRowChunks_ContentHashDeduplication (0.00s)
PASS
ok  	github.com/spherical-ai/spherical/libs/knowledge-engine/internal/ingest	0.640s
```

All tests should pass! ✅

