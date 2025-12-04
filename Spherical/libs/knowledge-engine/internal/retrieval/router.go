// Package retrieval provides hybrid retrieval services combining structured and semantic search.
package retrieval

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
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

// RequestMode represents the type of request format.
type RequestMode string

const (
	RequestModeNaturalLanguage RequestMode = "natural_language"
	RequestModeStructured      RequestMode = "structured"
	RequestModeHybrid          RequestMode = "hybrid" // Both formats
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
	// New: Structured spec name list from LLM
	RequestedSpecs []string `json:"requested_specs,omitempty"`
	// New: Request mode (natural language vs structured)
	RequestMode RequestMode `json:"request_mode,omitempty"`
}

// RetrievalFilters holds filtering options.
type RetrievalFilters struct {
	Categories        []string
	ChunkTypes        []storage.ChunkType
	SpecificationType *string // Filter by specification_type for row chunks
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
	// New: Per-spec availability status
	SpecAvailability []SpecAvailabilityStatus `json:"spec_availability,omitempty"`
	// New: Overall confidence score
	OverallConfidence float64 `json:"overall_confidence"`
	// New: Optional natural language summary
	Summary *string `json:"summary,omitempty"`
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
	ChunkID          uuid.UUID
	ChunkType        storage.ChunkType
	Text             string
	Distance         float32
	Score            float32
	Metadata         map[string]interface{}
	Source           SourceRef
	ParentCategory   string // For hierarchical grouping display
	SubCategory      string // For hierarchical grouping display
	SpecificationType string // For filtering
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

// AvailabilityStatus represents the availability status of a spec.
type AvailabilityStatus string

const (
	AvailabilityStatusFound      AvailabilityStatus = "found"
	AvailabilityStatusUnavailable AvailabilityStatus = "unavailable"
	AvailabilityStatusPartial   AvailabilityStatus = "partial" // Found but low confidence
)

// SpecAvailabilityStatus represents the availability status for a requested spec.
type SpecAvailabilityStatus struct {
	SpecName        string            `json:"spec_name"`
	Status          AvailabilityStatus `json:"status"`
	MatchedSpecs    []SpecFact        `json:"matched_specs,omitempty"` // If found
	MatchedChunks   []SemanticChunk   `json:"matched_chunks,omitempty"` // If found
	Confidence      float64           `json:"confidence"`
	AlternativeNames []string         `json:"alternative_names,omitempty"` // Synonyms found
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
	// New fields for structured requests
	MinAvailabilityConfidence float64       // Threshold for "found" vs "partial"
	BatchProcessingWorkers    int          // Parallel workers for batch processing
	BatchProcessingTimeout    time.Duration // Timeout for batch operations
	EnableSummaryGeneration   bool         // Enable NL summary generation
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
	if cfg.MinAvailabilityConfidence <= 0 {
		cfg.MinAvailabilityConfidence = 0.6 // Default: 60% confidence required
	}
	if cfg.BatchProcessingWorkers <= 0 {
		cfg.BatchProcessingWorkers = 5 // Default: 5 concurrent workers
	}
	if cfg.BatchProcessingTimeout <= 0 {
		cfg.BatchProcessingTimeout = 30 * time.Second // Default: 30 second timeout
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

	// Detect structured request mode
	if len(req.RequestedSpecs) > 0 {
		return r.ProcessStructuredSpecs(ctx, req)
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

	// Calculate overall confidence and availability status for natural language queries
	confidenceCalc := NewConfidenceCalculator()
	response.OverallConfidence = confidenceCalc.CalculateConfidenceForResponse(response)
	
	// Determine availability for natural language queries (if we can extract spec names)
	if req.Question != "" {
		response.SpecAvailability = r.determineAvailabilityForQuery(ctx, req, response)
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

// ProcessStructuredSpecs handles LLM-generated spec name lists.
func (r *Router) ProcessStructuredSpecs(ctx context.Context, req RetrievalRequest) (*RetrievalResponse, error) {
	start := time.Now()

	if len(req.RequestedSpecs) == 0 {
		// Fallback to natural language processing
		return r.Query(ctx, req)
	}

	r.logger.Debug().
		Str("tenant_id", req.TenantID.String()).
		Int("spec_count", len(req.RequestedSpecs)).
		Msg("Processing structured spec request")

	// Normalize all spec names
	normalizer := NewSpecNormalizer()
	normalizedSpecs := make([]string, 0, len(req.RequestedSpecs))
	specNameMap := make(map[string][]string) // Canonical -> original variations

	for _, specName := range req.RequestedSpecs {
		canonical, alternatives := normalizer.NormalizeSpecName(specName)
		normalizedSpecs = append(normalizedSpecs, canonical)
		specNameMap[canonical] = append(specNameMap[canonical], specName)
		specNameMap[canonical] = append(specNameMap[canonical], alternatives...)
	}

	// Create batch processor
	batchProcessor := NewBatchProcessor(r, r.config.BatchProcessingWorkers, r.config.BatchProcessingTimeout)

	// Process all specs in parallel
	specStatuses, err := batchProcessor.ProcessSpecsInParallel(ctx, req.RequestedSpecs, req)
	if err != nil {
		r.logger.Warn().Err(err).Msg("Batch processing failed, falling back to sequential")
		// Fallback to sequential processing
		specStatuses = make([]SpecAvailabilityStatus, 0, len(req.RequestedSpecs))
		detector := NewAvailabilityDetector(r.config.MinAvailabilityConfidence, 0.5)
		for _, specName := range req.RequestedSpecs {
			canonical, alternatives := normalizer.NormalizeSpecName(specName)
			specReq := req
			specReq.Question = canonical
			specReq.RequestedSpecs = nil

			// Try structured search
			facts, _, _ := r.queryStructuredSpecs(ctx, specReq)
			// Try semantic search
			chunks, _ := r.querySemanticChunks(ctx, specReq)

			status := detector.DetermineAvailability(canonical, facts, chunks)
			status.SpecName = specName
			status.AlternativeNames = alternatives
			specStatuses = append(specStatuses, status)
		}
	}

	// Aggregate all matched specs and chunks
	allFacts := make([]SpecFact, 0)
	allChunks := make([]SemanticChunk, 0)
	factMap := make(map[string]bool) // Deduplicate facts
	chunkMap := make(map[uuid.UUID]bool) // Deduplicate chunks

	for _, status := range specStatuses {
		for _, fact := range status.MatchedSpecs {
			key := fmt.Sprintf("%s:%s:%s", fact.Category, fact.Name, fact.Value)
			if !factMap[key] {
				allFacts = append(allFacts, fact)
				factMap[key] = true
			}
		}
		for _, chunk := range status.MatchedChunks {
			if !chunkMap[chunk.ChunkID] {
				allChunks = append(allChunks, chunk)
				chunkMap[chunk.ChunkID] = true
			}
		}
	}

	// Build response
	response := &RetrievalResponse{
		Intent:            IntentSpecLookup,
		StructuredFacts:   allFacts,
		SemanticChunks:    allChunks,
		SpecAvailability:  specStatuses,
		LatencyMs:         time.Since(start).Milliseconds(),
	}

	// Calculate overall confidence
	confidenceCalc := NewConfidenceCalculator()
	response.OverallConfidence = confidenceCalc.CalculateConfidenceForResponse(response)

	// Generate summary if requested (will be implemented in summary generator)
	if req.RequestMode == RequestModeHybrid {
		summaryGen := NewSummaryGenerator()
		summary := summaryGen.GenerateSummary(specStatuses, allFacts, allChunks)
		response.Summary = &summary
	}

	r.logger.Info().
		Int64("latency_ms", response.LatencyMs).
		Int("specs_requested", len(req.RequestedSpecs)).
		Int("specs_found", countFoundSpecs(specStatuses)).
		Int("specs_unavailable", countUnavailableSpecs(specStatuses)).
		Float64("overall_confidence", response.OverallConfidence).
		Msg("Structured spec retrieval complete")

	return response, nil
}

// Helper functions for ProcessStructuredSpecs
func countFoundSpecs(statuses []SpecAvailabilityStatus) int {
	count := 0
	for _, status := range statuses {
		if status.Status == AvailabilityStatusFound {
			count++
		}
	}
	return count
}

func countUnavailableSpecs(statuses []SpecAvailabilityStatus) int {
	count := 0
	for _, status := range statuses {
		if status.Status == AvailabilityStatusUnavailable {
			count++
		}
	}
	return count
}

// determineAvailabilityForQuery determines availability for natural language queries.
func (r *Router) determineAvailabilityForQuery(ctx context.Context, req RetrievalRequest, resp *RetrievalResponse) []SpecAvailabilityStatus {
	// Extract potential spec names from the question
	// This is a simple implementation - could be enhanced with NLP
	normalizer := NewSpecNormalizer()
	keywords := r.extractKeywords(req.Question)

	// Try to identify if question is asking about specific specs
	// For now, return empty - this can be enhanced later
	_ = normalizer
	_ = keywords

	return []SpecAvailabilityStatus{}
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
	r.logger.Debug().Int("facts_after_ranking", len(facts)).Msg("Facts after ranking")

	// Filter out low-relevance facts
	facts = r.filterLowRelevanceFacts(facts, keywords)
	r.logger.Debug().Int("facts_after_filtering", len(facts)).Msg("Facts after filtering")

	// Limit to top results to avoid noise, but keep more for color searches or multi-keyword queries
	maxResults := 30 // Increased default to handle multi-keyword queries better
	queryLower := strings.ToLower(req.Question)
	
	// Check for color-related queries (handle both singular/plural and US/UK spelling)
	// Also check for queries that might be asking about colors (body colors, exterior colors, etc.)
	hasColorKeyword := false
	for _, kw := range keywords {
		kwLower := strings.ToLower(kw)
		if kwLower == "color" || kwLower == "colors" || kwLower == "colour" || kwLower == "colours" {
			hasColorKeyword = true
			break
		}
	}
	isColorQuery := hasColorKeyword || strings.Contains(queryLower, "color") || strings.Contains(queryLower, "colour") || 
	   strings.Contains(queryLower, "colors") || strings.Contains(queryLower, "colours") ||
	   strings.Contains(queryLower, "body color") || strings.Contains(queryLower, "body colour") ||
	   strings.Contains(queryLower, "exterior color") || strings.Contains(queryLower, "exterior colour")
	
	if isColorQuery {
		maxResults = 100 // Allow many results for color queries to get all color options
	}
	
	// Multi-keyword queries - limit results more aggressively for focused queries
	// BUT: Don't limit color queries - they need to return all available colors
	if len(keywords) >= 2 && !isColorQuery {
		// For focused 2-keyword queries (like "child seat"), limit to top 5
		if len(keywords) == 2 {
			maxResults = 5 // Focused queries - only top 5 most relevant
		} else {
			maxResults = 60 // Multi-keyword queries like "weight wheels colors length" need more results
		}
	}
	r.logger.Debug().
		Int("facts_before_limit", len(facts)).
		Int("max_results", maxResults).
		Bool("is_color_query", isColorQuery).
		Int("keyword_count", len(keywords)).
		Msg("Applying maxResults limit")
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
			// Extract chunk type and text from metadata if available
			if result.Metadata != nil {
				if ct, ok := result.Metadata["chunk_type"].(string); ok {
					chunk.ChunkType = storage.ChunkType(ct)
				}
				// Extract text from metadata for display
				if chunkText, ok := result.Metadata["text"].(string); ok {
					chunk.Text = chunkText
					// Additional text-based relevance check using chunk text
					if len(queryKeywords) > 0 {
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
				// Extract source document info from metadata
				if sourceDocStr, ok := result.Metadata["source_doc"].(string); ok && sourceDocStr != "" {
					if sourceDocID, err := uuid.Parse(sourceDocStr); err == nil {
						chunk.Source.DocumentSourceID = &sourceDocID
					}
				}
				
				// Extract category metadata for spec_row chunks
				if chunk.ChunkType == storage.ChunkTypeSpecRow {
					if pc, ok := result.Metadata["parent_category"].(string); ok {
						chunk.ParentCategory = pc
					}
					if sc, ok := result.Metadata["sub_category"].(string); ok {
						chunk.SubCategory = sc
					}
					if st, ok := result.Metadata["specification_type"].(string); ok {
						chunk.SpecificationType = st
					}
					
					// Extract parsed_spec_ids for source references
					if psids, ok := result.Metadata["parsed_spec_ids"].([]interface{}); ok {
						// Store in metadata for reference
						chunk.Metadata["parsed_spec_ids"] = psids
					}
					
					// Apply specification type filtering if requested
					if req.Filters.SpecificationType != nil && chunk.SpecificationType != *req.Filters.SpecificationType {
						continue // Skip chunks that don't match specification type filter
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

	// Apply hierarchical grouping for spec_row chunks
	if len(filteredChunks) > 0 {
		// Check if any chunks are spec_row type
		hasSpecRows := false
		for _, chunk := range filteredChunks {
			if chunk.ChunkType == storage.ChunkTypeSpecRow {
				hasSpecRows = true
				break
			}
		}
		
		if hasSpecRows {
			// Group spec_row chunks hierarchically
			groupedChunks := r.groupChunksByCategory(filteredChunks)
			return groupedChunks, nil
		}
	}

	return filteredChunks, nil
}

// groupChunksByCategory groups chunks hierarchically by parent_category then sub_category.
// Returns chunks in grouped order: parent categories first, then sub-categories within each parent.
func (r *Router) groupChunksByCategory(chunks []SemanticChunk) []SemanticChunk {
	// Separate spec_row chunks from other chunks
	var specRowChunks []SemanticChunk
	var otherChunks []SemanticChunk
	
	for _, chunk := range chunks {
		if chunk.ChunkType == storage.ChunkTypeSpecRow {
			specRowChunks = append(specRowChunks, chunk)
		} else {
			otherChunks = append(otherChunks, chunk)
		}
	}
	
	if len(specRowChunks) == 0 {
		return chunks // No spec_row chunks, return as-is
	}
	
	// Build hierarchical structure: parent_category -> sub_category -> chunks
	type SubCategoryGroup struct {
		SubCategory string
		Chunks      []SemanticChunk
	}
	type ParentCategoryGroup struct {
		ParentCategory string
		SubCategories  []SubCategoryGroup
	}
	
	parentMap := make(map[string]*ParentCategoryGroup)
	
	// Group chunks by category
	for _, chunk := range specRowChunks {
		metadata := extractChunkMetadata(chunk)
		parentCategory := metadata.ParentCategory
		subCategory := metadata.SubCategory
		
		// Use defaults if empty
		if parentCategory == "" {
			parentCategory = "Uncategorized"
		}
		if subCategory == "" {
			subCategory = "General"
		}
		
		// Get or create parent category group
		parentGroup, exists := parentMap[parentCategory]
		if !exists {
			parentGroup = &ParentCategoryGroup{
				ParentCategory: parentCategory,
				SubCategories:  make([]SubCategoryGroup, 0),
			}
			parentMap[parentCategory] = parentGroup
		}
		
		// Find or create sub-category group
		subGroupIdx := -1
		for i, sg := range parentGroup.SubCategories {
			if sg.SubCategory == subCategory {
				subGroupIdx = i
				break
			}
		}
		if subGroupIdx == -1 {
			parentGroup.SubCategories = append(parentGroup.SubCategories, SubCategoryGroup{
				SubCategory: subCategory,
				Chunks:      make([]SemanticChunk, 0),
			})
			subGroupIdx = len(parentGroup.SubCategories) - 1
		}
		
		// Add chunk to sub-category group
		parentGroup.SubCategories[subGroupIdx].Chunks = append(parentGroup.SubCategories[subGroupIdx].Chunks, chunk)
	}
	
	// Flatten hierarchical structure into ordered list
	// Order: parent categories alphabetically, then sub-categories within each parent, then chunks
	var groupedChunks []SemanticChunk
	
	// Sort parent categories
	parentCategories := make([]string, 0, len(parentMap))
	for pc := range parentMap {
		parentCategories = append(parentCategories, pc)
	}
	// Simple alphabetical sort
	for i := 0; i < len(parentCategories)-1; i++ {
		for j := i + 1; j < len(parentCategories); j++ {
			if parentCategories[i] > parentCategories[j] {
				parentCategories[i], parentCategories[j] = parentCategories[j], parentCategories[i]
			}
		}
	}
	
	// Build grouped list
	for _, parentCategory := range parentCategories {
		parentGroup := parentMap[parentCategory]
		
		// Sort sub-categories
		for i := 0; i < len(parentGroup.SubCategories)-1; i++ {
			for j := i + 1; j < len(parentGroup.SubCategories); j++ {
				if parentGroup.SubCategories[i].SubCategory > parentGroup.SubCategories[j].SubCategory {
					parentGroup.SubCategories[i], parentGroup.SubCategories[j] = parentGroup.SubCategories[j], parentGroup.SubCategories[i]
				}
			}
		}
		
		// Add chunks from each sub-category
		for _, subGroup := range parentGroup.SubCategories {
			groupedChunks = append(groupedChunks, subGroup.Chunks...)
		}
	}
	
	// Append other chunks at the end
	groupedChunks = append(groupedChunks, otherChunks...)
	
	return groupedChunks
}

// ChunkMetadata holds extracted metadata from a chunk.
type ChunkMetadata struct {
	ParentCategory    string
	SubCategory       string
	SpecificationType string
	Value             string
	AdditionalMetadata string
	ParsedSpecIDs     []string
}

// extractChunkMetadata extracts metadata from a SemanticChunk.
func extractChunkMetadata(chunk SemanticChunk) ChunkMetadata {
	metadata := ChunkMetadata{}
	
	if chunk.Metadata != nil {
		if pc, ok := chunk.Metadata["parent_category"].(string); ok {
			metadata.ParentCategory = pc
		}
		if sc, ok := chunk.Metadata["sub_category"].(string); ok {
			metadata.SubCategory = sc
		}
		if st, ok := chunk.Metadata["specification_type"].(string); ok {
			metadata.SpecificationType = st
		}
		if v, ok := chunk.Metadata["value"].(string); ok {
			metadata.Value = v
		}
		if am, ok := chunk.Metadata["additional_metadata"].(string); ok {
			metadata.AdditionalMetadata = am
		}
		if psids, ok := chunk.Metadata["parsed_spec_ids"].([]interface{}); ok {
			for _, id := range psids {
				if idStr, ok := id.(string); ok {
					metadata.ParsedSpecIDs = append(metadata.ParsedSpecIDs, idStr)
				}
			}
		} else if psids, ok := chunk.Metadata["parsed_spec_ids"].([]string); ok {
			metadata.ParsedSpecIDs = psids
		}
	}
	
	return metadata
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
	}

	// Handle structured requests differently
	if len(req.RequestedSpecs) > 0 {
		parts = append(parts, "structured")
		// Use normalized spec names for consistent caching
		normalizer := NewSpecNormalizer()
		normalized := make([]string, len(req.RequestedSpecs))
		for i, spec := range req.RequestedSpecs {
			canonical, _ := normalizer.NormalizeSpecName(spec)
			normalized[i] = canonical
		}
		// Sort for consistent ordering
		sort.Strings(normalized)
		parts = append(parts, strings.Join(normalized, ","))
	} else {
		parts = append(parts, req.Question)
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
	// Use word boundaries to avoid false matches (e.g., "suspension" matching "usp")
	for _, pattern := range c.uspPatterns {
		// Match whole words only to avoid substring matches
		patternLower := strings.ToLower(pattern)
		// Check if pattern appears as a whole word (with word boundaries)
		wordBoundaryPattern := "\\b" + regexp.QuoteMeta(patternLower) + "\\b"
		matched, _ := regexp.MatchString(wordBoundaryPattern, q)
		if matched {
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
				matched := false
				if strings.Contains(nameLower, kwLower) {
					score += 2.0
					matchesInName++
					matched = true
				} else if strings.Contains(categoryLower, kwLower) {
					score += 1.5
					matchesInCategory++
					matched = true
				} else if strings.Contains(valueLower, kwLower) {
					score += 0.5
					matched = true
				}
				
				// Also check for variants if no exact match (for facts found via variant search)
				if !matched {
					// Special handling for color/colour keywords
					if kwLower == "colors" || kwLower == "colours" {
						// Try singular variant
						if strings.Contains(nameLower, "color") || strings.Contains(nameLower, "colour") {
							score += 2.0
							matchesInName++
						} else if strings.Contains(categoryLower, "color") || strings.Contains(categoryLower, "colour") {
							score += 1.5
							matchesInCategory++
						} else if strings.Contains(valueLower, "color") || strings.Contains(valueLower, "colour") {
							score += 0.5
						}
					} else if kwLower == "color" || kwLower == "colour" {
						// Try plural variant
						if strings.Contains(nameLower, "colors") || strings.Contains(nameLower, "colours") {
							score += 2.0
							matchesInName++
						} else if strings.Contains(categoryLower, "colors") || strings.Contains(categoryLower, "colours") {
							score += 1.5
							matchesInCategory++
						} else if strings.Contains(valueLower, "colors") || strings.Contains(valueLower, "colours") {
							score += 0.5
						}
					} else {
						// For other keywords, try singular/plural variants
						if strings.HasSuffix(kwLower, "s") && len(kwLower) > 1 {
							// Try singular variant (if keyword is plural)
							singular := kwLower[:len(kwLower)-1]
							if strings.Contains(nameLower, singular) {
								score += 1.5 // Slightly lower score for variant match
								matchesInName++
							} else if strings.Contains(categoryLower, singular) {
								score += 1.0
								matchesInCategory++
							} else if strings.Contains(valueLower, singular) {
								score += 0.3
							}
						} else {
							// Try plural variant (if keyword is singular)
							plural := kwLower + "s"
							if strings.Contains(nameLower, plural) {
								score += 1.5 // Slightly lower score for variant match
								matchesInName++
							} else if strings.Contains(categoryLower, plural) {
								score += 1.0
								matchesInCategory++
							} else if strings.Contains(valueLower, plural) {
								score += 0.3
							}
						}
					}
				}
			}
		}
		
		// Bonus for multiple keyword matches
		if matchesInName >= 2 || (matchesInCategory > 0 && matchesInName > 0) {
			score += 2.0
		}
		
		// For multi-keyword queries, give bonus if fact matches a keyword even if not all keywords match
		// This ensures facts matching individual keywords are included
		if len(keywords) >= 3 && (matchesInName > 0 || matchesInCategory > 0) {
			score += 1.0 // Bonus for matching at least one keyword in multi-keyword queries
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
			// For multi-keyword queries, adjust threshold based on number of keywords
			// More keywords = more permissive (user wants to see facts for each keyword)
			if len(nonPhraseKeywords) >= 4 {
				minScore = 0.5 // Very permissive for 4+ keywords - match any keyword
			} else if len(nonPhraseKeywords) == 3 {
				minScore = 1.0 // Moderate for 3 keywords
			} else {
				minScore = 3.0 // Strict for 2 keywords - must match both
			}
			
			// Check if this is a color-related query - be more lenient with matching
			isColorQuery := false
			for _, kw := range nonPhraseKeywords {
				kwLower := strings.ToLower(kw)
				if kwLower == "color" || kwLower == "colors" || kwLower == "colour" || kwLower == "colours" {
					isColorQuery = true
					break
				}
			}
			
		// Check each fact to ensure it matches keywords
		// Phrase keywords (quoted) and non-phrase keywords are treated separately
		// A fact can match EITHER a phrase keyword OR a non-phrase keyword
		tempScored := make([]scoredFact, 0, len(scored))
		
		// Extract phrase keywords separately
		phraseKeywords := []string{}
		for _, kw := range keywords {
			if strings.Contains(kw, " ") {
				phraseKeywords = append(phraseKeywords, kw)
			}
		}
		
		for _, s := range scored {
			fact := s.fact
			categoryLower := strings.ToLower(fact.Category)
			nameLower := strings.ToLower(fact.Name)
			valueLower := strings.ToLower(fact.Value)
			combinedText := categoryLower + " " + nameLower + " " + valueLower
			
			// Check if fact matches any phrase keyword (quoted phrases)
			// Phrase keywords require the full phrase to appear together
			matchesPhrase := false
			for _, phraseKw := range phraseKeywords {
				phraseLower := strings.ToLower(phraseKw)
				if strings.Contains(combinedText, phraseLower) {
					matchesPhrase = true
					break
				}
			}
			
			// Check if fact matches non-phrase keywords
			// Also check for singular/plural variants (e.g., "speaker" matches "speakers", "color" matches "colors")
			keywordMatches := 0
			hasColorKeyword := false
			for _, kw := range nonPhraseKeywords {
				kwLower := strings.ToLower(kw)
				// Check ALL fields (category, name, value) for keyword matches
				matches := strings.Contains(categoryLower, kwLower) || 
				   strings.Contains(nameLower, kwLower) || 
				   strings.Contains(valueLower, kwLower)
				
				// Also check for singular/plural variants (facts found via variant search)
				// Special handling for color/colour keywords first
				if !matches {
					if kwLower == "colors" || kwLower == "colours" {
						// Try singular variant for color keywords
						matches = strings.Contains(categoryLower, "color") || strings.Contains(categoryLower, "colour") ||
						   strings.Contains(nameLower, "color") || strings.Contains(nameLower, "colour") ||
						   strings.Contains(valueLower, "color") || strings.Contains(valueLower, "colour")
					} else if kwLower == "color" || kwLower == "colour" {
						// Try plural variant for color keywords
						matches = strings.Contains(categoryLower, "colors") || strings.Contains(categoryLower, "colours") ||
						   strings.Contains(nameLower, "colors") || strings.Contains(nameLower, "colours") ||
						   strings.Contains(valueLower, "colors") || strings.Contains(valueLower, "colours")
					}
					// Also check cross-spelling variants (US vs UK)
					if !matches {
						if kwLower == "colors" || kwLower == "color" {
							matches = strings.Contains(categoryLower, "colour") || strings.Contains(categoryLower, "colours") ||
							   strings.Contains(nameLower, "colour") || strings.Contains(nameLower, "colours") ||
							   strings.Contains(valueLower, "colour") || strings.Contains(valueLower, "colours")
						} else if kwLower == "colours" || kwLower == "colour" {
							matches = strings.Contains(categoryLower, "color") || strings.Contains(categoryLower, "colors") ||
							   strings.Contains(nameLower, "color") || strings.Contains(nameLower, "colors") ||
							   strings.Contains(valueLower, "color") || strings.Contains(valueLower, "colors")
						}
					}
					// For other keywords, try singular/plural variants
					if !matches && kwLower != "color" && kwLower != "colors" && kwLower != "colour" && kwLower != "colours" {
						// Try singular variant (if keyword is plural like "speakers")
						if strings.HasSuffix(kwLower, "s") && len(kwLower) > 1 {
							singular := kwLower[:len(kwLower)-1]
							matches = strings.Contains(categoryLower, singular) || 
							   strings.Contains(nameLower, singular) || 
							   strings.Contains(valueLower, singular)
						} else {
							// Try plural variant (if keyword is singular like "speaker")
							plural := kwLower + "s"
							matches = strings.Contains(categoryLower, plural) || 
							   strings.Contains(nameLower, plural) || 
							   strings.Contains(valueLower, plural)
						}
					}
				}
				
				if matches {
					keywordMatches++
				}
				// Track if this keyword is color-related
				if kwLower == "color" || kwLower == "colors" || kwLower == "colour" || kwLower == "colours" {
					hasColorKeyword = true
				}
			}
			
			// Include fact if it matches a phrase keyword OR matches non-phrase keywords
			shouldInclude := false
			
			// If it matches a phrase keyword, include it
			if matchesPhrase {
				shouldInclude = true
			}
			
			// Also check non-phrase keyword matching
			// For color queries, if the fact is about colors and matches the color keyword, include it
			if isColorQuery && hasColorKeyword {
				if strings.Contains(categoryLower, "color") || strings.Contains(categoryLower, "colour") ||
				   strings.Contains(nameLower, "color") || strings.Contains(nameLower, "colour") {
					shouldInclude = true
				}
			}
			
			// For queries with many keywords (4+), be more permissive - return facts matching ANY keyword
			// This allows users to get all relevant facts for each keyword they asked about
			if len(nonPhraseKeywords) >= 4 {
				// For 4+ keywords, include facts that match at least 1 keyword
				// This allows queries like "upholstery length weight suspension" to return all relevant facts
				if keywordMatches >= 1 {
					shouldInclude = true
				}
			} else if len(nonPhraseKeywords) == 3 {
				// For 3 keywords, require at least 1 match (more permissive than before)
				// This handles queries like "fuel efficiency power" better
				if keywordMatches >= 1 {
					shouldInclude = true
				}
			} else if len(nonPhraseKeywords) == 2 {
				// For 2 keywords, require both to match (focused queries)
				if keywordMatches == 2 {
					shouldInclude = true
				}
			} else if len(nonPhraseKeywords) == 1 {
				// For single keyword, include if it matches
				if keywordMatches >= 1 {
					shouldInclude = true
				}
			}
			
			if shouldInclude {
				tempScored = append(tempScored, s)
			}
		}
			
			// Use filtered results
			// For 4+ keywords: facts matching at least 1 keyword
			// For 3 keywords: facts matching at least 1 keyword  
			// For 2 keywords: facts matching both keywords
			if len(tempScored) > 0 {
				scored = tempScored
				var matchDesc string
				if len(nonPhraseKeywords) >= 4 {
					matchDesc = "matching at least 1 keyword"
				} else if len(nonPhraseKeywords) == 3 {
					matchDesc = "matching at least 1 keyword"
				} else {
					matchDesc = "matching all keywords"
				}
				r.logger.Debug().
					Int("filtered_facts", len(tempScored)).
					Int("total_facts", len(scored)).
					Strs("keywords", nonPhraseKeywords).
					Msg(fmt.Sprintf("Filtered to facts %s", matchDesc))
			} else {
				// If no facts match keywords, it means the query might be too specific
				// In this case, don't return irrelevant results - return empty or very few
				var matchDesc string
				if len(nonPhraseKeywords) >= 4 {
					matchDesc = "at least 1 keyword"
				} else if len(nonPhraseKeywords) == 3 {
					matchDesc = "at least 1 keyword"
				} else {
					matchDesc = "all keywords"
				}
				r.logger.Debug().
					Int("total_facts", len(scored)).
					Strs("keywords", nonPhraseKeywords).
					Msg(fmt.Sprintf("No facts matched %s - returning empty results", matchDesc))
				scored = []scoredFact{} // Return empty - better than irrelevant results
			}
		}
	
	// Check if we have phrase keywords - require phrase match
	// But be more lenient for multi-keyword queries where we want to return facts for each keyword
	for _, kw := range keywords {
		if strings.Contains(kw, " ") {
			// For multi-keyword queries with phrase keywords, be more lenient
			// The phrase keyword matching is already handled in the keyword matching logic above
			if len(nonPhraseKeywords) >= 4 {
				minScore = 0.5 // Very lenient for multi-keyword queries
			} else {
				minScore = 2.0 // Moderate for queries with phrase + few keywords
			}
			break
		}
	}
	
	// If no phrase keyword, use the minScore already set based on keyword count
	// For multi-keyword queries (4+), we already set minScore = 0.5, which is lenient
	
	filtered := make([]SpecFact, 0)
	for _, s := range scored {
		if s.score >= minScore {
			filtered = append(filtered, s.fact)
		}
	}
	
	// If filtering removed too many results, be more lenient
	// This is especially important for multi-keyword queries where we want to return facts for each keyword
	// For multi-keyword queries (4+ non-phrase keywords), be very lenient to return facts for each keyword
	if len(nonPhraseKeywords) >= 4 {
		// For 4+ keywords, accept any fact that matches at least one keyword (already filtered above)
		// Just ensure we don't filter by score too aggressively
		if len(filtered) < len(scored)/2 && len(scored) > 0 {
			minScore = 0.0 // Accept all facts that passed keyword matching
			filtered = make([]SpecFact, 0)
			for _, s := range scored {
				if s.score >= minScore {
					filtered = append(filtered, s.fact)
				}
			}
		}
	} else if len(filtered) < len(scored)/2 && len(scored) > 0 {
		// Lower threshold if too aggressive - be more permissive
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
	keywords := make([]string, 0)
	
	// First, extract quoted phrases (they should be treated as single phrase keywords)
	quotedPhrases := regexp.MustCompile(`"([^"]+)"`)
	matches := quotedPhrases.FindAllStringSubmatch(query, -1)
	quotedText := make(map[string]bool)
	
	// Extract quoted phrases and remove them from the query
	queryWithoutQuotes := query
	for _, match := range matches {
		if len(match) > 1 {
			phrase := strings.ToLower(strings.TrimSpace(match[1]))
			if len(phrase) > 0 {
				keywords = append(keywords, phrase) // Add as phrase keyword
				quotedText[phrase] = true
				// Remove the quoted phrase from query (with quotes) to avoid double-processing
				queryWithoutQuotes = strings.Replace(queryWithoutQuotes, match[0], " ", -1)
			}
		}
	}
	
	// Now extract individual words from the remaining query (without quoted phrases)
	words := strings.Fields(strings.ToLower(queryWithoutQuotes))

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
		// Note: "feature" and "features" are NOT stop words - they're meaningful query terms
		// when users specifically ask for features (e.g., "What are the key features?")
		"size": true, "sizes": true,
	}

	// British to American spelling normalization map
	spellingMap := map[string]string{
		"colour":  "color",
		"colours": "colors",
		"favour":  "favor",
		"favours": "favors",
		"metre":   "meter",
		"metres":  "meters",
		"litre":   "liter",
		"litres":  "liters",
		"centre":  "center",
		"centres": "centers",
	}
	
	// Track which words are part of quoted phrases (to avoid adding them as individual keywords)
	wordsInQuotes := make(map[string]bool)
	for quoted := range quotedText {
		quotedWords := strings.Fields(quoted)
		for _, w := range quotedWords {
			wordsInQuotes[strings.ToLower(w)] = true
		}
	}
	
	for _, word := range words {
		// Remove punctuation
		word = strings.Trim(word, ".,!?;:()[]{}'\"")
		wordLower := strings.ToLower(word)
		// Skip if this word is part of a quoted phrase (already added as phrase keyword)
		if wordsInQuotes[wordLower] {
			continue
		}
		if len(word) > 1 && !stopWords[word] {
			// Normalize British to American spelling
			if americanSpelling, ok := spellingMap[wordLower]; ok {
				word = americanSpelling
			}
			keywords = append(keywords, word)
		}
	}

	// Add common multi-word phrases as keywords if they appear in the query
	// But skip if they're already in quoted phrases (to avoid duplicates)
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
		"body color", "body colors", "body colour", "body colours",
		"exterior color", "exterior colors", "exterior colour", "exterior colours",
		"interior color", "interior colors", "interior colour", "interior colours",
		"color option", "color options", "colour option", "colour options",
		"paint color", "paint colors", "paint colour", "paint colours",
	}
	
	queryLower := strings.ToLower(query)
	for _, phrase := range phrasePatterns {
		// Check if this phrase is already in quoted phrases
		alreadyQuoted := false
		for quoted := range quotedText {
			if strings.Contains(quoted, phrase) || strings.Contains(phrase, quoted) {
				alreadyQuoted = true
				break
			}
		}
		// Only add if not already in quotes and appears in query
		if !alreadyQuoted && strings.Contains(queryLower, phrase) {
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

