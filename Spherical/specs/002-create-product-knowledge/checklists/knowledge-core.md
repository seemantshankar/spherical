# Unit Tests for Requirements: Product Knowledge Engine (Ingestion & Retrieval)
Purpose: Validate requirement quality for the core ingestion, retrieval, and validation flows as a lighter spec/PR review aid.
Created: 2025-11-27
Depth: Lighter review focus
Audience: Reviewer

## Requirement Completeness
- [ ] CHK001 - Are all ingestion pipeline inputs and deterministic stages (Markdown from the PDF extractor, JSON uploads, deduping, publishing) enumerated so connectors know what is supported? [Completeness, Spec §FR-002]
- [ ] CHK002 - Are normalization rules for spec categories and canonical IDs defined for each brochure dimension to avoid ambiguity when duplicate brochures import? [Completeness, Spec §FR-003]
- [ ] CHK003 - Does the retrieval requirement describe intent classification, thresholds, and hybrid execution paths for structured, vector, and fallback queries so every path is owned? [Completeness, Spec §FR-006]

## Requirement Clarity
- [ ] CHK004 - Is the configurable threshold that chooses SQL vs vector vs hybrid execution clarified with measurable operators (metrics, priority order, escalation) rather than described only as “configurable”? [Clarity, Spec §FR-006]
- [ ] CHK005 - Are the expectations around vector query scoring metadata (ties, scoring scale, inclusion/exclusion of metadata) spelled out for the top-k ≤ 8 results? [Clarity, Spec §FR-007]
- [ ] CHK006 - Is the “deterministic” ingestion pipeline defined with explicit validation rules (conflict flags, confidence tracking, rollback gating) so implementers know when to block publishing? [Clarity, Spec §FR-002; Spec §FR-003]

## Requirement Consistency
- [ ] CHK007 - Do the multi-tenant tagging requirements align with vector filters, cache controls, and audit logging so no retrieval or caching path leaks records with the wrong visibility label? [Consistency, Spec §FR-001; Spec §FR-007; Spec §FR-010; Spec §FR-013]

## Acceptance Criteria Quality
- [ ] CHK008 - Do the measurable success criteria for ingestion time and retrieval latency map back to the corresponding functional requirements so reviewers can tell when acceptance gates are met? [Acceptance Criteria, Spec §SC-001; Spec §SC-002; Spec §FR-002; Spec §FR-008]

## Scenario Coverage
- [ ] CHK009 - Are the stories for onboarding an empty campaign, publishing pending edits, and triggering the PDF extractor via CLI documented with clear expected inputs and outputs? [Coverage, Spec §User Story 1 (acceptance scenarios 1–3)]
- [ ] CHK010 - Are the trim-specific retrieval path and mis-specified trim fallback scenarios described with explicit requirements for structured vs semantic fallback behavior? [Coverage, Spec §User Story 2]

## Edge Case Coverage
- [ ] CHK011 - Are the ingestion/retrieval edge cases for conflicting numeric values, deleting campaigns mid-conversation, and mismatched embedding versions documented so the requirements state how such failures should surface? [Edge Case, Spec §Edge Cases]

## Non-Functional Requirements
- [ ] CHK012 - Are performance budgets for SQLite vs Postgres structured lookups and vector responses specified so both local development and prod deployments know their latency expectations? [Non-Functional, Spec §FR-008; Spec §FR-012; Spec §SC-002]

## Dependencies & Assumptions
- [ ] CHK013 - Are the dependencies on the PDF extractor, lineage drift monitors, and related services called out so the ingestion/retrieval requirements include the expected contracts for upstream tooling? [Dependency, Spec §User Story 1 acceptance scenario 3; Spec §User Story 4; Spec §FR-014]

## Ambiguities & Conflicts
- [ ] CHK014 - Are the outstanding authentication and retention clarifications from FR-016/FR-017 resolved before this checklist is used so reviewers know whether those assumptions are still in flux? [Ambiguity, Spec §FR-016; Spec §FR-017]

