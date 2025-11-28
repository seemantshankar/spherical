# USP Extraction Investigation

## Issue
Only 14 USPs are stored in the database, but the markdown contains 49 USP bullets.

## Findings

### 1. Parser Logic
- The parser uses regex pattern: `(?i)##\s*(?:USPs?|Unique Selling (?:Points?|Propositions?)|Why (?:Buy|Choose))\s*\n((?:[-*]\s*.+\n?)+)`
- Python simulation shows this regex can extract all 49 USP bullets from 17 sections
- The `parseBulletList()` function should correctly parse all bullets

### 2. Storage Logic Bug (FIXED)
- **CRITICAL**: Line 466 in `knowledge-demo/main.go` uses `db.Exec()` without error checking
- Database insert failures are silently ignored
- Added error checking to prevent silent failures

### 3. Possible Causes

#### A. Parser Not Extracting All USPs
- The markdown has 17 USP sections across multiple pages
- Some USP sections are inside code blocks (e.g., Page 4)
- Page breaks (`# Page X`) might interfere with regex matching
- Need to verify: What does `len(result.USPs)` actually show when parsing?

#### B. Database Constraint Violations
- No unique constraints on `knowledge_chunks` table that would prevent duplicates
- But inserts might fail due to other constraints (foreign keys, etc.)
- Error checking now added to surface these issues

#### C. Embedding Generation Failures
- If embedding API fails for many USPs, they might be skipped
- But the code stores USPs even if embedding fails (embVector can be empty)
- Added error logging for embedding failures

### 4. Recommended Actions

1. **Add error checking** ✅ DONE
   - Database insert errors now logged
   - Embedding errors now logged
   - Vector adapter insert errors now logged

2. **Verify parser output**
   - Check what `len(result.USPs)` actually returns
   - The demo app prints this at line 333

3. **Test with real ingestion**
   - Re-run ingestion with error checking enabled
   - Check console output for errors
   - Verify all 49 USPs are extracted and stored

4. **Check markdown formatting**
   - Some USP sections might be inside code blocks
   - Page breaks might interfere with regex
   - Consider improving parser to handle edge cases

### 5. Resolution ✅

**Root Cause Identified**: The demo app was using the wrong markdown file!
- Demo app was using: `e-brochure-camry-hybrid-specs.md` (only 14 USPs)
- Should be using: `camry-output-v3.md` (has all 49 USPs)

**Fixes Applied**:
1. ✅ Updated brochure file paths to prioritize `camry-output-v3.md`
2. ✅ Added error checking for database inserts
3. ✅ Added error logging for embedding failures
4. ✅ Fixed FAISS adapter dimension mismatch issue

**Results After Fix**:
- ✅ Parser now extracts **49 USPs** (was 14)
- ✅ All **49 USPs stored** in database (verified)
- ✅ All **49 USPs have embeddings** (verified)
- ✅ No errors during ingestion

The parser logic was working correctly all along - it was just using the wrong source file!

---

## Additional Issue: Vector Dimension Mismatch During Search

### Problem
When querying, there's a dimension mismatch error:
- **Stored vectors**: Dimension 768 (mock embeddings used during ingestion)
- **Query vectors**: Dimension 3072 (real embeddings from OpenRouter API)

Error: `vector dimension mismatch: query dimension 3072 doesn't match stored vector dimension 768`

### Root Cause
- During ingestion: Demo app used mock embeddings (768 dim) because `OPENROUTER_API_KEY` wasn't set
- During querying: Demo app uses real embeddings (3072 dim) because `OPENROUTER_API_KEY` is now set
- Cannot compare vectors of different dimensions

### Fix Applied ✅
Updated `Search` method in `vector_adapter.go` to:
1. Check dimension mismatch early and return empty results gracefully (instead of error)
2. Skip vectors with mismatched dimensions when collecting candidates
3. Allow fallback to keyword search when vector search returns no results

This prevents errors and allows the system to continue functioning, though vector search won't work until vectors are re-ingested with the same embedding model used for queries.

### Solution Options
1. **Re-ingest with real embeddings** (recommended):
   - Set `OPENROUTER_API_KEY` before ingestion
   - Delete database and re-run ingestion
   - All vectors will have dimension 3072

2. **Use mock embeddings for queries**:
   - Unset `OPENROUTER_API_KEY` during queries
   - Both ingestion and query use dimension 768

