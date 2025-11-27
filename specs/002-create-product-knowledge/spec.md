# Feature Specification: Product Knowledge Engine Library

**Feature Branch**: `002-create-product-knowledge`  
**Created**: 2025-11-27  
**Status**: Draft  
**Input**: User description: "Create Product Knowledge Engine Library"

## User Scenarios & Testing *(mandatory)*

<!--
  IMPORTANT: User stories should be PRIORITIZED as user journeys ordered by importance.
  Each user story/journey must be INDEPENDENTLY TESTABLE - meaning if you implement just ONE of them,
  you should still have a viable MVP (Minimum Viable Product) that delivers value.
  
  Assign priorities (P1, P2, P3, etc.) to each story, where P1 is the most critical.
  Think of each story as a standalone slice of functionality that can be:
  - Developed independently
  - Tested independently
  - Deployed independently
  - Demonstrated to users independently
-->

### User Story 1 - Tenant admin onboards a new campaign (Priority: P1)

An OEM program manager can ingest brochure-derived specs, features, USPs, and metadata for a new make/model campaign, validate them, and publish them without affecting other tenants.

**Why this priority**: Without reliable ingestion plus scoping, no downstream AI experience can answer questions accurately or respect data ownership.

**Independent Test**: Run the ingestion CLI/API against a Toyota Camry brochure, review the generated records in a staging environment, and publish them while ensuring no Honda assets move.

**Acceptance Scenarios**:

1. **Given** an empty campaign for Toyota Camry 2025, **When** the admin uploads the latest brochure bundle, **Then** the system creates `spec_values`, `feature_blocks`, `knowledge_chunks`, and `usp` entries tagged with the correct `tenant_id`, `product_id`, and `campaign_variant_id`.
2. **Given** a campaign with pending edits, **When** the admin toggles “Publish”, **Then** the draft version becomes active, the `effective_from` timestamp updates, and other tenants cannot query the draft data.

---

### User Story 2 - AI sales agent answers trim-specific questions (Priority: P2)

The runtime retrieval service can serve the conversational agent with deterministic specs plus semantic excerpts within 150 ms median latency for questions like “What’s the Camry Hybrid’s fuel efficiency?”

**Why this priority**: Fast, grounded answers directly impact user trust and booking conversion rates.

**Independent Test**: Simulate a question via the retrieval API, assert the SQL payload + top vector chunks are returned with citations, and confirm latency budgets.

**Acceptance Scenarios**:

1. **Given** a question about fuel efficiency, **When** the agent invokes the retrieval API with `product_id=CamryHybrid` and intent `spec_lookup`, **Then** the service returns the latest structured row (25.49 km/l) and at least one supporting chunk with metadata `category=Fuel Efficiency`.
2. **Given** a mis-specified trim, **When** the user asks “Does the XSE include ventilated seats?”, **Then** the system detects the trim mismatch, falls back to default campaign variant filters, and surfaces both the structured seat feature and the descriptive USP chunk.

---

### User Story 3 - Comparative assistant responds to cross-make prompts (Priority: P3)

The platform can safely combine public benchmark data and per-tenant products to answer “How does Camry mileage compare to Accord?” without leaking private trims.

**Why this priority**: Competitive comparisons are a key differentiator for the SaaS offering and justify cross-pollination investments.

**Independent Test**: Flag two products as comparable, run a comparison query, and verify the response only uses data flagged as sharable while citing both products.

**Acceptance Scenarios**:

1. **Given** Camry Hybrid (private) and Accord Hybrid (public benchmark) data, **When** the agent issues a comparison intent, **Then** the retrieval layer issues a filtered SQL + vector query for both product IDs and returns structured `comparisons` rows with provenance.
2. **Given** a competitor product not marked `is_public_benchmark`, **When** a user attempts cross-tenant comparison, **Then** the router declines the comparison and responds with a compliant fallback (“Comparison data unavailable”).

---

### User Story 4 - Data team audits lineage & drift (Priority: P4)

Compliance analysts can trace any response back to the brochure page or upload event and detect stale campaigns needing refresh.

**Why this priority**: OEMs expect auditability before approving AI copilots in regulated regions.

**Independent Test**: Pick a random USP, fetch its lineage via API, and verify timestamps plus source documents are logged.

**Acceptance Scenarios**:

1. **Given** a published campaign, **When** the analyst queries the lineage API for `chunk_id=abc123`, **Then** the response shows the source brochure page, extraction run ID, operator, and embedding version.
2. **Given** a campaign older than 180 days, **When** drift monitoring detects missing updates, **Then** the system flags the campaign as “Needs Refresh” and surfaces it in the admin dashboard without affecting live responses.

### Edge Cases

- Brochure ingestion produces conflicting numeric values across pages (e.g., wheelbase mismatch) → system stores both under same spec item but marks lower-confidence row as `status=conflict` and prevents publishing until resolved.
- Tenant deletes a campaign while an agent session is mid-conversation → retrieval layer must fall back to previous published version or respond with “information unavailable” without errors.
- Vector embeddings refreshed with a new model version while SQLite fallback cache still holds legacy vectors → re-ingest job must toggle `embedding_version` filters to avoid mixing incompatible vectors.
- Comparison request references a product ID the tenant has not licensed → router must decline and log policy violation rather than returning empty data that might look like zero capability.

## Requirements *(mandatory)*

<!--
  ACTION REQUIRED: The content in this section represents placeholders.
  Fill them out with the right functional requirements.
-->

### Functional Requirements

- **FR-001**: System MUST support multi-tenant row-level security by tagging every record with `tenant_id`, `product_id`, `campaign_variant_id`, and `visibility` (private, shared, benchmark).
- **FR-002**: System MUST ingest structured specs, feature bullets, USPs, and document metadata via a deterministic pipeline that accepts Markdown from the PDF extractor or JSON uploads.
- **FR-003**: System MUST normalize spec categories and items (e.g., Engine → Fuel Efficiency) so duplicate brochures map to the same canonical IDs.
- **FR-004**: System MUST version campaign data (`version`, `effective_from`, `effective_through`, `is_draft`) and expose publish/rollback operations.
- **FR-005**: System MUST store unstructured snippets in `knowledge_chunks` with embeddings, metadata JSON (category, trim, source_page, chunk_type), and `embedding_version`.
- **FR-006**: Retrieval API MUST route each query through intent classification and choose SQL, vector, or hybrid execution paths with configurable thresholds (structured first, semantic fallback).
- **FR-007**: Vector queries MUST support filtered search (tenant/product/campaign + optional chunk_type) and return top-k ≤ 8 with scoring metadata.
- **FR-008**: Structured spec queries MUST respond within 120 ms p50 (SQLite) and 80 ms p50 (Postgres+PGVector) for single product lookups.
- **FR-009**: Comparison service MUST assemble `comparisons` rows for approved product pairs and provide templated deltas (better/worse/equal) for the LLM prompt.
- **FR-010**: All ingestion and retrieval operations MUST emit audit logs containing operator (or service) identity, request payload summary, and affected record IDs.
- **FR-011**: System MUST expose REST/GraphQL endpoints plus gRPC/Go SDK methods for embedding into the AI sales agent service.
- **FR-012**: Local development MUST operate with SQLite (structured tables) plus FAISS or in-memory ANN for vectors, while prod deployments MUST support Postgres + PGVector without schema rewrites.
- **FR-013**: Cache layer (Redis or in-memory) MUST store hot spec lookups and intent classification results with TTL and per-tenant eviction controls.
- **FR-014**: Drift monitor MUST alert when a campaign or embedding set exceeds defined freshness thresholds (e.g., 180 days) or when upstream brochures change hash.
- **FR-015**: System MUST provide export/import tooling (CSV or Parquet) so OEMs can audit their own data or migrate between environments.

*Example of marking unclear requirements:*

- **FR-016**: System MUST authenticate admin and API clients via [NEEDS CLARIFICATION: SSO provider vs API key strategy].
- **FR-017**: System MUST retain historical campaign versions for [NEEDS CLARIFICATION: retention period / regional compliance rules].

### Key Entities *(include if feature involves data)*

- **Tenant**: OEM/customer account. Attributes: name, plan_tier, contact_email, row-level security policies, feature flags.
- **Product**: Make/model-year definition. Attributes: tenant_id, name, segment, body_type, `is_public_benchmark`, default campaign variant.
- **CampaignVariant**: Market/trim-specific slice (e.g., Camry Hybrid India). Attributes: product_id, locale, trim, status, effective dates.
- **SpecItem**: Canonical leaf like “Fuel Efficiency”. Attributes: category_id, display_name, unit, data_type, validation rules.
- **SpecValue**: Concrete measurement tied to a campaign. Attributes: spec_item_id, product_id, value_numeric, value_text, unit, confidence, source_doc_id.
- **FeatureBlock / USP**: Marketing bullets and longer narratives. Attributes: type, body, priority, tags (comfort, safety), shareability flag.
- **KnowledgeChunk**: Vectorized excerpt with chunk_type, embedding, metadata JSON, source_doc reference, embedding_version.
- **ComparisonRow**: Pre-computed comparison results per dimension. Attributes: primary_product_id, secondary_product_id, dimension, primary_value, secondary_value, verdict, shareability scope.
- **DocumentSource**: Brochure or manual ingestion record. Attributes: storage_uri, checksum, extractor_version, uploaded_by, processed_at.

## Success Criteria *(mandatory)*

<!--
  ACTION REQUIRED: Define measurable success criteria.
  These must be technology-agnostic and measurable.
-->

### Measurable Outcomes

- **SC-001**: Ingestion of a 20-page brochure to published status completes in ≤15 minutes end-to-end (including validations and deduping) on reference hardware.
- **SC-002**: Retrieval API p50 latency ≤150 ms and p95 ≤350 ms for single-product queries under a 200 RPS mixed workload.
- **SC-003**: ≥95% of agent responses referencing structured specs cite the latest published version (verified via nightly QA replay).
- **SC-004**: ≥80% of comparative queries automatically route through approved benchmark data with zero cross-tenant leakage incidents per quarter.
- **SC-005**: Drift monitor surfaces ≥90% of campaigns older than 180 days and triggers notifications within 1 hour of detection.
