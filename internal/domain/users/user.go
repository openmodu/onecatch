package users

import "time"

const DevUserID = "local-dev"

type User struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"displayName"`
	Email       string    `json:"email"`
	AvatarURL   string    `json:"avatarUrl,omitempty"`
	Status      string    `json:"status,omitempty"`
	CreatedAt   time.Time `json:"createdAt,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt,omitempty"`
}

type AuthIdentity struct {
	ID              string    `json:"id"`
	UserID          string    `json:"userId"`
	Provider        string    `json:"provider"`
	ProviderSubject string    `json:"providerSubject"`
	Email           string    `json:"email,omitempty"`
	DisplayName     string    `json:"displayName,omitempty"`
	AvatarURL       string    `json:"avatarUrl,omitempty"`
	CreatedAt       time.Time `json:"createdAt,omitempty"`
	UpdatedAt       time.Time `json:"updatedAt,omitempty"`
}
