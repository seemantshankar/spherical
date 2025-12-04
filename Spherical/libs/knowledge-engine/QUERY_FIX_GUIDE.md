# Fix Guide: Query Returns 0 Results

## Your Current Situation

✅ **Working:**
- 15 knowledge chunks exist in the database
- Query command executes successfully (2ms latency)
- Intent classification works correctly

❌ **Problem:**
- Keyword search finds 0 results
- Vector search finds 0 results
- No structured facts or semantic chunks returned

## Root Cause

Looking at your query logs:

```
Keyword search results keyword=fuel results=0
Keyword search results keyword=efficiency results=0
Vector search completed query_dimension=768 results_count=0
```

**The Issue:**
1. **FAISS adapter is empty** - When the query runs, it creates a NEW empty FAISS adapter
2. **Vectors aren't loaded** - The adapter doesn't load vectors from the database
3. **In-memory only** - Vectors added during ingestion are lost when ingestion completes

## Why This Happens

The FAISS adapter in the knowledge-engine is **in-memory only**. Here's the flow:

```
Ingestion: Creates vectors → Adds to in-memory FAISS → Ingestion ends → Vectors lost
Query:     Creates NEW empty FAISS → Searches empty index → Returns 0 results
```

## Solutions

### Option 1: Implement Vector Loading (Recommended)

The query CLI needs to load vectors from the database into the FAISS adapter before searching. This requires modifying the query command to:

1. Load knowledge chunks with embeddings from the database
2. Insert them into the FAISS adapter before searching

### Option 2: Use Knowledge Chunks Repository Directly

Instead of relying on FAISS, query the database directly for knowledge chunks and perform simple text matching.

### Option 3: Verify Embeddings Were Stored

First, let's verify if embeddings are actually in the database:

```bash
cd /Users/seemant/Documents/Projects/spherical/Spherical/libs/knowledge-engine

# Check if embeddings exist in database
sqlite3 /tmp/knowledge-engine.db \
  "SELECT id, chunk_type, LENGTH(text) as text_len, 
   CASE WHEN embedding_vector IS NOT NULL AND LENGTH(embedding_vector) > 0 
   THEN 'YES' ELSE 'NO' END as has_embedding
   FROM knowledge_chunks 
   WHERE tenant_id = 'efc6ed22-b61c-5af1-a85b-11b36af6069c' 
   LIMIT 5;"
```

## Quick Diagnostic Commands

Run these to understand your current state:

```bash
cd /Users/seemant/Documents/Projects/spherical/Spherical/libs/knowledge-engine

# 1. Check total chunks
echo "=== Chunk Summary ===" && \
sqlite3 /tmp/knowledge-engine.db \
  "SELECT COUNT(*) as total_chunks FROM knowledge_chunks WHERE tenant_id = 'efc6ed22-b61c-5af1-a85b-11b36af6069c';"

# 2. Check chunks with embeddings
echo "=== Embedding Status ===" && \
sqlite3 /tmp/knowledge-engine.db \
  "SELECT COUNT(*) as chunks_with_embeddings FROM knowledge_chunks 
   WHERE tenant_id = 'efc6ed22-b61c-5af1-a85b-11b36af6069c' 
   AND embedding_vector IS NOT NULL 
   AND LENGTH(embedding_vector) > 0;"

# 3. Sample chunk content
echo "=== Sample Chunks ===" && \
sqlite3 /tmp/knowledge-engine.db \
  "SELECT chunk_type, SUBSTR(text, 1, 80) as preview 
   FROM knowledge_chunks 
   WHERE tenant_id = 'efc6ed22-b61c-5af1-a85b-11b36af6069c' 
   LIMIT 3;"

# 4. Check campaign status
echo "=== Campaign Status ===" && \
sqlite3 /tmp/knowledge-engine.db \
  "SELECT status, trim, locale FROM campaign_variants 
   WHERE id = '54c7e4c1-fef2-541f-86ce-086511973dd5';"
```

## Expected vs Actual

### Expected Behavior
- Vectors stored in database during ingestion
- Query loads vectors from database into FAISS adapter
- Vector search finds matching chunks
- Results returned with similarity scores

### Actual Behavior
- Chunks created in database (15 chunks ✅)
- Embeddings may not be persisted to database
- Query creates empty FAISS adapter
- No vectors to search → 0 results

## Next Steps to Fix

1. **Check embedding storage**: Verify if embeddings are in the database
2. **Implement vector loading**: Add code to load vectors from DB into FAISS on query startup
3. **Or use direct DB queries**: Query knowledge_chunks table directly with text search

## Workaround: Direct Database Query

As a temporary workaround, you can query the database directly:

```bash
# Find chunks containing "fuel"
sqlite3 /tmp/knowledge-engine.db \
  "SELECT chunk_type, text FROM knowledge_chunks 
   WHERE tenant_id = 'efc6ed22-b61c-5af1-a85b-11b36af6069c' 
   AND text LIKE '%fuel%' 
   LIMIT 5;"
```

This bypasses the vector search and does simple text matching.

## Summary

- **Command works**: Query CLI is functional ✅
- **Data exists**: 15 chunks in database ✅
- **Issue**: FAISS adapter doesn't load vectors from database ❌
- **Solution needed**: Implement vector loading from database or use direct DB queries

