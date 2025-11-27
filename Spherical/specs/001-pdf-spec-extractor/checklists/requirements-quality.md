# Requirements Quality Checklist – PDF Specification Extractor

**Purpose**: Unit-test the requirements quality for the complete PDF extractor (CLI, pipeline, streaming, error handling).  
**Created**: 2025-11-23  
**Audience**: Formal reviewer / QA gate  
**Source Docs**: `spec.md`, `plan.md`, `tasks.md`

## Requirement Completeness

- [x] **CHK001** – Are storage, naming, and retention rules for the “temporary Markdown file” fully specified, including cleanup responsibilities? [Completeness, Spec §FR-006, §FR-012]
- [x] **CHK002** – Is there documented guidance for configuring or overriding the JPG quality threshold (>85%) per different source documents? [Completeness, Spec §FR-002, Plan §Technical Context]
- [x] **CHK003** – Do requirements cover how callers receive aggregated extraction outputs (library API vs CLI) beyond the Markdown artifact (e.g., return structs, events)? [Completeness, Spec §FR-011–§FR-014, Plan §Project Structure]
- [x] **CHK003A** – Are document categorization requirements (Domain, Subdomain, Country Code, Model Year, Condition, Make, Model) fully specified with detection methodology, output format, and fallback behavior for uncertain values? [Completeness, Spec §FR-016]

## Requirement Clarity

- [x] **CHK004** – Is “filter out non-relevant information” defined with explicit inclusion/exclusion heuristics (e.g., marketing text vs USP) to avoid subjective interpretation? [Clarity, Spec §FR-004–§FR-005]
- [x] **CHK005** – Are table preservation requirements precise about handling merged cells, repeated headers, and column alignment rules? [Clarity, Spec §FR-007]
- [x] **CHK006** – Is the term "temporary Markdown file" clarified to prevent conflicting expectations around persistence, auditing, or downstream reuse? [Clarity, Spec §FR-006, §FR-012]
- [x] **CHK006A** – Is the categorization header format (YAML frontmatter vs structured Markdown table) and placement (top of document, before page content) clearly specified to ensure consistent output structure? [Clarity, Spec §FR-016]

## Requirement Consistency

- [x] **CHK007** – Do sequential processing requirements (Spec §FR-015) align with the success criterion of processing 20 pages in under 2 minutes (Spec §SC-003), or is there a conflict that needs reconciliation? [Consistency]
- [x] **CHK008** – Are streaming requirements (Spec §FR-013–§FR-014, Acceptance Scenario 5, Spec §SC-005) consistent regarding latency expectations and consumer interfaces (CLI spinner vs channel events)? [Consistency]

## Scenario Coverage

- [x] **CHK009** – Are requirements defined for marketing-heavy pages or pages lacking specs so that extraction behavior (include vs skip) is unambiguous? [Coverage, Spec §US1 Scenario 3, Scenario B]
- [x] **CHK010** – Do streaming UX requirements cover both CLI output and embedders of the Go library (callbacks, channels) with equal rigor? [Coverage, Spec §FR-013–§FR-014, Plan §Project Structure]
- [x] **CHK010A** – Are categorization requirements defined for edge cases: documents with missing/ambiguous categorization fields, multi-domain documents, documents without clear country indicators, or documents with conflicting information across pages? [Coverage, Spec §FR-016, Acceptance Scenario F]

## Edge Case Coverage

- [x] **CHK011** – Is behavior defined when conversion fails partway through (partial Markdown generation, cleanup, retries), especially for multi-page PDFs? [Edge Case, Spec §FR-012, Spec §US2 Scenario 4]
- [x] **CHK012** – Are retry/backoff policies for OpenRouter (Spec §FR-010) explicit about maximum attempts, jitter, and failure reporting under persistent 429/5xx responses? [Edge Case, Spec §FR-010, Acceptance Scenario E]
- [x] **CHK012A** – Is behavior defined when categorization detection fails or returns low confidence (<70%) for one or more fields? Are "Unknown" values acceptable, and is there a mechanism to validate categorization accuracy? [Edge Case, Spec §FR-016]

## Acceptance Criteria Quality

- [x] **CHK013** – Are SC-001/SC-002 accompanied by objective validation methods (sampling percentage, reviewer workflow) so the “95% specs captured / 100% tables preserved” goals can be measured? [Acceptance Criteria, Spec §SC-001–§SC-002, Plan §QA & Validation]
- [x] **CHK014** – Does SC-003 (20-page brochure < 2 mins) state environmental assumptions (network latency, model choice) to ensure reproducible benchmarking? [Acceptance Criteria, Spec §SC-003, Plan §Technical Context]

## Non-Functional Requirements

- [x] **CHK015** – Are memory footprint expectations (sequential processing, limited temp storage) captured as explicit non-functional requirements rather than only implied in the plan? [Non-Functional, Spec §FR-015, §NFR-001]
- [x] **CHK016** – Do security requirements extend beyond `.env` handling to cover logging redaction, secret rotation, and third-party API key protection? [Non-Functional, Spec §FR-008–§FR-009]

## Dependencies & Assumptions

- [x] **CHK017** – Are external dependencies (MuPDF version 1.24.9, OpenRouter availability, Gemini model access) enumerated with compatibility and upgrade guidance? [Dependency, Plan §Technical Context, §Dependencies]
- [x] **CHK018** – Are assumptions regarding OpenRouter latency/rate limits documented together with mitigation strategies (queueing, fallbacks)? [Assumption, Spec §FR-010, Plan §Constraints & Assumptions]

## Ambiguities & Conflicts

- [x] **CHK019** – Is the division of responsibilities between CLI vs library API clearly documented to avoid overlapping or conflicting streaming/reporting behaviors? [Ambiguity, Spec §FR-014, Plan §Project Structure]
- [x] **CHK020** – Are USP vs specification extraction rules defined to avoid conflicts when the LLM output needs marketing tone adjustments (e.g., handling Orrefors crystal, Thor Hammer cues)? [Ambiguity, Spec §FR-004–§FR-005]
- [x] **CHK021** – Is the relationship between document categorization (FR-016) and existing extraction requirements (specs, USPs, tables) clearly defined to avoid conflicts or duplication in the output structure? [Ambiguity, Spec §FR-016, §FR-011]
