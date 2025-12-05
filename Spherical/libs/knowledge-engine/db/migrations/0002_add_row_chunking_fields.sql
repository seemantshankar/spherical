-- Migration: Add row-level chunking fields to knowledge_chunks table
-- Supports both SQLite (dev) and Postgres+PGVector (prod)
-- Date: 2025-12-04
-- Feature: 004-row-level-chunking

-- ============================================================================
-- Add content_hash column for content-based deduplication
-- ============================================================================

-- Postgres
ALTER TABLE knowledge_chunks 
ADD COLUMN IF NOT EXISTS content_hash VARCHAR(64);

-- SQLite (handled separately in 0002_add_row_chunking_fields_sqlite.sql)
-- ALTER TABLE knowledge_chunks ADD COLUMN content_hash TEXT;

-- ============================================================================
-- Add completion_status column for tracking embedding generation status
-- ============================================================================

-- Postgres
ALTER TABLE knowledge_chunks 
ADD COLUMN IF NOT EXISTS completion_status VARCHAR(20) DEFAULT 'complete' 
CHECK (completion_status IN ('complete', 'incomplete', 'retry-needed'));

-- SQLite (handled separately in 0002_add_row_chunking_fields_sqlite.sql)
-- ALTER TABLE knowledge_chunks ADD COLUMN completion_status TEXT DEFAULT 'complete' CHECK (completion_status IN ('complete', 'incomplete', 'retry-needed'));

-- ============================================================================
-- Create unique index on content_hash (allows NULLs for legacy chunks)
-- ============================================================================

-- Postgres
CREATE UNIQUE INDEX IF NOT EXISTS idx_chunks_content_hash 
ON knowledge_chunks(content_hash) 
WHERE content_hash IS NOT NULL;

-- SQLite (handled separately in 0002_add_row_chunking_fields_sqlite.sql)
-- CREATE UNIQUE INDEX IF NOT EXISTS idx_chunks_content_hash ON knowledge_chunks(content_hash) WHERE content_hash IS NOT NULL;

-- ============================================================================
-- Create partial index for retry queue (incomplete chunks only)
-- ============================================================================

-- Postgres
CREATE INDEX IF NOT EXISTS idx_chunks_completion_status 
ON knowledge_chunks(completion_status) 
WHERE completion_status != 'complete';

-- SQLite (handled separately in 0002_add_row_chunking_fields_sqlite.sql)
-- CREATE INDEX IF NOT EXISTS idx_chunks_completion_status ON knowledge_chunks(completion_status) WHERE completion_status != 'complete';

-- ============================================================================
-- Update existing chunks: set completion_status based on embedding presence
-- ============================================================================

-- Postgres
UPDATE knowledge_chunks 
SET completion_status = CASE 
    WHEN embedding_vector IS NULL THEN 'incomplete'
    ELSE 'complete'
END
WHERE completion_status IS NULL;

-- SQLite (handled separately in 0002_add_row_chunking_fields_sqlite.sql)
-- UPDATE knowledge_chunks 
-- SET completion_status = CASE 
--     WHEN embedding_vector IS NULL THEN 'incomplete'
--     ELSE 'complete'
-- END
-- WHERE completion_status IS NULL;



