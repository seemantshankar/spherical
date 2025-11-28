// Package comparison provides product comparison services.
package comparison

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/observability"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// Common errors.
var (
	ErrProductNotAccessible  = errors.New("product not accessible for comparison")
	ErrProductsNotComparable = errors.New("products are not comparable")
)

// Materializer handles pre-computed comparisons between products.
type Materializer struct {
	logger *observability.Logger
	cache  ComparisonCache
	store  ComparisonStore

	mu          sync.RWMutex
	comparisons map[string]*CachedComparison // key: pairKey
}

// ComparisonCache provides caching for comparison results.
type ComparisonCache interface {
	Get(ctx context.Context, key string) ([]ComparisonRow, bool)
	Set(ctx context.Context, key string, value []ComparisonRow, ttl time.Duration)
}

// ComparisonStore persists comparison data.
type ComparisonStore interface {
	GetComparison(ctx context.Context, tenantID, primaryID, secondaryID uuid.UUID) ([]storage.ComparisonRow, error)
	SaveComparison(ctx context.Context, rows []storage.ComparisonRow) error
}

// CachedComparison holds cached comparison data.
type CachedComparison struct {
	Rows      []ComparisonRow
	CreatedAt time.Time
	Hash      string
}

// Config for the materializer.
type Config struct {
	CacheTTL         time.Duration
	RefreshInterval  time.Duration
	PolicyFile       string
	AllowCrossTenant bool
}

// NewMaterializer creates a new comparison materializer.
func NewMaterializer(logger *observability.Logger, cache ComparisonCache, store ComparisonStore, cfg Config) *Materializer {
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 1 * time.Hour
	}

	return &Materializer{
		logger:      logger,
		cache:       cache,
		store:       store,
		comparisons: make(map[string]*CachedComparison),
	}
}

// ComparisonRequest represents a request to compare products.
type ComparisonRequest struct {
	TenantID           uuid.UUID
	PrimaryProductID   uuid.UUID
	SecondaryProductID uuid.UUID
	Dimensions         []string
	MaxRows            int
}

// ComparisonResponse contains comparison results.
type ComparisonResponse struct {
	Comparisons []ComparisonRow
	ComputedAt  time.Time
	Hash        string
}

// ComparisonRow represents a single comparison dimension.
type ComparisonRow struct {
	ID                 uuid.UUID
	TenantID           uuid.UUID
	PrimaryProductID   uuid.UUID
	SecondaryProductID uuid.UUID
	Dimension          string
	PrimaryValue       string
	SecondaryValue     string
	Verdict            storage.Verdict
	Narrative          string
	Shareability       storage.Shareability
}

// Compare retrieves or computes a comparison between two products.
func (m *Materializer) Compare(ctx context.Context, req ComparisonRequest) (*ComparisonResponse, error) {
	m.logger.Info().
		Str("tenant_id", req.TenantID.String()).
		Str("primary_product", req.PrimaryProductID.String()).
		Str("secondary_product", req.SecondaryProductID.String()).
		Msg("Processing comparison request")

	// Generate cache key
	cacheKey := m.pairKey(req.TenantID, req.PrimaryProductID, req.SecondaryProductID)

	// Check in-memory cache first
	m.mu.RLock()
	cached, ok := m.comparisons[cacheKey]
	m.mu.RUnlock()

	if ok && time.Since(cached.CreatedAt) < time.Hour {
		rows := m.filterRows(cached.Rows, req.Dimensions, req.MaxRows)
		return &ComparisonResponse{
			Comparisons: rows,
			ComputedAt:  cached.CreatedAt,
			Hash:        cached.Hash,
		}, nil
	}

	// Check external cache
	if m.cache != nil {
		if rows, found := m.cache.Get(ctx, cacheKey); found {
			filtered := m.filterRows(rows, req.Dimensions, req.MaxRows)
			return &ComparisonResponse{
				Comparisons: filtered,
				ComputedAt:  time.Now(),
				Hash:        m.computeHash(rows),
			}, nil
		}
	}

	// Load from store
	if m.store != nil {
		stored, err := m.store.GetComparison(ctx, req.TenantID, req.PrimaryProductID, req.SecondaryProductID)
		if err == nil && len(stored) > 0 {
			rows := m.convertFromStorage(stored)
			filtered := m.filterRows(rows, req.Dimensions, req.MaxRows)

			// Update cache
			m.cacheResult(ctx, cacheKey, rows)

			return &ComparisonResponse{
				Comparisons: filtered,
				ComputedAt:  time.Now(),
				Hash:        m.computeHash(rows),
			}, nil
		}
	}

	// No pre-computed comparison available, return empty
	m.logger.Warn().
		Str("primary_product", req.PrimaryProductID.String()).
		Str("secondary_product", req.SecondaryProductID.String()).
		Msg("No pre-computed comparison found")

	return &ComparisonResponse{
		Comparisons: []ComparisonRow{},
		ComputedAt:  time.Now(),
		Hash:        "",
	}, nil
}

// Materialize pre-computes comparisons for a product pair.
func (m *Materializer) Materialize(ctx context.Context, tenantID, primaryID, secondaryID uuid.UUID, rows []ComparisonRow) error {
	if len(rows) == 0 {
		return nil
	}

	m.logger.Info().
		Str("tenant_id", tenantID.String()).
		Str("primary_product", primaryID.String()).
		Str("secondary_product", secondaryID.String()).
		Int("rows", len(rows)).
		Msg("Materializing comparison")

	// Convert to storage format
	storageRows := make([]storage.ComparisonRow, len(rows))
	now := time.Now()
	for i, row := range rows {
		primaryVal := row.PrimaryValue
		secondaryVal := row.SecondaryValue
		narrative := row.Narrative

		storageRows[i] = storage.ComparisonRow{
			ID:                 uuid.New(),
			PrimaryProductID:   primaryID,
			SecondaryProductID: secondaryID,
			Dimension:          row.Dimension,
			PrimaryValue:       &primaryVal,
			SecondaryValue:     &secondaryVal,
			Verdict:            row.Verdict,
			Narrative:          &narrative,
			Shareability:       row.Shareability,
			ComputedAt:         now,
		}
	}

	// Save to store
	if m.store != nil {
		if err := m.store.SaveComparison(ctx, storageRows); err != nil {
			return fmt.Errorf("save comparison: %w", err)
		}
	}

	// Update cache
	cacheKey := m.pairKey(tenantID, primaryID, secondaryID)
	m.cacheResult(ctx, cacheKey, rows)

	return nil
}

// InvalidatePair removes cached comparisons for a product pair.
func (m *Materializer) InvalidatePair(tenantID, productID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Remove all comparisons involving this product
	prefix := tenantID.String() + ":"
	for key := range m.comparisons {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			// Check if either product ID matches
			if containsProduct(key, productID.String()) {
				delete(m.comparisons, key)
			}
		}
	}
}

func (m *Materializer) pairKey(tenantID, primaryID, secondaryID uuid.UUID) string {
	// Ensure consistent ordering
	p1, p2 := primaryID.String(), secondaryID.String()
	if p1 > p2 {
		p1, p2 = p2, p1
	}
	return fmt.Sprintf("%s:%s:%s", tenantID, p1, p2)
}

func (m *Materializer) filterRows(rows []ComparisonRow, dimensions []string, maxRows int) []ComparisonRow {
	if len(dimensions) == 0 && maxRows <= 0 {
		return rows
	}

	filtered := rows

	// Filter by dimensions
	if len(dimensions) > 0 {
		dimSet := make(map[string]bool)
		for _, d := range dimensions {
			dimSet[d] = true
		}

		var temp []ComparisonRow
		for _, row := range filtered {
			if dimSet[row.Dimension] {
				temp = append(temp, row)
			}
		}
		filtered = temp
	}

	// Limit rows
	if maxRows > 0 && len(filtered) > maxRows {
		filtered = filtered[:maxRows]
	}

	return filtered
}

func (m *Materializer) cacheResult(ctx context.Context, key string, rows []ComparisonRow) {
	cached := &CachedComparison{
		Rows:      rows,
		CreatedAt: time.Now(),
		Hash:      m.computeHash(rows),
	}

	m.mu.Lock()
	m.comparisons[key] = cached
	m.mu.Unlock()

	if m.cache != nil {
		m.cache.Set(ctx, key, rows, time.Hour)
	}
}

func (m *Materializer) computeHash(rows []ComparisonRow) string {
	h := sha256.New()
	for _, row := range rows {
		h.Write([]byte(row.Dimension))
		h.Write([]byte(row.PrimaryValue))
		h.Write([]byte(row.SecondaryValue))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func (m *Materializer) convertFromStorage(stored []storage.ComparisonRow) []ComparisonRow {
	rows := make([]ComparisonRow, len(stored))
	for i, s := range stored {
		primaryVal := ""
		if s.PrimaryValue != nil {
			primaryVal = *s.PrimaryValue
		}
		secondaryVal := ""
		if s.SecondaryValue != nil {
			secondaryVal = *s.SecondaryValue
		}
		narrative := ""
		if s.Narrative != nil {
			narrative = *s.Narrative
		}

		rows[i] = ComparisonRow{
			ID:                 s.ID,
			PrimaryProductID:   s.PrimaryProductID,
			SecondaryProductID: s.SecondaryProductID,
			Dimension:          s.Dimension,
			PrimaryValue:       primaryVal,
			SecondaryValue:     secondaryVal,
			Verdict:            s.Verdict,
			Narrative:          narrative,
			Shareability:       s.Shareability,
		}
	}
	return rows
}

func containsProduct(key, productID string) bool {
	return len(key) >= len(productID) &&
		(key[len(key)-len(productID):] == productID ||
			key[:len(productID)] == productID ||
			contains(key, productID))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// MemoryComparisonCache provides an in-memory cache for comparisons.
type MemoryComparisonCache struct {
	mu    sync.RWMutex
	items map[string]cacheItem
}

type cacheItem struct {
	rows    []ComparisonRow
	expires time.Time
}

// NewMemoryComparisonCache creates a new in-memory comparison cache.
func NewMemoryComparisonCache() *MemoryComparisonCache {
	return &MemoryComparisonCache{
		items: make(map[string]cacheItem),
	}
}

// Get retrieves a cached comparison.
func (c *MemoryComparisonCache) Get(ctx context.Context, key string) ([]ComparisonRow, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, ok := c.items[key]
	if !ok || time.Now().After(item.expires) {
		return nil, false
	}
	return item.rows, true
}

// Set stores a comparison in the cache.
func (c *MemoryComparisonCache) Set(ctx context.Context, key string, value []ComparisonRow, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = cacheItem{
		rows:    value,
		expires: time.Now().Add(ttl),
	}
}
