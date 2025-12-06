-- Migration: Add key_features and variant_availability to spec_values (PostgreSQL)
-- Also recreate spec_view_latest to project the new columns
-- Date: 2025-12-05

-- Add new columns (idempotent)
ALTER TABLE spec_values
    ADD COLUMN IF NOT EXISTS key_features TEXT;

ALTER TABLE spec_values
    ADD COLUMN IF NOT EXISTS variant_availability TEXT;

-- Recreate materialized view with new columns
DROP MATERIALIZED VIEW IF EXISTS spec_view_latest;

CREATE MATERIALIZED VIEW spec_view_latest AS
SELECT 
    sv.id,
    sv.tenant_id,
    sv.product_id,
    sv.campaign_variant_id,
    sv.spec_item_id,
    si.display_name AS spec_name,
    sc.name AS category_name,
    COALESCE(sv.value_text, sv.value_numeric::TEXT) AS value,
    sv.unit,
    sv.confidence,
    sv.source_doc_id,
    sv.source_page,
    sv.version,
    cv.locale,
    cv.trim,
    cv.market,
    p.name AS product_name,
    sv.key_features,
    sv.variant_availability
FROM spec_values sv
JOIN spec_items si ON sv.spec_item_id = si.id
JOIN spec_categories sc ON si.category_id = sc.id
JOIN campaign_variants cv ON sv.campaign_variant_id = cv.id
JOIN products p ON sv.product_id = p.id
WHERE sv.status = 'active'
  AND cv.status = 'published';

CREATE UNIQUE INDEX idx_spec_view_latest_pk ON spec_view_latest(id);
CREATE INDEX idx_spec_view_latest_tenant ON spec_view_latest(tenant_id);
CREATE INDEX idx_spec_view_latest_product ON spec_view_latest(product_id);


