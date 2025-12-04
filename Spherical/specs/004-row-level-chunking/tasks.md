# Implementation Tasks: Row-Level Chunking for Table Data

**Feature**: `004-row-level-chunking`  
**Branch**: `004-row-level-chunking`  
**Date**: 2025-12-04  
**Status**: Ready for Implementation

## Summary

This document breaks down the row-level chunking feature into actionable, dependency-ordered tasks organized by user story priority. Each user story phase is independently testable and can be implemented incrementally.

**Total Tasks**: 42  
**User Story 1 (P1)**: 12 tasks  
**User Story 2 (P2)**: 8 tasks  
**User Story 3 (P3)**: 6 tasks  
**Setup & Foundational**: 12 tasks  
**Polish & Cross-Cutting**: 4 tasks

## Implementation Strategy

### MVP Scope
Start with **User Story 1** (P1) to deliver core value: precise row-level retrieval with hierarchical grouping. This provides immediate user benefit and validates the approach.

### Incremental Delivery
1. **Phase 1-2**: Setup and foundational infrastructure
2. **Phase 3**: User Story 1 (MVP) - Row-level chunking and query grouping
3. **Phase 4**: User Story 2 - Batch processing and error handling
4. **Phase 5**: User Story 3 - Metadata and linkage
5. **Phase 6**: Polish - Integration tests and documentation

### Parallel Opportunities
- Database migration and model updates can be done in parallel
- Parser changes and content hash utility are independent
- Retrieval grouping can be developed alongside parser changes
- Integration tests can be written in parallel with implementation

## Dependencies

### Story Completion Order
1. **Setup & Foundational** (Phases 1-2) → Must complete before all user stories
2. **User Story 1** (Phase 3) → Independent, can be completed first (MVP)
3. **User Story 2** (Phase 4) → Depends on Phase 3 (needs row chunking from US1)
4. **User Story 3** (Phase 5) → Depends on Phase 3 (needs row chunking from US1)
5. **Polish** (Phase 6) → Depends on all user stories

### Cross-Story Dependencies
- User Story 2 and 3 both depend on User Story 1 (row chunking infrastructure)
- User Story 2 and 3 can be developed in parallel after User Story 1
- Polish phase requires all user stories to be complete

---

## Phase 1: Setup

**Goal**: Initialize database schema and model updates to support row-level chunking.

**Independent Test**: Run migration, verify new columns exist, verify model structs compile.

### Tasks

- [x] T001 Create database migration file `libs/knowledge-engine/db/migrations/0002_add_row_chunking_fields.sql` with content_hash and completion_status columns
- [x] T002 [P] Add content_hash VARCHAR(64) column to knowledge_chunks table in migration file `libs/knowledge-engine/db/migrations/0002_add_row_chunking_fields.sql`
- [x] T003 [P] Add completion_status VARCHAR(20) column with CHECK constraint in migration file `libs/knowledge-engine/db/migrations/0002_add_row_chunking_fields.sql`
- [x] T004 [P] Create unique index on content_hash in migration file `libs/knowledge-engine/db/migrations/0002_add_row_chunking_fields.sql`
- [x] T005 [P] Create partial index on completion_status in migration file `libs/knowledge-engine/db/migrations/0002_add_row_chunking_fields.sql`
- [x] T006 Update KnowledgeChunk struct in `libs/knowledge-engine/internal/storage/models.go` to include ContentHash and CompletionStatus fields

---

## Phase 2: Foundational

**Goal**: Implement core utilities and extend parser to detect and extract table rows.

**Independent Test**: Unit test content hash generation, verify table detection in parser.

### Tasks

- [x] T007 Create content hash utility function `computeContentHash` in `libs/knowledge-engine/internal/ingest/parser.go` that generates SHA-256 hash of normalized structured text
- [x] T008 [P] Add function `formatRowChunkText` in `libs/knowledge-engine/internal/ingest/parser.go` that formats table row as structured text (key-value pairs)
- [x] T009 [P] Extend `parseSpecTables` function in `libs/knowledge-engine/internal/ingest/parser.go` to extract parent_category and sub_category from columns 1 and 2
- [x] T010 [P] Add function `extractTableRowMetadata` in `libs/knowledge-engine/internal/ingest/parser.go` that extracts all 5 columns and builds metadata JSON structure
- [x] T011 Add function `generateRowChunks` in `libs/knowledge-engine/internal/ingest/parser.go` that converts table rows to ParsedChunk with chunk_type='spec_row'
- [x] T012 Update `Parse` function in `libs/knowledge-engine/internal/ingest/parser.go` to call generateRowChunks for tables and preserve existing paragraph chunking for non-table content

---

## Phase 3: User Story 1 - Query Specific Table Row Information (P1)

**Goal**: Enable precise retrieval of individual table rows grouped hierarchically by category.

**Independent Test**: Ingest document with table, query for specific attribute, verify only relevant rows returned grouped by category.

**Acceptance Criteria**:
- Each table row = 1 chunk
- Queries return only relevant rows (≥90% precision)
- Results grouped hierarchically (parent category, then sub-category)
- Query response time within budget (p50 ≤150 ms, p95 ≤350 ms)

### Tasks

- [x] T013 [US1] Modify `parseSpecTables` in `libs/knowledge-engine/internal/ingest/parser.go` to generate one ParsedChunk per table row instead of only ParsedSpec
- [x] T014 [US1] Update `generateRowChunks` in `libs/knowledge-engine/internal/ingest/parser.go` to set chunk_type to storage.ChunkTypeSpecRow for table row chunks
- [x] T015 [US1] Implement content hash computation in `generateRowChunks` in `libs/knowledge-engine/internal/ingest/parser.go` using computeContentHash utility
- [x] T016 [US1] Add metadata JSON structure to row chunks in `generateRowChunks` in `libs/knowledge-engine/internal/ingest/parser.go` with parent_category, sub_category, specification_type, value, additional_metadata
- [x] T017 [US1] Update `storeChunks` function in `libs/knowledge-engine/internal/ingest/pipeline.go` to handle row chunks with content_hash and completion_status fields
- [x] T018 [US1] Add content hash deduplication check in `storeChunks` in `libs/knowledge-engine/internal/ingest/pipeline.go` to find existing chunks by content_hash before creating new ones
- [x] T019 [US1] Implement hierarchical grouping function `groupChunksByCategory` in `libs/knowledge-engine/internal/retrieval/router.go` that groups by parent_category then sub_category
- [x] T020 [US1] Update `Retrieve` method in `libs/knowledge-engine/internal/retrieval/router.go` to apply hierarchical grouping to SemanticChunk results for chunk_type='spec_row'
- [x] T021 [US1] Add metadata filtering in `Retrieve` method in `libs/knowledge-engine/internal/retrieval/router.go` to filter results by category and specification_type from metadata JSON
- [x] T022 [US1] Update `SemanticChunk` struct in `libs/knowledge-engine/internal/retrieval/router.go` to include category metadata for grouping display
- [x] T023 [US1] Add repository method `FindByContentHash` in `libs/knowledge-engine/internal/storage/repositories.go` for deduplication lookups
- [x] T024 [US1] Update repository method `CreateChunk` in `libs/knowledge-engine/internal/storage/repositories.go` to handle content_hash and completion_status fields

---

## Phase 4: User Story 2 - Efficient Processing of Large Tables (P2)

**Goal**: Process tables with 100+ rows efficiently with batch embedding and error handling.

**Independent Test**: Ingest document with 200 table rows, verify all processed, embeddings generated in batches, completes within 10 minutes.

**Acceptance Criteria**:
- All 200 rows converted to chunks
- Embeddings generated in batches of 50-100
- Process completes within 10 minutes
- Failed embeddings don't block ingestion
- Incomplete chunks stored for retry

### Tasks

- [x] T025 [US2] Modify batch embedding logic in `storeChunks` function in `libs/knowledge-engine/internal/ingest/pipeline.go` to process chunks in batches of 50-100
- [x] T026 [US2] Add per-chunk error handling in batch embedding in `storeChunks` in `libs/knowledge-engine/internal/ingest/pipeline.go` to catch individual chunk failures
- [x] T027 [US2] Implement incomplete chunk storage in `storeChunks` in `libs/knowledge-engine/internal/ingest/pipeline.go` that sets completion_status='incomplete' when embedding fails
- [x] T028 [US2] Add error logging for failed embeddings in `storeChunks` in `libs/knowledge-engine/internal/ingest/pipeline.go` without stopping entire ingestion
- [x] T029 [US2] Update `storeChunks` in `libs/knowledge-engine/internal/ingest/pipeline.go` to continue processing other chunks when one fails embedding generation
- [x] T030 [US2] Add batch size configuration option in `PipelineConfig` struct in `libs/knowledge-engine/internal/ingest/pipeline.go` for embedding batch size (default 75)
- [x] T031 [US2] Implement fallback to individual chunk embedding in `storeChunks` in `libs/knowledge-engine/internal/ingest/pipeline.go` when batch embedding fails
- [x] T032 [US2] Add retry queue query method `FindIncompleteChunks` in `libs/knowledge-engine/internal/storage/repositories.go` to find chunks with completion_status='incomplete'

---

## Phase 5: User Story 3 - Maintain Context and Linkage (P3)

**Goal**: Ensure row chunks maintain proper linkage to ParsedSpec records and preserve metadata.

**Independent Test**: Ingest document, query for chunks, verify source document references and metadata are present and correct.

**Acceptance Criteria**:
- Chunks linked to ParsedSpec records via metadata.parsed_spec_ids
- Source document references included in results
- Metadata enables filtering by specification type
- 100% of chunks have valid source references

### Tasks

- [x] T033 [US3] Update `generateRowChunks` in `libs/knowledge-engine/internal/ingest/parser.go` to link chunks to ParsedSpec records by storing parsed_spec_id in metadata.parsed_spec_ids array
- [x] T034 [US3] Modify deduplication logic in `storeChunks` in `libs/knowledge-engine/internal/ingest/pipeline.go` to append parsed_spec_id to existing chunk's metadata.parsed_spec_ids when content_hash matches
- [x] T035 [US3] Update `Retrieve` method in `libs/knowledge-engine/internal/retrieval/router.go` to include source document references (source_doc_id, parsed_spec_ids) in SemanticChunk results
- [x] T036 [US3] Add metadata extraction helper `extractChunkMetadata` in `libs/knowledge-engine/internal/retrieval/router.go` to parse metadata JSON and extract parent_category, sub_category, specification_type
- [x] T037 [US3] Implement specification type filtering in `Retrieve` method in `libs/knowledge-engine/internal/retrieval/router.go` using metadata.specification_type field
- [x] T038 [US3] Update repository method `UpdateChunkMetadata` in `libs/knowledge-engine/internal/storage/repositories.go` to append to parsed_spec_ids array when updating existing chunks

---

## Phase 6: Polish & Cross-Cutting Concerns

**Goal**: Integration tests, documentation updates, and final validation.

**Independent Test**: Run full integration test suite, verify all acceptance criteria met, check documentation completeness.

### Tasks

- [x] T039 Create integration test file `libs/knowledge-engine/tests/integration/row_chunking_test.go` with test for ingesting document with tables and verifying row chunks created
- [x] T040 [P] Add integration test in `libs/knowledge-engine/tests/integration/row_chunking_test.go` for querying row chunks and verifying hierarchical grouping
- [x] T041 [P] Add integration test in `libs/knowledge-engine/tests/integration/row_chunking_test.go` for batch embedding with error handling and incomplete chunk storage
- [x] T042 Update README.md in `libs/knowledge-engine/README.md` with row-level chunking feature documentation and usage examples

---

## Parallel Execution Examples

### Example 1: Setup Phase Parallelization
```
T002, T003, T004, T005 can run in parallel (all modify same migration file, but different sections)
```

### Example 2: Foundational Phase Parallelization
```
T008, T009, T010 can run in parallel (different functions in parser.go)
```

### Example 3: User Story 1 Parallelization
```
T014, T015, T016 can run in parallel (all in generateRowChunks function, but different aspects)
T019, T020, T021 can run in parallel (all in router.go, different grouping/filtering aspects)
T023, T024 can run in parallel (both in repositories.go, different methods)
```

### Example 4: User Story 2 Parallelization
```
T025, T026, T027 can run in parallel (all modify storeChunks, but different error handling aspects)
T030, T031 can run in parallel (config and fallback logic)
```

### Example 5: User Story 3 Parallelization
```
T033, T034 can run in parallel (parser and pipeline changes)
T035, T036, T037 can run in parallel (all in router.go, different metadata aspects)
```

### Example 6: Polish Phase Parallelization
```
T040, T041 can run in parallel (different integration tests)
```

---

## Task Validation Checklist

- [x] All tasks follow format: `- [ ] [TaskID] [P?] [Story?] Description with file path`
- [x] All tasks have sequential Task IDs (T001-T042)
- [x] All user story tasks have [US1], [US2], or [US3] labels
- [x] All parallelizable tasks marked with [P]
- [x] All tasks include exact file paths
- [x] Tasks organized by phase and user story
- [x] Dependencies clearly documented
- [x] Independent test criteria provided for each phase
- [x] MVP scope identified (User Story 1)
- [x] Parallel execution opportunities identified

---

## Notes

- **TDD Approach**: Write tests before implementation where specified in plan
- **Backward Compatibility**: Ensure existing paragraph chunking continues to work
- **Performance**: Monitor ingestion time and query latency during implementation
- **Error Handling**: All embedding failures must be logged and tracked for retry
- **Content Hash**: Must be deterministic (same content = same hash)
- **Metadata**: JSON structure must be validated during chunk creation

