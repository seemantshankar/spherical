// Package integration provides integration tests for the Knowledge Engine.
package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/ingest"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
)

func TestIngestionPipeline_ParseCamrySample(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Read the sample file
	samplePath := filepath.Join("..", "..", "testdata", "camry-sample.md")
	content, err := os.ReadFile(samplePath)
	require.NoError(t, err, "Failed to read sample file")

	// Create parser
	parser := ingest.NewParser(ingest.ParserConfig{
		ChunkSize:    512,
		ChunkOverlap: 64,
	})

	// Parse content
	result, err := parser.Parse(string(content))
	require.NoError(t, err)

	// Verify metadata
	assert.Equal(t, "Toyota Camry Hybrid 2025 Specifications", result.Metadata.Title)
	assert.Equal(t, "Camry Hybrid", result.Metadata.ProductName)
	assert.Equal(t, 2025, result.Metadata.ModelYear)
	assert.Equal(t, "en-IN", result.Metadata.Locale)
	assert.Equal(t, "India", result.Metadata.Market)
	assert.Equal(t, "XLE Hybrid", result.Metadata.Trim)

	// Verify specs extracted
	assert.GreaterOrEqual(t, len(result.SpecValues), 15, "Should extract at least 15 specs")

	// Check for specific specs
	foundFuelEfficiency := false
	foundPower := false
	for _, spec := range result.SpecValues {
		if spec.Name == "Combined Mileage" && spec.Category == "Fuel Efficiency" {
			foundFuelEfficiency = true
			assert.Equal(t, "25.49", spec.Value)
			assert.Equal(t, "km/l", spec.Unit)
		}
		if spec.Name == "Maximum Power" && spec.Category == "Engine" {
			foundPower = true
			assert.Equal(t, "176", spec.Value)
			assert.Equal(t, "hp", spec.Unit)
		}
	}
	assert.True(t, foundFuelEfficiency, "Should find fuel efficiency spec")
	assert.True(t, foundPower, "Should find power spec")

	// Verify features extracted
	assert.GreaterOrEqual(t, len(result.Features), 5, "Should extract at least 5 features")

	// Verify USPs extracted
	assert.GreaterOrEqual(t, len(result.USPs), 3, "Should extract at least 3 USPs")

	// Verify chunks generated
	assert.GreaterOrEqual(t, len(result.RawChunks), 1, "Should generate at least 1 chunk")
}

func TestIngestionPipeline_FullIngestion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Skip if no database available
	if os.Getenv("KNOWLEDGE_ENGINE_ENV") != "ci" {
		t.Skip("Skipping full ingestion test - requires database")
	}

	logger := observability.DefaultLogger()
	pipeline := ingest.NewPipeline(logger, ingest.PipelineConfig{
		ChunkSize:         512,
		ChunkOverlap:      64,
		MaxConcurrentJobs: 2,
		DedupeThreshold:   0.95,
	})

	// Read sample file
	samplePath := filepath.Join("..", "..", "testdata", "camry-sample.md")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Run ingestion
	result, err := pipeline.Ingest(ctx, ingest.IngestionRequest{
		TenantID:     uuid.New(),
		ProductID:    uuid.New(),
		CampaignID:   uuid.New(),
		MarkdownPath: samplePath,
		Operator:     "test-runner",
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, result.JobID)
	assert.Greater(t, result.SpecsCreated, 0)
	assert.Greater(t, result.FeaturesCreated, 0)
	assert.Greater(t, result.USPsCreated, 0)
	assert.Greater(t, result.ChunksCreated, 0)
}

func TestIngestionPipeline_DuplicateDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := observability.DefaultLogger()
	pipeline := ingest.NewPipeline(logger, ingest.PipelineConfig{
		DedupeThreshold: 0.95,
	})

	content := "Test content for deduplication"

	// Check if duplicate
	isDupe, hash, err := pipeline.Deduplicate(content, 0.95)
	require.NoError(t, err)
	assert.False(t, isDupe, "First submission should not be duplicate")
	assert.NotEmpty(t, hash)

	// Same content should produce same hash
	_, hash2, _ := pipeline.Deduplicate(content, 0.95)
	assert.Equal(t, hash, hash2, "Same content should produce same hash")
}

