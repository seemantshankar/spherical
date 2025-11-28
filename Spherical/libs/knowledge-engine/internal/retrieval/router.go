// Package retrieval provides hybrid retrieval services combining structured and semantic search.
package retrieval

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/cache"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/embedding"
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
	embedder         embedding.Embedder // For generating query embeddings
	intentClassifier *IntentClassifier
	specViewRepo     *storage.SpecViewRepository
	config           RouterConfig
	metrics          *RouterMetrics
}


// RouterConfig holds router configuration.
type RouterConfig struct {
	MaxChunks                 int
	StructuredFirst           bool
	SemanticFallback          bool
	IntentConfidenceThreshold float64
	KeywordConfidenceThreshold float64 // Threshold for keyword-only path (default 0.8)
	CacheResults              bool
	CacheTTL                  time.Duration
}

// RouterMetrics tracks router performance metrics.
type RouterMetrics struct {
	KeywordOnlyCount int64
	HybridCount      int64
	VectorOnlyCount  int64
	KeywordLatencyMs []int64
	HybridLatencyMs  []int64
	VectorLatencyMs  []int64
	ConfidenceScores []float64
}

// NewRouterMetrics creates a new metrics tracker.
func NewRouterMetrics() *RouterMetrics {
	return &RouterMetrics{
		KeywordLatencyMs: make([]int64, 0),
		HybridLatencyMs:  make([]int64, 0),
		VectorLatencyMs:  make([]int64, 0),
		ConfidenceScores: make([]float64, 0),
	}
}

// NewRouter creates a new retrieval router.
func NewRouter(
	logger *observability.Logger,
	cache cache.Client,
	vectorAdapter VectorAdapter,
	embedder embedding.Embedder,
	specViewRepo *storage.SpecViewRepository,
	cfg RouterConfig,
) *Router {
	if cfg.MaxChunks <= 0 {
		cfg.MaxChunks = 8
	}
	if cfg.IntentConfidenceThreshold <= 0 {
		cfg.IntentConfidenceThreshold = 0.7
	}
	if cfg.KeywordConfidenceThreshold <= 0 {
		cfg.KeywordConfidenceThreshold = 0.8
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 5 * time.Minute
	}

	return &Router{
		logger:           logger,
		cache:            cache,
		vectorAdapter:    vectorAdapter,
		embedder:         embedder,
		intentClassifier: NewIntentClassifier(),
		specViewRepo:     specViewRepo,
		config:           cfg,
		metrics:          NewRouterMetrics(),
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

	// Track which path was used for metrics
	usedVectorSearch := false

	// Route based on intent
	switch intent {
	case IntentSpecLookup:
		if r.config.StructuredFirst {
			facts, confidence, err := r.queryStructuredSpecs(ctx, req)
			if err != nil {
				r.logger.Warn().Err(err).Msg("Structured query failed")
				// On error, still try vector search as fallback
				facts = nil
				confidence = 0.0
			}
			
			response.StructuredFacts = facts
			if confidence > 0 {
				r.metrics.ConfidenceScores = append(r.metrics.ConfidenceScores, confidence)
			}

			// Only fallback to vector if NO results found (don't fallback if we have results, even with low confidence)
			// Vector search should only be used when keyword search completely fails
			if len(facts) == 0 && r.config.SemanticFallback {
				r.logger.Debug().
					Int("facts_count", len(facts)).
					Float64("confidence", confidence).
					Msg("No keyword results, triggering vector search fallback")
				chunks, err := r.querySemanticChunks(ctx, req)
				if err != nil {
					r.logger.Warn().Err(err).Msg("Semantic query failed")
				} else {
					response.SemanticChunks = chunks
					if len(chunks) > 0 {
						usedVectorSearch = true
						r.metrics.HybridCount++
						r.logger.Debug().Int("chunks_found", len(chunks)).Msg("Vector search found chunks")
					} else {
						r.logger.Debug().Msg("Vector search returned no chunks")
					}
				}
			} else if len(facts) > 0 {
				r.metrics.KeywordOnlyCount++
				// Don't run vector search if keyword search found results - it's fast and sufficient
			}
		} else {
			// StructuredFirst is false, go straight to vector
			chunks, err := r.querySemanticChunks(ctx, req)
			if err != nil {
				r.logger.Warn().Err(err).Msg("Semantic query failed")
			} else {
				response.SemanticChunks = chunks
				usedVectorSearch = true
				r.metrics.VectorOnlyCount++
			}
		}

	case IntentUSPLookup:
		// USPs are stored as semantic chunks with chunk_type='usp'
		// Filter vector search to only return USP chunks for better accuracy
		req.Filters.ChunkTypes = []storage.ChunkType{storage.ChunkTypeUSP}
		chunks, err := r.querySemanticChunks(ctx, req)
		if err != nil {
			r.logger.Warn().Err(err).Msg("Semantic query failed")
		} else {
			response.SemanticChunks = chunks
			if len(chunks) > 0 {
				usedVectorSearch = true
				r.metrics.VectorOnlyCount++
				r.logger.Debug().Int("chunks_found", len(chunks)).Msg("Found USP chunks")
			} else {
				r.logger.Debug().Msg("No USP chunks found in vector database")
			}
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
			usedVectorSearch = true
			r.metrics.HybridCount++
		}

	default:
		// Unknown intent: try structured first, only fallback to vector if low confidence or no results
		facts, confidence, _ := r.queryStructuredSpecs(ctx, req)
		response.StructuredFacts = facts

		// Only try vector search if structured search had low confidence or no results
		// If we found structured facts with decent confidence, prefer those and skip semantic chunks
		if len(facts) == 0 && r.config.SemanticFallback {
			// No structured facts found, try semantic search
			chunks, _ := r.querySemanticChunks(ctx, req)
			if len(chunks) > 0 {
				response.SemanticChunks = chunks
				usedVectorSearch = true
				r.metrics.HybridCount++
			}
		} else if confidence < r.config.KeywordConfidenceThreshold && len(facts) > 0 && r.config.SemanticFallback {
			// We have facts but low confidence - add semantic chunks for context, but be selective
			chunks, _ := r.querySemanticChunks(ctx, req)
			// Only add top 3 most relevant semantic chunks when we already have structured facts
			maxSemanticChunks := 3
			if len(chunks) > maxSemanticChunks {
				chunks = chunks[:maxSemanticChunks]
			}
			if len(chunks) > 0 {
				response.SemanticChunks = chunks
				usedVectorSearch = true
				r.metrics.HybridCount++
			}
		} else if len(facts) > 0 {
			// We have facts with good confidence, don't add semantic chunks to avoid noise
			r.metrics.KeywordOnlyCount++
		}
	}

	// Add lineage if requested
	if req.IncludeLineage {
		lineage, err := r.queryLineage(ctx, req, response)
		if err == nil {
			response.Lineage = lineage
		}
	}

	response.LatencyMs = time.Since(start).Milliseconds()

	// Track latency metrics
	if usedVectorSearch {
		if len(response.StructuredFacts) > 0 {
			r.metrics.HybridLatencyMs = append(r.metrics.HybridLatencyMs, response.LatencyMs)
		} else {
			r.metrics.VectorLatencyMs = append(r.metrics.VectorLatencyMs, response.LatencyMs)
		}
	} else {
		r.metrics.KeywordLatencyMs = append(r.metrics.KeywordLatencyMs, response.LatencyMs)
	}

	// Only cache if vector search was used (not keyword-only results)
	if r.config.CacheResults && r.cache != nil && usedVectorSearch {
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

// queryStructuredSpecs retrieves structured specification facts via keyword search.
// Returns facts, confidence score, and error.
func (r *Router) queryStructuredSpecs(ctx context.Context, req RetrievalRequest) ([]SpecFact, float64, error) {
	r.logger.Debug().Msg("Querying structured specs")

	if r.specViewRepo == nil {
		// If spec view repo is not configured, return empty results with low confidence
		// This allows the router to fall back to vector search
		return nil, 0.0, nil
	}

	// Extract keywords from query
	keywords := r.extractKeywords(req.Question)
	r.logger.Debug().
		Str("query", req.Question).
		Strs("keywords", keywords).
		Msg("Extracted keywords")
	if len(keywords) == 0 {
		return nil, 0.0, nil
	}

	// Determine search limit based on query type
	searchLimit := 50
	if len(keywords) > 1 {
		searchLimit = 100 // Get more results per keyword when querying multiple keywords
	}
	
	// Perform keyword search for each keyword
	factMap := make(map[string]*SpecFact)
	for _, keyword := range keywords {
		// Search using SpecViewRepository
		specs, err := r.specViewRepo.SearchByKeyword(ctx, req.TenantID, keyword, searchLimit)
		if err != nil {
			r.logger.Warn().Err(err).Str("keyword", keyword).Msg("Keyword search failed")
			continue
		}
		r.logger.Debug().Str("keyword", keyword).Int("results", len(specs)).Msg("Keyword search results")

		// Also try singular/plural variations and spelling variants
		// Always try variants (not just when len(specs) == 0) to improve coverage
		variantsToTry := []string{}
		
		// Handle irregular plurals
		keywordLower := strings.ToLower(keyword)
		irregularPlurals := map[string]string{
			"children": "child",
			"child":    "children",
			"babies":   "baby",
			"baby":     "babies",
			"people":   "person",
			"person":   "people",
		}
		
		if variant, ok := irregularPlurals[keywordLower]; ok {
			variantsToTry = append(variantsToTry, variant)
		}
		
		// Try singular/plural (regular forms)
		if keywordLower != "children" && keywordLower != "child" {
			if strings.HasSuffix(keyword, "s") && len(keyword) > 1 && keywordLower != "children" {
				variantsToTry = append(variantsToTry, keyword[:len(keyword)-1])
			} else {
				variantsToTry = append(variantsToTry, keyword+"s")
			}
		}
		
		// Try spelling variants (colour/color, etc.)
		if keywordLower == "colours" || keywordLower == "colour" {
			variantsToTry = append(variantsToTry, "color", "colors")
		} else if keywordLower == "colors" || keywordLower == "color" {
			variantsToTry = append(variantsToTry, "colour", "colours")
		}
		
		// Try all variants and merge results
		for _, variant := range variantsToTry {
			if variant != "" && variant != keyword {
				variantSpecs, err := r.specViewRepo.SearchByKeyword(ctx, req.TenantID, variant, searchLimit)
				if err == nil && len(variantSpecs) > 0 {
					specs = append(specs, variantSpecs...)
					r.logger.Debug().Str("variant", variant).Int("results", len(variantSpecs)).Msg("Variant search found results")
				}
			}
		}

		// Convert to SpecFact and deduplicate
		for _, sv := range specs {
			// Filter by product IDs if specified
			if len(req.ProductIDs) > 0 {
				found := false
				for _, pid := range req.ProductIDs {
					if sv.ProductID == pid {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			// Filter by campaign variant if specified
			if req.CampaignVariantID != nil && sv.CampaignVariantID != *req.CampaignVariantID {
				continue
			}

			key := fmt.Sprintf("%s|%s|%s", sv.CategoryName, sv.SpecName, sv.Value)
			if existing, ok := factMap[key]; ok {
				// Keep the one with higher confidence
				if sv.Confidence > existing.Confidence {
					existing.Confidence = sv.Confidence
				}
			} else {
				unit := ""
				if sv.Unit != nil {
					unit = *sv.Unit
				}
				factMap[key] = &SpecFact{
					SpecItemID:        sv.SpecItemID,
					Category:           sv.CategoryName,
					Name:               sv.SpecName,
					Value:              sv.Value,
					Unit:               unit,
					Confidence:         sv.Confidence,
					CampaignVariantID:  sv.CampaignVariantID,
					Source: SourceRef{
						DocumentSourceID: sv.SourceDocID,
						Page:             sv.SourcePage,
					},
				}
			}
		}
	}

	// Convert map to slice and rank by relevance
	facts := make([]SpecFact, 0, len(factMap))
	for _, fact := range factMap {
		facts = append(facts, *fact)
	}

	// Rank facts by relevance to query keywords
	facts = r.rankFactsByRelevance(facts, keywords, req.Question)

	// Filter out low-relevance facts
	facts = r.filterLowRelevanceFacts(facts, keywords)

	// Limit to top results to avoid noise, but keep more for color searches or multi-keyword queries
	maxResults := 30 // Increased default to handle multi-keyword queries better
	queryLower := strings.ToLower(req.Question)
	// Check for color-related queries (handle both singular/plural and US/UK spelling)
	if strings.Contains(queryLower, "color") || strings.Contains(queryLower, "colour") || 
	   strings.Contains(queryLower, "colors") || strings.Contains(queryLower, "colours") {
		maxResults = 100 // Allow many results for color queries to get all color options
	}
	// Multi-keyword queries - limit results more aggressively for focused queries
	if len(keywords) >= 2 {
		// For focused 2-keyword queries (like "child seat"), limit to top 5
		if len(keywords) == 2 {
			maxResults = 5 // Focused queries - only top 5 most relevant
		} else {
			maxResults = 60 // Multi-keyword queries like "weight wheels colors length" need more results
		}
	}
	if len(facts) > maxResults {
		facts = facts[:maxResults]
	}

	// Calculate confidence
	confidence := r.calculateKeywordConfidence(facts, req.Question)

	// If high confidence, log and return early
	if confidence >= r.config.KeywordConfidenceThreshold {
		r.logger.Debug().
			Float64("confidence", confidence).
			Int("results", len(facts)).
			Msg("Keyword search sufficient, skipping vector search")
		return facts, confidence, nil
	}

	// Low confidence - will trigger vector fallback
	r.logger.Debug().
		Float64("confidence", confidence).
		Msg("Keyword search low confidence, will use vector fallback")
	return facts, confidence, nil
}

// querySemanticChunks retrieves semantic chunks via vector search.
func (r *Router) querySemanticChunks(ctx context.Context, req RetrievalRequest) ([]SemanticChunk, error) {
	r.logger.Debug().Msg("Querying semantic chunks")

	// Generate embedding for the question
	var queryVector []float32
	var err error
	if r.embedder != nil {
		queryVector, err = r.embedder.EmbedSingle(ctx, req.Question)
		if err != nil {
			r.logger.Warn().Err(err).Msg("Failed to generate query embedding, skipping vector search")
			return nil, err
		}
	} else {
		r.logger.Warn().Msg("No embedder configured, cannot perform vector search")
		return nil, fmt.Errorf("embedder not configured")
	}

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
		r.logger.Warn().Err(err).Msg("Vector search failed")
		return nil, fmt.Errorf("vector search: %w", err)
	}

	r.logger.Debug().
		Int("results_count", len(results)).
		Int("query_dimension", len(queryVector)).
		Msg("Vector search completed")

	// Filter by relevance score - only return chunks with good similarity scores
	// Cosine similarity scores range from 0-1, with higher being more similar
	// Distance is the inverse (1 - similarity), so lower distance = higher similarity
	minScore := float32(0.4) // Increased minimum similarity score (40% match) - be more selective
	if len(results) > 0 {
		// Get the best score to use as reference
		bestScore := results[0].Score
		// If best score is good (>0.6), we can be very selective
		// If best score is lower, we need to be more lenient
		if bestScore > 0.6 {
			minScore = bestScore * 0.65 // Accept chunks within 65% of best score
		} else if bestScore > 0.4 {
			minScore = bestScore * 0.75 // Accept chunks within 75% of best score if best is mediocre
		} else if bestScore > 0.3 {
			minScore = bestScore * 0.8 // Accept chunks within 80% of best score if best is low
		} else {
			// If best score is very low (<0.3), only return top 3 most relevant
			minScore = 0.25 // Very low threshold, but we'll limit results
		}
	}

	// Convert to response format and filter by relevance
	filteredChunks := make([]SemanticChunk, 0, len(results))
	queryKeywords := r.extractKeywords(req.Question)
	
	maxChunksToReturn := 10 // Default limit
	if len(results) > 0 && results[0].Score < 0.3 {
		maxChunksToReturn = 3 // Very strict limit if best match is poor
	}
	
	for _, result := range results {
		// Filter by minimum score threshold
		if result.Score >= minScore {
			chunk := SemanticChunk{
				ChunkID:   result.ID,
				ChunkType: storage.ChunkTypeGlobal, // Would be extracted from metadata
				Distance:  result.Distance,
				Score:     result.Score,
				Metadata:  result.Metadata,
			}
			// Extract chunk type from metadata if available
			if result.Metadata != nil {
				if ct, ok := result.Metadata["chunk_type"].(string); ok {
					chunk.ChunkType = storage.ChunkType(ct)
				}
				// Additional text-based relevance check using chunk text
				if chunkText, ok := result.Metadata["text"].(string); ok && len(queryKeywords) > 0 {
					chunkTextLower := strings.ToLower(chunkText)
					// Check if any keywords appear in the chunk text
					hasKeywordMatch := false
					for _, kw := range queryKeywords {
						if strings.Contains(chunkTextLower, strings.ToLower(kw)) {
							hasKeywordMatch = true
							break
						}
					}
					// If no keyword match and score is mediocre, skip it
					if !hasKeywordMatch && result.Score < 0.5 {
						continue
					}
				}
			}
			filteredChunks = append(filteredChunks, chunk)
			
			// Limit total results
			if len(filteredChunks) >= maxChunksToReturn {
				break
			}
		}
	}

	r.logger.Debug().
		Int("original_results", len(results)).
		Int("filtered_chunks", len(filteredChunks)).
		Float64("min_score", float64(minScore)).
		Msg("Filtered semantic chunks by relevance")

	return filteredChunks, nil
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
			"what are",
			"what is",
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
			"color",
			"colour",
			"paint",
			"exterior color",
			"interior color",
			"available in",
			"come in",
			"what colors",
			"what colours",
			"speaker",
			"speakers",
			"audio",
			"audio system",
			"sound system",
			"sound",
			"jbl",
			"music",
			"stereo",
			"material",
			"materials",
			"upholstery",
			"leather",
			"fabric",
			"what material",
			"exterior",
			"interior",
			"screen",
			"display",
			"safe",
			"safety",
			"baby",
			"babies",
			"child",
			"children",
			"isofix",
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
			"usp",
			"usps",
			"what are the usps",
			"what are the unique selling",
			"unique selling points",
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

// isSingleWordQuery checks if the query is essentially a single word/keyword.
func (c *IntentClassifier) isSingleWordQuery(query string) bool {
	words := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	// Remove stop words and punctuation
	nonStopWords := 0
	for _, word := range words {
		word = strings.Trim(word, ".,!?;:()[]{}'\"")
		if len(word) > 1 && !c.isStopWord(word) {
			nonStopWords++
		}
	}
	return nonStopWords <= 1
}

// isStopWord checks if a word is a stop word.
func (c *IntentClassifier) isStopWord(word string) bool {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "from": true,
		"is": true, "are": true, "was": true, "were": true, "be": true,
		"been": true, "being": true, "have": true, "has": true, "had": true,
		"do": true, "does": true, "did": true, "will": true, "would": true,
		"should": true, "could": true, "may": true, "might": true, "must": true,
		"can": true, "what": true, "which": true, "who": true, "where": true,
		"when": true, "why": true, "how": true, "about": true, "tell": true,
		"me": true, "my": true, "your": true, "our": true, "their": true,
		"this": true, "that": true, "these": true, "those": true,
		"it": true, "its": true,
	}
	return stopWords[word]
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
	// 1 match = 0.7, 2 matches = 0.85, 3+ matches = 0.95
	if specMatches > 0 {
		specConf := 0.6 + float64(specMatches)*0.15
		if specConf > 0.95 {
			specConf = 0.95
		}
		return IntentSpecLookup, specConf
	}

	// Fallback: "what" questions are usually spec lookups
	if strings.HasPrefix(q, "what ") {
		return IntentSpecLookup, 0.6
	}

	// Fallback: "is", "does", "can" questions are usually spec lookups
	if strings.HasPrefix(q, "is ") || strings.HasPrefix(q, "does ") || strings.HasPrefix(q, "can ") {
		return IntentSpecLookup, 0.6
	}

	// Fallback: Single word queries are likely spec lookups (e.g., "exterior", "screen", "display", "child")
	if c.isSingleWordQuery(q) {
		return IntentSpecLookup, 0.75 // Increased confidence to pass 0.7 threshold
	}

	// Last resort: Most queries are spec lookups unless explicitly something else
	// Only return unknown if query is very short or clearly not a spec question
	if len(q) < 3 {
		return IntentUnknown, 0.3
	}
	
	// Default to spec lookup for most queries (better to try than return unknown)
	return IntentSpecLookup, 0.4
}

// calculateKeywordConfidence computes confidence for keyword search results.
func (r *Router) calculateKeywordConfidence(results []SpecFact, query string) float64 {
	if len(results) == 0 {
		return 0.0
	}

	queryLower := strings.ToLower(query)
	keywords := r.extractKeywords(queryLower)

	exactMatches := 0.0
	partialMatches := 0.0

	for _, fact := range results {
		// Check exact matches
		if strings.EqualFold(fact.Category, queryLower) ||
			strings.EqualFold(fact.Name, queryLower) ||
			strings.EqualFold(fact.Value, queryLower) {
			exactMatches += 0.3
		}

		// Check partial matches
		for _, kw := range keywords {
			if strings.Contains(strings.ToLower(fact.Category), kw) ||
				strings.Contains(strings.ToLower(fact.Name), kw) ||
				strings.Contains(strings.ToLower(fact.Value), kw) {
				partialMatches += 0.1
			}
		}
	}

	// Query complexity bonus/penalty
	complexityBonus := 0.0
	if len(keywords) <= 2 {
		complexityBonus = 0.2 // Simple queries get bonus
	} else if len(keywords) > 4 {
		complexityBonus = -0.1 // Complex queries get penalty
	}

	// Result count bonus (diminishing returns) - but only if results are actually relevant
	// Only give bonus if we have some exact or strong partial matches
	hasRelevantMatches := exactMatches > 0 || partialMatches > 0.3
	countBonus := 0.0
	if hasRelevantMatches && len(results) > 0 {
		// Normalize by result count - more results doesn't always mean better
		// Only give bonus for first few highly relevant results
		relevantCount := math.Min(float64(len(results)), 5.0)
		countBonus = relevantCount * 0.05 // Much smaller bonus
	}

	confidence := exactMatches + partialMatches + complexityBonus + countBonus
	// Cap confidence more strictly - require actual relevance
	if exactMatches == 0 && partialMatches < 0.5 {
		confidence = math.Min(confidence, 0.65) // Lower cap - 0.65 if no strong matches
	}
	// Additional penalty if we have many results but low relevance
	if len(results) > 10 && exactMatches == 0 && partialMatches < 0.8 {
		confidence *= 0.8 // Penalty for too many low-relevance results
	}
	return math.Min(1.0, math.Max(0.0, confidence))
}

// rankFactsByRelevance ranks facts by how relevant they are to the query keywords.
func (r *Router) rankFactsByRelevance(facts []SpecFact, keywords []string, query string) []SpecFact {
	if len(keywords) == 0 {
		return facts
	}

	// Create a map to store relevance scores
	type scoredFact struct {
		fact   SpecFact
		score  float64
	}
	scored := make([]scoredFact, len(facts))

	for i, fact := range facts {
		score := fact.Confidence // Start with base confidence
		
		// Pre-compute lowercase versions once
		categoryLower := strings.ToLower(fact.Category)
		nameLower := strings.ToLower(fact.Name)
		valueLower := strings.ToLower(fact.Value)
		
		// Count how many keywords match
		matchesInName := 0
		matchesInCategory := 0
		matchesInValue := 0
		
		// Check each keyword
		for _, kw := range keywords {
			kwLower := strings.ToLower(kw)

			// Exact match in category or name gets highest score
			if categoryLower == kwLower || nameLower == kwLower {
				score += 2.0
				if categoryLower == kwLower {
					matchesInCategory++
				}
				if nameLower == kwLower {
					matchesInName++
				}
			} else if strings.Contains(categoryLower, kwLower) || strings.Contains(nameLower, kwLower) {
				// Partial match in category or name
				score += 1.0
				if strings.Contains(categoryLower, kwLower) {
					matchesInCategory++
				}
				if strings.Contains(nameLower, kwLower) {
					matchesInName++
				}
			} else if strings.Contains(valueLower, kwLower) {
				// Match in value gets lower score
				score += 0.5
				matchesInValue++
			}
		}
		
		// Bonus for matching multiple keywords in the same field (indicates high relevance)
		if matchesInName > 1 {
			score += float64(matchesInName-1) * 1.5 // Extra bonus for multiple keyword matches in name
		}
		if matchesInCategory > 0 && matchesInName > 0 {
			score += 1.0 // Bonus when both category and name match
		}
		
		// Bonus for related terms (e.g., "seating" relates to "seats", "material" relates to "materials")
		combinedText := categoryLower + " " + nameLower + " " + valueLower
		
		// Check for related terms
		hasSeatsKeyword := false
		hasMaterialKeyword := false
		for _, kw := range keywords {
			kwLower := strings.ToLower(kw)
			if kwLower == "seats" || kwLower == "seat" {
				hasSeatsKeyword = true
			}
			if kwLower == "material" || kwLower == "materials" {
				hasMaterialKeyword = true
			}
		}
		
		if hasSeatsKeyword && (strings.Contains(nameLower, "seating") || strings.Contains(nameLower, "seat")) {
			score += 1.5 // Bonus for seat/seating relationship
		}
		if hasMaterialKeyword && strings.Contains(nameLower, "material") {
			score += 1.5 // Bonus for material match in name
		}
		
		// Check for phrase matches (e.g., "audio system", "child seat" as phrases)
		// Extract multi-word phrases from keywords (phrases with 2+ words)
		for _, kw := range keywords {
			if strings.Contains(kw, " ") {
				// This is a phrase keyword (e.g., "audio system", "child seat")
				phraseLower := strings.ToLower(kw)
				// Big bonus if the phrase appears as a unit in name or category
				if strings.Contains(nameLower, phraseLower) {
					score += 5.0 // Very strong bonus for phrase match in name
				} else if strings.Contains(categoryLower, phraseLower) {
					score += 4.0 // Very strong bonus for phrase match in category
				} else if strings.Contains(combinedText, phraseLower) {
					score += 3.0 // Strong bonus for phrase match anywhere
				}
			}
		}
		
		// Also check if multiple keywords appear together as a phrase (even if not extracted as phrase keyword)
		// This helps with queries like "child seat" where keywords are separate
		if len(keywords) >= 2 {
			queryLower := strings.ToLower(query)
			// Check if query itself forms a phrase that appears in the fact
			words := strings.Fields(queryLower)
			if len(words) >= 2 {
				// Try 2-word phrases from the query
				for i := 0; i < len(words)-1; i++ {
					phrase := words[i] + " " + words[i+1]
					if strings.Contains(combinedText, phrase) {
						score += 4.0 // Strong bonus for matching query phrase
						break
					}
				}
			}
		}
		
		// If multiple keywords appear together in the same fact, give bonus
		keywordCount := 0
		for _, kw := range keywords {
			// Skip phrase keywords in individual word count
			if !strings.Contains(kw, " ") {
				if strings.Contains(combinedText, strings.ToLower(kw)) {
					keywordCount++
				}
			}
		}
		if keywordCount >= 2 {
			score += 1.0 // Bonus when multiple keywords appear in the same fact
		}
		
		// Penalty if fact only matches generic/stop words (should be filtered out by stop words, but just in case)
		// Require at least one substantial keyword match for multi-keyword queries
		// Count phrase keywords separately
		phraseKeywordCount := 0
		nonPhraseKeywords := []string{}
		for _, kw := range keywords {
			if strings.Contains(kw, " ") {
				phraseKeywordCount++
			} else {
				nonPhraseKeywords = append(nonPhraseKeywords, kw)
			}
		}
		
		// For multi-keyword queries (2+ non-phrase keywords), heavily penalize facts that only match one
		if len(nonPhraseKeywords) >= 2 {
			substantialMatches := 0
			for _, kw := range nonPhraseKeywords {
				kwLower := strings.ToLower(kw)
				// Count matches in all fields (category, name, value)
				// This ensures facts like "Child Safety ISOFIX" with "child" in name and "seat" in value both count
				if strings.Contains(nameLower, kwLower) || strings.Contains(categoryLower, kwLower) || strings.Contains(valueLower, kwLower) {
					substantialMatches++
				}
			}
			// Check if both keywords appear close together (indicating phrase match)
			bothKeywordsPresent := true
			for _, kw := range nonPhraseKeywords {
				kwLower := strings.ToLower(kw)
				if !strings.Contains(combinedText, kwLower) {
					bothKeywordsPresent = false
					break
				}
			}
			
			// Heavy penalty if fact only matches one keyword
			if substantialMatches < len(nonPhraseKeywords) {
				if bothKeywordsPresent {
					// Both keywords present somewhere - apply penalty based on match ratio
					if substantialMatches == 1 && len(nonPhraseKeywords) == 2 {
						score *= 0.1 // Very heavy penalty - only matches 1 out of 2 keywords (90% reduction)
					} else if substantialMatches < len(nonPhraseKeywords)/2 {
						score *= 0.2 // Heavy penalty - only matches less than half
					} else {
						score *= 0.5 // Moderate penalty
					}
				} else {
					// Keywords not both present - very heavy penalty
					score *= 0.05 // Extremely heavy penalty (95% reduction)
				}
			}
			
			// Big bonus if fact matches all keywords
			if substantialMatches == len(nonPhraseKeywords) && bothKeywordsPresent {
				score += 10.0 // Very big bonus for matching all keywords (increased from 5.0)
			}
		}

		scored[i] = scoredFact{fact: fact, score: score}
	}

	// Sort by score (descending)
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[i].score < scored[j].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	// Extract sorted facts
	result := make([]SpecFact, len(facts))
	for i, s := range scored {
		result[i] = s.fact
	}

	return result
}

// filterLowRelevanceFacts filters out facts with low relevance scores.
func (r *Router) filterLowRelevanceFacts(facts []SpecFact, keywords []string) []SpecFact {
	if len(keywords) == 0 || len(facts) == 0 {
		return facts
	}

	// Re-rank to get scores (we need scores for filtering)
	type scoredFact struct {
		fact   SpecFact
		score  float64
	}
	scored := make([]scoredFact, len(facts))
	
	for i, fact := range facts {
		score := 0.0
		categoryLower := strings.ToLower(fact.Category)
		nameLower := strings.ToLower(fact.Name)
		valueLower := strings.ToLower(fact.Value)
		combinedText := categoryLower + " " + nameLower + " " + valueLower
		
		// Count keyword matches
		matchesInName := 0
		matchesInCategory := 0
		
		for _, kw := range keywords {
			kwLower := strings.ToLower(kw)
			
			// Phrase keywords get higher weight
			if strings.Contains(kw, " ") {
				phraseLower := strings.ToLower(kw)
				if strings.Contains(nameLower, phraseLower) {
					score += 5.0 // Very high score for phrase match in name
					matchesInName++
				} else if strings.Contains(categoryLower, phraseLower) {
					score += 4.0
					matchesInCategory++
				} else if strings.Contains(combinedText, phraseLower) {
					score += 3.0
				}
			} else {
				// Single word keywords
				if strings.Contains(nameLower, kwLower) {
					score += 2.0
					matchesInName++
				} else if strings.Contains(categoryLower, kwLower) {
					score += 1.5
					matchesInCategory++
				} else if strings.Contains(valueLower, kwLower) {
					score += 0.5
				}
			}
		}
		
		// Bonus for multiple keyword matches
		if matchesInName >= 2 || (matchesInCategory > 0 && matchesInName > 0) {
			score += 2.0
		}
		
		scored[i] = scoredFact{fact: fact, score: score}
	}
	
	// Filter by minimum relevance threshold
	// For multi-keyword queries, require higher relevance and match multiple keywords
	minScore := 1.0 // Minimum score to include
	nonPhraseKeywords := []string{}
	for _, kw := range keywords {
		if !strings.Contains(kw, " ") {
			nonPhraseKeywords = append(nonPhraseKeywords, kw)
		}
	}
	
		if len(nonPhraseKeywords) >= 2 {
			// For multi-keyword queries, require facts that match at least 2 keywords
			minScore = 3.0 // Higher threshold - must match multiple keywords
			
			// Check each fact to ensure it matches multiple keywords
			// Must check ALL fields (category, name, value) to find keyword matches
			tempScored := make([]scoredFact, 0, len(scored))
			for _, s := range scored {
				fact := s.fact
				categoryLower := strings.ToLower(fact.Category)
				nameLower := strings.ToLower(fact.Name)
				valueLower := strings.ToLower(fact.Value)
				
				keywordMatches := 0
				for _, kw := range nonPhraseKeywords {
					kwLower := strings.ToLower(kw)
					// Check ALL fields (category, name, value) for keyword matches
					if strings.Contains(categoryLower, kwLower) || 
					   strings.Contains(nameLower, kwLower) || 
					   strings.Contains(valueLower, kwLower) {
						keywordMatches++
					}
				}
				
				// Only include facts that match ALL keywords for 2-keyword queries
				// For 2-keyword queries, require both keywords to match
				if len(nonPhraseKeywords) == 2 {
					if keywordMatches == 2 {
						tempScored = append(tempScored, s)
					}
				} else if keywordMatches >= 2 {
					// For 3+ keyword queries, require at least 2 to match
					tempScored = append(tempScored, s)
				}
			}
			
			// Use filtered results (facts that match all keywords)
			// This ensures we only return facts that are truly relevant
			if len(tempScored) > 0 {
				scored = tempScored
				r.logger.Debug().
					Int("filtered_facts", len(tempScored)).
					Int("total_facts", len(scored)).
					Strs("keywords", nonPhraseKeywords).
					Msg("Filtered to facts matching all keywords")
			} else {
				// If no facts match all keywords, it means the query might be too specific
				// In this case, don't return irrelevant results - return empty or very few
				r.logger.Debug().
					Int("total_facts", len(scored)).
					Strs("keywords", nonPhraseKeywords).
					Msg("No facts matched all keywords - returning empty results")
				scored = []scoredFact{} // Return empty - better than irrelevant results
			}
		}
	
	// Check if we have phrase keywords - require phrase match
	for _, kw := range keywords {
		if strings.Contains(kw, " ") {
			minScore = 4.0 // Require strong phrase match for phrase queries
			break
		}
	}
	
	filtered := make([]SpecFact, 0)
	for _, s := range scored {
		if s.score >= minScore {
			filtered = append(filtered, s.fact)
		}
	}
	
	// If filtering removed too many results, be more lenient
	if len(filtered) < len(facts)/3 && len(facts) > 0 {
		// Lower threshold if too aggressive
		minScore = 0.5
		filtered = make([]SpecFact, 0)
		for _, s := range scored {
			if s.score >= minScore {
				filtered = append(filtered, s.fact)
			}
		}
	}
	
	// If still too strict, return top results by score
	if len(filtered) == 0 && len(scored) > 0 {
		// Return top 10 by score
		maxReturn := 10
		if len(scored) < maxReturn {
			maxReturn = len(scored)
		}
		for i := 0; i < maxReturn; i++ {
			// Find highest score
			bestIdx := 0
			bestScore := scored[0].score
			for j := 1; j < len(scored); j++ {
				if scored[j].score > bestScore {
					bestScore = scored[j].score
					bestIdx = j
				}
			}
			filtered = append(filtered, scored[bestIdx].fact)
			// Remove from scored
			scored = append(scored[:bestIdx], scored[bestIdx+1:]...)
		}
	}
	
	return filtered
}

// extractKeywords extracts keywords from a query string.
func (r *Router) extractKeywords(query string) []string {
	// Simple keyword extraction - split on whitespace and filter common words
	words := strings.Fields(strings.ToLower(query))
	keywords := make([]string, 0, len(words))

	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "from": true,
		"is": true, "are": true, "was": true, "were": true, "be": true,
		"been": true, "being": true, "have": true, "has": true, "had": true,
		"do": true, "does": true, "did": true, "will": true, "would": true,
		"should": true, "could": true, "may": true, "might": true, "must": true,
		"can": true, "what": true, "which": true, "who": true, "where": true,
		"when": true, "why": true, "how": true, "about": true, "tell": true,
		"me": true, "my": true, "your": true, "our": true, "their": true,
		"this": true, "that": true, "these": true, "those": true,
		"come": true, "comes": true, "came": true,
		"car": true, "cars": true, // Generic "car" is usually not useful
		"vehicle": true, "vehicles": true,
		"it": true, "its": true,
		// Generic words that appear in too many specs
		"system": true, "systems": true, "type": true, "types": true,
		"feature": true, "features": true, "size": true, "sizes": true,
	}

	for _, word := range words {
		// Remove punctuation
		word = strings.Trim(word, ".,!?;:()[]{}'\"")
		if len(word) > 1 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}

	// Add common multi-word phrases as keywords if they appear in the query
	phrasePatterns := []string{
		"audio system", "sound system", "speaker system",
		"climate control", "air conditioning", "air conditioner",
		"safety system", "brake system", "suspension system",
		"fuel efficiency", "fuel economy", "fuel consumption",
		"drive mode", "driving mode", "transmission",
		"infotainment system", "multimedia system", "entertainment system",
		"cruise control", "lane assist", "parking assist",
		"child seat", "child safety", "children safety",
		"rear seat", "front seat", "seat belt", "seatbelt",
	}
	
	queryLower := strings.ToLower(query)
	for _, phrase := range phrasePatterns {
		if strings.Contains(queryLower, phrase) {
			// Add the phrase as a keyword
			keywords = append(keywords, phrase)
		}
	}

	return keywords
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

