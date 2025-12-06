package integration

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/embedding"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/ingest"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/monitoring"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

func TestSemanticSpecFactsIngest_StoresExplanationAndChunks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := observability.DefaultLogger()

	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	applyMigrations(t, db)

	repos := storage.NewRepositories(db)
	embedder := embedding.NewMockClient(768)
	vectorAdapter, err := retrieval.NewFAISSAdapter(retrieval.FAISSConfig{Dimension: 768})
	require.NoError(t, err)
	lineageWriter := monitoring.NewLineageWriter(logger, repos.Lineage, monitoring.DefaultLineageConfig())

	p := ingest.NewPipeline(
		logger,
		ingest.PipelineConfig{
			ChunkSize:         512,
			ChunkOverlap:      64,
			MaxConcurrentJobs: 1,
			DedupeThreshold:   0.95,
		},
		repos,
		embedder,
		vectorAdapter,
		lineageWriter,
	)

	// Create sample markdown with one valid spec and one intentionally too-long value to trigger explanation failure
	content := `
## Specifications

| Category | Specification | Value | Key Features | Variant Availability |
|----------|---------------|-------|--------------|----------------------|
| Engine | Battery Range | 300 | Fast charge support | Standard |
| Engine | Diagnostic Code | very long diagnostic string that should exceed the maximum explanation length because it keeps going without adding value but stays within the table format to exercise guardrail behavior |  |  |
`
	tmpFile := filepath.Join(t.TempDir(), "sample.md")
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o644))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	req := ingest.IngestionRequest{
		TenantID:     uuid.New(),
		ProductID:    uuid.New(),
		CampaignID:   uuid.New(),
		MarkdownPath: tmpFile,
		Operator:     "test",
	}

	result, err := p.Ingest(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, storage.JobStatusSucceeded, result.Status)

	// Verify spec_values row contains explanation
	var explanation sql.NullString
	var failed bool
	row := db.QueryRow(`SELECT explanation, explanation_failed FROM spec_values LIMIT 1`)
	require.NoError(t, row.Scan(&explanation, &failed))
	assert.True(t, explanation.Valid)
	assert.False(t, failed)

	// Verify a guardrail-triggered spec marks explanation_failed
	var failedCount int64
	row = db.QueryRow(`SELECT COUNT(*) FROM spec_values WHERE explanation_failed = 1`)
	require.NoError(t, row.Scan(&failedCount))
	assert.Equal(t, int64(1), failedCount)

	// Verify spec_fact_chunks entry with embedding
	var chunkText sql.NullString
	var embeddingLen int
	row = db.QueryRow(`SELECT chunk_text, LENGTH(embedding_vector) FROM spec_fact_chunks LIMIT 1`)
	require.NoError(t, row.Scan(&chunkText, &embeddingLen))
	assert.True(t, chunkText.Valid)
	assert.Contains(t, chunkText.String, "Engine > Battery Range: 300")
	assert.Greater(t, embeddingLen, 0)

	// Verify vector index populated
	count, err := vectorAdapter.Count(ctx)
	require.NoError(t, err)
	assert.Greater(t, count, int64(0))
}

// applyMigrations runs the SQLite migrations needed for ingest tests.
func applyMigrations(t *testing.T, db *sql.DB) {
	t.Helper()

	migrations := []string{
		"0001_init_sqlite.sql",
		"0002_add_row_chunking_fields_sqlite.sql",
		"0003_add_key_features_variant_availability_sqlite.sql",
		"0004_add_explanation_and_spec_fact_chunks_sqlite.sql",
	}

	for _, name := range migrations {
		path := filepath.Join("..", "..", "db", "migrations", name)
		sqlBytes, err := os.ReadFile(path)
		require.NoErrorf(t, err, "read migration %s", name)
		_, err = db.Exec(string(sqlBytes))
		require.NoErrorf(t, err, "apply migration %s", name)
	}
}
