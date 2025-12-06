-- Migration: Add key_features and variant_availability to spec_values (SQLite)
-- Also recreate spec_view_latest to project the new columns
-- Date: 2025-12-05

-- Add new columns to spec_values
ALTER TABLE spec_values
ADD COLUMN key_features TEXT;

ALTER TABLE spec_values
ADD COLUMN variant_availability TEXT;

-- Recreate spec_view_latest with the new columns
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


