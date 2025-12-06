-- Migration: Add explanation fields and spec_fact_chunks for semantic facts (SQLite)
-- Date: 2025-12-06
-- Notes:
-- - Adds explanation/explanation_failed to spec_values
-- - Creates spec_fact_chunks table to store chunk_text/gloss + embeddings
-- - Recreates spec_view_latest to surface explanation for retrieval

-- ============================================================================
-- spec_values: explanation fields
-- ============================================================================
ALTER TABLE spec_values
ADD COLUMN explanation TEXT;

ALTER TABLE spec_values
ADD COLUMN explanation_failed INTEGER NOT NULL DEFAULT 0 CHECK (explanation_failed IN (0, 1));

-- ============================================================================
-- spec_fact_chunks: semantic fact chunks + embeddings
-- ============================================================================
CREATE TABLE IF NOT EXISTS spec_fact_chunks (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    campaign_variant_id TEXT NOT NULL REFERENCES campaign_variants(id) ON DELETE CASCADE,
    spec_value_id TEXT NOT NULL REFERENCES spec_values(id) ON DELETE CASCADE,
    chunk_text TEXT NOT NULL,
    gloss TEXT,
    embedding_vector BLOB,
    embedding_model TEXT,
    embedding_version TEXT,
    source TEXT NOT NULL DEFAULT 'ingest' CHECK (source IN ('ingest')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- 1:1 mapping to spec_values
CREATE UNIQUE INDEX IF NOT EXISTS idx_spec_fact_chunks_spec_value ON spec_fact_chunks(spec_value_id);

-- Fast lookup by campaign/tenant
CREATE INDEX IF NOT EXISTS idx_spec_fact_chunks_campaign ON spec_fact_chunks(campaign_variant_id, tenant_id);

-- ============================================================================
-- spec_view_latest: include explanation fields
-- ============================================================================
DROP VIEW IF EXISTS spec_view_latest;

CREATE VIEW IF NOT EXISTS spec_view_latest AS
SELECT 
    sv.id,
    sv.tenant_id,
    sv.product_id,
    sv.campaign_variant_id,
    sv.spec_item_id,
    si.display_name AS spec_name,
    sc.name AS category_name,
    COALESCE(sv.value_text, CAST(sv.value_numeric AS TEXT)) AS value,
    sv.unit,
    sv.confidence,
    sv.key_features,
    sv.variant_availability,
    sv.explanation,
    sv.explanation_failed,
    sv.source_doc_id,
    sv.source_page,
    sv.version,
    cv.locale,
    cv.trim,
    cv.market,
    p.name AS product_name
FROM spec_values sv
JOIN spec_items si ON sv.spec_item_id = si.id
JOIN spec_categories sc ON si.category_id = sc.id
JOIN campaign_variants cv ON sv.campaign_variant_id = cv.id
JOIN products p ON sv.product_id = p.id
WHERE sv.status = 'active'
  AND cv.status = 'published';

