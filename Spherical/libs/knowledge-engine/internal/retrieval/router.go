// Package retrieval provides hybrid retrieval services combining structured and semantic search.
package retrieval

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/cache"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// Intent represents the classified intent of a query.
type Intent string

const (
	IntentSpecLookup  Intent = "spec_lookup"
	IntentUSPLookup   Intent = "usp_lookup"
	IntentComparison  Intent = "comparison"
	IntentFAQ         Intent = "faq"
	IntentUnknown     Intent = "unknown"
)

// RetrievalRequest represents a knowledge retrieval query.
type RetrievalRequest struct {
	TenantID            uuid.UUID
	ProductIDs          []uuid.UUID
	CampaignVariantID   *uuid.UUID
	Question            string
	IntentHint          *Intent
	ConversationContext []ConversationMessage
	Filters             RetrievalFilters
	MaxChunks           int
	IncludeLineage      bool
}

// RetrievalFilters holds filtering options.
type RetrievalFilters struct {
	Categories []string
	ChunkTypes []storage.ChunkType
}

// ConversationMessage represents a conversation turn.
type ConversationMessage struct {
	Role    string
	Content string
}

// RetrievalResponse contains the retrieval results.
type RetrievalResponse struct {
	Intent          Intent
	LatencyMs       int64
	StructuredFacts []SpecFact
	SemanticChunks  []SemanticChunk
	Comparisons     []ComparisonResult
	Lineage         []LineageInfo
}

// SpecFact represents a structured specification fact.
type SpecFact struct {
	SpecItemID        uuid.UUID
	Category          string
	Name              string
	Value             string
	Unit              string
	Confidence        float64
	CampaignVariantID uuid.UUID
	Source            SourceRef
}

// SemanticChunk represents a retrieved semantic chunk.
type SemanticChunk struct {
	ChunkID   uuid.UUID
	ChunkType storage.ChunkType
	Text      string
	Distance  float32
	Score     float32
	Metadata  map[string]interface{}
	Source    SourceRef
}

// SourceRef contains source document reference.
type SourceRef struct {
	DocumentSourceID *uuid.UUID
	Page             *int
	URL              *string
}

// ComparisonResult represents a comparison row.
type ComparisonResult struct {
	Dimension          string
	PrimaryProductID   uuid.UUID
	SecondaryProductID uuid.UUID
	PrimaryValue       string
	SecondaryValue     string
	Verdict            storage.Verdict
	Narrative          string
}

// LineageInfo contains lineage metadata.
type LineageInfo struct {
	ResourceType     string
	ResourceID       uuid.UUID
	Action           storage.LineageAction
	DocumentSourceID *uuid.UUID
	OccurredAt       time.Time
}

// Router orchestrates hybrid retrieval combining structured and semantic search.
type Router struct {
	logger           *observability.Logger
	cache            cache.Client
	vectorAdapter    VectorAdapter
	intentClassifier *IntentClassifier
	config           RouterConfig
}

// RouterConfig holds router configuration.
type RouterConfig struct {
	MaxChunks                 int
	StructuredFirst           bool
	SemanticFallback          bool
	IntentConfidenceThreshold float64
	CacheResults              bool
	CacheTTL                  time.Duration
}

// NewRouter creates a new retrieval router.
func NewRouter(
	logger *observability.Logger,
	cache cache.Client,
	vectorAdapter VectorAdapter,
	cfg RouterConfig,
) *Router {
	if cfg.MaxChunks <= 0 {
		cfg.MaxChunks = 8
	}
	if cfg.IntentConfidenceThreshold <= 0 {
		cfg.IntentConfidenceThreshold = 0.7
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 5 * time.Minute
	}

	return &Router{
		logger:           logger,
		cache:            cache,
		vectorAdapter:    vectorAdapter,
		intentClassifier: NewIntentClassifier(),
		config:           cfg,
	}
}

// Query executes a hybrid retrieval query.
func (r *Router) Query(ctx context.Context, req RetrievalRequest) (*RetrievalResponse, error) {
	start := time.Now()

	// Apply defaults
	if req.MaxChunks <= 0 {
		req.MaxChunks = r.config.MaxChunks
	}

	// Classify intent
	intent := r.classifyIntent(req)

	r.logger.Debug().
		Str("tenant_id", req.TenantID.String()).
		Str("question", req.Question).
		Str("intent", string(intent)).
		Msg("Processing retrieval query")

	response := &RetrievalResponse{
		Intent: intent,
	}

	// Check cache
	if r.config.CacheResults {
		cacheKey := r.buildCacheKey(req)
		if cached, err := r.checkCache(ctx, cacheKey); err == nil && cached != nil {
			r.logger.Debug().Msg("Cache hit")
			cached.LatencyMs = time.Since(start).Milliseconds()
			return cached, nil
		}
	}

	// Route based on intent
	switch intent {
	case IntentSpecLookup:
		if r.config.StructuredFirst {
			facts, err := r.queryStructuredSpecs(ctx, req)
			if err != nil {
				r.logger.Warn().Err(err).Msg("Structured query failed")
			} else {
				response.StructuredFacts = facts
			}
		}

		// Fallback to semantic if no structured results
		if len(response.StructuredFacts) == 0 && r.config.SemanticFallback {
			chunks, err := r.querySemanticChunks(ctx, req)
			if err != nil {
				r.logger.Warn().Err(err).Msg("Semantic query failed")
			} else {
				response.SemanticChunks = chunks
			}
		}

	case IntentUSPLookup:
		// USPs are primarily in semantic chunks
		chunks, err := r.querySemanticChunks(ctx, req)
		if err != nil {
			r.logger.Warn().Err(err).Msg("Semantic query failed")
		} else {
			response.SemanticChunks = chunks
		}

	case IntentComparison:
		// Query comparison rows if available
		comparisons, err := r.queryComparisons(ctx, req)
		if err != nil {
			r.logger.Warn().Err(err).Msg("Comparison query failed")
		} else {
			response.Comparisons = comparisons
		}

		// Also get semantic context
		chunks, err := r.querySemanticChunks(ctx, req)
		if err == nil {
			response.SemanticChunks = chunks
		}

	default:
		// Unknown intent: try both paths
		facts, _ := r.queryStructuredSpecs(ctx, req)
		response.StructuredFacts = facts

		chunks, _ := r.querySemanticChunks(ctx, req)
		response.SemanticChunks = chunks
	}

	// Add lineage if requested
	if req.IncludeLineage {
		lineage, err := r.queryLineage(ctx, req, response)
		if err == nil {
			response.Lineage = lineage
		}
	}

	response.LatencyMs = time.Since(start).Milliseconds()

	// Cache result
	if r.config.CacheResults && r.cache != nil {
		cacheKey := r.buildCacheKey(req)
		_ = r.cacheResult(ctx, cacheKey, response)
	}

	r.logger.Info().
		Int64("latency_ms", response.LatencyMs).
		Int("structured_facts", len(response.StructuredFacts)).
		Int("semantic_chunks", len(response.SemanticChunks)).
		Msg("Retrieval complete")

	return response, nil
}

// classifyIntent determines the query intent.
func (r *Router) classifyIntent(req RetrievalRequest) Intent {
	// Use hint if provided and confident
	if req.IntentHint != nil {
		return *req.IntentHint
	}

	// Use classifier
	intent, confidence := r.intentClassifier.Classify(req.Question)
	if confidence >= r.config.IntentConfidenceThreshold {
		return intent
	}

	return IntentUnknown
}

// queryStructuredSpecs retrieves structured specification facts.
func (r *Router) queryStructuredSpecs(ctx context.Context, req RetrievalRequest) ([]SpecFact, error) {
	// TODO: Implement actual database query using sqlc-generated code
	// For now, return placeholder

	r.logger.Debug().Msg("Querying structured specs")

	// This would query spec_view_latest with filters
	// SELECT * FROM spec_view_latest
	// WHERE tenant_id = $1
	//   AND product_id = ANY($2)
	//   AND (campaign_variant_id = $3 OR $3 IS NULL)
	//   AND (category_name = ANY($4) OR $4 IS NULL)

	return nil, nil
}

// querySemanticChunks retrieves semantic chunks via vector search.
func (r *Router) querySemanticChunks(ctx context.Context, req RetrievalRequest) ([]SemanticChunk, error) {
	r.logger.Debug().Msg("Querying semantic chunks")

	// TODO: Generate embedding for the question
	// For now, use a placeholder embedding
	queryVector := make([]float32, 768)

	// Build filters
	filters := VectorFilters{
		TenantID:   &req.TenantID,
		ProductIDs: req.ProductIDs,
	}
	if req.CampaignVariantID != nil {
		filters.CampaignVariantID = req.CampaignVariantID
	}
	if len(req.Filters.ChunkTypes) > 0 {
		for _, ct := range req.Filters.ChunkTypes {
			filters.ChunkTypes = append(filters.ChunkTypes, string(ct))
		}
	}

	// Execute vector search
	results, err := r.vectorAdapter.Search(ctx, queryVector, req.MaxChunks, filters)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	// Convert to response format
	chunks := make([]SemanticChunk, len(results))
	for i, result := range results {
		chunks[i] = SemanticChunk{
			ChunkID:   result.ID,
			ChunkType: storage.ChunkTypeGlobal, // Would be extracted from metadata
			Distance:  result.Distance,
			Score:     result.Score,
			Metadata:  result.Metadata,
		}
	}

	return chunks, nil
}

// queryComparisons retrieves pre-computed comparison rows.
func (r *Router) queryComparisons(ctx context.Context, req RetrievalRequest) ([]ComparisonResult, error) {
	r.logger.Debug().Msg("Querying comparisons")

	// TODO: Implement comparison query
	// SELECT * FROM comparison_rows
	// WHERE primary_product_id = ANY($1)
	//   AND shareability IN ('tenant_only', 'public')

	return nil, nil
}

// queryLineage retrieves lineage information for results.
func (r *Router) queryLineage(ctx context.Context, req RetrievalRequest, resp *RetrievalResponse) ([]LineageInfo, error) {
	r.logger.Debug().Msg("Querying lineage")

	// TODO: Implement lineage query based on returned resource IDs

	return nil, nil
}

// buildCacheKey creates a cache key for the request.
func (r *Router) buildCacheKey(req RetrievalRequest) string {
	parts := []string{
		"retrieval",
		req.TenantID.String(),
		req.Question,
	}

	for _, pid := range req.ProductIDs {
		parts = append(parts, pid.String())
	}

	if req.CampaignVariantID != nil {
		parts = append(parts, req.CampaignVariantID.String())
	}

	return cache.CacheKey(parts...)
}

// checkCache attempts to retrieve a cached response.
func (r *Router) checkCache(ctx context.Context, key string) (*RetrievalResponse, error) {
	if r.cache == nil {
		return nil, nil
	}

	// TODO: Implement cache deserialization
	return nil, cache.ErrCacheMiss
}

// cacheResult stores the response in cache.
func (r *Router) cacheResult(ctx context.Context, key string, resp *RetrievalResponse) error {
	if r.cache == nil {
		return nil
	}

	// TODO: Implement cache serialization
	return nil
}

// IntentClassifier classifies query intent using rules and patterns.
type IntentClassifier struct {
	specPatterns       []string
	uspPatterns        []string
	comparisonPatterns []string
	faqPatterns        []string
}

// NewIntentClassifier creates a new intent classifier.
func NewIntentClassifier() *IntentClassifier {
	return &IntentClassifier{
		specPatterns: []string{
			"what is the",
			"what's the",
			"how much",
			"how many",
			"fuel efficiency",
			"mileage",
			"horsepower",
			"torque",
			"engine",
			"dimensions",
			"weight",
			"capacity",
			"range",
			"tell me about the",
			"displacement",
		},
		uspPatterns: []string{
			"why should",
			"what makes",
			"unique",
			"special",
			"best feature",
			"advantage",
			"benefit",
			"selling point",
		},
		comparisonPatterns: []string{
			"compare",
			"comparison",
			"versus",
			"vs",
			"better than",
			"difference between",
			"how does it compare",
			"which is better",
			"or accord",
			"or camry",
		},
		faqPatterns: []string{
			"how do i",
			"how can i",
			"can i",
			"is it possible",
			"what if",
			"help me",
		},
	}
}

// Classify determines the intent and confidence for a question.
func (c *IntentClassifier) Classify(question string) (Intent, float64) {
	q := strings.ToLower(question)

	// Check comparison patterns first (highest priority)
	for _, pattern := range c.comparisonPatterns {
		if strings.Contains(q, pattern) {
			return IntentComparison, 0.9
		}
	}

	// Check USP patterns next (before spec to catch "special", "unique", etc.)
	for _, pattern := range c.uspPatterns {
		if strings.Contains(q, pattern) {
			return IntentUSPLookup, 0.85
		}
	}

	// Check FAQ patterns
	for _, pattern := range c.faqPatterns {
		if strings.Contains(q, pattern) {
			return IntentFAQ, 0.8
		}
	}

	// Check spec patterns - score based on matches
	specMatches := 0
	for _, pattern := range c.specPatterns {
		if strings.Contains(q, pattern) {
			specMatches++
		}
	}
	
	// Calculate spec confidence based on number of matches
	// 1 match = 0.6, 2 matches = 0.8, 3+ matches = 0.9
	if specMatches > 0 {
		specConf := 0.5 + float64(specMatches)*0.15
		if specConf > 0.95 {
			specConf = 0.95
		}
		return IntentSpecLookup, specConf
	}

	// Default to unknown with low confidence
	return IntentUnknown, 0.3
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

