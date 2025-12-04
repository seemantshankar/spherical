# Data Model: Row-Level Chunking

**Feature**: `004-row-level-chunking`  
**Date**: 2025-12-04  
**Status**: Design Complete

## Overview

This document describes the data model changes required to support row-level chunking for table data. The changes extend the existing `knowledge_chunks` table and related entities to support content-based deduplication, completion status tracking, and hierarchical category metadata.

## Entity Changes

### KnowledgeChunk (Extended)

**Table**: `knowledge_chunks`

#### New Columns

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `content_hash` | VARCHAR(64) | UNIQUE, INDEX | SHA-256 hash of normalized structured text content. Used for deduplication - same content across documents maps to same chunk. |
| `completion_status` | VARCHAR(20) | CHECK IN ('complete', 'incomplete', 'retry-needed'), DEFAULT 'complete' | Status of chunk processing. 'complete' = has embedding, 'incomplete' = embedding generation failed, 'retry-needed' = marked for retry. |

#### Existing Columns (Relevant)

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `chunk_type` | chunk_type | Enum: 'spec_row' for table row chunks |
| `text` | TEXT | Structured text format (key-value pairs) |
| `metadata` | JSONB | Contains: parent_category, sub_category, specification_type, value, additional_metadata, parsed_spec_ids[] |
| `embedding_vector` | vector (Postgres) / BLOB (SQLite) | Nullable - null when completion_status = 'incomplete' |
| `embedding_model` | TEXT | Nullable |
| `embedding_version` | TEXT | Nullable |
| `source_doc_id` | UUID | Reference to document_source |
| `tenant_id` | UUID | Multi-tenant isolation |
| `product_id` | UUID | Product reference |
| `campaign_variant_id` | UUID | Campaign reference |

#### Metadata JSON Structure

For table row chunks (`chunk_type = 'spec_row'`), the `metadata` JSONB field contains:

```json
{
  "parent_category": "Exterior",
  "sub_category": "Colors",
  "specification_type": "Color",
  "value": "Pearl Metallic Gallant Red",
  "additional_metadata": "Standard",
  "parsed_spec_ids": ["uuid1", "uuid2"],
  "table_column_1": "Exterior",
  "table_column_2": "Colors",
  "table_column_3": "Color",
  "table_column_4": "Pearl Metallic Gallant Red",
  "table_column_5": "Standard"
}
```

**Fields**:
- `parent_category` (string): Extracted from table column 1
- `sub_category` (string): Extracted from table column 2
- `specification_type` (string): Extracted from table column 3
- `value` (string): Extracted from table column 4
- `additional_metadata` (string): Extracted from table column 5
- `parsed_spec_ids` (array of UUIDs): Array of ParsedSpec record IDs linked to this chunk (supports multiple documents with same content)
- `table_column_N` (string): Raw column values for reference

#### Indexes

**New Indexes**:
```sql
CREATE UNIQUE INDEX idx_chunks_content_hash ON knowledge_chunks(content_hash) WHERE content_hash IS NOT NULL;
CREATE INDEX idx_chunks_completion_status ON knowledge_chunks(completion_status) WHERE completion_status != 'complete';
```

**Rationale**:
- Unique index on `content_hash` enables fast O(log n) lookups for deduplication
- Partial index on `completion_status` for efficient retry queue queries (only incomplete chunks)

## Relationships

### KnowledgeChunk → ParsedSpec (Many-to-Many via Metadata)

**Relationship**: A KnowledgeChunk can be linked to multiple ParsedSpec records through the `metadata.parsed_spec_ids` array. This enables content-based deduplication where the same table row content appearing in multiple documents maps to a single chunk.

**Implementation**:
- During ingestion, when a table row is parsed:
  1. Generate content hash from structured text
  2. Check if chunk with same `content_hash` exists
  3. If exists: Append current `parsed_spec_id` to `metadata.parsed_spec_ids` array
  4. If not exists: Create new chunk with `parsed_spec_ids = [current_id]`

**Query Pattern**:
```sql
-- Find chunks linked to a specific ParsedSpec
SELECT * FROM knowledge_chunks
WHERE metadata->'parsed_spec_ids' @> '["<spec_id>"]'::jsonb;

-- Find all ParsedSpecs linked to a chunk
SELECT metadata->'parsed_spec_ids' FROM knowledge_chunks
WHERE id = '<chunk_id>';
```

### KnowledgeChunk → DocumentSource (Existing)

**Relationship**: Maintained via `source_doc_id` column. For deduplicated chunks, the `source_doc_id` may reference the first document where the content appeared, while `metadata.parsed_spec_ids` tracks all source documents.

## Data Flow

### Ingestion Flow

1. **Parse Table Row**:
   - Extract columns: Parent Category, Sub-Category, Specification, Value, Additional metadata
   - Format as structured text: `"Category: {parent}\nSub-Category: {sub}\nSpecification: {spec}\nValue: {value}\nAdditional: {meta}"`
   - Normalize (trim, normalize whitespace)

2. **Generate Content Hash**:
   - Compute SHA-256 hash of normalized structured text
   - Result: 64-character hex string

3. **Deduplication Check**:
   - Query: `SELECT id FROM knowledge_chunks WHERE content_hash = ?`
   - If found: Update existing chunk's `metadata.parsed_spec_ids` array
   - If not found: Proceed to create new chunk

4. **Create Chunk Record**:
   - Set `chunk_type = 'spec_row'`
   - Set `text` to structured format
   - Set `metadata` with category and column values
   - Set `content_hash`
   - Set `completion_status = 'complete'` (will update if embedding fails)
   - Link to `parsed_spec_id` in metadata

5. **Batch Embedding**:
   - Collect chunks in batches of 50-100
   - Call embedding service
   - On success: Update `embedding_vector`, `embedding_model`, `embedding_version`
   - On failure: Set `completion_status = 'incomplete'`, leave `embedding_vector` NULL

### Query Flow

1. **Retrieve Chunks**:
   - Vector search or keyword search returns chunks
   - Filter by `chunk_type = 'spec_row'` if needed

2. **Extract Metadata**:
   - Parse `metadata` JSONB to get `parent_category`, `sub_category`

3. **Hierarchical Grouping**:
   - Group by `parent_category` first
   - Within each parent, group by `sub_category`
   - Return nested structure

## Validation Rules

### Content Hash

- **Format**: 64-character hexadecimal string (SHA-256)
- **Uniqueness**: Must be unique across all chunks (enforced by UNIQUE index)
- **Nullability**: Can be NULL for non-table chunks (legacy chunks, prose chunks)
- **Computation**: Hash of normalized structured text (trimmed, whitespace-normalized)

### Completion Status

- **Values**: 'complete', 'incomplete', 'retry-needed'
- **Default**: 'complete'
- **Constraint**: `completion_status = 'complete'` implies `embedding_vector IS NOT NULL`
- **Constraint**: `completion_status IN ('incomplete', 'retry-needed')` implies `embedding_vector IS NULL`

### Metadata JSON

- **Required Fields** (for `chunk_type = 'spec_row'`):
  - `parent_category`: Non-empty string
  - `sub_category`: Non-empty string
  - `specification_type`: Non-empty string
  - `value`: Non-empty string
  - `parsed_spec_ids`: Array of UUIDs (at least one)

- **Optional Fields**:
  - `additional_metadata`: String (can be empty)
  - `table_column_N`: Raw column values for reference

## Migration Strategy

### Database Migration

**New Migration**: `0002_add_row_chunking_fields.sql`

```sql
-- Add content_hash column
ALTER TABLE knowledge_chunks 
ADD COLUMN content_hash VARCHAR(64);

-- Add completion_status column
ALTER TABLE knowledge_chunks 
ADD COLUMN completion_status VARCHAR(20) DEFAULT 'complete' 
CHECK (completion_status IN ('complete', 'incomplete', 'retry-needed'));

-- Create unique index on content_hash (allows NULLs)
CREATE UNIQUE INDEX idx_chunks_content_hash 
ON knowledge_chunks(content_hash) 
WHERE content_hash IS NOT NULL;

-- Create partial index for retry queue
CREATE INDEX idx_chunks_completion_status 
ON knowledge_chunks(completion_status) 
WHERE completion_status != 'complete';

-- Update existing chunks: set completion_status based on embedding presence
UPDATE knowledge_chunks 
SET completion_status = CASE 
    WHEN embedding_vector IS NULL THEN 'incomplete'
    ELSE 'complete'
END;
```

### Backward Compatibility

- **Existing Chunks**: Remain unchanged, `content_hash` is NULL, `completion_status = 'complete'` (if has embedding) or 'incomplete' (if no embedding)
- **Legacy Chunks**: Continue to work with existing queries
- **Re-ingestion Required**: To benefit from row-level chunking, documents must be re-ingested (per spec requirement)

## Performance Considerations

### Index Usage

- **Content Hash Lookup**: O(log n) via unique index for deduplication checks
- **Retry Queue**: O(log n) via partial index on `completion_status`
- **Metadata Queries**: GIN index on `metadata` JSONB for category filtering

### Storage Impact

- **Content Hash**: +64 bytes per chunk (VARCHAR(64))
- **Completion Status**: +20 bytes per chunk (VARCHAR(20))
- **Metadata**: Additional JSON fields (~200-500 bytes per row chunk)
- **Total**: ~300-600 bytes per row chunk

### Query Performance

- **Deduplication Check**: Fast (indexed lookup)
- **Hierarchical Grouping**: In-memory after retrieval (small result sets)
- **Metadata Extraction**: JSONB parsing (negligible for <100 results)

## Security & Compliance

### Content Hash Privacy

- **Hash Function**: SHA-256 (cryptographically secure, one-way)
- **Collision Resistance**: 2^256 possible values, negligible collision probability
- **Privacy**: Hash alone doesn't reveal content (requires brute force)

### Multi-Tenant Isolation

- **Row-Level Security**: Maintained via `tenant_id` column
- **Deduplication**: Content hash enables cross-tenant deduplication if needed (future enhancement)
- **Current Implementation**: Deduplication scoped to same tenant (via `tenant_id` filter in queries)

