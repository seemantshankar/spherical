# Feature Specification: Semantic spec facts with explanations

**Feature Branch**: `005-semantic-spec-facts`  
**Created**: 2025-12-06  
**Status**: Draft  
**Input**: User description: "we need a scalable semantic solution, not hand-patched synonyms. Here's the concrete fix I'll implement next:"

## Clarifications

### Session 2025-12-06

- Q: How should keyword confidence be determined before switching to semantic spec_fact embeddings? → A: Use a configurable static threshold (default: 0.7) with an optional minimum-result check (default: 3 results) to trigger fallback. Both threshold and minimum-result count are configurable via environment variables or configuration file.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Enrich specs with semantic facts (Priority: P1)

Product owner ingests a new vehicle spec sheet and expects each row to gain a readable explanation plus an enriched spec_fact chunk so downstream search can answer intent-based queries (e.g., "phone connectivity" finds CarPlay/Android Auto).

**Why this priority**: Without enriched chunks and explanations, semantic recall will miss relevant facts and the feature cannot demonstrate value.

**Independent Test**: Run the ingest job on a sample spec sheet and verify every spec row outputs an explanation and stored spec_fact chunk with embeddings.

**Acceptance Scenarios**:

1. **Given** a spec row containing category, name, value, key features, and variant availability, **When** ingest runs, **Then** it stores an explanation sentence tied to that row.  
2. **Given** the same row, **When** ingest runs, **Then** it stores a spec_fact text chunk capturing category, name, value (with unit), key features, and variant availability and embeds it into the vector store.

---

### User Story 2 - Retrieve facts when keywords fail (Priority: P2)

A shopper searches with natural language terms (e.g., "phone integration" or "USB for charging") and expects relevant structured facts, even if exact keywords differ from stored labels.

**Why this priority**: Semantic fallback prevents zero-results experiences and improves answer relevance beyond keyword matching.

**Independent Test**: Issue queries that lack direct keyword matches but align semantically (CarPlay/Android Auto/Bluetooth/USB) and verify results return the correct facts with explanations.

**Acceptance Scenarios**:

1. **Given** a query with low keyword confidence, **When** the system falls back to spec_fact embeddings, **Then** it returns matching structured facts instead of empty results.  
2. **Given** a returned fact, **When** it is presented to the user, **Then** the explanation shows as a single unwrapped sentence alongside the fact details.

---

### User Story 3 - Guardrails on LLM-generated text (Priority: P3)

As a content reviewer, I want assurance that explanations and glosses stay within provided fields and refuse to add external or speculative claims.

**Why this priority**: Prevents misleading or marketing-heavy statements that could erode trust.

**Independent Test**: Provide inputs lacking certain details and verify the LLM produces concise, field-bounded sentences or declines extraneous additions.

**Acceptance Scenarios**:

1. **Given** a spec row with limited key features, **When** the LLM generates an explanation, **Then** it uses only provided fields and avoids invented benefits.  
2. **Given** a request for more detail than provided, **When** the LLM prompt is applied, **Then** it refuses to add extra details beyond supplied text.

---

### Edge Cases

- **Missing optional fields**: Ingest receives rows missing optional fields (e.g., no key features, no variant availability); generation must produce explanations and chunks using only available fields, omitting missing sections entirely without placeholder text or hallucination. The spec_fact chunk format must exclude sections for missing optional fields (no "Key features: N/A" or empty strings).

- **LLM failures**: LLM call fails or times out; ingest must retry up to 2 times with exponential backoff (1s, 2s) for transient errors, then store a clear failure marker (`explanation_failed=true` or NULL explanation with logged failure reason) for that row without blocking the pipeline. Permanent failures (invalid format, policy violation) skip retries and record failure immediately. All failures logged with row ID, error type, timestamp, and retry count.

- **Duplicate handling**: Duplicate or near-duplicate spec rows; embeddings and storage must avoid redundant chunks by deduplicating on composite key (category, name, value, variant_availability) with exact match. If a duplicate is detected, reuse existing embedding or update timestamp but do not create duplicate chunks.

- **Multi-intent queries**: Queries that mix multiple intents (e.g., "wireless Apple CarPlay and heated seats"); retrieval must rank all relevant facts by confidence score and return top-K results (default: 10) without dropping matches. Results should be sorted by confidence descending, with provenance indicated for each result.

- **Variant availability conflicts**: Variant-specific availability conflicts across trims; explanations must align exactly with the provided variant availability text from the source row, without interpretation or normalization that could introduce errors.

- **Zero/low-result scenarios**: When keyword search returns zero results or semantic fallback also returns zero/low results (below minimum threshold), the system must return an empty result set with appropriate user-facing messaging (e.g., "No matching specifications found"). The empty state must not trigger errors or exceptions.

- **Missing optional fields in explanation generation**: When generating explanations for rows with missing optional fields, the LLM must be explicitly instructed to work only with provided fields and must not invent or infer missing information. The explanation should focus on available fields (category, name, value) and omit references to missing optional fields entirely.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Add an `explanation` TEXT field to spec_values schema and propagate it through models, views, and repository layers so it is readable everywhere facts are used.  
- **FR-002**: During ingest, for each spec row, generate a single-sentence explanation constrained to the row's fields using the system prompt "You are a concise automotive brochure explainer. Use only the provided fields. One sentence. No marketing fluff." and refuse to add external details. A "single sentence" is defined as: ending with a single period, exclamation mark, or question mark; maximum length of 200 characters; no line breaks or wrapping characters; must be grammatically complete.  
- **FR-003**: During ingest, build a user-friendly spec_fact text chunk per row with the following format and ordering: "Category > Name: Value [unit]; Key features: [features]; Availability: [availability]; Gloss: [gloss]". All fields are required in this order, with separators " > ", ": ", "; " between sections. Optional fields (key features, availability, gloss) are omitted entirely if not present (no "N/A" or empty placeholders). Store and embed this chunk in the vector store. The chunk_text field must contain this formatted string exactly as specified.  
- **FR-004**: Ensure explanation generation is resilient: if LLM output is unavailable or invalid, record a fallback indicator (boolean flag `explanation_failed` or NULL explanation with failure marker) and continue ingest without blocking other rows. Implement retry logic with maximum 2 retries for transient failures (timeout, rate limit), with exponential backoff (1s, 2s delays). For permanent failures (invalid response format, content policy violation), record failure immediately without retries. All failures must be logged with row identifier, error type, and timestamp.  
- **FR-005**: Retrieval keeps keyword path as primary, but when keyword confidence is below a configurable static threshold (default: 0.7) and/or returns fewer than the minimum result count (default: 3 results), it must query spec_fact embeddings and return structured facts with explanations in the existing block layout. Both keyword and semantic retrieval paths must return results with the following fields: fact_id, category, name, value, unit (if available), key_features (optional), variant_availability (optional), explanation (single sentence), confidence (float score), and provenance (enum: "keyword" | "semantic"). When semantic fallback is triggered, all returned facts must have provenance="semantic" and include explanation from the spec_value row.  
- **FR-006**: Display responses with explanation shown as a single-line element next to each fact (no wrapping), preserving current block layout formatting.  
- **FR-007**: Provide safety filtering so explanations and glosses stay within provided text and reject speculative or promotional additions.

### Non-Functional Requirements

- **NFR-001**: **Observability - Ingest Metrics**: The ingest process must emit metrics for: (a) total rows processed, (b) successful explanation generations, (c) failed explanation generations (by error type: timeout, rate limit, invalid format, policy violation), (d) retry counts and success rates, (e) embedding write success/failure counts, (f) processing latency (p50, p95, p99) per row, (g) batch processing throughput (rows per second). All metrics must be tagged with source_id and timestamp.

- **NFR-002**: **Observability - Retrieval Metrics**: The retrieval process must emit metrics for: (a) total queries processed, (b) keyword path usage count and average confidence, (c) semantic fallback trigger count and reason (low confidence vs. low result count), (d) semantic query latency (p50, p95, p99), (e) result counts per query (keyword vs. semantic), (f) empty result set count. All metrics must be tagged with query pattern (hashed) and timestamp.

- **NFR-003**: **Observability - Logging**: The system must log: (a) all LLM failures with row identifier, error type, retry count, and timestamp, (b) all semantic fallback triggers with query, keyword confidence, result count, and fallback reason, (c) all embedding write failures with chunk identifier and error details, (d) configuration changes (threshold updates, retry limit changes). Logs must be structured (JSON format) and include correlation IDs for tracing requests across ingest and retrieval.

- **NFR-004**: **Observability - Tracing**: For each ingest row and retrieval query, the system must generate a trace ID that links: explanation generation attempts, embedding writes, retrieval path decisions (keyword vs. semantic), and result assembly. Traces must be queryable to debug end-to-end flows.

- **NFR-005**: **Performance Targets**: Ingest must process rows at a minimum throughput of 10 rows per second (p95). Embedding batch writes must complete within 2 seconds per batch (p95). Retrieval semantic fallback queries must complete within 500ms (p95) including vector similarity search. System must support embedding batch sizes of up to 100 chunks per batch.

- **NFR-006**: **Safety Guardrails**: Explanations must be validated against source fields before storage. Validation must check: (a) no terms appear in explanation that are not present in source fields (category, name, value, key_features, variant_availability), (b) explanation length does not exceed 200 characters, (c) explanation is a single sentence (ends with single punctuation, no line breaks). Failed validations must log the violation type and reject the explanation, storing a failure marker instead.

### Key Entities *(include if feature involves data)*

- **Spec Value**: A structured row containing category, spec name, value (with unit if present), key features, and variant availability.  
- **Explanation**: A one-sentence, field-bounded summary stored alongside each spec value for display and retrieval.  
- **Spec Fact Chunk**: Enriched, human-readable text derived from a spec value (category > name, value, key features, variant availability, gloss) that is stored and embedded for semantic search.  
- **Embedding Result**: Vector-search match against spec_fact chunks used to supplement or replace low-confidence keyword results.

### Assumptions & Dependencies

- **LLM Provider**: An LLM provider is available during ingest to return single-sentence, field-bounded outputs within acceptable latency (target: p95 < 5 seconds per row). If the LLM provider is unavailable or rate-limited, ingest must handle failures gracefully with retry logic and failure markers as specified in FR-004.

- **Vector Store**: A vector store (pgvector or equivalent) capable of storing and querying spec_fact embeddings is available and sized for current spec volume. If the vector store reaches capacity or becomes unavailable, ingest must log errors and continue with explanation storage, deferring embedding writes until capacity is restored.

- **UI Layout**: The existing fact block layout supports rendering a single-line explanation without design changes. If layout constraints prevent single-line display, the explanation must be truncated with ellipsis rather than wrapped.

- **Keyword Confidence Signal**: Retrieval has access to a keyword confidence signal (float 0.0-1.0) to decide when to invoke semantic fallback. If the confidence signal is unavailable, the system must default to semantic fallback for all queries.

- **Configuration Management**: Threshold values (keyword confidence threshold, minimum result count), retry limits (max retries, backoff delays), and deduplication rules (matching keys, tolerance) must be configurable via environment variables or configuration files, with documented defaults. Changes to configuration must not require code deployment.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of ingested spec rows store an explanation sentence or an explicit failure marker without blocking ingest completion.  
- **SC-002**: At least 90% of evaluated semantic queries (without direct keyword matches) return a relevant structured fact via spec_fact embeddings in user acceptance testing.  
- **SC-003**: Explanations render as a single line with no text wrapping in the current fact block layout across supported viewports during UI review.  
- **SC-004**: Ingest and retrieval logs show zero instances of explanations containing information not present in the source fields during controlled QA sampling. Sampling requirements: minimum 50 ingested rows and 50 retrieval queries must be manually reviewed. Failure tolerance: 0 failures allowed (100% accuracy required). Any explanation containing information not in source fields is considered a failure and must trigger investigation and prompt refinement.  
- **SC-005**: End-to-end search time with semantic fallback remains within the existing acceptable response threshold defined for search experiences. Measurable targets: p95 latency < 500ms for keyword path, p95 latency < 1000ms for semantic fallback path (including embedding query time). User acceptance testing must show no perceptible slowdown compared to keyword-only search (measured via user feedback and timing metrics).

- **SC-006**: Observability metrics are collected and available for monitoring: ingest success rate > 95% (excluding LLM provider outages), retrieval semantic fallback trigger rate is measurable, and all failures are logged with sufficient context for debugging.

## Traceability

All functional requirements, success criteria, and edge cases are assigned unique identifiers for traceability:

- **Functional Requirements**: FR-001 through FR-007
- **Non-Functional Requirements**: NFR-001 through NFR-006
- **Success Criteria**: SC-001 through SC-006
- **Edge Cases**: EC-001 (Missing optional fields), EC-002 (LLM failures), EC-003 (Duplicate handling), EC-004 (Multi-intent queries), EC-005 (Variant availability conflicts), EC-006 (Zero/low-result scenarios), EC-007 (Missing optional fields in explanation generation)

These identifiers must be referenced in implementation plans, test cases, and task breakdowns to ensure complete coverage and alignment between specification and implementation.
