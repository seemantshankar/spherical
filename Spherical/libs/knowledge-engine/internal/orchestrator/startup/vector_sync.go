package startup

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/campaign"
	"github.com/spherical-ai/spherical/libs/knowledge-engine/internal/orchestrator/vector"
	kestorage "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/storage"
)

// SyncVectorStores syncs all campaign vector stores from the database on startup.
// This ensures FAISS indexes are populated from persistent database storage.
func SyncVectorStores(ctx context.Context, vectorMgr *vector.StoreManager, campaignMgr *campaign.Manager, tenantID uuid.UUID, repos *kestorage.Repositories) error {
	// List all campaigns
	campaigns, err := campaignMgr.ListCampaigns(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("list campaigns: %w", err)
	}

	if len(campaigns) == 0 {
		// No campaigns to sync
		return nil
	}

	syncedCount := 0
	for _, camp := range campaigns {
		// Get or load the store (will sync from DB if needed)
		_, err := vectorMgr.GetOrLoadStore(ctx, camp.ID, tenantID, repos.KnowledgeChunks, repos.SpecFactChunks, repos.SpecValues)
		if err != nil {
			// Log error but continue with other campaigns
			fmt.Printf("Warning: Failed to sync vector store for campaign %s: %v\n", camp.ID, err)
			continue
		}
		syncedCount++
	}

	if syncedCount > 0 {
		fmt.Printf("Synced %d vector store(s) from database\n", syncedCount)
	}

	return nil
}
