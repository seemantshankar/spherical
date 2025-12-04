# Research: Row-Level Chunking for Table Data

**Feature**: `004-row-level-chunking`  
**Date**: 2025-12-04  
**Status**: Complete

## Research Questions & Decisions

### RQ-001: Content Hash Algorithm

**Question**: What algorithm should be used for generating content hashes to uniquely identify table row chunks?

**Decision**: Use SHA-256 hash of the structured text content (normalized).

**Rationale**: 
- SHA-256 is already used in the codebase for document source hashing (see `pipeline.go:createDocumentSource`)
- Provides strong collision resistance (2^256 possible values)
- Deterministic and fast computation
- Standard library support (`crypto/sha256`)
- 32-byte output (64 hex characters) is efficient for storage and indexing

**Alternatives Considered**:
- MD5: Faster but weaker collision resistance, not recommended for security-sensitive applications
- SHA-512: Stronger but larger output (64 bytes), unnecessary for content identification
- Content-based hash with normalization: SHA-256 of normalized content (trimmed, whitespace-normalized) ensures same content produces same hash

**Implementation**: Hash the structured text format string (key-value pairs) after normalization (trim whitespace, normalize line endings).

---

### RQ-002: Batch Embedding Processing

**Question**: How should batch embedding generation handle failures for individual chunks within a batch?

**Decision**: Process batches with partial failure handling - store successful chunks with embeddings, store failed chunks without embeddings marked as incomplete/retry-needed.

**Rationale**:
- Existing `EmbedBatch` method in `embedding/client.go` processes entire batches atomically (all succeed or all fail)
- Need to handle individual chunk failures within a batch
- Solution: Wrap batch processing with per-chunk error handling, retry failed chunks individually
- Store incomplete chunks for later retry without blocking ingestion

**Alternatives Considered**:
- Fail entire batch on any error: Too strict, blocks ingestion unnecessarily
- Skip failed chunks entirely: Loses data, violates FR-012 (process all rows)
- Retry failed chunks immediately: Could cause infinite loops, better to defer retry

**Implementation**: 
- Process chunks in batches of 50-100
- For each batch, attempt batch embedding
- If batch fails, fall back to individual chunk embedding with error handling
- Store chunks with `completion_status = 'incomplete'` when embedding fails
- Log errors for monitoring and retry queue

---

### RQ-003: Content Hash Storage and Indexing

**Question**: How should content hash be stored and indexed in the database?

**Decision**: Add `content_hash` column to `knowledge_chunks` table as VARCHAR(64) with unique index for deduplication lookups.

**Rationale**:
- SHA-256 hex string is 64 characters
- Unique index enables fast O(log n) lookups for deduplication
- Allows linking multiple ParsedSpec references to same chunk
- Existing `KnowledgeChunk` model can be extended

**Alternatives Considered**:
- Separate content_hash table: Adds complexity, unnecessary join overhead
- Binary storage (BLOB): More efficient but harder to query/debug
- No unique constraint: Allows duplicates, violates uniqueness requirement

**Implementation**:
- Add `content_hash VARCHAR(64) UNIQUE` column
- Index for fast lookups during ingestion
- Query by content_hash to find existing chunks before creating new ones

---

### RQ-004: Hierarchical Grouping in Query Results

**Question**: How should query results be grouped hierarchically by parent category and sub-category?

**Decision**: Group results in-memory after retrieval, using metadata JSON field to extract parent_category and sub_category.

**Rationale**:
- Metadata already stored as JSON in `KnowledgeChunk.Metadata` field
- Parent category and sub-category are extracted during parsing and stored in metadata
- Grouping can be done in application layer after vector/SQL retrieval
- Maintains flexibility for different grouping strategies

**Alternatives Considered**:
- Database-level grouping: Requires complex SQL with JSON functions, less portable
- Pre-computed grouping: Adds storage overhead, less flexible
- Single-level grouping: Doesn't meet requirement for hierarchical grouping

**Implementation**:
- Store `parent_category` and `sub_category` in chunk metadata JSON during ingestion
- After retrieval, extract metadata and group by parent_category first, then sub_category
- Return grouped structure in `RetrievalResponse` with category labels

---

### RQ-005: Completion Status Storage

**Question**: How should completion status (complete/incomplete/retry-needed) be stored?

**Decision**: Add `completion_status` column as ENUM or VARCHAR with values: 'complete', 'incomplete', 'retry-needed'.

**Rationale**:
- Simple, explicit state tracking
- Enables querying for incomplete chunks for retry processing
- Clear audit trail of chunk processing status
- Can be indexed for efficient retry queue queries

**Alternatives Considered**:
- Nullable embedding field: Less explicit, harder to query for retry candidates
- Separate retry queue table: Adds complexity, unnecessary for current scale
- Status in metadata JSON: Less queryable, harder to index

**Implementation**:
- Add `completion_status VARCHAR(20)` column with CHECK constraint
- Default to 'complete' for chunks with embeddings
- Set to 'incomplete' when embedding generation fails
- Query for `completion_status = 'incomplete'` to find retry candidates

---

## Technical Dependencies

### Existing Infrastructure (Available)

- ✅ Embedding service with batch support (`embedding.Client.EmbedBatch`)
- ✅ KnowledgeChunk storage model with metadata JSON field
- ✅ Table parsing logic in `parser.go` (parseSpecTables)
- ✅ Pipeline batch processing infrastructure
- ✅ SHA-256 hashing utilities (used in document source creation)

### New Requirements

- Content hash generation utility function
- Batch embedding with per-chunk error handling
- Hierarchical grouping logic in retrieval router
- Database migration for new columns (content_hash, completion_status)
- Retry mechanism for incomplete chunks (can be deferred to later phase)

---

## Performance Considerations

### Batch Size Optimization

- **Decision**: Use batch size of 50-100 chunks for embedding generation
- **Rationale**: Balance between API rate limits, latency, and error recovery
- **Testing**: Validate with 200-row table to ensure 10-minute ingestion target

### Content Hash Computation

- **Decision**: Compute hash during parsing, before storage
- **Rationale**: Enables early deduplication check, avoids re-computation
- **Performance**: SHA-256 is fast (~100MB/s), negligible overhead for table rows

### Query Grouping

- **Decision**: In-memory grouping after retrieval
- **Rationale**: Small result sets (top-k ≤ 8), grouping overhead is minimal
- **Performance**: O(n log n) for grouping, acceptable for <100 results

---

## Security & Compliance

### Content Hash Collisions

- **Risk**: SHA-256 collision probability is negligible (2^-256)
- **Mitigation**: Monitor for hash collisions in logs, add content comparison fallback if needed

### Data Deduplication

- **Benefit**: Reduces storage for duplicate content across documents
- **Consideration**: Multiple ParsedSpec references linked to same chunk maintains audit trail

---

## Open Questions (Deferred)

- Retry mechanism implementation details (CLI command vs background job)
- Monitoring/alerting for incomplete chunk counts
- Migration strategy for existing chunks (re-ingestion required per spec)

---

## References

- Existing code: `libs/knowledge-engine/internal/ingest/pipeline.go`
- Existing code: `libs/knowledge-engine/internal/embedding/client.go`
- Existing code: `libs/knowledge-engine/internal/storage/models.go`
- SHA-256 specification: RFC 6234

