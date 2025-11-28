// Package retrieval provides hybrid retrieval services combining structured and semantic search.
package retrieval

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/google/uuid"
)

// VectorAdapter defines the interface for vector similarity search.
type VectorAdapter interface {
	// Search finds the k nearest neighbors to the query vector.
	Search(ctx context.Context, query []float32, k int, filters VectorFilters) ([]VectorResult, error)
	
	// Insert adds vectors to the index.
	Insert(ctx context.Context, vectors []VectorEntry) error
	
	// Delete removes vectors from the index.
	Delete(ctx context.Context, ids []uuid.UUID) error
	
	// Count returns the number of vectors in the index.
	Count(ctx context.Context) (int64, error)
	
	// Close releases resources.
	Close() error
}

// VectorFilters defines filtering options for vector search.
type VectorFilters struct {
	TenantID          *uuid.UUID
	ProductIDs        []uuid.UUID
	CampaignVariantID *uuid.UUID
	ChunkTypes        []string
	Visibility        []string
	EmbeddingVersion  *string
}

// VectorEntry represents a vector to be indexed.
type VectorEntry struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	ProductID         uuid.UUID
	CampaignVariantID *uuid.UUID
	ChunkType         string
	Visibility        string
	EmbeddingVersion  string
	Vector            []float32
	Metadata          map[string]interface{}
}

// VectorResult represents a search result.
type VectorResult struct {
	ID       uuid.UUID
	Distance float32
	Score    float32 // 1 - distance for cosine
	Metadata map[string]interface{}
}

// ErrVectorDimensionMismatch indicates a dimension mismatch.
var ErrVectorDimensionMismatch = errors.New("vector dimension mismatch")

// FAISSAdapter implements VectorAdapter using an in-memory FAISS-like index.
// For production, this would use actual FAISS C bindings.
// This is a simplified pure-Go implementation for development.
type FAISSAdapter struct {
	mu        sync.RWMutex
	dimension int
	vectors   map[uuid.UUID]indexedVector
}

type indexedVector struct {
	entry  VectorEntry
	vector []float32
}

// FAISSConfig holds FAISS adapter configuration.
type FAISSConfig struct {
	Dimension int
	IndexPath string
	NList     int
}

// NewFAISSAdapter creates a new FAISS adapter.
func NewFAISSAdapter(cfg FAISSConfig) (*FAISSAdapter, error) {
	if cfg.Dimension <= 0 {
		cfg.Dimension = 768
	}
	
	return &FAISSAdapter{
		dimension: cfg.Dimension,
		vectors:   make(map[uuid.UUID]indexedVector),
	}, nil
}

// Search finds the k nearest neighbors using cosine similarity.
func (a *FAISSAdapter) Search(ctx context.Context, query []float32, k int, filters VectorFilters) ([]VectorResult, error) {
	a.mu.RLock()
	hasVectors := len(a.vectors) > 0
	a.mu.RUnlock()
	
	// Update dimension dynamically if query has different dimension (for first-time detection)
	if len(query) != a.dimension {
		// If we have existing vectors, check their dimension
		if hasVectors {
			a.mu.RLock()
			// Get dimension from first stored vector that matches filters
			var storedDimension int
			for _, iv := range a.vectors {
				if len(iv.vector) > 0 && matchesFilters(iv.entry, filters) {
					storedDimension = len(iv.vector)
					break
				}
			}
			a.mu.RUnlock()
			
			if storedDimension > 0 {
				if len(query) == storedDimension {
					// Update dimension to match
					a.mu.Lock()
					a.dimension = len(query)
					a.mu.Unlock()
				} else {
					// Dimension mismatch - cannot compare vectors of different dimensions
					// Return empty results instead of error (better UX - allows fallback to keyword search)
					return []VectorResult{}, nil
				}
			} else {
				// No vectors match the filters, update dimension for future inserts
				a.mu.Lock()
				a.dimension = len(query)
				a.mu.Unlock()
			}
		} else {
			// No vectors stored yet, just update dimension to match query
			a.mu.Lock()
			a.dimension = len(query)
			a.mu.Unlock()
		}
	}
	
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	// Collect all vectors that match filters
	var candidates []struct {
		id       uuid.UUID
		vector   []float32
		metadata map[string]interface{}
	}
	
	for id, iv := range a.vectors {
		if !matchesFilters(iv.entry, filters) {
			continue
		}
		// Skip vectors with dimension mismatch
		if len(iv.vector) != len(query) {
			continue
		}
		candidates = append(candidates, struct {
			id       uuid.UUID
			vector   []float32
			metadata map[string]interface{}
		}{
			id:       id,
			vector:   iv.vector,
			metadata: iv.entry.Metadata,
		})
	}
	
	// Compute distances
	type scored struct {
		id       uuid.UUID
		distance float32
		metadata map[string]interface{}
	}
	
	results := make([]scored, len(candidates))
	for i, c := range candidates {
		dist := cosineDistance(query, c.vector)
		results[i] = scored{
			id:       c.id,
			distance: dist,
			metadata: c.metadata,
		}
	}
	
	// Sort by distance (ascending)
	sort.Slice(results, func(i, j int) bool {
		return results[i].distance < results[j].distance
	})
	
	// Return top k
	if k > len(results) {
		k = len(results)
	}
	
	output := make([]VectorResult, k)
	for i := 0; i < k; i++ {
		output[i] = VectorResult{
			ID:       results[i].id,
			Distance: results[i].distance,
			Score:    1 - results[i].distance, // Convert distance to similarity
			Metadata: results[i].metadata,
		}
	}
	
	return output, nil
}

// Insert adds vectors to the index.
func (a *FAISSAdapter) Insert(ctx context.Context, vectors []VectorEntry) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// If no vectors stored yet and we have vectors to insert, detect dimension from first vector
	if len(a.vectors) == 0 && len(vectors) > 0 {
		// Find first vector with non-zero length
		for _, v := range vectors {
			if len(v.Vector) > 0 {
				a.dimension = len(v.Vector)
				break
			}
		}
	}
	
	for _, v := range vectors {
		if len(v.Vector) == 0 {
			continue // Skip empty vectors
		}
		
		// Dynamically adapt dimension if needed (similar to Search method)
		// This handles the case where vectors are loaded from database with different dimension
		if len(v.Vector) != a.dimension {
			if len(a.vectors) == 0 {
				// No vectors stored yet, update dimension to match this vector
				a.dimension = len(v.Vector)
			} else {
				// Check if all existing vectors have the same dimension as this one
				var existingDim int
				for _, iv := range a.vectors {
					if len(iv.vector) > 0 {
						existingDim = len(iv.vector)
						break
					}
				}
				if len(v.Vector) == existingDim {
					// Update dimension to match existing vectors
					a.dimension = len(v.Vector)
				} else {
					return fmt.Errorf("%w: expected %d, got %d for id %s (existing vectors have dimension %d)", 
						ErrVectorDimensionMismatch, a.dimension, len(v.Vector), v.ID, existingDim)
				}
			}
		}
		
		// Normalize vector for cosine similarity
		normalized := normalizeVector(v.Vector)
		
		a.vectors[v.ID] = indexedVector{
			entry:  v,
			vector: normalized,
		}
	}
	
	return nil
}

// Delete removes vectors from the index.
func (a *FAISSAdapter) Delete(ctx context.Context, ids []uuid.UUID) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	for _, id := range ids {
		delete(a.vectors, id)
	}
	
	return nil
}

// Count returns the number of vectors in the index.
func (a *FAISSAdapter) Count(ctx context.Context) (int64, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return int64(len(a.vectors)), nil
}

// Close releases resources.
func (a *FAISSAdapter) Close() error {
	// In a real implementation, this would persist the index
	return nil
}

// matchesFilters checks if an entry matches the given filters.
func matchesFilters(entry VectorEntry, filters VectorFilters) bool {
	if filters.TenantID != nil && entry.TenantID != *filters.TenantID {
		return false
	}
	
	if len(filters.ProductIDs) > 0 {
		found := false
		for _, pid := range filters.ProductIDs {
			if entry.ProductID == pid {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	
	if filters.CampaignVariantID != nil {
		if entry.CampaignVariantID == nil || *entry.CampaignVariantID != *filters.CampaignVariantID {
			return false
		}
	}
	
	if len(filters.ChunkTypes) > 0 {
		found := false
		for _, ct := range filters.ChunkTypes {
			if entry.ChunkType == ct {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	
	if len(filters.Visibility) > 0 {
		found := false
		for _, v := range filters.Visibility {
			if entry.Visibility == v {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	
	if filters.EmbeddingVersion != nil && entry.EmbeddingVersion != *filters.EmbeddingVersion {
		return false
	}
	
	return true
}

// cosineDistance computes cosine distance between two normalized vectors.
// For normalized vectors: distance = 1 - dot(a, b)
func cosineDistance(a, b []float32) float32 {
	if len(a) != len(b) {
		return 1.0
	}
	
	var dot float32
	for i := range a {
		dot += a[i] * b[i]
	}
	
	// Clamp to [-1, 1] range due to floating point errors
	if dot > 1 {
		dot = 1
	} else if dot < -1 {
		dot = -1
	}
	
	return 1 - dot
}

// normalizeVector returns a unit vector.
func normalizeVector(v []float32) []float32 {
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	norm = math.Sqrt(norm)
	
	if norm == 0 {
		return v
	}
	
	normalized := make([]float32, len(v))
	for i, x := range v {
		normalized[i] = float32(float64(x) / norm)
	}
	
	return normalized
}

// PGVectorAdapter implements VectorAdapter using PostgreSQL's pgvector extension.
type PGVectorAdapter struct {
	// db *pgxpool.Pool
	dimension int
}

// PGVectorConfig holds PGVector adapter configuration.
type PGVectorConfig struct {
	DSN       string
	Dimension int
	IndexType string // ivfflat or hnsw
	Lists     int
}

// NewPGVectorAdapter creates a new PGVector adapter.
func NewPGVectorAdapter(cfg PGVectorConfig) (*PGVectorAdapter, error) {
	// TODO: Initialize pgx pool connection
	// For now, return a placeholder
	
	if cfg.Dimension <= 0 {
		cfg.Dimension = 768
	}
	
	return &PGVectorAdapter{
		dimension: cfg.Dimension,
	}, nil
}

// Search finds the k nearest neighbors using PGVector.
func (a *PGVectorAdapter) Search(ctx context.Context, query []float32, k int, filters VectorFilters) ([]VectorResult, error) {
	// TODO: Implement PGVector search
	// SELECT id, embedding_vector <-> $1 AS distance
	// FROM knowledge_chunks
	// WHERE tenant_id = $2 AND ...
	// ORDER BY distance
	// LIMIT $3
	
	return nil, errors.New("pgvector adapter not yet implemented")
}

// Insert adds vectors using PGVector.
func (a *PGVectorAdapter) Insert(ctx context.Context, vectors []VectorEntry) error {
	// TODO: Implement PGVector insert
	return errors.New("pgvector adapter not yet implemented")
}

// Delete removes vectors from PGVector.
func (a *PGVectorAdapter) Delete(ctx context.Context, ids []uuid.UUID) error {
	// TODO: Implement PGVector delete
	return errors.New("pgvector adapter not yet implemented")
}

// Count returns the number of vectors.
func (a *PGVectorAdapter) Count(ctx context.Context) (int64, error) {
	// TODO: Implement count query
	return 0, errors.New("pgvector adapter not yet implemented")
}

// Close closes the database connection.
func (a *PGVectorAdapter) Close() error {
	// TODO: Close pgx pool
	return nil
}

