# USP Verification Tool

This tool verifies that USP chunks are properly stored in the database and can be retrieved via vector search.

## Usage

```bash
cd libs/knowledge-engine
go run ./cmd/verify-usps [database_path]
```

If no database path is provided, it defaults to `/tmp/knowledge_demo.db`

## What it checks

1. **USP chunks in database**: Counts how many USP chunks exist in the `knowledge_chunks` table
2. **Embeddings**: Verifies that embeddings were generated and stored for each USP chunk
3. **Vector search filtering**: Tests that vector search can find USP chunks when filtering by `chunk_type='usp'`

## Expected Output

- Number of USP chunks in database
- Sample USP chunks with their embedding status
- Embedding statistics (with/without embeddings)
- Vector search test results

