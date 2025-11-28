// Package retrieval provides Redis-backed response caching.
package retrieval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/cache"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
)

// ResponseCache provides caching for retrieval responses.
type ResponseCache struct {
	client *cache.RedisClient
	logger *observability.Logger
	config ResponseCacheConfig
}

// ResponseCacheConfig configures the response cache.
type ResponseCacheConfig struct {
	// DefaultTTL is the default cache TTL
	DefaultTTL time.Duration
	// StructuredFactsTTL for spec lookups (longer, more stable)
	StructuredFactsTTL time.Duration
	// SemanticChunksTTL for semantic results (shorter, more dynamic)
	SemanticChunksTTL time.Duration
	// ComparisonsTTL for comparison results
	ComparisonsTTL time.Duration
	// KeyPrefix is the cache key prefix
	KeyPrefix string
	// Enabled controls whether caching is active
	Enabled bool
}

// DefaultResponseCacheConfig returns default cache configuration.
func DefaultResponseCacheConfig() ResponseCacheConfig {
	return ResponseCacheConfig{
		DefaultTTL:         5 * time.Minute,
		StructuredFactsTTL: 15 * time.Minute,
		SemanticChunksTTL:  5 * time.Minute,
		ComparisonsTTL:     30 * time.Minute,
		KeyPrefix:          "retrieval:response:",
		Enabled:            true,
	}
}

// NewResponseCache creates a new response cache.
func NewResponseCache(client *cache.RedisClient, logger *observability.Logger, config ResponseCacheConfig) *ResponseCache {
	if config.KeyPrefix == "" {
		config.KeyPrefix = "retrieval:response:"
	}
	if config.DefaultTTL == 0 {
		config.DefaultTTL = 5 * time.Minute
	}

	return &ResponseCache{
		client: client,
		logger: logger,
		config: config,
	}
}

// CacheKey generates a cache key for a retrieval request.
func (c *ResponseCache) CacheKey(req RetrievalRequest) string {
	// Create deterministic key from request parameters
	parts := []string{
		req.TenantID.String(),
		req.Question,
	}

	// Sort product IDs for deterministic key
	productIDs := make([]string, len(req.ProductIDs))
	for i, pid := range req.ProductIDs {
		productIDs[i] = pid.String()
	}
	sort.Strings(productIDs)
	for _, pid := range productIDs {
		parts = append(parts, pid)
	}

	// Add campaign variant if specified
	if req.CampaignVariantID != nil {
		parts = append(parts, req.CampaignVariantID.String())
	}

	// Add intent hint if specified
	if req.IntentHint != nil {
		parts = append(parts, string(*req.IntentHint))
	}

	// Add filters
	if len(req.Filters.Categories) > 0 {
		sort.Strings(req.Filters.Categories)
		for _, cat := range req.Filters.Categories {
			parts = append(parts, "cat:"+cat)
		}
	}

	// Hash the combined parts
	combined := ""
	for _, p := range parts {
		combined += p + "|"
	}
	hash := sha256.Sum256([]byte(combined))
	hashStr := hex.EncodeToString(hash[:16]) // Use first 16 bytes

	return c.config.KeyPrefix + hashStr
}

// CachedResponse represents a cached retrieval response.
type CachedResponse struct {
	Response  *RetrievalResponse `json:"response"`
	CachedAt  time.Time          `json:"cached_at"`
	ExpiresAt time.Time          `json:"expires_at"`
	Version   int64              `json:"version"`
}

// Get retrieves a cached response if available.
func (c *ResponseCache) Get(ctx context.Context, req RetrievalRequest) (*RetrievalResponse, bool) {
	if !c.config.Enabled || c.client == nil {
		return nil, false
	}

	key := c.CacheKey(req)
	data, err := c.client.Get(ctx, key)
	if err != nil {
		if err != cache.ErrCacheMiss {
			c.logger.Debug().Err(err).Str("key", key).Msg("Cache get error")
		}
		return nil, false
	}

	var cached CachedResponse
	if err := json.Unmarshal(data, &cached); err != nil {
		c.logger.Warn().Err(err).Str("key", key).Msg("Failed to unmarshal cached response")
		return nil, false
	}

	// Check if expired
	if time.Now().After(cached.ExpiresAt) {
		return nil, false
	}

	c.logger.Debug().Str("key", key).Msg("Cache hit")
	return cached.Response, true
}

// Set caches a retrieval response.
func (c *ResponseCache) Set(ctx context.Context, req RetrievalRequest, resp *RetrievalResponse) error {
	if !c.config.Enabled || c.client == nil {
		return nil
	}

	key := c.CacheKey(req)
	ttl := c.getTTLForResponse(resp)

	cached := CachedResponse{
		Response:  resp,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(ttl),
		Version:   time.Now().UnixNano(),
	}

	data, err := json.Marshal(cached)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	if err := c.client.Set(ctx, key, data, ttl); err != nil {
		c.logger.Warn().Err(err).Str("key", key).Msg("Failed to cache response")
		return err
	}

	c.logger.Debug().Str("key", key).Dur("ttl", ttl).Msg("Cached response")
	return nil
}

// Invalidate invalidates cache entries for a tenant/product.
func (c *ResponseCache) Invalidate(ctx context.Context, tenantID uuid.UUID, productID *uuid.UUID) error {
	if !c.config.Enabled || c.client == nil {
		return nil
	}

	// Create invalidation pattern
	pattern := c.config.KeyPrefix + "*"

	// In production, we'd use Redis SCAN + DEL or pub/sub for invalidation
	// For now, we'll delete by pattern
	c.logger.Info().
		Str("tenant_id", tenantID.String()).
		Msg("Invalidating cache for tenant")

	return c.client.DeleteByPrefix(ctx, pattern)
}

// InvalidateForCampaign invalidates cache entries for a specific campaign.
func (c *ResponseCache) InvalidateForCampaign(ctx context.Context, tenantID, campaignID uuid.UUID) error {
	if !c.config.Enabled || c.client == nil {
		return nil
	}

	c.logger.Info().
		Str("tenant_id", tenantID.String()).
		Str("campaign_id", campaignID.String()).
		Msg("Invalidating cache for campaign")

	// Campaign-specific invalidation would use a more targeted pattern
	pattern := c.config.KeyPrefix + "*"
	return c.client.DeleteByPrefix(ctx, pattern)
}

// getTTLForResponse determines TTL based on response content.
func (c *ResponseCache) getTTLForResponse(resp *RetrievalResponse) time.Duration {
	switch resp.Intent {
	case IntentSpecLookup:
		// Structured facts are more stable, cache longer
		return c.config.StructuredFactsTTL
	case IntentComparison:
		// Comparisons are pre-computed, cache longest
		return c.config.ComparisonsTTL
	case IntentUSPLookup, IntentFAQ:
		// Semantic content may change more frequently
		return c.config.SemanticChunksTTL
	default:
		return c.config.DefaultTTL
	}
}

// Stats returns cache statistics.
func (c *ResponseCache) Stats(ctx context.Context) (*CacheStats, error) {
	if c.client == nil {
		return &CacheStats{}, nil
	}

	// This would query Redis for stats
	// For now, return placeholder
	return &CacheStats{
		Enabled: c.config.Enabled,
	}, nil
}

// CacheStats contains cache statistics.
type CacheStats struct {
	Enabled    bool    `json:"enabled"`
	Hits       int64   `json:"hits"`
	Misses     int64   `json:"misses"`
	HitRate    float64 `json:"hit_rate"`
	KeyCount   int64   `json:"key_count"`
	MemoryUsed int64   `json:"memory_used_bytes"`
}

// InvalidationTrigger handles cache invalidation on data changes.
type InvalidationTrigger struct {
	cache  *ResponseCache
	logger *observability.Logger
}

// NewInvalidationTrigger creates a new invalidation trigger.
func NewInvalidationTrigger(cache *ResponseCache, logger *observability.Logger) *InvalidationTrigger {
	return &InvalidationTrigger{
		cache:  cache,
		logger: logger,
	}
}

// OnCampaignPublished invalidates cache when a campaign is published.
func (t *InvalidationTrigger) OnCampaignPublished(ctx context.Context, tenantID, productID, campaignID uuid.UUID) {
	if err := t.cache.InvalidateForCampaign(ctx, tenantID, campaignID); err != nil {
		t.logger.Error().Err(err).Msg("Failed to invalidate cache on campaign publish")
	}
}

// OnSpecValueUpdated invalidates cache when spec values are updated.
func (t *InvalidationTrigger) OnSpecValueUpdated(ctx context.Context, tenantID, productID uuid.UUID) {
	if err := t.cache.Invalidate(ctx, tenantID, &productID); err != nil {
		t.logger.Error().Err(err).Msg("Failed to invalidate cache on spec update")
	}
}

// OnComparisonRecomputed invalidates cache when comparisons are recomputed.
func (t *InvalidationTrigger) OnComparisonRecomputed(ctx context.Context, tenantID uuid.UUID) {
	if err := t.cache.Invalidate(ctx, tenantID, nil); err != nil {
		t.logger.Error().Err(err).Msg("Failed to invalidate cache on comparison recompute")
	}
}

