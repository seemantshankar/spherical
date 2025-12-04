# Feature Specification: Row-Level Chunking for Table Data

**Feature Branch**: `004-row-level-chunking`  
**Created**: 2025-12-04  
**Status**: Draft  
**Input**: User description: "Implement row-level chunking for table data to improve semantic search accuracy and query results"

## Clarifications

### Session 2025-12-04

- Q: How should the system determine the "category" for each table row? → A: Extract from table columns - tables have 5 columns where the first two columns are "Parent Category" and "Sub-Category", which provide the category information for each row.
- Q: How should the remaining 3 columns map to "specification type" and "value" in the chunk format? → A: Column 3 = Specification, Column 4 = Value, Column 5 = Additional metadata (e.g., Key Features, Variant Availability).
- Q: What uniquely identifies a table row chunk? Can the same row content appear in multiple chunks? → A: Unique by row content hash only - same content equals same chunk, enabling content-level deduplication across documents.
- Q: When embedding generation fails for a row chunk, what should happen to that chunk? → A: Store chunk without embedding, mark as incomplete/retry-needed, allow later retry.
- Q: When grouping query results by category, should results be grouped by parent category only, sub-category only, or both (hierarchical)? → A: Hierarchical grouping - first by parent category, then by sub-category within each parent.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Query specific table row information (Priority: P1)

A user asks a question about a specific attribute from a product specification table (e.g., "What colors does this car come in?") and receives precise, relevant results showing only the matching rows grouped by category, rather than entire table chunks.

**Why this priority**: This is the core value proposition - enabling precise retrieval of individual table rows instead of returning large table chunks that bury the answer.

**Independent Test**: Submit a query about a specific table attribute (e.g., "car colors"), verify that only relevant table rows are returned, grouped by category, and that the results are clear and concise.

**Acceptance Scenarios**:

1. **Given** a product specification document containing a table with 100+ rows of exterior colors, **When** a user queries "What colors does this car come in?", **Then** the system returns only the color-related rows, grouped under the "Exterior" category, showing each color option clearly.
2. **Given** a specification document with multiple tables (exterior, interior, performance), **When** a user queries about a specific attribute like "fuel efficiency", **Then** the system returns only rows from the relevant table section, not rows from unrelated tables.
3. **Given** a table with mixed content (some rows about colors, some about dimensions), **When** a user queries specifically about colors, **Then** the system filters and returns only color-related rows, excluding dimension rows.

---

### User Story 2 - Efficient processing of large tables (Priority: P2)

The system processes specification documents containing tables with hundreds of rows efficiently, generating embeddings in batches and completing ingestion within acceptable time limits.

**Why this priority**: Large specification documents are common, and the system must handle them without performance degradation or timeouts.

**Independent Test**: Ingest a document with 200+ table rows, verify all rows are processed, embeddings are generated in batches, and the entire process completes successfully.

**Acceptance Scenarios**:

1. **Given** a specification document with 200 table rows, **When** the ingestion process runs, **Then** all 200 rows are converted to individual chunks, embeddings are generated in batches of 50-100, and the process completes without errors or timeouts.
2. **Given** multiple specification documents being ingested simultaneously, **When** each contains tables with 100+ rows, **Then** the system processes all documents without resource exhaustion or significant performance degradation.
3. **Given** a table row that fails embedding generation, **When** the batch processing encounters the error, **Then** the system stores the chunk without embedding, marks it as incomplete/retry-needed, logs the error, continues processing other rows, and reports the failure without stopping the entire ingestion. The incomplete chunk can be retried later to generate its embedding.

---

### User Story 3 - Maintain context and linkage (Priority: P3)

Each table row chunk maintains proper linkage to its source specification document and preserves metadata that enables accurate filtering and grouping of results.

**Why this priority**: Users need to understand where information comes from, and the system needs metadata to filter and organize results correctly.

**Independent Test**: Query for table rows, verify that results include proper source document references, category metadata, and can be filtered by specification attributes.

**Acceptance Scenarios**:

1. **Given** a table row chunk created during ingestion, **When** a user queries and receives that row in results, **Then** the result includes metadata linking back to the source ParsedSpec record and document.
2. **Given** table rows from different categories (e.g., Exterior, Interior, Performance), **When** results are returned, **Then** they are grouped hierarchically by parent category and sub-category with clear category labels.
3. **Given** table rows with different specification types (e.g., Color, Dimensions, Features), **When** a user filters results by specification type, **Then** only rows matching that type are returned.

---

### Edge Cases

- Table contains empty or malformed rows → system skips invalid rows, logs warnings, and processes valid rows without failing the entire ingestion.
- Table has fewer than 3 columns → system skips the table entirely, logs a warning that tables with <3 columns are not supported, and continues processing other content.
- Table has more than 5 columns → system processes only the first 5 columns, logs a warning that additional columns are ignored, and continues processing.
- Table row has empty or null values in category or specification columns → system uses default values ("Uncategorized" for empty parent_category, "General" for empty sub_category, "Unknown" for empty specification_type) and processes the row, logging a warning for data quality issues.
- Embedding generation fails for a row chunk → system stores chunk without embedding, marks as incomplete/retry-needed, logs error, continues processing, and allows retry later.
- Table spans multiple pages in source document → system correctly identifies table boundaries and processes all rows as a single logical table.
- Non-table content appears between table rows → system correctly distinguishes table content from prose and applies appropriate chunking strategy to each.
- Query matches multiple rows from the same table → system returns all matching rows grouped by category, avoiding duplicates based on content hash.
- Same row content appears in multiple documents or table positions → system treats them as the same chunk (identified by content hash) and links all source ParsedSpec references to that single chunk.
- Specification document contains nested tables or tables within tables → system handles the primary table structure and processes rows appropriately.
- Re-ingestion of previously processed documents → system replaces old chunks with new row-level chunks, maintaining data consistency.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST detect tables in markdown specification documents and distinguish them from prose content. Tables are identified by standard markdown table syntax: rows starting with pipe character (`|`), containing at least one pipe-separated cell, with optional header row followed by separator row (containing `---` or `===`). The system MUST detect tables with 3, 4, or 5 columns and process them accordingly.
- **FR-002**: System MUST convert each table row into an individual chunk, where one table row equals one chunk. Chunks are uniquely identified by row content hash, enabling deduplication when the same row content appears in multiple documents or positions.
- **FR-003**: System MUST format table row chunks using a structured text format that includes category (from Parent Category and Sub-Category columns), specification (from column 3), value (from column 4), and additional metadata (from column 5) as key-value pairs. The structured text format MUST follow this template:

```
Category: {parent_category}
Sub-Category: {sub_category}
Specification: {specification_type}
Value: {value}
Additional Metadata: {additional_metadata}
```

Where each field is extracted from the corresponding table column. For 3-column and 4-column tables, missing columns are omitted from the structured format. The format uses newline-separated key-value pairs with colon separators, and all values are trimmed of leading/trailing whitespace.
- **FR-004**: System MUST link each table row chunk to its source ParsedSpec record via metadata.
- **FR-005**: System MUST preserve non-table content using existing paragraph-based chunking strategy.
- **FR-006**: System MUST support all table types (3-column, 4-column, 5-column tables) without requiring different handling logic. For 5-column tables, the first two columns are Parent Category and Sub-Category, which provide category information for grouping. For tables with fewer than 3 columns, the system MUST skip the table and log a warning (tables with <3 columns are not supported). For tables with more than 5 columns, the system MUST process only the first 5 columns and log a warning that additional columns are ignored. Column mapping: Column 1 = Parent Category (if present), Column 2 = Sub-Category (if present), Column 3 = Specification, Column 4 = Value, Column 5 = Additional metadata (if present).
- **FR-007**: System MUST generate embeddings for table row chunks in batches of 50-100 chunks at a time to optimize performance. The batch size is configurable with a default of 75 chunks per batch, allowing adjustment based on embedding service capabilities and performance requirements. If embedding generation fails for a chunk, the system MUST store the chunk without embedding, mark it as incomplete/retry-needed, and allow retry later without blocking other chunks. If an entire batch fails, the system MUST fall back to processing chunks individually, storing successful chunks with embeddings and marking failed chunks as incomplete.
- **FR-008**: System MUST handle tables with 100+ rows efficiently without performance degradation or timeouts. "Efficiently" is defined as: ingestion of a table with 200 rows completes within 10 minutes (as per SC-004), and processing time scales linearly with row count (e.g., 100 rows completes within 5 minutes). "Without performance degradation" means: query response times remain within existing performance budgets (p50 ≤150 ms, p95 ≤350 ms per SC-007) regardless of table size, and ingestion throughput does not decrease by more than 10% when processing tables with 100+ rows compared to smaller tables. The system MUST support tables up to 1000 rows per table, with larger tables requiring special handling or splitting. Timeout thresholds: ingestion operations MUST not exceed 30 minutes for any single document, and individual batch embedding operations MUST not exceed 5 minutes.
- **FR-009**: System MUST store table context metadata (parent category from column 1, sub-category from column 2, specification type from column 3, value from column 4, additional metadata from column 5) with each row chunk, extracted from the table column structure.
- **FR-010**: System MUST enable query results to be grouped hierarchically by category when multiple rows match a query - first by parent category, then by sub-category within each parent category.
- **FR-011**: System MUST filter query results using metadata (category, specification type) to return only relevant rows.
- **FR-012**: System MUST process all table rows during ingestion, ensuring no rows are skipped or lost.
- **FR-013**: System MUST maintain backward compatibility by requiring re-ingestion of existing documents to benefit from row-level chunking (no automatic migration of old chunks).

### Key Entities *(include if feature involves data)*

- **Table Row Chunk**: Individual chunk representing one table row. Attributes: structured text content, content hash (unique identifier), parent category (column 1), sub-category (column 2), specification type (column 3), value (column 4), additional metadata (column 5), source ParsedSpec reference(s), metadata JSON, embedding (nullable), completion status (complete/incomplete/retry-needed). Uniqueness: determined by content hash - same row content across different documents or positions maps to the same chunk.
- **ParsedSpec**: Source specification document record. Attributes: document ID, tenant ID, product ID, extraction metadata, processing timestamp.
- **Parent Category**: Primary grouping dimension extracted from the first column of 5-column tables (e.g., Exterior, Interior, Performance). Attributes: name, display label.
- **Sub-Category**: Secondary grouping dimension extracted from the second column of 5-column tables. Attributes: name, display label.
- **Specification Type**: The type of specification in a table row (e.g., Color, Dimensions, Features). Attributes: name, data type, validation rules.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Each table row is converted to exactly one chunk (100% row-to-chunk mapping accuracy verified through ingestion validation).
- **SC-002**: Queries about specific table attributes (e.g., "car colors") return only relevant rows with ≥90% precision (relevant rows returned / total rows returned).
- **SC-003**: Query results for table-based queries are grouped hierarchically by category (parent category, then sub-category) with clear category labels, improving result readability for users. "Clear category labels" means: each category level is displayed with a distinct label format (e.g., "**Exterior**" for parent category, "  Colors:" for sub-category with indentation), labels are extracted from the metadata fields (parent_category and sub_category), and the hierarchical structure is visually apparent (parent categories are top-level headings, sub-categories are nested under their parent). Category labels MUST be non-empty strings extracted from table columns; if a category value is empty or null, the system MUST use a default label (e.g., "Uncategorized" for parent category, "General" for sub-category).
- **SC-004**: Ingestion of a document with 200 table rows completes successfully within 10 minutes, including batch embedding generation.
- **SC-005**: All table rows from ingested documents are processed and stored (0% row loss rate verified through row count validation).
- **SC-006**: Table row chunks maintain proper linkage to source ParsedSpec records (100% of chunks have valid source references verified through metadata validation).
- **SC-007**: Query response time for table-based queries remains within existing performance budgets (p50 ≤150 ms, p95 ≤350 ms) despite increased chunk count. Load conditions for these targets: single concurrent query, typical data volume (up to 10,000 chunks per product), standard hardware configuration. Under higher load (10+ concurrent queries) or larger data volumes (50,000+ chunks), performance may degrade proportionally but MUST remain within 2x the baseline targets (p50 ≤300 ms, p95 ≤700 ms).

## Assumptions

- Existing paragraph-based chunking for non-table content will continue to work as-is and does not require modification.
- Re-ingestion of existing documents is acceptable and expected to enable row-level chunking benefits.
- Table structure in source markdown documents is consistent and follows standard markdown table format. For 5-column tables: Column 1 = Parent Category, Column 2 = Sub-Category, Column 3 = Specification, Column 4 = Value, Column 5 = Additional metadata (e.g., Key Features, Variant Availability).
- Batch embedding generation (50-100 chunks per batch) provides optimal balance between performance and resource usage.
- Structured text format for row chunks (key-value pairs) provides better semantic search results than raw table row text.

## Dependencies

- Existing ingestion pipeline must support batch processing of chunks.
- Embedding service must support batch embedding generation.
- Storage system must support metadata linking between chunks and ParsedSpec records.
- Query/retrieval system must support metadata-based filtering and category grouping.

## Out of Scope

- Automatic migration of existing chunks to row-level format (re-ingestion required).
- Custom table parsing for non-standard table formats.
- Real-time chunking updates (chunking occurs during ingestion only).
- UI components for displaying grouped results (this specification focuses on data structure and retrieval capabilities).

