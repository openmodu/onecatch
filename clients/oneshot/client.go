package oneshot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Client interface {
	CurrentUser(context.Context) (User, error)
	StartWechat(context.Context) (OAuthStart, error)
	LoginWithWechat(context.Context) (Session, error)
	LoginWithGoogle(context.Context) (Session, error)
	Logout(context.Context) error
	ListAgents(context.Context) ([]Agent, error)
	GetAgent(context.Context, string) (Agent, error)
	GetBalance(context.Context) (Balance, error)
	ListLedger(context.Context) ([]LedgerEntry, error)
	StartPurchase(context.Context, StartPurchaseRequest) (Purchase, error)
	CreateOrder(context.Context, CreateOrderRequest) (Order, error)
	ListOrders(context.Context) ([]Order, error)
	GetOrder(context.Context, string) (Order, error)
	CancelOrder(context.Context, string) (Order, error)
	ListArtifacts(context.Context, string) ([]Artifact, error)
	DownloadArtifact(context.Context, string) (ArtifactDownload, error)
	ShareArtifact(context.Context, string) (ArtifactShare, error)
}

type HTTPClient struct {
	baseURL    *url.URL
	httpClient *http.Client
	token      string
}

func NewHTTPClient(baseURL string) *HTTPClient {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		parsed, _ = url.Parse("http://127.0.0.1:8080")
	}

	client := &HTTPClient{
		baseURL: parsed,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
	client.token, _ = loadSessionToken()
	return client
}

func (c *HTTPClient) CurrentUser(ctx context.Context) (User, error) {
	var out User
	err := c.do(ctx, http.MethodGet, "/api/me", nil, &out)
	return out, err
}

func (c *HTTPClient) StartWechat(ctx context.Context) (OAuthStart, error) {
	var out OAuthStart
	err := c.do(ctx, http.MethodPost, "/api/auth/wechat/start", nil, &out)
	return out, err
}

func (c *HTTPClient) LoginWithWechat(ctx context.Context) (Session, error) {
	var out Session
	err := c.do(ctx, http.MethodPost, "/api/auth/wechat/callback", nil, &out)
	if err == nil {
		c.setToken(out.Token)
	}
	return out, err
}

func (c *HTTPClient) LoginWithGoogle(ctx context.Context) (Session, error) {
	var out Session
	err := c.do(ctx, http.MethodPost, "/api/auth/google/callback", nil, &out)
	if err == nil {
		c.setToken(out.Token)
	}
	return out, err
}

func (c *HTTPClient) Logout(ctx context.Context) error {
	err := c.do(ctx, http.MethodPost, "/api/auth/logout", nil, nil)
	if err == nil {
		c.setToken("")
	}
	return err
}

func (c *HTTPClient) ListAgents(ctx context.Context) ([]Agent, error) {
	var out []Agent
	err := c.do(ctx, http.MethodGet, "/api/agents", nil, &out)
	return out, err
}

func (c *HTTPClient) GetAgent(ctx context.Context, agentID string) (Agent, error) {
	var out Agent
	err := c.do(ctx, http.MethodGet, "/api/agents/"+url.PathEscape(agentID), nil, &out)
	return out, err
}

func (c *HTTPClient) GetBalance(ctx context.Context) (Balance, error) {
	var out Balance
	err := c.do(ctx, http.MethodGet, "/api/billing/balance", nil, &out)
	return out, err
}

func (c *HTTPClient) ListLedger(ctx context.Context) ([]LedgerEntry, error) {
	var out []LedgerEntry
	err := c.do(ctx, http.MethodGet, "/api/billing/ledger", nil, &out)
	return out, err
}

func (c *HTTPClient) StartPurchase(ctx context.Context, input StartPurchaseRequest) (Purchase, error) {
	var out Purchase
	err := c.do(ctx, http.MethodPost, "/api/billing/purchases", input, &out)
	return out, err
}

func (c *HTTPClient) CreateOrder(ctx context.Context, input CreateOrderRequest) (Order, error) {
	var out Order
	err := c.do(ctx, http.MethodPost, "/api/orders", input, &out)
	return out, err
}

func (c *HTTPClient) ListOrders(ctx context.Context) ([]Order, error) {
	var out []Order
	err := c.do(ctx, http.MethodGet, "/api/orders", nil, &out)
	return out, err
}

func (c *HTTPClient) GetOrder(ctx context.Context, orderID string) (Order, error) {
	var out Order
	err := c.do(ctx, http.MethodGet, "/api/orders/"+url.PathEscape(orderID), nil, &out)
	return out, err
}

func (c *HTTPClient) CancelOrder(ctx context.Context, orderID string) (Order, error) {
	var out Order
	err := c.do(ctx, http.MethodPost, "/api/orders/"+url.PathEscape(orderID)+"/cancel", nil, &out)
	return out, err
}

func (c *HTTPClient) ListArtifacts(ctx context.Context, orderID string) ([]Artifact, error) {
	var out []Artifact
	err := c.do(ctx, http.MethodGet, "/api/orders/"+url.PathEscape(orderID)+"/artifacts", nil, &out)
	return out, err
}

func (c *HTTPClient) DownloadArtifact(ctx context.Context, artifactID string) (ArtifactDownload, error) {
	endpoint := c.baseURL.ResolveReference(&url.URL{Path: "/api/artifacts/" + url.PathEscape(artifactID) + "/download"})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return ArtifactDownload{}, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ArtifactDownload{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err == nil && apiErr.Error != "" {
			return ArtifactDownload{}, errors.New(apiErr.Error)
		}
		return ArtifactDownload{}, fmt.Errorf("oneshot api request failed: %s", resp.Status)
	}
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return ArtifactDownload{}, err
	}
	fileName := downloadFileName(resp.Header.Get("Content-Disposition"), artifactID+".pdf")
	path, err := SaveDownload(fileName, content)
	if err != nil {
		return ArtifactDownload{}, err
	}
	return ArtifactDownload{
		ArtifactID:  artifactID,
		FileName:    fileName,
		FilePath:    path,
		ContentType: resp.Header.Get("Content-Type"),
	}, nil
}

func (c *HTTPClient) ShareArtifact(ctx context.Context, artifactID string) (ArtifactShare, error) {
	var out ArtifactShare
	err := c.do(ctx, http.MethodPost, "/api/artifacts/"+url.PathEscape(artifactID)+"/share", nil, &out)
	return out, err
}

func (c *HTTPClient) do(ctx context.Context, method string, path string, input any, output any) error {
	endpoint := c.baseURL.ResolveReference(&url.URL{Path: path})

	var body *bytes.Reader
	if input == nil {
		body = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err == nil && apiErr.Error != "" {
			return errors.New(apiErr.Error)
		}
		return fmt.Errorf("oneshot api request failed: %s", resp.Status)
	}
	if output == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(output)
}

func SaveDownload(fileName string, content []byte) (string, error) {
	if strings.TrimSpace(fileName) == "" {
		fileName = "oneshot-report.pdf"
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "Downloads", filepath.Base(fileName))
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func (c *HTTPClient) setToken(token string) {
	c.token = token
	_ = saveSessionToken(token)
}

func sessionTokenPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "oneshot", "session.token"), nil
}

func loadSessionToken() (string, error) {
	path, err := sessionTokenPath()
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}

func saveSessionToken(token string) error {
	path, err := sessionTokenPath()
	if err != nil {
		return err
	}
	if token == "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(token), 0o600)
}

func downloadFileName(disposition string, fallback string) string {
	for _, part := range strings.Split(disposition, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), "filename=") {
			return strings.Trim(strings.TrimPrefix(part, "filename="), `"`)
		}
	}
	return fallback
}
