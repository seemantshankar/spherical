# Query Troubleshooting Guide

## Issue: Queries Return 0 Results

If your query command runs successfully but returns no structured facts or semantic chunks, here's what's happening and how to diagnose it.

### Your Current Output

```
Intent: spec_lookup (latency: 8ms)
```

This means:
- ✅ Query command is working correctly
- ✅ Intent classification is working
- ❌ No structured facts found (0 results)
- ❌ No semantic chunks found (0 results)

### Root Cause Analysis

Based on your query logs, the issue is:

1. **Keyword Search Finds 0 Results**: 
   - `Keyword search results keyword=fuel results=0`
   - This means no structured specs are in the database

2. **Vector Search Finds 0 Results**:
   - `Vector search completed query_dimension=768 results_count=0`
   - The FAISS vector index is empty

### Why This Happens

The FAISS adapter is **in-memory only**. Here's what happens:

1. **During Ingestion**:
   - Vectors are created and added to the in-memory FAISS adapter
   - Chunks are stored in the database
   - But embeddings may not be persisted to the database correctly

2. **During Query**:
   - A NEW empty FAISS adapter instance is created
   - It has no vectors to search
   - The database has chunks, but they don't have embeddings stored

### Diagnosis Steps

Run these commands to check your data:

```bash
cd /Users/seemant/Documents/Projects/spherical/Spherical/libs/knowledge-engine

# 1. Check if chunks exist
sqlite3 /tmp/knowledge-engine.db \
  "SELECT COUNT(*) as total_chunks FROM knowledge_chunks WHERE tenant_id = 'efc6ed22-b61c-5af1-a85b-11b36af6069c';"

# 2. Check if embeddings are stored in database
sqlite3 /tmp/knowledge-engine.db \
  "SELECT COUNT(*) as chunks_with_embeddings FROM knowledge_chunks WHERE tenant_id = 'efc6ed22-b61c-5af1-a85b-11b36af6069c' AND embedding_vector IS NOT NULL AND LENGTH(embedding_vector) > 0;"

# 3. Check campaign status
sqlite3 /tmp/knowledge-engine.db \
  "SELECT status FROM campaign_variants WHERE id = '54c7e4c1-fef2-541f-86ce-086511973dd5';"

# 4. Sample chunk content
sqlite3 /tmp/knowledge-engine.db \
  "SELECT chunk_type, SUBSTR(text, 1, 100) as sample FROM knowledge_chunks WHERE tenant_id = 'efc6ed22-b61c-5af1-a85b-11b36af6069c' LIMIT 3;"
```

### Expected Results

If everything is working:
- **total_chunks**: Should be > 0 (you have 15)
- **chunks_with_embeddings**: Should be > 0 (you have 0 - this is the problem!)
- **status**: Should be 'published' for queries to work (currently 'draft')

### Solutions

#### Option 1: Fix Embedding Storage (Recommended)

The embeddings need to be properly stored in the database during ingestion. Currently, they're being created but not persisted. This requires:

1. Ensuring the `embedding_vector` column in `knowledge_chunks` is properly populated
2. Or implementing a vector loading mechanism that reads from the database

#### Option 2: Use Published Campaign

Publish your campaign first:

```bash
go run ./cmd/knowledge-engine-cli publish \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --campaign wagon-r-2025-india
```

Note: This may not fully solve the vector search issue if embeddings aren't stored.

#### Option 3: Re-run Ingestion

If embeddings weren't stored, re-run the ingestion:

```bash
go run ./cmd/knowledge-engine-cli ingest \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --product wagon-r-2025 \
  --campaign wagon-r-2025-india \
  --markdown /tmp/arena-extraction/arena-wagon-specs.md \
  --overwrite
```

### Current Status Summary

✅ **Working**:
- PDF extraction
- Markdown cleanup
- Ingestion pipeline execution
- Query CLI functionality
- 15 chunks created in database

❌ **Issues**:
- Embeddings not persisted to database (0 chunks with embeddings)
- FAISS index is in-memory only (lost between sessions)
- Campaign in draft status
- Schema issues preventing specs/features from being stored

### Quick Verification

Check if your setup is correct:

```bash
# Quick diagnostic script
cd /Users/seemant/Documents/Projects/spherical/Spherical/libs/knowledge-engine

cat << 'EOF' | sqlite3 /tmp/knowledge-engine.db
.headers on
.mode column

SELECT '=== Data Summary ===' as '';
SELECT COUNT(*) as total_chunks FROM knowledge_chunks WHERE tenant_id = 'efc6ed22-b61c-5af1-a85b-11b36af6069c';
SELECT COUNT(*) as chunks_with_embeddings FROM knowledge_chunks WHERE tenant_id = 'efc6ed22-b61c-5af1-a85b-11b36af6069c' AND embedding_vector IS NOT NULL AND LENGTH(embedding_vector) > 0;
SELECT status as campaign_status FROM campaign_variants WHERE id = '54c7e4c1-fef2-541f-86ce-086511973dd5';
SELECT COUNT(*) as document_sources FROM document_sources WHERE tenant_id = 'efc6ed22-b61c-5af1-a85b-11b36af6069c';
EOF
```

### What to Check Next

1. Verify embeddings were actually stored in the database during ingestion
2. Check if the FAISS adapter can load vectors from the database
3. Ensure campaign is published if required
4. Review ingestion logs for errors that prevented data storage

### Next Steps

The query infrastructure is working correctly (8ms latency is excellent!). The issue is that:
- The vector index isn't persisting or loading properly
- Embeddings may not be stored in the database

You'll need to either:
1. Implement vector loading from database on query startup
2. Fix embedding persistence during ingestion
3. Use a persistent vector store (like PGVector in production)

See `QUERY_GUIDE.md` for the complete query usage guide.

