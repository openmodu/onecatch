package httptransport

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	repoagents "github.com/openmodu/oneshot/internal/repo/agents"
	repobilling "github.com/openmodu/oneshot/internal/repo/billing"
	repoorders "github.com/openmodu/oneshot/internal/repo/orders"
	repousers "github.com/openmodu/oneshot/internal/repo/users"
	"github.com/openmodu/oneshot/internal/service"
	usecaseagents "github.com/openmodu/oneshot/internal/usecase/agents"
	usecaseauth "github.com/openmodu/oneshot/internal/usecase/auth"
	usecasebilling "github.com/openmodu/oneshot/internal/usecase/billing"
	usecaseorders "github.com/openmodu/oneshot/internal/usecase/orders"
)

func TestAuthRoutesSessionLifecycle(t *testing.T) {
	handler := newTestHandler()

	assertStatus(t, handler, http.MethodGet, "/api/me", nil, http.StatusUnauthorized)

	loginResp := httptest.NewRecorder()
	handler.ServeHTTP(loginResp, httptest.NewRequest(http.MethodPost, "/api/auth/google/callback", nil))
	if loginResp.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d; body: %s", loginResp.Code, http.StatusOK, loginResp.Body.String())
	}

	var session struct {
		Provider string `json:"provider"`
		User     struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.NewDecoder(loginResp.Body).Decode(&session); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if session.Provider != "google" {
		t.Fatalf("provider = %q, want google", session.Provider)
	}
	if session.User.ID == "" {
		t.Fatal("user id is empty")
	}

	assertStatus(t, handler, http.MethodGet, "/api/me", nil, http.StatusOK)
	assertStatus(t, handler, http.MethodPost, "/api/auth/logout", nil, http.StatusOK)
	assertStatus(t, handler, http.MethodGet, "/api/me", nil, http.StatusUnauthorized)
}

func TestWechatAuthRoutes(t *testing.T) {
	handler := newTestHandler()

	for _, path := range []string{"/api/auth/wechat/start", "/api/auth/wechat/callback"} {
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, path, nil))
		if resp.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d; body: %s", path, resp.Code, http.StatusOK, resp.Body.String())
		}

		var session struct {
			Provider string `json:"provider"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
			t.Fatalf("decode %s response: %v", path, err)
		}
		if session.Provider != "wechat" {
			t.Fatalf("%s provider = %q, want wechat", path, session.Provider)
		}

		assertStatus(t, handler, http.MethodPost, "/api/auth/logout", nil, http.StatusOK)
	}
}

func TestCreateOrderRequiresLogin(t *testing.T) {
	handler := newTestHandler()
	body := bytes.NewBufferString(`{"agentId":"industry-research","requirement":{"prompt":"需要一份行业研究"}}`)

	assertStatus(t, handler, http.MethodPost, "/api/orders", body, http.StatusUnauthorized)
}

func newTestHandler() http.Handler {
	agentRepo := repoagents.NewAgentsRepo(nil)
	billingRepo := repobilling.NewBillingRepo(nil)
	orderRepo := repoorders.NewOrdersRepo(nil)
	userRepo := repousers.NewUsersRepo(nil)
	billingUsecase := usecasebilling.NewUsecase(billingRepo)

	return NewServer(service.NewServices(
		usecaseauth.NewUsecase(userRepo),
		usecaseagents.NewUsecase(agentRepo),
		billingUsecase,
		usecaseorders.NewUsecase(agentRepo, orderRepo, billingUsecase),
	))
}

func assertStatus(t *testing.T, handler http.Handler, method string, path string, body *bytes.Buffer, want int) {
	t.Helper()

	var payload *bytes.Buffer
	if body == nil {
		payload = bytes.NewBuffer(nil)
	} else {
		payload = body
	}

	req := httptest.NewRequest(method, path, payload)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != want {
		t.Fatalf("%s %s status = %d, want %d; body: %s", method, path, resp.Code, want, resp.Body.String())
	}
}
