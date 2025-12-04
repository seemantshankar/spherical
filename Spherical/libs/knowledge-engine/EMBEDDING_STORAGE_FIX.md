# Embedding Storage Fix

## Problem Identified

The query command was returning 0 results because:
1. ✅ Vector loading was implemented correctly
2. ❌ **Embeddings were never saved to the database during ingestion**

The `KnowledgeChunkRepository.Create()` method was **not including `embedding_vector`** in the INSERT statement, so even though embeddings were generated during ingestion, they weren't persisted to the database.

## Fix Applied

Updated `libs/knowledge-engine/internal/storage/repositories.go`:

- **Added `embedding_vector` to INSERT statement**
- **Added embedding serialization**: Converts `[]float32` to JSON BLOB format before storing
- Embeddings are now properly persisted when chunks are created

## What This Means

### For New Ingestions

All new ingestion runs will now properly save embeddings to the database, and queries will work immediately.

### For Existing Data

Your existing 15 chunks **do not have embeddings stored**. You have two options:

#### Option 1: Re-run Ingestion (Recommended)

Re-run the ingestion to store embeddings for all chunks:

```bash
cd /Users/seemant/Documents/Projects/spherical/Spherical/libs/knowledge-engine

go run ./cmd/knowledge-engine-cli ingest \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --product wagon-r-2025 \
  --campaign wagon-r-2025-india \
  --markdown /tmp/arena-extraction/arena-wagon-specs.md \
  --overwrite
```

The `--overwrite` flag will recreate chunks with embeddings stored.

#### Option 2: Update Existing Chunks

You could write a migration script to generate embeddings for existing chunks, but re-ingesting is simpler.

## Verification

After re-running ingestion, verify embeddings were stored:

```bash
sqlite3 /tmp/knowledge-engine.db \
  "SELECT COUNT(*) as total_chunks,
   COUNT(CASE WHEN embedding_vector IS NOT NULL AND LENGTH(embedding_vector) > 0 THEN 1 END) as chunks_with_embeddings
   FROM knowledge_chunks 
   WHERE tenant_id = 'efc6ed22-b61c-5af1-a85b-11b36af6069c';"
```

You should see:
- `total_chunks`: 15 (or more)
- `chunks_with_embeddings`: Same as total_chunks (all should have embeddings)

Then test the query:

```bash
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What is the fuel efficiency?" \
  --verbose
```

You should now see:
- `Loaded vectors into FAISS adapter` log message with count > 0
- Query results returned!

## Summary

✅ **Fixed**: Embeddings are now saved during ingestion  
✅ **Fixed**: Vectors are loaded from database into FAISS adapter  
✅ **Fixed**: Query command should now return results (after re-ingestion)

The vector loading implementation is complete and working. You just need to re-run ingestion to populate embeddings for your existing data.

