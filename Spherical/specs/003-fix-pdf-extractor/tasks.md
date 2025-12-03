# Tasks: Fix PDF Extractor Output Quality

**Input**: Design documents from `/specs/003-fix-pdf-extractor/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md

**Tests**: Tests are REQUIRED per Constitution Principle I (TDD). All tests must be written first and verified to fail before implementation.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3, US4)
- Include exact file paths in descriptions

## Path Conventions

- **Single project**: `libs/pdf-extractor/` at repository root
- Paths shown use actual project structure from plan.md

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Test structure and preparation for TDD workflow

- [x] T001 Create test file structure for new functionality in libs/pdf-extractor/tests/integration/
- [x] T002 [P] Create unit test file for post-processing in libs/pdf-extractor/internal/extract/
- [x] T003 [P] Prepare test data: Create sample markdown files with codeblocks for testing in libs/pdf-extractor/testdata/

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

**Note**: This feature modifies existing code, so foundational tasks are minimal. The existing extraction service and LLM client infrastructure is already in place.

- [x] T004 Review existing test suite to understand current test patterns in libs/pdf-extractor/tests/integration/pdf_test.go

**Checkpoint**: Foundation ready - user story implementation can now begin in parallel

---

## Phase 3: User Story 1 - Clean Markdown Output Without Codeblocks (Priority: P1) 🎯 MVP

**Goal**: Remove markdown codeblock delimiters from LLM output to ensure clean markdown that can be ingested by the knowledge engine without manual cleanup.

**Independent Test**: Extract a PDF that previously produced codeblock-wrapped output and verify the resulting markdown file contains no codeblock delimiters (```) and is directly parseable by the ingestion engine.

### Tests for User Story 1 ⚠️

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [x] T005 [P] [US1] Write unit test for cleanCodeblocks function with codeblock-wrapped markdown in libs/pdf-extractor/internal/extract/service_test.go
- [x] T006 [P] [US1] Write unit test for cleanCodeblocks with empty codeblocks in libs/pdf-extractor/internal/extract/service_test.go
- [x] T007 [P] [US1] Write unit test for cleanCodeblocks with nested codeblocks in libs/pdf-extractor/internal/extract/service_test.go
- [x] T008 [P] [US1] Write integration test for PDF extraction without codeblocks in output in libs/pdf-extractor/tests/integration/pdf_test.go

### Implementation for User Story 1

- [x] T009 [US1] Implement cleanCodeblocks function with regex pattern in libs/pdf-extractor/internal/extract/service.go
- [x] T010 [US1] Integrate cleanCodeblocks call in extractPage method after LLM extraction, before deduplication in libs/pdf-extractor/internal/extract/service.go
- [x] T011 [US1] Update LLM prompt to explicitly instruct no codeblock delimiters in libs/pdf-extractor/internal/llm/client.go buildPrompt() function
- [x] T012 [US1] Add edge case handling for codeblocks in table cells in libs/pdf-extractor/internal/extract/service.go

**Checkpoint**: At this point, User Story 1 should be fully functional and testable independently. All extracted markdown files should contain zero codeblock delimiters.

---

## Phase 4: User Story 2 - Standard Hierarchical Nomenclature (Priority: P1)

**Goal**: Implement consistent, standard hierarchical nomenclature for specification categories (e.g., Interior > Seats > Upholstery) rather than ad-hoc terms created by the LLM.

**Independent Test**: Extract specifications from multiple brochures and verify that similar features use consistent hierarchical category paths (e.g., all seat-related specs use "Interior > Seats" as the category prefix).

### Tests for User Story 2 ⚠️

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [x] T013 [P] [US2] Write integration test for standard nomenclature mapping in libs/pdf-extractor/tests/integration/pdf_test.go
- [x] T014 [P] [US2] Write test with brochure using non-standard section names to verify semantic mapping in libs/pdf-extractor/tests/integration/pdf_test.go

### Implementation for User Story 2

- [x] T015 [US2] Add standard hierarchical nomenclature guide to LLM prompt with examples in libs/pdf-extractor/internal/llm/client.go buildPrompt() function
- [x] T016 [US2] Add variable depth hierarchy instructions (2-4 levels based on semantic meaning) to prompt in libs/pdf-extractor/internal/llm/client.go buildPrompt() function
- [x] T017 [US2] Add semantic mapping instructions to prompt (map brochure terms to standard categories) in libs/pdf-extractor/internal/llm/client.go buildPrompt() function
- [x] T017a [US2] Add mapping examples to prompt (e.g., "Cabin Experience" → "Interior > Comfort") in libs/pdf-extractor/internal/llm/client.go buildPrompt() function

**Checkpoint**: At this point, User Stories 1 AND 2 should both work independently. Specifications should use standard hierarchical categories.

---

## Phase 5: User Story 4 - Capture Variant Differentiation in Specification Tables (Priority: P1)

**Goal**: Capture detailed information about which features are available in which variants, especially when presented in table format with checkboxes or symbols indicating variant availability.

**Independent Test**: Process specification tables with checkbox/symbol indicators (like Skoda Kodiaq pages 25-28) and verify that the output clearly shows which variants have which features.

### Tests for User Story 4 ⚠️

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [x] T018 [P] [US4] Write integration test for variant table extraction with checkboxes in libs/pdf-extractor/tests/integration/pdf_test.go
- [x] T019 [P] [US4] Write integration test for variant availability parsing (✓, ✗, ●, ○ symbols) in libs/pdf-extractor/tests/integration/pdf_test.go
- [x] T020 [P] [US4] Write test for multi-page variant tables maintaining context in libs/pdf-extractor/tests/integration/pdf_test.go

### Implementation for User Story 4

- [x] T021 [US4] Add 5-column table format specification to prompt (Category | Specification | Value | Key Features | Variant Availability) in libs/pdf-extractor/internal/llm/client.go buildPrompt() function
- [x] T022 [US4] Add checkbox/symbol parsing instructions to LLM prompt in libs/pdf-extractor/internal/llm/client.go buildPrompt() function
- [x] T023 [US4] Add variant availability format instructions to prompt (e.g., "Lounge: ✓, Sportline: ✓, Selection L&K: ✗") in libs/pdf-extractor/internal/llm/client.go buildPrompt() function
- [x] T024 [US4] Add instructions for "Standard" (single word) notation when feature available in all variants in libs/pdf-extractor/internal/llm/client.go buildPrompt() function
- [x] T025 [US4] Add instructions for "Exclusive to: [Variant]" notation for exclusive features in libs/pdf-extractor/internal/llm/client.go buildPrompt() function
- [x] T026 [US4] Add instructions for "Unknown" notation when variant boundaries are ambiguous in libs/pdf-extractor/internal/llm/client.go buildPrompt() function

**Checkpoint**: At this point, User Stories 1, 2, AND 4 should all work independently. Variant differentiation should be captured in specification tables.

---

## Phase 6: User Story 3 - Extract Variant and Trim Information (Priority: P2)

**Goal**: Extract variant and trim names from specification tables and feature descriptions, and associate them with their respective features.

**Independent Test**: Process a brochure with variant specification tables (like the Skoda Kodiaq pages 25-28) and verify that all variant names (Lounge, Sportline, Selection L&K) are extracted and associated with their respective features.

### Tests for User Story 3 ⚠️

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [x] T027 [P] [US3] Write integration test for variant name extraction from table headers in libs/pdf-extractor/tests/integration/pdf_test.go
- [x] T028 [P] [US3] Write integration test for variant-exclusive feature tagging from text mentions in libs/pdf-extractor/tests/integration/pdf_test.go
- [x] T029 [P] [US3] Write test for single trim models (no variant information) in libs/pdf-extractor/tests/integration/pdf_test.go

### Implementation for User Story 3

- [x] T030 [US3] Add variant extraction instructions to LLM prompt (identify from table headers and text mentions) in libs/pdf-extractor/internal/llm/client.go buildPrompt() function
- [x] T031 [US3] Add instructions for extracting variant names from table column headers in libs/pdf-extractor/internal/llm/client.go buildPrompt() function
- [x] T032 [US3] Add instructions for identifying variant-exclusive features from text (e.g., "Exclusive to L&K") in libs/pdf-extractor/internal/llm/client.go buildPrompt() function
- [x] T033 [US3] Add instructions for including variant information in Variant Availability column when feature differs between variants in libs/pdf-extractor/internal/llm/client.go buildPrompt() function

**Checkpoint**: All user stories should now be independently functional. Variant information should be extracted and associated with features.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories, documentation, and validation

- [x] T034 [P] Update README.md with new 5-column output format documentation in libs/pdf-extractor/README.md
- [x] T035 [P] Add inline code comments documenting new prompt structure (5-column format, variable depth hierarchies) in libs/pdf-extractor/internal/llm/client.go
- [x] T036 [P] Add inline code comments documenting post-processing behavior in libs/pdf-extractor/internal/extract/service.go
- [x] T037 Run all existing tests to ensure no regressions: `cd libs/pdf-extractor && go test ./...` (Unit tests pass; integration tests require API key and PDF files)
- [x] T038 [P] Validate quickstart.md test scenarios in libs/pdf-extractor/ (Verification checklist documented in quickstart.md)
- [x] T039 Run integration tests with real PDFs (including Skoda Kodiaq if available) to verify all features work together (Requires OPENROUTER_API_KEY and test PDFs) - Completed with Arena Wagon R brochure: All new tests pass (TestPDFExtractionWithoutCodeblocks, TestStandardNomenclatureMapping, TestSemanticMappingNonStandardSections, TestVariantTableExtractionWithCheckboxes, TestVariantAvailabilitySymbolParsing, TestMultiPageVariantTables)
- [x] T040 Verify ingestion engine compatibility by testing extracted markdown with knowledge engine parser (verify 5-column format is accepted) - Updated parser to support 5-column format with backward compatibility

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - minimal for this feature
- **User Stories (Phase 3+)**: All depend on Foundational phase completion
  - User stories can proceed in priority order (P1 → P1 → P1 → P2)
  - US1, US2, and US4 are all P1 and can be done in parallel after foundational
  - US3 (P2) should come after P1 stories
- **Polish (Final Phase)**: Depends on all desired user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational (Phase 2) - No dependencies on other stories
- **User Story 2 (P1)**: Can start after Foundational (Phase 2) - Independent of other stories, but shares prompt file with US1, US3, US4
- **User Story 4 (P1)**: Can start after Foundational (Phase 2) - Independent of other stories, but shares prompt file with US1, US2, US3
- **User Story 3 (P2)**: Can start after Foundational (Phase 2) - Independent of other stories, but shares prompt file with US1, US2, US4

**Note**: All user stories modify the same prompt file (`libs/pdf-extractor/internal/llm/client.go buildPrompt()`), so they should be implemented sequentially or carefully coordinated if done in parallel.

### Within Each User Story

- Tests (REQUIRED per TDD) MUST be written and FAIL before implementation
- Prompt updates before integration testing
- Core implementation before edge cases
- Story complete before moving to next priority

### Parallel Opportunities

- All Setup tasks marked [P] can run in parallel (T002, T003)
- All test tasks within a user story marked [P] can run in parallel
- Documentation tasks in Polish phase marked [P] can run in parallel (T032, T033, T034, T036)
- User Stories 1, 2, and 4 (all P1) can be worked on in parallel if prompt updates are coordinated carefully

---

## Parallel Example: User Story 1

```bash
# Launch all tests for User Story 1 together:
Task: T005 - Unit test for cleanCodeblocks with codeblock-wrapped markdown
Task: T006 - Unit test for cleanCodeblocks with empty codeblocks
Task: T007 - Unit test for cleanCodeblocks with nested codeblocks
Task: T008 - Integration test for PDF extraction without codeblocks

# After tests are written and failing, implement:
Task: T009 - Implement cleanCodeblocks function
Task: T011 - Update LLM prompt (can be done in parallel with T009)
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (minimal for this feature)
3. Complete Phase 3: User Story 1 (Clean Markdown Output)
4. **STOP and VALIDATE**: Test User Story 1 independently with real PDFs
5. Verify ingestion engine can parse output without errors
6. Deploy/demo if ready

### Incremental Delivery

1. Complete Setup + Foundational → Foundation ready
2. Add User Story 1 → Test independently → Verify ingestion → Deploy/Demo (MVP!)
3. Add User Story 2 → Test independently → Verify nomenclature consistency → Deploy/Demo
4. Add User Story 4 → Test independently → Verify variant differentiation → Deploy/Demo
5. Add User Story 3 → Test independently → Verify variant extraction → Deploy/Demo
6. Each story adds value without breaking previous stories

### Sequential Strategy (Recommended)

Since all user stories modify the same prompt file, sequential implementation is recommended:

1. Team completes Setup + Foundational together
2. Implement User Story 1 → Test → Commit
3. Implement User Story 2 → Test → Commit
4. Implement User Story 4 → Test → Commit
5. Implement User Story 3 → Test → Commit
6. Polish phase → Final validation

**Alternative**: If multiple developers work on this, coordinate prompt updates carefully:

- Developer A: User Story 1 (codeblock removal + prompt update)
- Developer B: User Story 2 (nomenclature + prompt update) - merge after US1
- Developer C: User Story 4 (variant differentiation + prompt update) - merge after US2
- Developer D: User Story 3 (variant extraction + prompt update) - merge after US4

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- **TDD REQUIRED**: Verify tests fail before implementing (Constitution Principle I)
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- All user stories modify the same prompt file - coordinate carefully if working in parallel
- Integration tests use real PDFs and real OpenRouter API (no mocks per Constitution Principle IV)
- Post-processing function must be idempotent (safe to run multiple times)
