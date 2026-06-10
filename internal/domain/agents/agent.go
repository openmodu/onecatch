package agents

import "errors"

var ErrNotFound = errors.New("agent not found")

type Agent struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Category          string   `json:"category"`
	Description       string   `json:"description"`
	PriceUses         int      `json:"priceUses"`
	EstimatedDuration string   `json:"estimatedDuration"`
	ArtifactTypes     []string `json:"artifactTypes"`
}

func SeedCatalog() []Agent {
	return []Agent{
		{
			ID:                "market-research",
			Name:              "Market Research Agent",
			Category:          "research",
			Description:       "Collects market signals and produces a concise research report.",
			PriceUses:         1,
			EstimatedDuration: "10-20 min",
			ArtifactTypes:     []string{"report"},
		},
	}
}
