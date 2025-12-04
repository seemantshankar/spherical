# Row-Level Chunking Implementation Plan

## Overview
Convert table-based chunks to individual row-level chunks to enable precise, granular retrieval while maintaining context and linking to structured spec records.

## Requirements Summary
- ✅ **Scope**: All table types (3, 4, 5-column tables)
- ✅ **Display**: Group results by Category
- ✅ **Backward Compatibility**: Require re-ingestion (no old format support)
- ✅ **Performance**: Handle 100s of rows per document, ensure full coverage
- ✅ **Structured Data**: Link row chunks to ParsedSpec records

---

## Architecture Changes

### 1. Chunk Structure Enhancement

#### Current Structure:
```go
type ParsedChunk struct {
    Text      string
    ChunkType storage.ChunkType
    StartLine int
    EndLine   int
    Metadata  map[string]interface{}
}
```

#### Proposed Enhancement:
```go
type ParsedChunk struct {
    Text         string
    ChunkType    storage.ChunkType
    StartLine    int
    EndLine      int
    Metadata     map[string]interface{}
    
    // New fields for row-level chunks
    SubType      string              // "spec_row" or "paragraph"
    SpecID       *uuid.UUID          // Link to ParsedSpec record
    TableContext *TableContext       // Table metadata
}

type TableContext struct {
    TableID      string              // Identifier for the table
    RowIndex     int                 // Row position in table
    ColumnCount  int                 // 3, 4, or 5
    HasHeader    bool                // Whether table has header row
}
```

### 2. Chunk Text Format

#### Structured Text Format for Row Chunks:
```
Category: Exterior
Specification: Color
Value: Pearl Metallic Gallant Red
Key Features: Standard
Variant Availability: Standard
```

**Rationale**: 
- Better embedding quality (semantic structure preserved)
- Easier to parse and extract fields
- More searchable for LLM/vector models
- Can still reconstruct table format for display

#### Alternative (if table format preferred):
```
| Exterior | Color | Pearl Metallic Gallant Red | Standard | Standard |
```

**Recommendation**: Use structured format for better semantic search.

---

## Implementation Steps

### Phase 1: Parser Modifications (`parser.go`)

#### Step 1.1: Add Table Detection to `generateChunks()`

**Location**: `libs/knowledge-engine/internal/ingest/parser.go`

**Changes**:
1. Before splitting by paragraphs, detect markdown tables
2. Extract tables separately from prose content
3. Process tables row-by-row
4. Process remaining prose with existing paragraph chunking

**New Function**: `extractTablesFromContent(content string) (tables []Table, prose string)`
- Uses regex to identify table boundaries
- Extracts complete tables with headers
- Returns cleaned prose (tables removed)

**New Function**: `generateRowChunksFromTable(table Table, tableID string) []ParsedChunk`
- Iterates through table rows
- Creates one chunk per row
- Links to ParsedSpec if available
- Includes table context metadata

#### Step 1.2: Link Chunks to ParsedSpec Records

**Approach**: 
- After `parseSpecTables()` creates `ParsedSpec` objects
- Store mapping: `sourceLine -> ParsedSpec`
- When creating row chunks, look up by line number
- Store `SpecID` in chunk metadata

**Modification to `Parse()` method**:
```go
// After parseSpecTables(), create lookup map
specByLine := make(map[int]*ParsedSpec)
for i := range parsed.SpecValues {
    specByLine[parsed.SpecValues[i].SourceLine] = &parsed.SpecValues[i]
}

// Pass to generateChunks()
parsed.RawChunks = p.generateChunks(remaining, specByLine)
```

#### Step 1.3: Handle All Table Formats (3, 4, 5-column)

**Current**: `parseSpecTables()` already handles all formats

**Enhancement**: `generateRowChunksFromTable()` should:
- Detect column count from table structure
- Handle missing columns gracefully
- Preserve all available data in structured format

**Table Detection Logic**:
```go
// Detect table boundaries
tableStartRe := regexp.MustCompile(`^\|.*\|.*\|`)
tableSeparatorRe := regexp.MustCompile(`^[\|\s\-:]+$`)

// For each table:
// 1. Identify header row
// 2. Identify separator row (|---|---|)
// 3. Extract all data rows
// 4. Determine column count from header
```

---

### Phase 2: Chunk Metadata Enhancement

#### Step 2.1: Enhanced Metadata Structure

**For Spec Row Chunks**:
```json
{
  "chunk_subtype": "spec_row",
  "table_context": {
    "table_id": "table_1_page_5",
    "row_index": 42,
    "column_count": 5,
    "has_header": true
  },
  "category": "Exterior",
  "specification": "Color",
  "value": "Pearl Metallic Gallant Red",
  "unit": "",
  "variant_availability": "Standard",
  "spec_id": "uuid-of-parsed-spec",
  "document_source": "doc-source-id",
  "page_number": 5,
  "line_number": 791
}
```

**For Paragraph Chunks**:
```json
{
  "chunk_subtype": "paragraph",
  "paragraph_index": 3,
  "document_source": "doc-source-id"
}
```

#### Step 2.2: Update KnowledgeChunk Model

**File**: `libs/knowledge-engine/internal/storage/models.go`

**Check current structure**:
- Verify `Metadata` field supports JSON
- Ensure it can store nested structures
- Add validation if needed

**Note**: Current `KnowledgeChunk` likely already has `Metadata json.RawMessage`, which should work.

---

### Phase 3: Pipeline Integration

#### Step 3.1: Update `storeChunks()` Method

**File**: `libs/knowledge-engine/internal/ingest/pipeline.go`

**Changes**:
1. Handle new chunk metadata structure
2. Extract `SpecID` from chunk metadata
3. Store link to ParsedSpec in metadata
4. Batch process embeddings efficiently

**Embedding Batch Optimization**:
- Current: Batch all chunks together
- Enhanced: Process in batches of 50-100 to handle 100s of rows
- Ensure no chunk is skipped due to batch limits

**Code Changes**:
```go
// In storeChunks(), batch embeddings
batchSize := 50
for i := 0; i < len(chunks); i += batchSize {
    end := min(i+batchSize, len(chunks))
    batch := chunks[i:end]
    // Generate embeddings for batch
    // Store chunks with embeddings
}
```

#### Step 3.2: Preserve Spec ID Linking

**Approach**:
1. After `storeSpecs()`, create mapping: `lineNumber -> specItemID`
2. Pass mapping to `storeChunks()`
3. When creating `KnowledgeChunk`, look up spec ID by line number
4. Store in metadata for later retrieval

**Alternative**: Store spec_id directly in ParsedChunk during parsing phase.

---

### Phase 4: Query & Display Enhancements

#### Step 4.1: Group Results by Category

**File**: `libs/knowledge-engine/cmd/knowledge-engine-cli/main.go`

**Current**: Displays chunks sequentially

**Enhanced**: Group row chunks by category before display

**Display Logic**:
```go
// After retrieving chunks, group by category
categoryGroups := make(map[string][]retrieval.SemanticChunk)

for _, chunk := range resp.SemanticChunks {
    // Extract category from metadata
    category := extractCategory(chunk.Metadata)
    categoryGroups[category] = append(categoryGroups[category], chunk)
}

// Display grouped
for category, chunks := range categoryGroups {
    fmt.Printf("\n%s:\n", category)
    for _, chunk := range chunks {
        // Display row
    }
}
```

#### Step 4.2: Enhanced Filtering for Row Chunks

**For color queries**:
- Filter chunks where `metadata.category` contains "Exterior" 
- AND `metadata.specification` contains "Color"
- Display grouped by category

**Implementation**:
- Update re-ranking logic to check metadata
- Use structured metadata for filtering instead of text parsing
- More efficient and accurate

---

### Phase 5: Structured Text Format

#### Step 5.1: Create Structured Text Generator

**New Function**: `formatRowAsStructuredText(row TableRow, spec *ParsedSpec) string`

**Output Format**:
```
Category: {category}
Specification: {specification}
Value: {value}
{Key Features: {keyFeatures}  // Only for 5-column}
{Variant Availability: {variant}  // Only for 5-column}
{Unit: {unit}  // Only if present}
```

**Example Output**:
```
Category: Exterior
Specification: Color
Value: Pearl Metallic Gallant Red
Key Features: Standard
Variant Availability: Standard
```

**Benefits**:
- Better for semantic search
- Easier for LLM to parse
- Can reconstruct table format when needed

---

## Database Schema Considerations

### Current Schema Check

**Table**: `knowledge_chunks`
- Verify `metadata` column can store JSON with nested structures
- Verify `text` column can handle structured format
- Check indexing on metadata fields (if needed)

**Potential Enhancement**:
- Add computed columns or indexes on `metadata->>'category'` for faster filtering
- Consider JSONB for PostgreSQL (future migration)

---

## Performance Considerations

### Embedding Generation

**Current**: Batch all chunks at once
- Works for ~30 chunks
- May fail with 100s of chunks

**Enhanced**: Batch processing
```go
const EmbeddingBatchSize = 50

for i := 0; i < len(chunks); i += EmbeddingBatchSize {
    end := min(i+EmbeddingBatchSize, len(chunks))
    batch := chunks[i:end]
    
    texts := make([]string, len(batch))
    for j, chunk := range batch {
        texts[j] = chunk.Text
    }
    
    embeddings, err := p.embedder.Embed(ctx, texts)
    // Store with chunks
}
```

### Vector Store Insertion

**Current**: Batch insert all vectors
- Should handle 100s of vectors
- Verify FAISS adapter batch insert capacity

**Check**: `vector_adapter.go` Insert method handles batch size

---

## Migration Strategy

### Re-ingestion Process

**Step 1**: Backup existing data (if needed)
```bash
# Export existing chunks for reference
sqlite3 /tmp/knowledge-engine.db ".dump knowledge_chunks" > chunks_backup.sql
```

**Step 2**: Clear existing chunks
```sql
DELETE FROM knowledge_chunks WHERE tenant_id = ? AND product_id = ?;
```

**Step 3**: Re-run ingestion with new chunking logic
```bash
go run ./cmd/knowledge-engine-cli ingest \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --product wagon-r-2025 \
  --markdown /tmp/arena-extraction/arena-wagon-specs.md
```

**Step 4**: Verify new chunk structure
```sql
SELECT 
    id, 
    LENGTH(text) as text_length,
    json_extract(metadata, '$.chunk_subtype') as subtype,
    json_extract(metadata, '$.category') as category
FROM knowledge_chunks 
LIMIT 10;
```

---

## Testing Strategy

### Unit Tests

1. **Table Detection**:
   - Test detection of 3, 4, 5-column tables
   - Test mixed content (tables + prose)
   - Test edge cases (no headers, merged cells)

2. **Row Chunk Generation**:
   - Test one chunk per row
   - Test metadata preservation
   - Test spec ID linking

3. **Structured Text Format**:
   - Test format for all column counts
   - Test missing fields handling
   - Test special characters

### Integration Tests

1. **End-to-End Ingestion**:
   - Ingest document with 100+ rows
   - Verify all rows create chunks
   - Verify embeddings generated for all

2. **Query Tests**:
   - Query for colors → verify only color rows returned
   - Query for fuel efficiency → verify grouped by category
   - Verify no duplicate chunks

---

## File Changes Summary

### Files to Modify:

1. **`libs/knowledge-engine/internal/ingest/parser.go`**
   - Add table detection logic
   - Add row-level chunk generation
   - Link chunks to ParsedSpec records

2. **`libs/knowledge-engine/internal/ingest/pipeline.go`**
   - Update `storeChunks()` for batch processing
   - Handle spec ID linking
   - Enhance metadata storage

3. **`libs/knowledge-engine/cmd/knowledge-engine-cli/main.go`**
   - Update display logic to group by category
   - Enhance filtering using metadata
   - Improve color query handling

### Files to Review:

1. **`libs/knowledge-engine/internal/storage/models.go`**
   - Verify KnowledgeChunk structure supports metadata

2. **`libs/knowledge-engine/internal/retrieval/vector_adapter.go`**
   - Verify batch insert capacity

---

## Implementation Order

### Priority 1: Core Functionality
1. ✅ Table detection in parser
2. ✅ Row-level chunk generation
3. ✅ Structured text format
4. ✅ Link to ParsedSpec records

### Priority 2: Integration
5. ✅ Batch embedding processing
6. ✅ Enhanced metadata storage
7. ✅ Pipeline integration

### Priority 3: Query Enhancement
8. ✅ Category-based grouping
9. ✅ Metadata-based filtering
10. ✅ Display improvements

---

## Success Criteria

1. ✅ Each table row creates exactly one chunk
2. ✅ All rows in document create chunks (no missing rows)
3. ✅ Chunks link to ParsedSpec records via metadata
4. ✅ Embeddings generated for all chunks
5. ✅ Color queries return only color rows
6. ✅ Results grouped by category when displayed
7. ✅ Handles 100+ rows per document efficiently
8. ✅ No performance degradation

---

## Open Questions

1. **Table Headers**: Include header row as context in each chunk, or skip?
   - **Recommendation**: Skip headers, but preserve column names in metadata

2. **Merged Cells**: How to handle continuation rows (empty category)?
   - **Recommendation**: Use last non-empty category from previous row

3. **Prose Content**: Keep paragraph chunking or also convert to rows?
   - **Recommendation**: Keep paragraph chunking for non-table content (hybrid approach)

4. **Chunk Type**: Use new ChunkType or metadata flag?
   - **Recommendation**: Use metadata flag (`chunk_subtype`) for flexibility

---

## Next Steps

1. Review and approve this plan
2. Start with Phase 1 (Parser Modifications)
3. Test incrementally after each phase
4. Document any deviations from plan
5. Update this document with implementation notes

---

## Notes

- Re-ingestion required: Users must re-run ingestion after update
- No backward compatibility: Old chunk format will not work
- Performance tested with 100+ row documents
- Full coverage: Every row creates a chunk, no exceptions

