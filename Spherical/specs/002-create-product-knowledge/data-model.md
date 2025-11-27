# Data Model – Product Knowledge Engine Library

## Overview

The Product Knowledge Engine library persists brochure-derived data in normalized relational tables while synchronizing semantic chunks inside PGVector. Every row carries tenant/product/campaign metadata to enforce row-level security and cross-tenant sharing rules. Below is the canonical entity catalog for the MVP.

---

## 1. `tenants`

| Field | Type | Notes |
|-------|------|-------|
| `id` | UUID (PK) | Stable identifier for OEM/customer. |
| `name` | text | Display name; unique. |
| `plan_tier` | enum(`sandbox`,`pro`,`enterprise`) | Drives quotas & drift SLA. |
| `contact_email` | text | For automation + alerts. |
| `settings` | jsonb | Feature flags, locale defaults, worktree requirements. |
| `created_at`/`updated_at` | timestamptz | Audit timestamps. |

**Relationships**: `tenants 1..n products`, `tenants 1..n document_sources`, `tenants 1..n drift_alerts`.

---

## 2. `products`

| Field | Type | Notes |
|-------|------|-------|
| `id` | UUID (PK) | Primary key. |
| `tenant_id` | UUID (FK → tenants.id) | Enforces ownership and RLS. |
| `name` | text | e.g., “Camry Hybrid 2025”. |
| `segment` | text | Sedan, SUV, EV, etc. |
| `body_type` | text | Additional taxonomy for comparisons. |
| `model_year` | smallint | Validated range 1900–2100. |
| `is_public_benchmark` | bool | Enables cross-tenant comparisons. |
| `default_campaign_variant_id` | UUID (FK → campaign_variants.id) | Fallback when trim unspecified. |
| `metadata` | jsonb | Storage for tags/markets. |
| `created_at`/`updated_at` | timestamptz | Audit columns. |

**Relationships**: `products 1..n campaign_variants`, `products 1..n spec_values`, `products 1..n knowledge_chunks`, `products 1..n comparison_rows` (primary or secondary).

---

## 3. `campaign_variants`

| Field | Type | Notes |
|-------|------|-------|
| `id` | UUID (PK) | Primary key. |
| `product_id` | UUID (FK) | Links to product. |
| `tenant_id` | UUID (FK) | Duplicate for faster tenant filtering. |
| `locale` | text | e.g., `en-US`, `en-IN`. |
| `trim` | text | “XLE Hybrid”, “Base”. |
| `market` | text | Country/region descriptor. |
| `status` | enum(`draft`,`published`,`archived`) | Drives retrieval filters. |
| `version` | integer | Incremented on publish. |
| `effective_from`/`effective_through` | timestamptz | Publication window. |
| `is_draft` | bool | Convenience flag for CLI. |
| `last_published_by` | text | Operator record. |

**Relationships**: referenced by `spec_values`, `feature_blocks`, `knowledge_chunks`, `comparison_rows`, `lineage_events`.

---

## 4. `document_sources`

| Field | Type | Notes |
|-------|------|-------|
| `id` | UUID (PK) | Primary key. |
| `tenant_id` | UUID (FK) | Owner. |
| `product_id` | UUID (FK) | Associated product. |
| `campaign_variant_id` | UUID (FK, nullable) | Optional trim-level linkage. |
| `storage_uri` | text | S3/object-store pointer to PDF/Markdown bundle. |
| `sha256` | text | Dedup + drift detection. |
| `extractor_version` | text | e.g., `pdf-extractor@1.3.0`. |
| `uploaded_by` | text | User or automation actor. |
| `uploaded_at` | timestamptz | Timestamp for SLA metrics. |

**Relationships**: `document_sources 1..n lineage_events`, `document_sources 1..n ingestion_jobs`.

---

## 5. `spec_categories` & `spec_items`

- `spec_categories`: `id`, `name`, `description`, `display_order`.
- `spec_items`: `id`, `category_id`, `display_name`, `unit`, `data_type`, `validation_rules(jsonb)`, `aliases(text[])`.

**Purpose**: Provide canonical taxonomy for deduplication across brochures and languages.

---

## 6. `spec_values`

| Field | Type | Notes |
|-------|------|-------|
| `id` | UUID (PK) | Primary key. |
| `tenant_id`/`product_id`/`campaign_variant_id` | UUID (FKs) | Composite used in unique indexes + RLS. |
| `spec_item_id` | UUID (FK) | Links to canonical item. |
| `value_numeric` | numeric (nullable) | Standardized measurement. |
| `value_text` | text (nullable) | Free-form spec. |
| `unit` | text | Normalized unit. |
| `confidence` | numeric(3,2) | Derived from extractor or human override. |
| `status` | enum(`active`,`conflict`,`deprecated`) | Conflict prevents publish. |
| `source_doc_id` | UUID (FK → document_sources.id) | Provenance. |
| `source_page` | integer | Quick trace to brochure page. |
| `version` | integer | Bumps on publish. |
| `effective_from`/`effective_through` | timestamptz | Validity range. |
| `created_at`/`updated_at` | timestamptz | Audit. |

**Indexes**: `(tenant_id, product_id, campaign_variant_id, spec_item_id, version)` unique; partial index for `status='active'` to accelerate retrieval.

---

## 7. `feature_blocks`

Represents qualitative marketing bullets (including USPs) with governance metadata.

| Field | Type | Notes |
|-------|------|-------|
| `id` | UUID (PK) | Primary key. |
| `tenant_id`/`product_id`/`campaign_variant_id` | UUID | Ownership tags. |
| `block_type` | enum(`feature`,`usp`,`accessory`) | Controls prompting. |
| `body` | text | Human-readable copy. |
| `priority` | smallint | Sort order. |
| `tags` | text[] | e.g., `["comfort","safety"]`. |
| `shareability` | enum(`private`,`tenant`,`public`) | Governs cross-tenant comparisons. |
| `source_doc_id` | UUID | Provenance. |
| `source_page` | integer | Page pointer. |
| `embedding_vector` | vector(768) | Optional denormalized copy for faster retrieval. |
| `embedding_version` | text | Model ident. |
| `created_at`/`updated_at` | timestamptz | Audit columns. |

---

## 8. `knowledge_chunks`

Holds chunked text for semantic retrieval.

| Field | Type | Notes |
|-------|------|-------|
| `id` | UUID (PK) | Primary key. |
| `tenant_id`/`product_id`/`campaign_variant_id` | UUID | Ownership tags. |
| `chunk_type` | enum(`spec_row`,`feature_block`,`usp`,`faq`,`comparison`,`global`) | Filtering dimension. |
| `text` | text | Chunk content. |
| `metadata` | jsonb | Category, trim, related products, etc. |
| `embedding_vector` | vector(768) | PGVector column. |
| `embedding_model` | text | Model slug. |
| `embedding_version` | text | Release/generator version. |
| `source_doc_id` | UUID | Provenance. |
| `source_page` | integer | Page pointer. |
| `visibility` | enum(`private`,`shared`,`benchmark`) | Determines cross-tenant scope. |
| `created_at`/`updated_at` | timestamptz | Audit columns. |

**Indexes**: PGVector `ivfflat` grouped by `tenant_id`; GIN index on `metadata` for filters.

---

## 9. `comparison_rows`

| Field | Type | Notes |
|-------|------|-------|
| `id` | UUID (PK) | Primary key. |
| `primary_product_id` | UUID | Owned by requesting tenant. |
| `secondary_product_id` | UUID | Must be flagged shared/benchmark. |
| `dimension` | text | e.g., `"Fuel Efficiency"`. |
| `primary_value` | text | Render-ready string. |
| `secondary_value` | text | Render-ready string. |
| `verdict` | enum(`primary_better`,`secondary_better`,`equal`,`cannot_compare`) | Helps LLM summarization. |
| `narrative` | text | Pre-baked delta messaging. |
| `shareability` | enum(`tenant_only`,`benchmark_only`,`public`) | Enforcement guard. |
| `source_primary_spec_id` | UUID | For lineage. |
| `source_secondary_spec_id` | UUID | For lineage. |
| `computed_at` | timestamptz | Staleness tracking. |

**Indexes**: Unique `(primary_product_id, secondary_product_id, dimension, shareability)` for idempotent recomputes.

---

## 10. `ingestion_jobs`

Tracks CLI/API ingestion runs.

| Field | Type | Notes |
|-------|------|-------|
| `id` | UUID (PK) | Job identifier. |
| `tenant_id` | UUID | Owner. |
| `product_id` | UUID | Product scope. |
| `campaign_variant_id` | UUID | Optional trim scope. |
| `document_source_id` | UUID | Back-reference to uploaded artifact. |
| `status` | enum(`pending`,`running`,`failed`,`succeeded`) | Drives CLI progress. |
| `error_payload` | jsonb | Captures extractor/LLM failures. |
| `started_at` | timestamptz | Execution start. |
| `completed_at` | timestamptz | Execution end. |
| `run_by` | text | CLI user or service token. |

---

## 11. `lineage_events`

| Field | Type | Notes |
|-------|------|-------|
| `id` | UUID (PK) | Event id. |
| `tenant_id`/`product_id`/`campaign_variant_id` | UUID | Ownership tags. |
| `resource_type` | enum(`spec_value`,`feature_block`,`knowledge_chunk`,`comparison`) | Type of asset. |
| `resource_id` | UUID | Asset identifier. |
| `document_source_id` | UUID | Provenance pointer. |
| `ingestion_job_id` | UUID | Which job caused the event. |
| `action` | enum(`created`,`updated`,`deleted`,`reconciled`) | Audit action. |
| `payload` | jsonb | Diff or metadata snapshot. |
| `occurred_at` | timestamptz | Timestamp. |

Provides traceability for audits and assistant explanations.

---

## 12. `drift_alerts`

| Field | Type | Notes |
|-------|------|-------|
| `id` | UUID (PK) | Alert id. |
| `tenant_id` | UUID | Owner. |
| `product_id` | UUID | Product scope. |
| `campaign_variant_id` | UUID | Campaign scope. |
| `alert_type` | enum(`stale_campaign`,`conflict_detected`,`hash_changed`) | Reason for alert. |
| `details` | jsonb | Additional payload (e.g., missing brochure pages). |
| `status` | enum(`open`,`acknowledged`,`resolved`) | Workflow state. |
| `detected_at` | timestamptz | When alert fired. |
| `resolved_at` | timestamptz | Optional resolution time. |

---

## Derived Views

- **`spec_view_latest`**: Materialized view joining `spec_values` + `spec_items` filtered to latest published version; used by structured retrieval path.
- **`knowledge_chunks_shared`**: View exposing only `visibility IN ('shared','benchmark')` for cross-tenant comparisons.
- **`campaign_health_view`**: Aggregates `drift_alerts`, `ingestion_jobs`, and `lineage_events` to power the admin dashboard.

These views feed both the CLI (for quick validations) and the HTTP API responses described in the contracts.
