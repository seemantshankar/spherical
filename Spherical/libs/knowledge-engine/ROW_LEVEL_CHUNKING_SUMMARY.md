# Row-Level Chunking - Quick Reference

## Key Decisions

| Aspect | Decision |
|--------|----------|
| **Scope** | All table types (3, 4, 5-column) |
| **Display** | Group results by Category |
| **Backward Compatibility** | Require re-ingestion (no old format support) |
| **Performance** | Handle 100s of rows, batch embedding generation |
| **Structured Data** | Link row chunks to ParsedSpec records |

---

## Approach: Hybrid Chunking

### Table Rows → Individual Chunks
- Each table row = 1 chunk
- Structured text format for better embeddings
- Link to ParsedSpec via metadata

### Non-Table Content → Paragraph Chunks
- Keep existing paragraph-based chunking
- Maintain context for prose content

---

## Chunk Text Format

### Structured Format (Recommended):
```
Category: Exterior
Specification: Color
Value: Pearl Metallic Gallant Red
Key Features: Standard
Variant Availability: Standard
```

**Why?** Better semantic search, easier parsing, can reconstruct table format.

---

## Implementation Phases

### Phase 1: Parser Modifications
- Detect tables in markdown
- Generate row-level chunks
- Link to ParsedSpec records

### Phase 2: Metadata Enhancement
- Store table context
- Store category/spec/value
- Link to spec ID

### Phase 3: Pipeline Integration
- Batch embedding generation (50-100 at a time)
- Handle 100s of rows efficiently
- Preserve all metadata

### Phase 4: Query & Display
- Group results by category
- Filter using metadata
- Clean, organized display

---

## Files to Modify

1. **`parser.go`**: Table detection, row chunk generation
2. **`pipeline.go`**: Batch processing, spec linking
3. **`main.go`**: Category grouping, metadata filtering

---

## Migration

**Re-ingestion Required**:
```bash
# 1. Delete old chunks (if needed)
# 2. Re-run ingestion
go run ./cmd/knowledge-engine-cli ingest \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --product wagon-r-2025 \
  --markdown /path/to/markdown.md
```

---

## Success Metrics

- ✅ Each row = 1 chunk
- ✅ All rows processed (no missing)
- ✅ Links to ParsedSpec records
- ✅ Color queries return only color rows
- ✅ Results grouped by category
- ✅ Handles 100+ rows efficiently

---

## Example Output After Implementation

### Query: "What colors does this car come in?"

**Before** (current):
```
Chunk 1: [100+ row table with colors buried inside]
```

**After** (row-level):
```
Exterior:
  - Color: Solid White
  - Color: Metallic Silky Silver
  - Color: Metallic Magma Grey
  - Color: Pearl Metallic Gallant Red
  - Color: Pearl Metallic Nutmeg Brown
  - Color: Pearl Metallic Poolside Blue
  - Color: Pearl Bluish Black
  - Color (Dual Tone): Pearl Metallic Gallant Red with Pearl Bluish Black Roof
  - Color (Dual Tone): Metallic Magma Grey with Pearl Bluish Black Roof
```

Clear, concise, and exactly what was asked!

