package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/openmodu/oneshot/internal/domain/users"
)

// HTTPVerifier exchanges OAuth authorization codes with the real providers
// (Google / WeChat) and returns the verified identity.
type HTTPVerifier struct {
	client *http.Client

	googleClientID     string
	googleClientSecret string
	googleRedirectURI  string

	wechatAppID  string
	wechatSecret string
}

// NewHTTPVerifierFromEnv builds a verifier from provider credentials in the
// environment. Returns nil when no provider is configured, in which case
// identity-bearing callbacks are rejected unless insecure mode is enabled.
func NewHTTPVerifierFromEnv() IdentityVerifier {
	verifier := &HTTPVerifier{
		client:             &http.Client{Timeout: 10 * time.Second},
		googleClientID:     os.Getenv("ONESHOT_GOOGLE_CLIENT_ID"),
		googleClientSecret: os.Getenv("ONESHOT_GOOGLE_CLIENT_SECRET"),
		googleRedirectURI:  os.Getenv("ONESHOT_GOOGLE_REDIRECT_URI"),
		wechatAppID:        os.Getenv("ONESHOT_WECHAT_APP_ID"),
		wechatSecret:       os.Getenv("ONESHOT_WECHAT_APP_SECRET"),
	}
	if verifier.googleClientSecret == "" && verifier.wechatSecret == "" {
		return nil
	}
	return verifier
}

func (v *HTTPVerifier) Verify(ctx context.Context, provider string, code string) (users.AuthIdentity, error) {
	switch provider {
	case "google":
		return v.verifyGoogle(ctx, code)
	case "wechat":
		return v.verifyWechat(ctx, code)
	default:
		return users.AuthIdentity{}, fmt.Errorf("unsupported oauth provider %q", provider)
	}
}

func (v *HTTPVerifier) verifyGoogle(ctx context.Context, code string) (users.AuthIdentity, error) {
	if v.googleClientSecret == "" {
		return users.AuthIdentity{}, fmt.Errorf("google oauth is not configured")
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", v.googleClientID)
	form.Set("client_secret", v.googleClientSecret)
	form.Set("redirect_uri", v.googleRedirectURI)

	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := v.postForm(ctx, "https://oauth2.googleapis.com/token", form, &token); err != nil {
		return users.AuthIdentity{}, fmt.Errorf("exchange google code: %w", err)
	}
	if token.AccessToken == "" {
		return users.AuthIdentity{}, fmt.Errorf("google token response missing access_token")
	}

	var info struct {
		Sub     string `json:"sub"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := v.getJSON(ctx, "https://openidconnect.googleapis.com/v1/userinfo", token.AccessToken, &info); err != nil {
		return users.AuthIdentity{}, fmt.Errorf("fetch google userinfo: %w", err)
	}
	if info.Sub == "" {
		return users.AuthIdentity{}, fmt.Errorf("google userinfo missing subject")
	}
	return users.AuthIdentity{
		Provider:        "google",
		ProviderSubject: info.Sub,
		DisplayName:     defaultString(info.Name, "Google 用户"),
		Email:           info.Email,
		AvatarURL:       info.Picture,
	}, nil
}

func (v *HTTPVerifier) verifyWechat(ctx context.Context, code string) (users.AuthIdentity, error) {
	if v.wechatSecret == "" {
		return users.AuthIdentity{}, fmt.Errorf("wechat oauth is not configured")
	}

	tokenURL := fmt.Sprintf(
		"https://api.weixin.qq.com/sns/oauth2/access_token?appid=%s&secret=%s&code=%s&grant_type=authorization_code",
		url.QueryEscape(v.wechatAppID), url.QueryEscape(v.wechatSecret), url.QueryEscape(code),
	)
	var token struct {
		AccessToken string `json:"access_token"`
		OpenID      string `json:"openid"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := v.getJSON(ctx, tokenURL, "", &token); err != nil {
		return users.AuthIdentity{}, fmt.Errorf("exchange wechat code: %w", err)
	}
	if token.ErrCode != 0 || token.OpenID == "" {
		return users.AuthIdentity{}, fmt.Errorf("wechat token exchange failed: %d %s", token.ErrCode, token.ErrMsg)
	}

	infoURL := fmt.Sprintf(
		"https://api.weixin.qq.com/sns/userinfo?access_token=%s&openid=%s",
		url.QueryEscape(token.AccessToken), url.QueryEscape(token.OpenID),
	)
	var info struct {
		Nickname   string `json:"nickname"`
		HeadImgURL string `json:"headimgurl"`
		ErrCode    int    `json:"errcode"`
	}
	if err := v.getJSON(ctx, infoURL, "", &info); err != nil || info.ErrCode != 0 {
		// Userinfo is best-effort: openid alone is a verified identity.
		info.Nickname = ""
		info.HeadImgURL = ""
	}
	return users.AuthIdentity{
		Provider:        "wechat",
		ProviderSubject: token.OpenID,
		DisplayName:     defaultString(info.Nickname, "微信用户"),
		AvatarURL:       info.HeadImgURL,
	}, nil
}

func (v *HTTPVerifier) postForm(ctx context.Context, endpoint string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return v.do(req, out)
}

func (v *HTTPVerifier) getJSON(ctx context.Context, endpoint string, bearer string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	return v.do(req, out)
}

func (v *HTTPVerifier) do(req *http.Request, out any) error {
	resp, err := v.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("provider returned status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
