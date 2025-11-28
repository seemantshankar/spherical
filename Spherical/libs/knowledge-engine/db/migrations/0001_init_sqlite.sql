-- Knowledge Engine Initial Schema (SQLite)
-- This is the SQLite-compatible version for development/testing

-- ============================================================================
-- TENANTS
-- ============================================================================

CREATE TABLE IF NOT EXISTS tenants (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    name TEXT NOT NULL UNIQUE,
    plan_tier TEXT NOT NULL DEFAULT 'sandbox' CHECK (plan_tier IN ('sandbox', 'pro', 'enterprise')),
    contact_email TEXT,
    settings TEXT DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_tenants_plan_tier ON tenants(plan_tier);

-- ============================================================================
-- PRODUCTS
-- ============================================================================

CREATE TABLE IF NOT EXISTS products (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    segment TEXT,
    body_type TEXT,
    model_year INTEGER,
    is_public_benchmark INTEGER NOT NULL DEFAULT 0,
    default_campaign_variant_id TEXT,
    metadata TEXT DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_products_tenant ON products(tenant_id);
CREATE INDEX IF NOT EXISTS idx_products_segment ON products(segment);
CREATE INDEX IF NOT EXISTS idx_products_body_type ON products(body_type);
CREATE INDEX IF NOT EXISTS idx_products_model_year ON products(model_year);
CREATE INDEX IF NOT EXISTS idx_products_benchmark ON products(is_public_benchmark) WHERE is_public_benchmark = 1;

-- ============================================================================
-- CAMPAIGN_VARIANTS
-- ============================================================================

CREATE TABLE IF NOT EXISTS campaign_variants (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    locale TEXT NOT NULL DEFAULT 'en-US',
    trim TEXT,
    market TEXT,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'archived')),
    version INTEGER NOT NULL DEFAULT 1,
    effective_from TEXT,
    effective_through TEXT,
    is_draft INTEGER NOT NULL DEFAULT 1,
    last_published_by TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_cv_product ON campaign_variants(product_id);
CREATE INDEX IF NOT EXISTS idx_cv_tenant ON campaign_variants(tenant_id);
CREATE INDEX IF NOT EXISTS idx_cv_locale ON campaign_variants(locale);
CREATE INDEX IF NOT EXISTS idx_cv_status ON campaign_variants(status);
CREATE INDEX IF NOT EXISTS idx_cv_effective ON campaign_variants(effective_from, effective_through);

-- ============================================================================
-- DOCUMENT_SOURCES
-- ============================================================================

CREATE TABLE IF NOT EXISTS document_sources (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    campaign_variant_id TEXT REFERENCES campaign_variants(id) ON DELETE SET NULL,
    storage_uri TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    extractor_version TEXT,
    uploaded_by TEXT,
    uploaded_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_ds_tenant ON document_sources(tenant_id);
CREATE INDEX IF NOT EXISTS idx_ds_product ON document_sources(product_id);
CREATE INDEX IF NOT EXISTS idx_ds_sha ON document_sources(sha256);

-- ============================================================================
-- SPEC_CATEGORIES
-- ============================================================================

CREATE TABLE IF NOT EXISTS spec_categories (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    display_order INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================================
-- SPEC_ITEMS
-- ============================================================================

CREATE TABLE IF NOT EXISTS spec_items (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    category_id TEXT NOT NULL REFERENCES spec_categories(id) ON DELETE CASCADE,
    display_name TEXT NOT NULL,
    unit TEXT,
    data_type TEXT NOT NULL DEFAULT 'text' CHECK (data_type IN ('text', 'numeric', 'boolean', 'json')),
    validation_rules TEXT DEFAULT '{}',
    aliases TEXT DEFAULT '[]',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_si_category ON spec_items(category_id);
CREATE INDEX IF NOT EXISTS idx_si_name ON spec_items(display_name);

-- ============================================================================
-- SPEC_VALUES
-- ============================================================================

CREATE TABLE IF NOT EXISTS spec_values (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    campaign_variant_id TEXT NOT NULL REFERENCES campaign_variants(id) ON DELETE CASCADE,
    spec_item_id TEXT NOT NULL REFERENCES spec_items(id) ON DELETE CASCADE,
    value_numeric REAL,
    value_text TEXT,
    unit TEXT,
    confidence REAL NOT NULL DEFAULT 1.0,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'conflict', 'deprecated')),
    source_doc_id TEXT REFERENCES document_sources(id) ON DELETE SET NULL,
    source_page INTEGER,
    version INTEGER NOT NULL DEFAULT 1,
    effective_from TEXT,
    effective_through TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_sv_tenant ON spec_values(tenant_id);
CREATE INDEX IF NOT EXISTS idx_sv_product ON spec_values(product_id);
CREATE INDEX IF NOT EXISTS idx_sv_campaign ON spec_values(campaign_variant_id);
CREATE INDEX IF NOT EXISTS idx_sv_spec ON spec_values(spec_item_id);
CREATE INDEX IF NOT EXISTS idx_sv_status ON spec_values(status);

-- ============================================================================
-- FEATURE_BLOCKS
-- ============================================================================

CREATE TABLE IF NOT EXISTS feature_blocks (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    campaign_variant_id TEXT NOT NULL REFERENCES campaign_variants(id) ON DELETE CASCADE,
    block_type TEXT NOT NULL DEFAULT 'feature' CHECK (block_type IN ('feature', 'usp', 'accessory')),
    body TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0,
    tags TEXT DEFAULT '[]',
    shareability TEXT NOT NULL DEFAULT 'private' CHECK (shareability IN ('private', 'tenant', 'public')),
    source_doc_id TEXT REFERENCES document_sources(id) ON DELETE SET NULL,
    source_page INTEGER,
    embedding_vector BLOB,
    embedding_version TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_fb_tenant ON feature_blocks(tenant_id);
CREATE INDEX IF NOT EXISTS idx_fb_product ON feature_blocks(product_id);
CREATE INDEX IF NOT EXISTS idx_fb_campaign ON feature_blocks(campaign_variant_id);
CREATE INDEX IF NOT EXISTS idx_fb_type ON feature_blocks(block_type);

-- ============================================================================
-- KNOWLEDGE_CHUNKS
-- ============================================================================

CREATE TABLE IF NOT EXISTS knowledge_chunks (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    campaign_variant_id TEXT REFERENCES campaign_variants(id) ON DELETE SET NULL,
    chunk_type TEXT NOT NULL DEFAULT 'global' CHECK (chunk_type IN ('spec_row', 'feature_block', 'usp', 'faq', 'comparison', 'global')),
    text TEXT NOT NULL,
    metadata TEXT DEFAULT '{}',
    embedding_vector BLOB,
    embedding_model TEXT,
    embedding_version TEXT,
    source_doc_id TEXT REFERENCES document_sources(id) ON DELETE SET NULL,
    source_page INTEGER,
    visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'shared', 'benchmark')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_kc_tenant ON knowledge_chunks(tenant_id);
CREATE INDEX IF NOT EXISTS idx_kc_product ON knowledge_chunks(product_id);
CREATE INDEX IF NOT EXISTS idx_kc_campaign ON knowledge_chunks(campaign_variant_id);
CREATE INDEX IF NOT EXISTS idx_kc_type ON knowledge_chunks(chunk_type);
CREATE INDEX IF NOT EXISTS idx_kc_visibility ON knowledge_chunks(visibility);

-- ============================================================================
-- COMPARISON_ROWS
-- ============================================================================

CREATE TABLE IF NOT EXISTS comparison_rows (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    primary_product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    secondary_product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    dimension TEXT NOT NULL,
    primary_value TEXT,
    secondary_value TEXT,
    verdict TEXT NOT NULL DEFAULT 'cannot_compare' CHECK (verdict IN ('primary_better', 'secondary_better', 'equal', 'cannot_compare')),
    narrative TEXT,
    shareability TEXT NOT NULL DEFAULT 'tenant_only' CHECK (shareability IN ('tenant_only', 'benchmark_only', 'public')),
    source_primary_spec_id TEXT REFERENCES spec_values(id) ON DELETE SET NULL,
    source_secondary_spec_id TEXT REFERENCES spec_values(id) ON DELETE SET NULL,
    computed_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_cr_pair_dim ON comparison_rows(primary_product_id, secondary_product_id, dimension);
CREATE INDEX IF NOT EXISTS idx_cr_primary ON comparison_rows(primary_product_id);
CREATE INDEX IF NOT EXISTS idx_cr_secondary ON comparison_rows(secondary_product_id);

-- ============================================================================
-- INGESTION_JOBS
-- ============================================================================

CREATE TABLE IF NOT EXISTS ingestion_jobs (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    campaign_variant_id TEXT REFERENCES campaign_variants(id) ON DELETE SET NULL,
    document_source_id TEXT REFERENCES document_sources(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'failed', 'succeeded')),
    error_payload TEXT,
    started_at TEXT,
    completed_at TEXT,
    run_by TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_ij_tenant ON ingestion_jobs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_ij_product ON ingestion_jobs(product_id);
CREATE INDEX IF NOT EXISTS idx_ij_status ON ingestion_jobs(status);

-- ============================================================================
-- LINEAGE_EVENTS
-- ============================================================================

CREATE TABLE IF NOT EXISTS lineage_events (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    product_id TEXT REFERENCES products(id) ON DELETE SET NULL,
    campaign_variant_id TEXT REFERENCES campaign_variants(id) ON DELETE SET NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    document_source_id TEXT REFERENCES document_sources(id) ON DELETE SET NULL,
    ingestion_job_id TEXT REFERENCES ingestion_jobs(id) ON DELETE SET NULL,
    action TEXT NOT NULL CHECK (action IN ('created', 'updated', 'deleted', 'reconciled')),
    payload TEXT DEFAULT '{}',
    occurred_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_le_tenant ON lineage_events(tenant_id);
CREATE INDEX IF NOT EXISTS idx_le_resource ON lineage_events(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_le_occurred ON lineage_events(occurred_at);

-- ============================================================================
-- DRIFT_ALERTS
-- ============================================================================

CREATE TABLE IF NOT EXISTS drift_alerts (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    product_id TEXT REFERENCES products(id) ON DELETE SET NULL,
    campaign_variant_id TEXT REFERENCES campaign_variants(id) ON DELETE SET NULL,
    alert_type TEXT NOT NULL CHECK (alert_type IN ('stale_campaign', 'conflict_detected', 'hash_changed')),
    details TEXT DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'acknowledged', 'resolved')),
    detected_at TEXT NOT NULL DEFAULT (datetime('now')),
    resolved_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_da_tenant ON drift_alerts(tenant_id);
CREATE INDEX IF NOT EXISTS idx_da_status ON drift_alerts(status);
CREATE INDEX IF NOT EXISTS idx_da_type ON drift_alerts(alert_type);

-- ============================================================================
-- TRIGGERS FOR updated_at
-- ============================================================================

CREATE TRIGGER IF NOT EXISTS trg_tenants_updated 
    AFTER UPDATE ON tenants 
    FOR EACH ROW 
    BEGIN UPDATE tenants SET updated_at = datetime('now') WHERE id = NEW.id; END;

CREATE TRIGGER IF NOT EXISTS trg_products_updated 
    AFTER UPDATE ON products 
    FOR EACH ROW 
    BEGIN UPDATE products SET updated_at = datetime('now') WHERE id = NEW.id; END;

CREATE TRIGGER IF NOT EXISTS trg_cv_updated 
    AFTER UPDATE ON campaign_variants 
    FOR EACH ROW 
    BEGIN UPDATE campaign_variants SET updated_at = datetime('now') WHERE id = NEW.id; END;

CREATE TRIGGER IF NOT EXISTS trg_sv_updated 
    AFTER UPDATE ON spec_values 
    FOR EACH ROW 
    BEGIN UPDATE spec_values SET updated_at = datetime('now') WHERE id = NEW.id; END;

CREATE TRIGGER IF NOT EXISTS trg_fb_updated 
    AFTER UPDATE ON feature_blocks 
    FOR EACH ROW 
    BEGIN UPDATE feature_blocks SET updated_at = datetime('now') WHERE id = NEW.id; END;

CREATE TRIGGER IF NOT EXISTS trg_kc_updated 
    AFTER UPDATE ON knowledge_chunks 
    FOR EACH ROW 
    BEGIN UPDATE knowledge_chunks SET updated_at = datetime('now') WHERE id = NEW.id; END;

