// Package ingest provides the brochure ingestion pipeline for the Knowledge Engine.
package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/embedding"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/monitoring"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// Pipeline orchestrates the brochure ingestion process.
type Pipeline struct {
	logger        *observability.Logger
	parser        *Parser
	config        PipelineConfig
	repos         *storage.Repositories
	embedder      embedding.Embedder
	vectorAdapter retrieval.VectorAdapter
	lineageWriter *monitoring.LineageWriter
}

// PipelineConfig holds pipeline configuration.
type PipelineConfig struct {
	PDFExtractorPath  string
	ChunkSize         int
	ChunkOverlap      int
	DedupeThreshold   float64
	MaxConcurrentJobs int
}

// IngestionRequest represents a request to ingest a brochure.
type IngestionRequest struct {
	TenantID     uuid.UUID
	ProductID    uuid.UUID
	CampaignID   uuid.UUID
	MarkdownPath string
	PDFPath      string
	SourceFile   string
	Operator     string
	Overwrite    bool
	AutoPublish  bool
}

// IngestionResult represents the result of an ingestion job.
type IngestionResult struct {
	JobID            uuid.UUID
	Status           storage.JobStatus
	SpecsCreated     int
	SpecsUpdated     int
	FeaturesCreated  int
	USPsCreated      int
	ChunksCreated    int
	ConflictingSpecs []uuid.UUID
	Errors           []string
	StartedAt        time.Time
	CompletedAt      time.Time
	Duration         time.Duration
}

// NewPipeline creates a new ingestion pipeline.
func NewPipeline(
	logger *observability.Logger,
	cfg PipelineConfig,
	repos *storage.Repositories,
	embedder embedding.Embedder,
	vectorAdapter retrieval.VectorAdapter,
	lineageWriter *monitoring.LineageWriter,
) *Pipeline {
	return &Pipeline{
		logger: logger,
		parser: NewParser(ParserConfig{
			ChunkSize:    cfg.ChunkSize,
			ChunkOverlap: cfg.ChunkOverlap,
		}),
		config:        cfg,
		repos:         repos,
		embedder:      embedder,
		vectorAdapter: vectorAdapter,
		lineageWriter: lineageWriter,
	}
}

// Ingest processes a brochure and stores the extracted content.
func (p *Pipeline) Ingest(ctx context.Context, req IngestionRequest) (*IngestionResult, error) {
	jobID := uuid.New()
	startTime := time.Now()

	result := &IngestionResult{
		JobID:     jobID,
		Status:    storage.JobStatusRunning,
		StartedAt: startTime,
	}

	p.logger.Info().
		Str("job_id", jobID.String()).
		Str("tenant_id", req.TenantID.String()).
		Str("product_id", req.ProductID.String()).
		Str("campaign_id", req.CampaignID.String()).
		Msg("Starting ingestion job")

	// Step 1: Get Markdown content
	markdownContent, err := p.getMarkdownContent(ctx, req)
	if err != nil {
		result.Status = storage.JobStatusFailed
		result.Errors = append(result.Errors, fmt.Sprintf("get markdown: %v", err))
		return result, err
	}

	// Step 2: Parse the Markdown
	parsed, err := p.parser.Parse(markdownContent)
	if err != nil {
		result.Status = storage.JobStatusFailed
		result.Errors = append(result.Errors, fmt.Sprintf("parse markdown: %v", err))
		return result, err
	}

	// Add parsing errors/warnings
	for _, parseErr := range parsed.Errors {
		result.Errors = append(result.Errors, parseErr.Message)
	}

	// Step 3: Validate parsed content
	validationErrors := ValidateParsedBrochure(parsed)
	for _, valErr := range validationErrors {
		result.Errors = append(result.Errors, valErr.Message)
	}

	// Step 4: Create document source record
	docSource, err := p.createDocumentSource(ctx, req, markdownContent)
	if err != nil {
		result.Status = storage.JobStatusFailed
		result.Errors = append(result.Errors, fmt.Sprintf("create doc source: %v", err))
		return result, err
	}

	// Step 5: Deduplicate and store specs
	specsResult, err := p.storeSpecs(ctx, req, parsed.SpecValues, docSource.ID)
	if err != nil {
		result.Status = storage.JobStatusFailed
		result.Errors = append(result.Errors, fmt.Sprintf("store specs: %v", err))
		return result, err
	}
	result.SpecsCreated = specsResult.Created
	result.SpecsUpdated = specsResult.Updated
	result.ConflictingSpecs = specsResult.Conflicts

	// Step 6: Store features
	featuresCreated, err := p.storeFeatures(ctx, req, parsed.Features, docSource.ID)
	if err != nil {
		p.logger.Warn().Err(err).Msg("Failed to store some features")
	}
	result.FeaturesCreated = featuresCreated

	// Step 7: Store USPs
	uspsCreated, err := p.storeUSPs(ctx, req, parsed.USPs, docSource.ID)
	if err != nil {
		p.logger.Warn().Err(err).Msg("Failed to store some USPs")
	}
	result.USPsCreated = uspsCreated

	// Step 8: Generate and store chunks
	chunksCreated, err := p.storeChunks(ctx, req, parsed.RawChunks, docSource.ID)
	if err != nil {
		p.logger.Warn().Err(err).Msg("Failed to store some chunks")
	}
	result.ChunksCreated = chunksCreated

	// Step 9: Emit lineage events
	if err := p.emitLineageEvents(ctx, req, result); err != nil {
		p.logger.Warn().Err(err).Msg("Failed to emit lineage events")
	}

	// Determine final status
	if len(result.ConflictingSpecs) > 0 {
		result.Status = storage.JobStatusSucceeded // Job succeeded but has conflicts
		result.Errors = append(result.Errors,
			fmt.Sprintf("%d conflicting specs detected - publish blocked until resolved",
				len(result.ConflictingSpecs)))
	} else {
		result.Status = storage.JobStatusSucceeded
	}

	result.CompletedAt = time.Now()
	result.Duration = result.CompletedAt.Sub(result.StartedAt)

	p.logger.Info().
		Str("job_id", jobID.String()).
		Str("status", string(result.Status)).
		Int("specs_created", result.SpecsCreated).
		Int("features_created", result.FeaturesCreated).
		Int("usps_created", result.USPsCreated).
		Int("chunks_created", result.ChunksCreated).
		Dur("duration", result.Duration).
		Msg("Ingestion job completed")

	return result, nil
}

// getMarkdownContent retrieves the Markdown content, extracting from PDF if needed.
func (p *Pipeline) getMarkdownContent(ctx context.Context, req IngestionRequest) (string, error) {
	// If Markdown path is provided, read it directly
	if req.MarkdownPath != "" {
		content, err := os.ReadFile(req.MarkdownPath)
		if err != nil {
			return "", fmt.Errorf("read markdown file: %w", err)
		}
		return string(content), nil
	}

	// If PDF path is provided, run the pdf-extractor
	if req.PDFPath != "" {
		return p.extractFromPDF(ctx, req.PDFPath)
	}

	return "", fmt.Errorf("no markdown or PDF path provided")
}

// extractFromPDF invokes the pdf-extractor binary to extract Markdown from a PDF.
func (p *Pipeline) extractFromPDF(ctx context.Context, pdfPath string) (string, error) {
	extractorPath := p.config.PDFExtractorPath
	if extractorPath == "" {
		extractorPath = "pdf-extractor"
	}

	// Create temp file for output
	tmpFile, err := os.CreateTemp("", "ke-extract-*.md")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Run the extractor
	cmd := exec.CommandContext(ctx,
		"go", "run", extractorPath,
		"--input", pdfPath,
		"--output", tmpPath,
	)

	p.logger.Debug().
		Str("pdf_path", pdfPath).
		Str("output_path", tmpPath).
		Msg("Running PDF extractor")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("pdf extractor failed: %w, output: %s", err, string(output))
	}

	// Read the extracted Markdown
	content, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf("read extracted markdown: %w", err)
	}

	return string(content), nil
}

// createDocumentSource creates a document source record.
func (p *Pipeline) createDocumentSource(ctx context.Context, req IngestionRequest, content string) (*storage.DocumentSource, error) {
	// Calculate SHA256
	hash := sha256.Sum256([]byte(content))
	sha256Hex := hex.EncodeToString(hash[:])

	// Determine storage URI
	storageURI := req.MarkdownPath
	if storageURI == "" {
		storageURI = req.PDFPath
	}
	if !filepath.IsAbs(storageURI) {
		absPath, err := filepath.Abs(storageURI)
		if err == nil {
			storageURI = absPath
		}
	}

	docSource := &storage.DocumentSource{
		ID:                uuid.New(),
		TenantID:          req.TenantID,
		ProductID:         req.ProductID,
		CampaignVariantID: &req.CampaignID,
		StorageURI:        storageURI,
		SHA256:            sha256Hex,
		UploadedBy:        &req.Operator,
		UploadedAt:        time.Now(),
	}

	// Persist to database
	if p.repos != nil && p.repos.DocumentSources != nil {
		if err := p.repos.DocumentSources.Create(ctx, docSource); err != nil {
			return nil, fmt.Errorf("persist document source: %w", err)
		}
		p.logger.Debug().
			Str("doc_source_id", docSource.ID.String()).
			Str("sha256", sha256Hex).
			Msg("Created document source")
	}

	return docSource, nil
}

// SpecsResult holds the result of storing specs.
type SpecsResult struct {
	Created   int
	Updated   int
	Conflicts []uuid.UUID
}

// storeSpecs persists spec values, handling deduplication and conflicts.
func (p *Pipeline) storeSpecs(ctx context.Context, req IngestionRequest, specs []ParsedSpec, docSourceID uuid.UUID) (*SpecsResult, error) {
	result := &SpecsResult{}

	if p.repos == nil {
		return result, fmt.Errorf("repositories not initialized")
	}

	for _, spec := range specs {
		// Look up or create spec category
		category, err := p.repos.SpecCategories.GetOrCreate(ctx, spec.Category)
		if err != nil {
			p.logger.Warn().
				Err(err).
				Str("category", spec.Category).
				Msg("Failed to get or create spec category")
			continue
		}

		// Look up or create spec item
		var unitPtr *string
		if spec.Unit != "" {
			unitPtr = &spec.Unit
		}
		specItem, err := p.repos.SpecItems.GetOrCreate(ctx, category.ID, spec.Name, unitPtr)
		if err != nil {
			p.logger.Warn().
				Err(err).
				Str("category", spec.Category).
				Str("name", spec.Name).
				Msg("Failed to get or create spec item")
			continue
		}

		// Generate deterministic ID
		specID := GenerateSpecID(req.TenantID, req.ProductID, spec.Category, spec.Name)

		// Check for existing spec with same ID
		existingSpecs, err := p.repos.SpecValues.GetByCampaign(ctx, req.TenantID, req.CampaignID)
		existing := false
		var existingSpec *storage.SpecValue
		if err == nil {
			for _, es := range existingSpecs {
				if es.ID == specID {
					existing = true
					existingSpec = es
					break
				}
			}
		}

		specValue := &storage.SpecValue{
			ID:                specID,
			TenantID:          req.TenantID,
			ProductID:         req.ProductID,
			CampaignVariantID: req.CampaignID,
			SpecItemID:        specItem.ID,
			ValueText:         &spec.Value,
			Unit:              unitPtr,
			Confidence:        spec.Confidence,
			Status:            storage.SpecStatusActive,
			SourceDocID:       &docSourceID,
			SourcePage:        &spec.SourcePage,
			Version:           1,
		}

		if spec.Numeric != nil {
			specValue.ValueNumeric = spec.Numeric
		}

		if existing {
			// Check for conflict: values differ and both have high confidence
			if existingSpec != nil {
				valueChanged := false
				if spec.Numeric != nil && existingSpec.ValueNumeric != nil {
					valueChanged = *spec.Numeric != *existingSpec.ValueNumeric
				} else if spec.Value != "" && existingSpec.ValueText != nil {
					valueChanged = spec.Value != *existingSpec.ValueText
				}

				if valueChanged && spec.Confidence > 0.8 && existingSpec.Confidence > 0.8 {
					specValue.Status = storage.SpecStatusConflict
					result.Conflicts = append(result.Conflicts, specID)
					p.logger.Warn().
						Str("spec_id", specID.String()).
						Str("category", spec.Category).
						Str("name", spec.Name).
						Msg("Spec conflict detected")
				}
			}
			result.Updated++
		} else {
			result.Created++
		}

		// Persist to database
		if err := p.repos.SpecValues.Create(ctx, specValue); err != nil {
			p.logger.Warn().
				Err(err).
				Str("spec_id", specID.String()).
				Str("category", spec.Category).
				Str("name", spec.Name).
				Msg("Failed to persist spec value")
			continue
		}

		// Record lineage event
		if p.lineageWriter != nil {
			_ = p.lineageWriter.RecordSpecCreation(ctx, req.TenantID, req.ProductID, req.CampaignID, specID, &docSourceID, nil)
		}

		p.logger.Debug().
			Str("spec_id", specID.String()).
			Str("category", spec.Category).
			Str("name", spec.Name).
			Str("value", spec.Value).
			Msg("Persisted spec value")
	}

	return result, nil
}

// storeFeatures persists feature blocks.
func (p *Pipeline) storeFeatures(ctx context.Context, req IngestionRequest, features []ParsedFeature, docSourceID uuid.UUID) (int, error) {
	created := 0

	if p.repos == nil {
		return 0, fmt.Errorf("repositories not initialized")
	}

	for _, feature := range features {
		featureBlock := &storage.FeatureBlock{
			ID:                uuid.New(),
			TenantID:          req.TenantID,
			ProductID:         req.ProductID,
			CampaignVariantID: req.CampaignID,
			BlockType:         storage.BlockTypeFeature,
			Body:              feature.Body,
			Priority:          int16(feature.Priority),
			Tags:              feature.Tags,
			Shareability:      storage.ShareabilityPrivate,
			SourceDocID:       &docSourceID,
			SourcePage:        &feature.SourcePage,
		}

		// Persist to database
		if err := p.repos.FeatureBlocks.Create(ctx, featureBlock); err != nil {
			p.logger.Warn().
				Err(err).
				Str("feature_id", featureBlock.ID.String()).
				Msg("Failed to persist feature block")
			continue
		}

		// Record lineage event
		if p.lineageWriter != nil {
			_ = p.lineageWriter.RecordFeatureCreation(ctx, req.TenantID, req.ProductID, req.CampaignID, featureBlock.ID, string(storage.BlockTypeFeature), &docSourceID)
		}

		created++
		p.logger.Debug().
			Str("feature_id", featureBlock.ID.String()).
			Int("priority", feature.Priority).
			Msg("Persisted feature block")
	}

	return created, nil
}

// storeUSPs persists USP blocks.
func (p *Pipeline) storeUSPs(ctx context.Context, req IngestionRequest, usps []ParsedUSP, docSourceID uuid.UUID) (int, error) {
	created := 0

	if p.repos == nil {
		return 0, fmt.Errorf("repositories not initialized")
	}

	for _, usp := range usps {
		uspBlock := &storage.FeatureBlock{
			ID:                uuid.New(),
			TenantID:          req.TenantID,
			ProductID:         req.ProductID,
			CampaignVariantID: req.CampaignID,
			BlockType:         storage.BlockTypeUSP,
			Body:              usp.Body,
			Priority:          int16(usp.Priority),
			Tags:              usp.Tags,
			Shareability:      storage.ShareabilityPrivate,
			SourceDocID:       &docSourceID,
			SourcePage:        &usp.SourcePage,
		}

		// Persist to database
		if err := p.repos.FeatureBlocks.Create(ctx, uspBlock); err != nil {
			p.logger.Warn().
				Err(err).
				Str("usp_id", uspBlock.ID.String()).
				Msg("Failed to persist USP block")
			continue
		}

		// Record lineage event
		if p.lineageWriter != nil {
			_ = p.lineageWriter.RecordFeatureCreation(ctx, req.TenantID, req.ProductID, req.CampaignID, uspBlock.ID, string(storage.BlockTypeUSP), &docSourceID)
		}

		created++
		p.logger.Debug().
			Str("usp_id", uspBlock.ID.String()).
			Int("priority", usp.Priority).
			Msg("Persisted USP block")
	}

	return created, nil
}

// storeChunks persists knowledge chunks for semantic search.
func (p *Pipeline) storeChunks(ctx context.Context, req IngestionRequest, chunks []ParsedChunk, docSourceID uuid.UUID) (int, error) {
	created := 0

	if p.repos == nil {
		return 0, fmt.Errorf("repositories not initialized")
	}

	// Batch generate embeddings if embedder is available
	var embeddings [][]float32
	if p.embedder != nil && len(chunks) > 0 {
		texts := make([]string, len(chunks))
		for i, chunk := range chunks {
			texts[i] = chunk.Text
		}

		var err error
		embeddings, err = p.embedder.Embed(ctx, texts)
		if err != nil {
			p.logger.Warn().
				Err(err).
				Int("chunk_count", len(chunks)).
				Msg("Failed to generate embeddings, storing chunks without embeddings")
		}
	}

	embeddingModel := "unknown"
	embeddingVersion := "1.0"
	if p.embedder != nil {
		embeddingModel = p.embedder.Model()
	}

	// Prepare vector entries for batch insert
	var vectorEntries []retrieval.VectorEntry

	for i, chunk := range chunks {
		knowledgeChunk := &storage.KnowledgeChunk{
			ID:                uuid.New(),
			TenantID:          req.TenantID,
			ProductID:         req.ProductID,
			CampaignVariantID: &req.CampaignID,
			ChunkType:         chunk.ChunkType,
			Text:              chunk.Text,
			SourceDocID:       &docSourceID,
			SourcePage:        &chunk.StartLine,
			Visibility:        storage.VisibilityPrivate,
		}

		// Set embedding if available
		if i < len(embeddings) && len(embeddings[i]) > 0 {
			knowledgeChunk.EmbeddingVector = embeddings[i]
			knowledgeChunk.EmbeddingModel = &embeddingModel
			knowledgeChunk.EmbeddingVersion = &embeddingVersion
		}

		// Persist to database
		if err := p.repos.KnowledgeChunks.Create(ctx, knowledgeChunk); err != nil {
			p.logger.Warn().
				Err(err).
				Str("chunk_id", knowledgeChunk.ID.String()).
				Msg("Failed to persist knowledge chunk")
			continue
		}

		// Add to vector store if embedding is available
		if p.vectorAdapter != nil && len(knowledgeChunk.EmbeddingVector) > 0 {
			vectorEntries = append(vectorEntries, retrieval.VectorEntry{
				ID:                knowledgeChunk.ID,
				TenantID:          req.TenantID,
				ProductID:         req.ProductID,
				CampaignVariantID: &req.CampaignID,
				ChunkType:         string(knowledgeChunk.ChunkType),
				Visibility:        string(knowledgeChunk.Visibility),
				EmbeddingVersion:  embeddingVersion,
				Vector:            knowledgeChunk.EmbeddingVector,
				Metadata: map[string]interface{}{
					"text":       knowledgeChunk.Text,
					"source_doc": docSourceID.String(),
				},
			})
		}

		// Record lineage event
		if p.lineageWriter != nil {
			_ = p.lineageWriter.RecordChunkCreation(ctx, req.TenantID, req.ProductID, &req.CampaignID, knowledgeChunk.ID, string(chunk.ChunkType), &docSourceID)
		}

		created++
		p.logger.Debug().
			Str("chunk_id", knowledgeChunk.ID.String()).
			Str("chunk_type", string(chunk.ChunkType)).
			Bool("has_embedding", len(knowledgeChunk.EmbeddingVector) > 0).
			Msg("Persisted knowledge chunk")
	}

	// Batch insert into vector store
	if p.vectorAdapter != nil && len(vectorEntries) > 0 {
		if err := p.vectorAdapter.Insert(ctx, vectorEntries); err != nil {
			p.logger.Warn().
				Err(err).
				Int("vector_count", len(vectorEntries)).
				Msg("Failed to insert vectors into vector store")
		} else {
			p.logger.Debug().
				Int("vector_count", len(vectorEntries)).
				Msg("Inserted vectors into vector store")
		}
	}

	return created, nil
}

// emitLineageEvents records audit events for the ingestion.
func (p *Pipeline) emitLineageEvents(ctx context.Context, req IngestionRequest, result *IngestionResult) error {
	if p.lineageWriter == nil {
		p.logger.Debug().Msg("Lineage writer not configured, skipping lineage events")
		return nil
	}

	// Record ingestion start
	if err := p.lineageWriter.RecordIngestionStart(ctx, req.TenantID, req.ProductID, req.CampaignID, result.JobID, req.Operator); err != nil {
		p.logger.Warn().Err(err).Msg("Failed to record ingestion start")
	}

	// Record ingestion completion with stats
	stats := map[string]int{
		"specs_created":    result.SpecsCreated,
		"specs_updated":    result.SpecsUpdated,
		"features_created": result.FeaturesCreated,
		"usps_created":     result.USPsCreated,
		"chunks_created":   result.ChunksCreated,
		"conflicts":        len(result.ConflictingSpecs),
	}
	if err := p.lineageWriter.RecordIngestionComplete(ctx, req.TenantID, req.ProductID, req.CampaignID, result.JobID, stats); err != nil {
		p.logger.Warn().Err(err).Msg("Failed to record ingestion completion")
	}

	p.logger.Debug().
		Str("job_id", result.JobID.String()).
		Int("specs", result.SpecsCreated+result.SpecsUpdated).
		Int("features", result.FeaturesCreated).
		Int("usps", result.USPsCreated).
		Int("chunks", result.ChunksCreated).
		Msg("Emitted lineage events")

	return nil
}

// Deduplicate checks for duplicate content based on hash similarity.
func (p *Pipeline) Deduplicate(content string, threshold float64) (bool, string, error) {
	hash := sha256.Sum256([]byte(content))
	hashHex := hex.EncodeToString(hash[:])

	// TODO: Query database for existing document sources with similar hash
	// For now, return false (not a duplicate)

	return false, hashHex, nil
}
