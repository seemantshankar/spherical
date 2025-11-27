# Tasks: Document Categorization Feature (FR-016)

**Input**: Design documents from `/specs/001-pdf-spec-extractor/`  
**Prerequisites**: plan.md, spec.md (FR-016)  
**Related Milestone**: M4 – Document Categorization (2025-12-10)

**Note**: This task list covers FR-016 (Document Categorization), FR-017 (4-Column Table), and FR-018 (Redundant Data Removal). All other functional requirements (FR-001 through FR-015) have already been implemented.

**Organization**: Tasks are organized by implementation phase to enable systematic development and testing.

## Development Workflow Requirements

**TDD Workflow (Constitution Principle I - NON-NEGOTIABLE)**:

- **MUST**: All development follows strict TDD methodology
- **Workflow**: Write tests first → Review and approve tests → Verify tests fail → Implement functionality → Verify tests pass → Refactor
- For test tasks (T608, T612, T616, T620, T624): Write the test implementation BEFORE starting corresponding implementation tasks
- Example: For T608 (unit tests), write tests that verify categorization detection behavior, ensure they fail, THEN implement T605-T607

**Integration Testing (Constitution Principle IV - NON-NEGOTIABLE)**:

- **MUST**: Integration tests use real dependencies, NOT mocks
- Integration test tasks (T612, T616, T620, T624) MUST use:
  - Real PDF converter (`pdf.NewConverter()`)
  - Real LLM client (`llm.NewClient()` with actual API key)
  - Real extraction service (`extract.NewService()`)
  - Real file system operations
- **Rationale**: Mocks hide integration issues; real tests catch configuration errors, compatibility issues, and environmental problems
- Reference existing integration tests in `tests/integration/` for patterns (e.g., `pdf_test.go` uses real dependencies)

## Implementation Notes

**FR-004 Heuristics**: The content filtering heuristics for "non-relevant information" are already implemented in the LLM prompt builder (`internal/llm/client.go:buildPrompt()`). The prompt explicitly defines:

- WHAT TO INCLUDE: Technical specifications, performance data, features, variant information, safety features, colors, trim levels
- WHAT TO EXCLUDE: Contact info, company names, branding, legal disclaimers, pricing, dealer info, copyright, terms, footnotes

**FR-016 Confidence Threshold**: The >70% confidence threshold for categorization fields will be implemented as part of T607. The measurement methodology should be documented in code comments and may use LLM response confidence scores or validation heuristics.

## Format: `[ID] [P?] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- Include exact file paths in descriptions

---

## Phase 1: Data Model & Domain Updates

**Purpose**: Define data structures for document categorization metadata

- [x] T601 [P] Add `DocumentMetadata` struct to `internal/domain/models.go` with fields: Domain, Subdomain, CountryCode, ModelYear, Condition, Make, Model
- [x] T602 [P] Update `ExtractionResult` struct in `internal/domain/models.go` to include `Metadata *DocumentMetadata` field
- [x] T603 [P] Update `EventComplete` payload type to include `DocumentMetadata` in `internal/domain/models.go`
- [x] T604 [P] Add validation helper functions for categorization fields in `internal/domain/models.go`: validate ISO 3166-1 alpha-2 country codes, validate model years in range 1900-2100, validate domains against predefined list, validate subdomain and condition formats

---

## Phase 2: LLM Categorization Detection

**Purpose**: Implement LLM-based categorization analysis

- [x] T605 Create categorization prompt builder function in `internal/llm/client.go` that generates prompt for detecting Domain, Subdomain, Country Code, Model Year, Condition, Make, Model from document images
- [x] T606 Implement `DetectCategorization` method in `internal/llm/client.go` that analyzes cover page (fallback to first page if cover blank/unreadable) and returns `DocumentMetadata` struct
- [x] T607 Add confidence threshold logic (>70%) for categorization fields using LLM response confidence scores (with heuristic fallback if model doesn't provide scores); mark as "Unknown" if below threshold
- [x] T608 [P] **TDD**: Write unit tests for categorization detection in `internal/llm/client_test.go` FIRST (before T605-T607). Tests should verify prompt generation, response parsing, and confidence threshold logic. Use mocks for HTTP client only (LLM API calls); test logic with real structs and validation functions.

---

## Phase 3: Integration with Extraction Service

**Purpose**: Integrate categorization detection into the extraction pipeline

- [x] T609 Update `internal/extract/service.go` to call categorization detection after PDF conversion (analyze cover page, fallback to first page if cover blank/unreadable)
- [x] T610 Modify `Process` method in `internal/extract/service.go` to store `DocumentMetadata` and include in `EventComplete` payload
- [x] T611 Update `extractPage` method flow to implement majority vote conflict resolution across pages when categorization conflicts are detected
- [x] T612 [P] **TDD + Integration**: Write integration tests in `tests/integration/pdf_test.go` FIRST (before T609-T611). Tests MUST use real dependencies (real PDF converter, real LLM client, real extraction service) per Constitution Principle IV. Verify categorization metadata is extracted and included in `EventComplete` events. Reference existing `TestPDFToMarkdownConversion` pattern.

---

## Phase 4: Markdown Output Formatting

**Purpose**: Inject categorization header at top of Markdown output

- [x] T613 Create `formatCategorizationHeader` function in `internal/extract/service.go` that generates YAML frontmatter (between `---` delimiters) with all categorization fields
- [x] T614 Update markdown aggregation logic in `internal/extract/service.go` to prepend categorization header before page content
- [x] T615 Ensure header format includes: Domain, Subdomain, Country Code, Model Year, Condition, Make, Model in machine-readable format
- [x] T616 [P] **TDD + Integration**: Write integration tests in `tests/integration/pdf_test.go` FIRST (before T613-T615). Tests MUST use real dependencies per Constitution Principle IV. Verify categorization header format (YAML frontmatter between `---` delimiters), placement at top of output, and all required fields present.

---

## Phase 5: CLI & Library API Updates

**Purpose**: Expose categorization metadata through CLI and library interfaces

- [x] T617 Update `pkg/extractor/extractor.go` to ensure `EventComplete` includes categorization metadata in payload
- [x] T618 Update `cmd/pdf-extractor/main.go` to include categorization metadata in `--summary-json` output
- [x] T619 Verify CLI Markdown output file includes categorization header at top
- [x] T620 [P] **TDD + Integration**: Write CLI integration test FIRST (before T617-T619). Test MUST use real CLI executable and real file system operations per Constitution Principle IV. Verify categorization metadata appears in `--summary-json` output and Markdown file header.

---

## Phase 6: Edge Cases & Error Handling

**Purpose**: Handle edge cases and validation

- [x] T621 Implement fallback behavior when categorization detection fails (mark all fields as "Unknown")
- [x] T622 Add logging for categorization detection confidence scores and any fields marked as "Unknown"
- [x] T623 Handle case where cover page and first page are blank or unreadable (continue to subsequent pages sequentially until a page with clear categorization information is found)
- [x] T624 [P] **TDD + Integration**: Write edge case integration tests FIRST (before T621-T623). Tests MUST use real dependencies per Constitution Principle IV. Test scenarios: missing categorization fields, low confidence (<70%), conflicting information across pages, blank/unreadable cover and first pages (sequential fallback to subsequent pages), complete detection failure. Verify fallback behavior (sequential page search, mark as "Unknown" if no clear page found) and logging of confidence scores.

---

## Phase 7: Documentation & Validation

**Purpose**: Update documentation and validate against requirements

- [x] T625 Update README.md to document categorization feature and output format
- [x] T626 Create example output showing categorization header format
- [x] T627 Validate against FR-016 requirements checklist
- [x] T628 [P] Run acceptance scenario F (Document Categorization) from spec.md and verify all fields correctly extracted

---

## Phase 8: 4-Column Table Structure (FR-017)

**Purpose**: Implement 4-column table extraction with "Key Features" column

- [x] T801 Update prompt builder in `internal/llm/client.go` to request 4-column table format (`| Category | Specification | Value | Key Features |`)
- [x] T802 Update markdown parsing logic in `internal/extract/service.go` to handle 4-column tables
- [x] T803 [P] **TDD**: Write unit tests for 4-column table parsing in `internal/extract/service_test.go` FIRST
- [x] T804 Verify "Key Features" are correctly associated with specifications in output

---

## Phase 9: Redundant Data Removal (FR-018)

**Purpose**: Remove redundant specification entries across pages

- [x] T901 Implement `DeduplicateSpecs` function in `internal/extract/service.go` to filter out duplicate specs (Category + Specification + Value match)
- [x] T902 Update `internal/extract/service.go` to apply deduplication during aggregation
- [x] T903 [P] **TDD**: Write unit tests for deduplication logic in `internal/extract/service_test.go` FIRST
- [x] T904 Verify redundant dimension specs are removed in multi-page documents

---

## Checkpoint: Feature Complete

**Exit Criteria**:

- All categorization fields (Domain, Subdomain, Country Code, Model Year, Condition, Make, Model) detected and included in Markdown header
- Categorization metadata available in `EventComplete` payload
- Categorization metadata included in `--summary-json` output
- Edge cases handled (Unknown values, low confidence, detection failures)
- Tests passing for categorization detection and output formatting
- Documentation updated

**Validation**: Run test suite with diverse document types (automobile, real estate, luxury goods) and verify categorization accuracy meets FR-016 requirements.
