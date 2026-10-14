package appupdate

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Release preserves the on-disk cache schema while keeping updates independent
// of the window toolkit. Signatures authenticate the SHA-256 artifact digest.
type Release struct {
	Version      string         `json:"version"`
	Channel      string         `json:"channel,omitempty"`
	Name         string         `json:"name,omitempty"`
	Notes        string         `json:"notes,omitempty"`
	PublishedAt  time.Time      `json:"publishedAt,omitempty"`
	Artifact     Artifact       `json:"artifact"`
	Verification *Verification  `json:"verification,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	Provider     string         `json:"provider,omitempty"`
}
type Artifact struct {
	Filename string `json:"filename"`
	Filetype string `json:"filetype,omitempty"`
	Size     int64  `json:"size,omitempty"`
	Platform string `json:"platform,omitempty"`
	Arch     string `json:"arch,omitempty"`
}
type Verification struct {
	DigestAlgo    string `json:"digestAlgo,omitempty"`
	Digest        []byte `json:"digest,omitempty"`
	SignatureAlgo string `json:"signatureAlgo,omitempty"`
	Signature     []byte `json:"signature,omitempty"`
}
type updateEngine struct {
	url, currentVersion            string
	key                            ed25519.PublicKey
	mu                             sync.RWMutex
	state, downloaded, artifactURL string
	release                        *Release
}

func (e *updateEngine) State() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.state == "" {
		return "idle"
	}
	return e.state
}
func (e *updateEngine) DownloadedPath() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.downloaded
}
func (e *updateEngine) setState(state string) { e.mu.Lock(); defer e.mu.Unlock(); e.state = state }
func (e *updateEngine) Check(ctx context.Context) (_ *Release, resultErr error) {
	e.setState("checking")
	defer func() {
		if resultErr != nil {
			e.setState("error")
		}
	}()
	response, err := updateRequest(ctx, e.url)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var feed struct {
		Items []struct {
			Title       string `xml:"title"`
			Description string `xml:"description"`
			Version     string `xml:"version"`
			OS          string `xml:"os"`
			Published   string `xml:"pubDate"`
			Enclosure   struct {
				URL       string `xml:"url,attr"`
				Size      int64  `xml:"length,attr"`
				Signature string `xml:"edSignature,attr"`
			} `xml:"enclosure"`
		} `xml:"channel>item"`
	}
	if err := xml.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&feed); err != nil {
		return nil, fmt.Errorf("app update: decode feed: %w", err)
	}
	var latest *Release
	var artifactURL string
	for _, item := range feed.Items {
		if item.OS != "" && item.OS != runtime.GOOS {
			continue
		}
		if !versionIsNewer(item.Version, e.currentVersion) || latest != nil && !versionIsNewer(item.Version, latest.Version) {
			continue
		}
		address, err := url.Parse(item.Enclosure.URL)
		if err != nil || address.Host == "" || address.Scheme != "https" && address.Scheme != "http" {
			return nil, errors.New("app update: invalid artifact URL")
		}
		signature, err := base64.StdEncoding.DecodeString(item.Enclosure.Signature)
		if err != nil {
			return nil, errors.New("app update: invalid signature encoding")
		}
		published, _ := http.ParseTime(item.Published)
		latest = &Release{Version: item.Version, Name: item.Title, Notes: item.Description, PublishedAt: published,
			Artifact:     Artifact{Filename: path.Base(address.Path), Size: item.Enclosure.Size, Platform: runtime.GOOS, Arch: runtime.GOARCH},
			Verification: &Verification{SignatureAlgo: "ed25519", Signature: signature}}
		if err := validateSignedRelease(latest); err != nil {
			return nil, err
		}
		artifactURL = address.String()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.release, e.artifactURL = latest, artifactURL
	e.state = "up-to-date"
	if latest != nil {
		e.state = "available"
	}
	return latest, nil
}
func updateRequest(ctx context.Context, address string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	response, err := updateHTTPClient().Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("app update: HTTP %d", response.StatusCode)
	}
	return response, nil
}
func (e *updateEngine) DownloadAndInstall(ctx context.Context) (resultErr error) {
	e.mu.RLock()
	release, address := e.release, e.artifactURL
	e.mu.RUnlock()
	if err := validateSignedRelease(release); err != nil {
		return err
	}
	e.setState("downloading")
	defer func() {
		if resultErr != nil {
			e.setState("error")
		}
	}()
	response, err := updateRequest(ctx, address)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	directory, err := os.MkdirTemp("", "onecatch-update-")
	if err != nil {
		return err
	}
	defer func() {
		if resultErr != nil {
			os.RemoveAll(directory)
		}
	}()
	filename := filepath.Base(release.Artifact.Filename)
	if !validCacheName(filename) {
		return errors.New("app update: unsafe artifact filename")
	}
	destination := filepath.Join(directory, filename)
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	digest := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, digest), response.Body)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if release.Artifact.Size > 0 && size != release.Artifact.Size {
		return errors.New("app update: artifact size mismatch")
	}
	if !ed25519.Verify(e.key, digest.Sum(nil), release.Verification.Signature) {
		return errors.New("app update: artifact signature verification failed")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.downloaded, e.state = destination, "ready"
	return nil
}
