# Checklist: Requirements Quality – Semantic spec facts with explanations

**Purpose**: Unit tests for the requirements (clarity, completeness, consistency) for semantic spec facts/explanations.  
**Created**: 2025-12-06  
**Feature**: [spec.md](../spec.md)

## Requirement Completeness
- [x] CHK001 Are all ingest outputs (explanation, chunk_text, embedding, failure marker) explicitly required for every spec row? [Completeness, Spec §FR-001–FR-004] ✅ Addressed in FR-001 (explanation), FR-003 (chunk_text, embedding), FR-004 (failure marker)
- [x] CHK002 Are retrieval outputs defined for both keyword and semantic fallback, including explanation and provenance fields? [Completeness, Spec §FR-005–FR-007] ✅ Addressed in FR-005 (explicitly defines both paths with explanation and provenance fields)
- [x] CHK003 Are observability/metrics requirements for ingest and retrieval results documented? [Gap] ✅ Addressed in NFR-001, NFR-002, NFR-003, NFR-004

## Requirement Clarity
- [x] CHK004 Is the keyword confidence threshold behavior (static threshold + min-result check) precisely specified, including defaults and configurability? [Clarity, Spec §Clarifications, Spec §FR-005] ✅ Addressed in Clarifications (default: 0.7 threshold, 3 min results) and Assumptions & Dependencies (configurable via env vars/config)
- [x] CHK005 Are the contents and formatting of `spec_fact` chunks (ordering, separators, optional gloss) unambiguous? [Clarity, Spec §FR-003] ✅ Addressed in FR-003 (exact format: "Category > Name: Value [unit]; Key features: ...; Availability: ...; Gloss: ..." with specified separators)
- [x] CHK006 Is the definition of "single sentence" for explanations constrained (e.g., punctuation rules, max length)? [Clarity, Spec §FR-002] ✅ Addressed in FR-002 (max 200 chars, ends with single punctuation, no line breaks, grammatically complete)

## Requirement Consistency
- [x] CHK007 Are ingestion resilience expectations (continue on LLM failure) consistent across edge cases and success criteria? [Consistency, Spec §FR-004, Edge Cases] ✅ Addressed - FR-004 and Edge Cases EC-002 consistently specify retry logic, failure markers, and non-blocking behavior
- [x] CHK008 Do retrieval behaviors (keyword-first, semantic fallback, display format) align between user stories and functional requirements? [Consistency, Spec §US2, Spec §FR-005–FR-006] ✅ Addressed - User Story 2 and FR-005, FR-006 align on keyword-first with semantic fallback and single-line display format

## Acceptance Criteria Quality
- [x] CHK009 Are success criteria measurable for latency/"no perceptible slowdown" with concrete thresholds? [Measurability, Spec §SC-005, Gap] ✅ Addressed in SC-005 (p95 < 500ms keyword, p95 < 1000ms semantic fallback)
- [x] CHK010 Do success criteria cover explanation safety (no hallucination) with sampling size and failure tolerance? [Measurability, Spec §SC-004] ✅ Addressed in SC-004 (50+ sampled rows/queries, 0 failures allowed, 100% accuracy required)

## Scenario Coverage
- [x] CHK011 Are zero/low-result retrieval scenarios fully specified (fallback, empty state messaging)? [Coverage, Spec §FR-005] ✅ Addressed in Edge Cases EC-006 (empty result set with user-facing messaging, no errors)
- [x] CHK012 Are multi-intent queries (mixed domains) handled with ranking/partial returns requirements? [Coverage, Edge Cases] ✅ Addressed in Edge Cases EC-004 (rank by confidence, return top-K, no dropping matches)
- [x] CHK013 Are variant-specific availability conflicts addressed in retrieval output rules? [Coverage, Edge Cases] ✅ Addressed in Edge Cases EC-005 (exact alignment with provided variant availability text, no interpretation)

## Edge Case Coverage
- [x] CHK014 Are missing optional fields (key features, availability) explicitly handled in explanation/chunk generation without hallucination? [Edge Case, Edge Cases] ✅ Addressed in Edge Cases EC-001 and EC-007 (omit missing fields entirely, no placeholders, no hallucination)
- [x] CHK015 Are LLM timeout/error retries and recorded failure markers specified with limits and logging expectations? [Edge Case, Spec §FR-004] ✅ Addressed in FR-004 (max 2 retries, exponential backoff 1s/2s, failure markers, logging with row ID, error type, timestamp)

## Non-Functional Requirements
- [x] CHK016 Are performance/throughput targets for ingest and retrieval specified (p95, QPS, embedding batch sizes)? [NFR, Gap] ✅ Addressed in NFR-005 (10 rows/sec ingest, 2s batch writes, 500ms semantic queries, batch size up to 100 chunks)
- [x] CHK017 Are observability signals (metrics, logs, traces) defined for fallback triggers, LLM failures, and embedding writes? [NFR, Gap] ✅ Addressed in NFR-001 (ingest metrics), NFR-002 (retrieval metrics), NFR-003 (logging), NFR-004 (tracing)
- [x] CHK018 Are safety/guardrail requirements for field-bounded explanations explicitly documented (beyond "no marketing fluff")? [NFR, Spec §FR-002, Spec §FR-007] ✅ Addressed in NFR-006 (validation checks: no terms outside source fields, max 200 chars, single sentence format, rejection on failure)

## Dependencies & Assumptions
- [x] CHK019 Are external dependencies (LLM provider availability, pgvector store capacity) and their failure modes captured? [Dependency, Assumptions] ✅ Addressed in Assumptions & Dependencies (LLM provider with latency targets and failure handling, vector store capacity and unavailability handling)
- [x] CHK020 Is configuration management for thresholds, retry limits, and deduplication rules documented? [Dependency, Spec §FR-005, Spec §FR-004] ✅ Addressed in Assumptions & Dependencies (thresholds, retry limits, deduplication rules configurable via env vars/config files, documented defaults, no code deployment required)

## Ambiguities & Conflicts
- [x] CHK021 Are terminology and field names (e.g., "gloss" vs "explanation") used consistently across requirements and contracts? [Ambiguity, Spec §FR-003, Contracts] ✅ Addressed - Data model explicitly distinguishes "explanation" (required, in spec_values) from "gloss" (optional, in chunk_text). Both spec and contracts use consistent terminology.
- [x] CHK022 Are deduplication rules for embeddings clearly defined (keys, tolerance) to avoid conflicting interpretations? [Ambiguity, Data Model] ✅ Addressed in Data Model (exact match on composite key: category, name, value, variant_availability; case-sensitive, no normalization or fuzzy matching; check before embedding generation)

## Traceability
- [x] CHK023 Is there an ID/traceability scheme linking functional requirements, success criteria, and tests/tasks? [Traceability, Gap] ✅ Addressed - New "Traceability" section in spec.md defines unique identifiers: FR-001 through FR-007, NFR-001 through NFR-006, SC-001 through SC-006, EC-001 through EC-007. These must be referenced in implementation plans, test cases, and task breakdowns.
