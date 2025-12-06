package ui

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CampaignDisplay represents a campaign for display purposes.
type CampaignDisplay struct {
	Number      int
	ID          uuid.UUID
	Name        string
	ProductName string
	Locale      string
	Trim        *string
	Market      *string
	Status      string
	CreatedAt   time.Time
}

// SelectCampaign displays campaigns in a table and allows user to select one.
func SelectCampaign(campaigns []CampaignDisplay) (uuid.UUID, error) {
	if len(campaigns) == 0 {
		return uuid.Nil, fmt.Errorf("no campaigns available")
	}
	
	// Display campaigns table (ASCII-safe for consistent rendering)
	fmt.Println()
	fmt.Println("+------------------------------------------------------------+")
	fmt.Println("| Available Campaigns                                        |")
	fmt.Println("+----+-------------------------+---------------+------------+-------+")
	fmt.Println("| #  | Campaign Name           | Product       | Status     | Loc   |")
	fmt.Println("+----+-------------------------+---------------+------------+-------+")
	for _, campaign := range campaigns {
		statusText := "Published"
		if campaign.Status == "draft" {
			statusText = "Draft"
		}
		fmt.Printf("| %-2d | %-23s | %-13s | %-10s | %-5s |\n",
			campaign.Number,
			truncate(campaign.Name, 23),
			truncate(campaign.ProductName, 13),
			statusText,
			campaign.Locale,
		)
	}
	fmt.Println("+----+-------------------------+---------------+------------+-------+")
	fmt.Println()
	
	// Prompt for selection
	choice, err := PromptInt(fmt.Sprintf("Select a campaign (1-%d) or '0' to go back", len(campaigns)))
	if err != nil {
		return uuid.Nil, err
	}
	
	if choice == 0 {
		return uuid.Nil, fmt.Errorf("cancelled")
	}
	
	if choice < 1 || choice > len(campaigns) {
		return uuid.Nil, fmt.Errorf("invalid choice: %d", choice)
	}
	
	return campaigns[choice-1].ID, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

