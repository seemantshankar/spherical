-- Knowledge Engine Initial Schema
-- Supports both SQLite (dev) and Postgres+PGVector (prod)

-- Enable pgvector extension (Postgres only, ignored in SQLite)
-- CREATE EXTENSION IF NOT EXISTS vector;

-- ============================================================================
-- ENUMS (Postgres) / CHECK constraints (SQLite)
-- ============================================================================

-- Plan tiers for tenants
CREATE TYPE plan_tier AS ENUM ('sandbox', 'pro', 'enterprise');

-- Campaign status
CREATE TYPE campaign_status AS ENUM ('draft', 'published', 'archived');

-- Spec value status
CREATE TYPE spec_status AS ENUM ('active', 'conflict', 'deprecated');

-- Feature block type
CREATE TYPE block_type AS ENUM ('feature', 'usp', 'accessory');

-- Shareability levels
CREATE TYPE shareability AS ENUM ('private', 'tenant', 'public');

-- Knowledge chunk types
CREATE TYPE chunk_type AS ENUM ('spec_row', 'feature_block', 'usp', 'faq', 'comparison', 'global');

-- Visibility levels for chunks
CREATE TYPE visibility AS ENUM ('private', 'shared', 'benchmark');

-- Comparison verdicts
CREATE TYPE verdict AS ENUM ('primary_better', 'secondary_better', 'equal', 'cannot_compare');

-- Comparison shareability
CREATE TYPE comparison_shareability AS ENUM ('tenant_only', 'benchmark_only', 'public');

-- Ingestion job status
CREATE TYPE job_status AS ENUM ('pending', 'running', 'failed', 'succeeded');

-- Lineage action types
CREATE TYPE lineage_action AS ENUM ('created', 'updated', 'deleted', 'reconciled');

-- Drift alert types
CREATE TYPE alert_type AS ENUM ('stale_campaign', 'conflict_detected', 'hash_changed');

-- Alert status
CREATE TYPE alert_status AS ENUM ('open', 'acknowledged', 'resolved');

-- ============================================================================
-- TENANTS
-- ============================================================================

CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    plan_tier plan_tier NOT NULL DEFAULT 'sandbox',
    contact_email TEXT,
    settings JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tenants_name ON tenants(name);

-- ============================================================================
-- PRODUCTS
-- ============================================================================

CREATE TABLE products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    segment TEXT,
    body_type TEXT,
    model_year SMALLINT CHECK (model_year >= 1900 AND model_year <= 2100),
    is_public_benchmark BOOLEAN NOT NULL DEFAULT FALSE,
    default_campaign_variant_id UUID, -- FK added after campaign_variants table
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

CREATE INDEX idx_products_tenant ON products(tenant_id);
CREATE INDEX idx_products_benchmark ON products(is_public_benchmark) WHERE is_public_benchmark = TRUE;

-- ============================================================================
-- CAMPAIGN VARIANTS
-- ============================================================================

CREATE TABLE campaign_variants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    locale TEXT NOT NULL DEFAULT 'en-US',
    trim TEXT,
    market TEXT,
    status campaign_status NOT NULL DEFAULT 'draft',
    version INTEGER NOT NULL DEFAULT 1,
    effective_from TIMESTAMPTZ,
    effective_through TIMESTAMPTZ,
    is_draft BOOLEAN NOT NULL DEFAULT TRUE,
    last_published_by TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(product_id, locale, trim, market, version)
);

CREATE INDEX idx_campaigns_tenant ON campaign_variants(tenant_id);
CREATE INDEX idx_campaigns_product ON campaign_variants(product_id);
CREATE INDEX idx_campaigns_status ON campaign_variants(status);
CREATE INDEX idx_campaigns_effective ON campaign_variants(effective_from, effective_through);

-- Add FK from products to campaign_variants
ALTER TABLE products 
    ADD CONSTRAINT fk_products_default_campaign 
    FOREIGN KEY (default_campaign_variant_id) 
    REFERENCES campaign_variants(id) ON DELETE SET NULL;

-- ============================================================================
-- DOCUMENT SOURCES
-- ============================================================================

CREATE TABLE document_sources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    campaign_variant_id UUID REFERENCES campaign_variants(id) ON DELETE SET NULL,
    storage_uri TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    extractor_version TEXT,
    uploaded_by TEXT,
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_docsources_tenant ON document_sources(tenant_id);
CREATE INDEX idx_docsources_sha256 ON document_sources(sha256);

-- ============================================================================
-- SPEC CATEGORIES & ITEMS
-- ============================================================================

CREATE TABLE spec_categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    display_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE spec_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category_id UUID NOT NULL REFERENCES spec_categories(id) ON DELETE CASCADE,
    display_name TEXT NOT NULL,
    unit TEXT,
    data_type TEXT NOT NULL DEFAULT 'text',
    validation_rules JSONB DEFAULT '{}',
    aliases TEXT[] DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(category_id, display_name)
);

CREATE INDEX idx_specitems_category ON spec_items(category_id);

-- ============================================================================
-- SPEC VALUES
-- ============================================================================

CREATE TABLE spec_values (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    campaign_variant_id UUID NOT NULL REFERENCES campaign_variants(id) ON DELETE CASCADE,
    spec_item_id UUID NOT NULL REFERENCES spec_items(id) ON DELETE CASCADE,
    value_numeric NUMERIC,
    value_text TEXT,
    unit TEXT,
    confidence NUMERIC(3,2) DEFAULT 1.00,
    status spec_status NOT NULL DEFAULT 'active',
    source_doc_id UUID REFERENCES document_sources(id) ON DELETE SET NULL,
    source_page INTEGER,
    version INTEGER NOT NULL DEFAULT 1,
    effective_from TIMESTAMPTZ,
    effective_through TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, product_id, campaign_variant_id, spec_item_id, version)
);

CREATE INDEX idx_specvalues_tenant_product ON spec_values(tenant_id, product_id);
CREATE INDEX idx_specvalues_campaign ON spec_values(campaign_variant_id);
CREATE INDEX idx_specvalues_item ON spec_values(spec_item_id);
CREATE INDEX idx_specvalues_active ON spec_values(status) WHERE status = 'active';

-- ============================================================================
-- FEATURE BLOCKS (includes USPs)
-- ============================================================================

CREATE TABLE feature_blocks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    campaign_variant_id UUID NOT NULL REFERENCES campaign_variants(id) ON DELETE CASCADE,
    block_type block_type NOT NULL DEFAULT 'feature',
    body TEXT NOT NULL,
    priority SMALLINT DEFAULT 0,
    tags TEXT[] DEFAULT '{}',
    shareability shareability NOT NULL DEFAULT 'private',
    source_doc_id UUID REFERENCES document_sources(id) ON DELETE SET NULL,
    source_page INTEGER,
    embedding_vector vector(768),
    embedding_version TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_featureblocks_tenant ON feature_blocks(tenant_id);
CREATE INDEX idx_featureblocks_campaign ON feature_blocks(campaign_variant_id);
CREATE INDEX idx_featureblocks_type ON feature_blocks(block_type);
CREATE INDEX idx_featureblocks_tags ON feature_blocks USING GIN(tags);

-- ============================================================================
-- KNOWLEDGE CHUNKS (for semantic retrieval)
-- ============================================================================

CREATE TABLE knowledge_chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    campaign_variant_id UUID REFERENCES campaign_variants(id) ON DELETE SET NULL,
    chunk_type chunk_type NOT NULL DEFAULT 'global',
    text TEXT NOT NULL,
    metadata JSONB DEFAULT '{}',
    embedding_vector vector(768),
    embedding_model TEXT,
    embedding_version TEXT,
    source_doc_id UUID REFERENCES document_sources(id) ON DELETE SET NULL,
    source_page INTEGER,
    visibility visibility NOT NULL DEFAULT 'private',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_chunks_tenant ON knowledge_chunks(tenant_id);
CREATE INDEX idx_chunks_product ON knowledge_chunks(product_id);
CREATE INDEX idx_chunks_campaign ON knowledge_chunks(campaign_variant_id);
CREATE INDEX idx_chunks_type ON knowledge_chunks(chunk_type);
CREATE INDEX idx_chunks_visibility ON knowledge_chunks(visibility);
CREATE INDEX idx_chunks_metadata ON knowledge_chunks USING GIN(metadata);

-- IVFFlat index for vector similarity search (Postgres only)
-- Run after initial data load: CREATE INDEX idx_chunks_vector ON knowledge_chunks USING ivfflat (embedding_vector vector_cosine_ops) WITH (lists = 100);

-- ============================================================================
-- COMPARISON ROWS
-- ============================================================================

CREATE TABLE comparison_rows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    primary_product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    secondary_product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    dimension TEXT NOT NULL,
    primary_value TEXT,
    secondary_value TEXT,
    verdict verdict NOT NULL DEFAULT 'cannot_compare',
    narrative TEXT,
    shareability comparison_shareability NOT NULL DEFAULT 'tenant_only',
    source_primary_spec_id UUID REFERENCES spec_values(id) ON DELETE SET NULL,
    source_secondary_spec_id UUID REFERENCES spec_values(id) ON DELETE SET NULL,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(primary_product_id, secondary_product_id, dimension, shareability)
);

CREATE INDEX idx_comparisons_primary ON comparison_rows(primary_product_id);
CREATE INDEX idx_comparisons_secondary ON comparison_rows(secondary_product_id);
CREATE INDEX idx_comparisons_dimension ON comparison_rows(dimension);

-- ============================================================================
-- INGESTION JOBS
-- ============================================================================

CREATE TABLE ingestion_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    campaign_variant_id UUID REFERENCES campaign_variants(id) ON DELETE SET NULL,
    document_source_id UUID REFERENCES document_sources(id) ON DELETE SET NULL,
    status job_status NOT NULL DEFAULT 'pending',
    error_payload JSONB,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    run_by TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_jobs_tenant ON ingestion_jobs(tenant_id);
CREATE INDEX idx_jobs_status ON ingestion_jobs(status);

-- ============================================================================
-- LINEAGE EVENTS
-- ============================================================================

CREATE TABLE lineage_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    product_id UUID REFERENCES products(id) ON DELETE SET NULL,
    campaign_variant_id UUID REFERENCES campaign_variants(id) ON DELETE SET NULL,
    resource_type TEXT NOT NULL,
    resource_id UUID NOT NULL,
    document_source_id UUID REFERENCES document_sources(id) ON DELETE SET NULL,
    ingestion_job_id UUID REFERENCES ingestion_jobs(id) ON DELETE SET NULL,
    action lineage_action NOT NULL,
    payload JSONB DEFAULT '{}',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_lineage_tenant ON lineage_events(tenant_id);
CREATE INDEX idx_lineage_resource ON lineage_events(resource_type, resource_id);
CREATE INDEX idx_lineage_occurred ON lineage_events(occurred_at DESC);

-- ============================================================================
-- DRIFT ALERTS
-- ============================================================================

CREATE TABLE drift_alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    product_id UUID REFERENCES products(id) ON DELETE SET NULL,
    campaign_variant_id UUID REFERENCES campaign_variants(id) ON DELETE SET NULL,
    alert_type alert_type NOT NULL,
    details JSONB DEFAULT '{}',
    status alert_status NOT NULL DEFAULT 'open',
    detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);

CREATE INDEX idx_alerts_tenant ON drift_alerts(tenant_id);
CREATE INDEX idx_alerts_status ON drift_alerts(status) WHERE status = 'open';
CREATE INDEX idx_alerts_type ON drift_alerts(alert_type);

-- ============================================================================
-- MATERIALIZED VIEWS
-- ============================================================================

-- Latest active spec values for retrieval
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
    p.name AS product_name
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

-- Shared knowledge chunks for cross-tenant queries
CREATE VIEW knowledge_chunks_shared AS
SELECT *
FROM knowledge_chunks
WHERE visibility IN ('shared', 'benchmark');

-- Campaign health aggregation
CREATE VIEW campaign_health_view AS
SELECT 
    cv.id AS campaign_id,
    cv.tenant_id,
    cv.product_id,
    cv.status,
    cv.version,
    cv.effective_from,
    COUNT(DISTINCT da.id) FILTER (WHERE da.status = 'open') AS open_alerts,
    COUNT(DISTINCT ij.id) FILTER (WHERE ij.status = 'failed') AS failed_jobs,
    MAX(le.occurred_at) AS last_activity,
    CASE 
        WHEN cv.effective_from < NOW() - INTERVAL '180 days' THEN 'stale'
        WHEN COUNT(DISTINCT da.id) FILTER (WHERE da.status = 'open') > 0 THEN 'needs_attention'
        ELSE 'healthy'
    END AS health_status
FROM campaign_variants cv
LEFT JOIN drift_alerts da ON cv.id = da.campaign_variant_id
LEFT JOIN ingestion_jobs ij ON cv.id = ij.campaign_variant_id
LEFT JOIN lineage_events le ON cv.id = le.campaign_variant_id
GROUP BY cv.id, cv.tenant_id, cv.product_id, cv.status, cv.version, cv.effective_from;

-- ============================================================================
-- ROW-LEVEL SECURITY POLICIES (Postgres only)
-- ============================================================================

-- Enable RLS on tenant-scoped tables
ALTER TABLE products ENABLE ROW LEVEL SECURITY;
ALTER TABLE campaign_variants ENABLE ROW LEVEL SECURITY;
ALTER TABLE document_sources ENABLE ROW LEVEL SECURITY;
ALTER TABLE spec_values ENABLE ROW LEVEL SECURITY;
ALTER TABLE feature_blocks ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_chunks ENABLE ROW LEVEL SECURITY;
ALTER TABLE ingestion_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE lineage_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE drift_alerts ENABLE ROW LEVEL SECURITY;

-- Create policies (example - actual implementation uses current_setting('app.tenant_id'))
-- These are commented out as they require application-level setup
/*
CREATE POLICY tenant_isolation_products ON products
    USING (tenant_id = current_setting('app.tenant_id')::UUID);

CREATE POLICY tenant_isolation_campaigns ON campaign_variants
    USING (tenant_id = current_setting('app.tenant_id')::UUID);
*/

-- ============================================================================
-- TRIGGERS
-- ============================================================================

-- Auto-update updated_at timestamps
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_tenants_updated_at BEFORE UPDATE ON tenants
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trg_products_updated_at BEFORE UPDATE ON products
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trg_campaigns_updated_at BEFORE UPDATE ON campaign_variants
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trg_specvalues_updated_at BEFORE UPDATE ON spec_values
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trg_featureblocks_updated_at BEFORE UPDATE ON feature_blocks
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trg_chunks_updated_at BEFORE UPDATE ON knowledge_chunks
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- ============================================================================
-- SEED DATA (spec categories)
-- ============================================================================

INSERT INTO spec_categories (name, description, display_order) VALUES
    ('Engine', 'Engine specifications and performance', 1),
    ('Fuel Efficiency', 'Mileage and fuel consumption', 2),
    ('Transmission', 'Gearbox and drive system', 3),
    ('Dimensions', 'Vehicle measurements', 4),
    ('Weight', 'Vehicle weight specifications', 5),
    ('Safety', 'Safety features and ratings', 6),
    ('Comfort', 'Interior comfort features', 7),
    ('Technology', 'Tech and infotainment features', 8),
    ('Exterior', 'Exterior design features', 9),
    ('Warranty', 'Warranty coverage', 10);

