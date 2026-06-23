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
	Token     string    `json:"token"`
	Provider  string    `json:"provider"`
	User      User      `json:"user"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
}

type OAuthStart struct {
	Provider string `json:"provider"`
	AuthURL  string `json:"authUrl"`
	State    string `json:"state"`
}

type ConversationMessage struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Kind      string    `json:"kind"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}

type Conversation struct {
	ID        string                `json:"id"`
	AgentID   string                `json:"agentId"`
	AgentName string                `json:"agentName"`
	Status    string                `json:"status"`
	OrderID   string                `json:"orderId,omitempty"`
	Messages  []ConversationMessage `json:"messages"`
	CreatedAt time.Time             `json:"createdAt"`
	UpdatedAt time.Time             `json:"updatedAt"`
}

type StartConversationRequest struct {
	AgentID string `json:"agentId"`
	// Workspace optionally points the agent at one of the user's own directories.
	Workspace string `json:"workspace,omitempty"`
}

type PostMessageRequest struct {
	Text string `json:"text"`
}

type Agent struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Category          string   `json:"category"`
	Tags              []string `json:"tags"`
	Description       string   `json:"description"`
	PriceUses         int      `json:"priceUses"`
	PriceCents        int      `json:"priceCents"`
	Rating            string   `json:"rating"`
	DealCount         int      `json:"dealCount"`
	EstimatedDuration string   `json:"estimatedDuration"`
	Deliverable       string   `json:"deliverable"`
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
	PaymentID    string    `json:"paymentId,omitempty"`
	Delta        int       `json:"delta"`
	BalanceAfter int       `json:"balanceAfter"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Purchase struct {
	ID           string    `json:"id"`
	UserID       string    `json:"userId"`
	PlanID       string    `json:"planId"`
	PaymentID    string    `json:"paymentId"`
	Uses         int       `json:"uses"`
	AmountCents  int       `json:"amountCents"`
	BalanceAfter int       `json:"balanceAfter"`
	CreatedAt    time.Time `json:"createdAt"`
}

type StartPurchaseRequest struct {
	PlanID    string `json:"planId"`
	PaymentID string `json:"paymentId,omitempty"`
}

type Requirement struct {
	Prompt string `json:"prompt"`
}

type Order struct {
	ID                    string         `json:"id"`
	UserID                string         `json:"userId"`
	AgentID               string         `json:"agentId"`
	AgentName             string         `json:"agentName"`
	Requirement           Requirement    `json:"requirement"`
	Status                string         `json:"status"`
	UsageCost             int            `json:"usageCost"`
	AmountCents           int            `json:"amountCents"`
	EstimatedCompletionAt time.Time      `json:"estimatedCompletionAt,omitempty"`
	FailureReason         string         `json:"failureReason,omitempty"`
	Progress              []ProgressStep `json:"progress"`
	CreatedAt             time.Time      `json:"createdAt"`
	UpdatedAt             time.Time      `json:"updatedAt"`
}

type ProgressStep struct {
	Key       string    `json:"key"`
	Label     string    `json:"label"`
	State     string    `json:"state"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}

type CreateOrderRequest struct {
	AgentID     string      `json:"agentId"`
	Requirement Requirement `json:"requirement"`
	// Workspace optionally points the agent at one of your own directories.
	Workspace string `json:"workspace,omitempty"`
}

// ContinueOrderRequest carries a follow-up instruction that resumes a finished
// task (multi-turn continuation) in its original workspace and session.
type ContinueOrderRequest struct {
	Prompt string `json:"prompt"`
}

// RunEvent is one normalized step streamed by the local agent runtime as it
// works (assistant message, tool use, file change, etc.).
type RunEvent struct {
	Kind string    `json:"kind"`
	Text string    `json:"text,omitempty"`
	At   time.Time `json:"at"`
}

// RunLog is the live view of an order's agent run, used by the desktop to show
// the agent working in real time.
type RunLog struct {
	OrderID      string     `json:"orderId"`
	Runtime      string     `json:"runtime,omitempty"`
	Status       string     `json:"status"`
	Events       []RunEvent `json:"events"`
	FinalMessage string     `json:"finalMessage,omitempty"`
	Error        string     `json:"error,omitempty"`
	StartedAt    time.Time  `json:"startedAt,omitempty"`
	UpdatedAt    time.Time  `json:"updatedAt,omitempty"`
}

type Artifact struct {
	ID        string    `json:"id"`
	OrderID   string    `json:"orderId"`
	UserID    string    `json:"userId"`
	FileName  string    `json:"fileName"`
	FileType  string    `json:"fileType"`
	SizeBytes int64     `json:"sizeBytes"`
	Preview   string    `json:"preview"`
	CreatedAt time.Time `json:"createdAt"`
}

type ArtifactDownload struct {
	ArtifactID  string `json:"artifactId"`
	FileName    string `json:"fileName"`
	FilePath    string `json:"filePath"`
	ContentType string `json:"contentType"`
}

type ArtifactShare struct {
	ArtifactID string    `json:"artifactId"`
	Token      string    `json:"token"`
	URL        string    `json:"url"`
	CreatedAt  time.Time `json:"createdAt"`
}
