// Package vector provides per-campaign vector store management.
package vector

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	keretrieval "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/retrieval"
	kestorage "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// StoreManager manages per-campaign FAISS vector stores.
type StoreManager struct {
	vectorStoreRoot string
	dimension       int
	stores          map[uuid.UUID]*keretrieval.FAISSAdapter
}

// NewStoreManager creates a new vector store manager.
func NewStoreManager(vectorStoreRoot string, dimension int) *StoreManager {
	return &StoreManager{
		vectorStoreRoot: vectorStoreRoot,
		dimension:       dimension,
		stores:          make(map[uuid.UUID]*keretrieval.FAISSAdapter),
	}
}

// GetOrCreateStore gets or creates a FAISS adapter for a campaign.
// If the FAISS index file exists, it loads it. Otherwise, it creates a new one.
func (m *StoreManager) GetOrCreateStore(campaignID uuid.UUID) (*keretrieval.FAISSAdapter, error) {
	// Check if store already exists in memory
	if store, exists := m.stores[campaignID]; exists {
		return store, nil
	}

	// Ensure store directory exists
	if err := m.EnsureStoreDir(campaignID); err != nil {
		return nil, fmt.Errorf("ensure store directory: %w", err)
	}

	// Create or load store
	storePath := m.getStorePath(campaignID)
	store, err := keretrieval.NewFAISSAdapter(keretrieval.FAISSConfig{
		Dimension: m.dimension,
		IndexPath: storePath,
	})
	if err != nil {
		return nil, fmt.Errorf("create/load FAISS adapter: %w", err)
	}

	// Cache it
	m.stores[campaignID] = store

	return store, nil
}

// GetOrLoadStore gets or loads a FAISS adapter, and if the index is empty,
// loads vectors from the database.
func (m *StoreManager) GetOrLoadStore(
	ctx context.Context,
	campaignID uuid.UUID,
	tenantID uuid.UUID,
	chunkRepo *kestorage.KnowledgeChunkRepository,
	specFactRepo *kestorage.SpecFactChunkRepository,
	specValueRepo *kestorage.SpecValueRepository,
) (*keretrieval.FAISSAdapter, error) {
	store, err := m.GetOrCreateStore(campaignID)
	if err != nil {
		return nil, err
	}

	// Check if index is empty or needs sync from DB
	count, err := store.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count vectors in index: %w", err)
	}

	// If index is empty or very small, sync from database
	if count == 0 {
		if err := m.LoadVectorsFromDB(ctx, store, tenantID, campaignID, chunkRepo, specFactRepo, specValueRepo); err != nil {
			return nil, fmt.Errorf("load vectors from DB: %w", err)
		}
	}

	return store, nil
}

// DeleteStore deletes a campaign's vector store.
func (m *StoreManager) DeleteStore(campaignID uuid.UUID) error {
	// Remove from cache
	if store, exists := m.stores[campaignID]; exists {
		_ = store.Close()
		delete(m.stores, campaignID)
	}

	// Delete directory
	storeDir := m.getStoreDir(campaignID)
	return os.RemoveAll(storeDir)
}

// DeleteAllStores deletes all vector stores.
func (m *StoreManager) DeleteAllStores() error {
	// Close all cached stores
	for _, store := range m.stores {
		_ = store.Close()
	}
	m.stores = make(map[uuid.UUID]*keretrieval.FAISSAdapter)

	// Delete root directory
	return os.RemoveAll(m.vectorStoreRoot)
}

// LoadVectorsFromDB loads vectors from database into the FAISS store for a campaign.
// This is used to populate FAISS from the persistent database on startup.
func (m *StoreManager) LoadVectorsFromDB(
	ctx context.Context,
	store *keretrieval.FAISSAdapter,
	tenantID uuid.UUID,
	campaignID uuid.UUID,
	chunkRepo *kestorage.KnowledgeChunkRepository,
	specFactRepo *kestorage.SpecFactChunkRepository,
	specValueRepo *kestorage.SpecValueRepository,
) error {
	// Get chunks with embeddings for this campaign
	chunks, err := chunkRepo.GetByCampaign(ctx, tenantID, campaignID)
	if err != nil {
		return fmt.Errorf("get chunks: %w", err)
	}

	// Convert to vector entries
	vectorEntries := make([]keretrieval.VectorEntry, 0, len(chunks))
	for _, chunk := range chunks {
		if len(chunk.EmbeddingVector) == 0 {
			continue
		}

		embeddingVersion := ""
		if chunk.EmbeddingVersion != nil {
			embeddingVersion = *chunk.EmbeddingVersion
		}

		vectorEntries = append(vectorEntries, keretrieval.VectorEntry{
			ID:                chunk.ID,
			TenantID:          chunk.TenantID,
			ProductID:         chunk.ProductID,
			CampaignVariantID: &campaignID,
			ChunkType:         string(chunk.ChunkType),
			Visibility:        string(chunk.Visibility),
			EmbeddingVersion:  embeddingVersion,
			Vector:            chunk.EmbeddingVector,
			Metadata: map[string]interface{}{
				"text":       chunk.Text,
				"chunk_type": string(chunk.ChunkType),
			},
		})
	}

	// Load spec_fact chunks (embeddings) if available
	if specFactRepo != nil {
		// Preload spec values for enrichment (explanation, features, availability)
		specValueByID := map[uuid.UUID]*kestorage.SpecValue{}
		if specValueRepo != nil {
			if specs, err := specValueRepo.GetByCampaign(ctx, tenantID, campaignID); err == nil {
				for _, sv := range specs {
					specValueByID[sv.ID] = sv
				}
			}
		}

		specFactChunks, err := specFactRepo.GetByCampaign(ctx, tenantID, campaignID)
		if err != nil {
			return fmt.Errorf("get spec_fact chunks: %w", err)
		}
		for _, chunk := range specFactChunks {
			if len(chunk.EmbeddingVector) == 0 {
				continue
			}
			embeddingVersion := ""
			if chunk.EmbeddingVersion != nil {
				embeddingVersion = *chunk.EmbeddingVersion
			}

			vectorEntries = append(vectorEntries, keretrieval.VectorEntry{
				ID:                chunk.ID,
				TenantID:          chunk.TenantID,
				ProductID:         chunk.ProductID,
				CampaignVariantID: &chunk.CampaignVariantID,
				ChunkType:         string(kestorage.ChunkTypeSpecFact),
				Visibility:        string(kestorage.VisibilityPrivate),
				EmbeddingVersion:  embeddingVersion,
				Vector:            chunk.EmbeddingVector,
				Metadata:          m.buildSpecFactMetadata(chunk, specValueByID[chunk.SpecValueID]),
			})
		}
	}

	// Insert into FAISS store
	if len(vectorEntries) > 0 {
		if err := store.Insert(ctx, vectorEntries); err != nil {
			return fmt.Errorf("insert vectors: %w", err)
		}
	}

	return nil
}

// getStorePath returns the file path for a campaign's FAISS index.
func (m *StoreManager) getStorePath(campaignID uuid.UUID) string {
	return filepath.Join(m.getStoreDir(campaignID), "index.faiss")
}

// getStoreDir returns the directory path for a campaign's vector store.
func (m *StoreManager) getStoreDir(campaignID uuid.UUID) string {
	return filepath.Join(m.vectorStoreRoot, campaignID.String())
}

// EnsureStoreDir ensures the store directory exists for a campaign.
func (m *StoreManager) EnsureStoreDir(campaignID uuid.UUID) error {
	storeDir := m.getStoreDir(campaignID)
	return os.MkdirAll(storeDir, 0755)
}

// buildSpecFactMetadata enriches spec_fact vector entries with structured fields used by semantic fallback.
func (m *StoreManager) buildSpecFactMetadata(chunk *kestorage.SpecFactChunk, specVal *kestorage.SpecValue) map[string]interface{} {
	meta := map[string]interface{}{
		"chunk_text":          chunk.ChunkText,
		"spec_value_id":       chunk.SpecValueID.String(),
		"chunk_type":          string(kestorage.ChunkTypeSpecFact),
		"campaign_variant_id": chunk.CampaignVariantID.String(),
	}

	// Attempt to parse category/name/value/unit/key features from chunk text as a fallback.
	category, name, value, unit, keyFeatures, variantAvailability := parseSpecFactChunkText(chunk.ChunkText)

	// Prefer canonical values from the spec_value row when available.
	if specVal != nil {
		if specVal.ValueText != nil && strings.TrimSpace(*specVal.ValueText) != "" {
			value = strings.TrimSpace(*specVal.ValueText)
		} else if specVal.ValueNumeric != nil {
			value = strings.TrimRight(strings.TrimRight(fmt.Sprintf("%g", *specVal.ValueNumeric), "0"), ".")
		}
		if specVal.Unit != nil {
			unit = strings.TrimSpace(*specVal.Unit)
		}
		if specVal.KeyFeatures != nil {
			keyFeatures = strings.TrimSpace(*specVal.KeyFeatures)
		}
		if specVal.VariantAvailability != nil {
			variantAvailability = strings.TrimSpace(*specVal.VariantAvailability)
		}
		if specVal.Explanation != nil {
			meta["explanation"] = strings.TrimSpace(*specVal.Explanation)
		}
		if specVal.SourceDocID != nil {
			meta["source_doc_id"] = specVal.SourceDocID.String()
		}
		if specVal.SourcePage != nil {
			meta["source_page"] = *specVal.SourcePage
		}
		if specVal.SpecItemID != uuid.Nil {
			meta["spec_item_id"] = specVal.SpecItemID.String()
		}
	}

	if category != "" {
		meta["category"] = category
	}
	if name != "" {
		meta["name"] = name
	}
	if value != "" {
		meta["value"] = value
	}
	if unit != "" {
		meta["unit"] = unit
	}
	if keyFeatures != "" {
		meta["key_features"] = keyFeatures
	}
	if variantAvailability != "" {
		meta["variant_availability"] = variantAvailability
	}

	return meta
}

// parseSpecFactChunkText extracts structured fields from the spec_fact chunk text format.
func parseSpecFactChunkText(text string) (category, name, value, unit, keyFeatures, variantAvailability string) {
	parts := strings.Split(text, ";")
	header := strings.TrimSpace(parts[0])

	if header != "" {
		headerParts := strings.SplitN(header, ">", 2)
		if len(headerParts) == 2 {
			category = strings.TrimSpace(headerParts[0])
			header = strings.TrimSpace(headerParts[1])
		}

		nameValue := strings.SplitN(header, ":", 2)
		if len(nameValue) == 2 {
			name = strings.TrimSpace(nameValue[0])
			valuePart := strings.TrimSpace(nameValue[1])
			valueTokens := strings.Fields(valuePart)
			if len(valueTokens) > 1 {
				value = strings.Join(valueTokens[:len(valueTokens)-1], " ")
				unit = valueTokens[len(valueTokens)-1]
			} else {
				value = valuePart
			}
		} else {
			name = strings.TrimSpace(header)
		}
	}

	// Parse optional sections (Key features, Availability)
	for _, part := range parts[1:] {
		p := strings.TrimSpace(part)
		lower := strings.ToLower(p)
		switch {
		case strings.HasPrefix(lower, "key features:"):
			keyFeatures = strings.TrimSpace(strings.TrimPrefix(p, "Key features:"))
		case strings.HasPrefix(lower, "availability:"):
			variantAvailability = strings.TrimSpace(strings.TrimPrefix(p, "Availability:"))
		case strings.HasPrefix(lower, "gloss:") && keyFeatures == "":
			// If gloss is present and key features missing, reuse gloss as a lightweight feature string.
			keyFeatures = strings.TrimSpace(strings.TrimPrefix(p, "Gloss:"))
		}
	}

	// Clean unit/value whitespace
	value = strings.TrimSpace(value)
	unit = strings.TrimSpace(unit)
	keyFeatures = strings.TrimSpace(keyFeatures)
	variantAvailability = strings.TrimSpace(variantAvailability)

	// If value still contains unit appended, try to split last token as unit
	if unit == "" {
		tokens := strings.Fields(value)
		if len(tokens) > 1 {
			if _, err := strconv.ParseFloat(tokens[len(tokens)-1], 64); err != nil {
				unit = tokens[len(tokens)-1]
				value = strings.Join(tokens[:len(tokens)-1], " ")
			}
		}
	}

	return
}
