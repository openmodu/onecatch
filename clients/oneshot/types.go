package oneshot

import "time"

type ErrorResponse struct {
	Error string `json:"error"`
}

type User struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
	AvatarURL   string `json:"avatarUrl,omitempty"`
}

type Session struct {
	Token    string `json:"token"`
	Provider string `json:"provider"`
	User     User   `json:"user"`
}

type Agent struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Category          string   `json:"category"`
	Description       string   `json:"description"`
	PriceUses         int      `json:"priceUses"`
	EstimatedDuration string   `json:"estimatedDuration"`
	ArtifactTypes     []string `json:"artifactTypes"`
}

type Balance struct {
	UserID    string `json:"userId"`
	Remaining int    `json:"remaining"`
}

type LedgerEntry struct {
	ID           string    `json:"id"`
	UserID       string    `json:"userId"`
	Type         string    `json:"type"`
	OrderID      string    `json:"orderId,omitempty"`
	Delta        int       `json:"delta"`
	BalanceAfter int       `json:"balanceAfter"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Requirement struct {
	Prompt string `json:"prompt"`
}

type Order struct {
	ID          string      `json:"id"`
	UserID      string      `json:"userId"`
	AgentID     string      `json:"agentId"`
	Requirement Requirement `json:"requirement"`
	Status      string      `json:"status"`
	UsageCost   int         `json:"usageCost"`
	CreatedAt   time.Time   `json:"createdAt"`
	UpdatedAt   time.Time   `json:"updatedAt"`
}

type CreateOrderRequest struct {
	AgentID     string      `json:"agentId"`
	Requirement Requirement `json:"requirement"`
}
