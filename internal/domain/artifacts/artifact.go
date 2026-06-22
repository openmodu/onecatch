package artifacts

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("artifact not found")
var ErrNotReady = errors.New("artifact is not ready")

type Artifact struct {
	ID        string    `json:"id"`
	OrderID   string    `json:"orderId"`
	UserID    string    `json:"userId"`
	FileName  string    `json:"fileName"`
	FileType  string    `json:"fileType"`
	SizeBytes int64     `json:"sizeBytes"`
	Preview   string    `json:"preview"`
	CreatedAt time.Time `json:"createdAt"`

	// StorageURI is the absolute path to the real file the agent produced in
	// its workspace. It is internal infrastructure detail and is never exposed
	// over the user-facing API (see the PRD's minimal-disclosure rule).
	StorageURI string `json:"-"`
}

type Download struct {
	Artifact    Artifact `json:"artifact"`
	ContentType string   `json:"contentType"`
	Content     []byte   `json:"-"`
}

type Share struct {
	ArtifactID string    `json:"artifactId"`
	Token      string    `json:"token"`
	URL        string    `json:"url"`
	CreatedAt  time.Time `json:"createdAt"`
}
