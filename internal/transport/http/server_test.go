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

func TestAgentCatalogRoutes(t *testing.T) {
	handler := newTestHandler()

	listResp := httptest.NewRecorder()
	handler.ServeHTTP(listResp, httptest.NewRequest(http.MethodGet, "/api/agents", nil))
	if listResp.Code != http.StatusOK {
		t.Fatalf("list agents status = %d, want %d; body: %s", listResp.Code, http.StatusOK, listResp.Body.String())
	}

	var agents []struct {
		ID                string   `json:"id"`
		Name              string   `json:"name"`
		Tags              []string `json:"tags"`
		PriceCents        int      `json:"priceCents"`
		Rating            string   `json:"rating"`
		DealCount         int      `json:"dealCount"`
		EstimatedDuration string   `json:"estimatedDuration"`
		Deliverable       string   `json:"deliverable"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&agents); err != nil {
		t.Fatalf("decode agents response: %v", err)
	}
	if len(agents) != 4 {
		t.Fatalf("agent count = %d, want 4", len(agents))
	}
	if agents[0].ID == "" || agents[0].Name == "" || agents[0].PriceCents == 0 || agents[0].Rating == "" || agents[0].DealCount == 0 || agents[0].Deliverable == "" || len(agents[0].Tags) == 0 {
		t.Fatalf("first agent missing catalog fields: %+v", agents[0])
	}

	detailResp := httptest.NewRecorder()
	handler.ServeHTTP(detailResp, httptest.NewRequest(http.MethodGet, "/api/agents/research-analyst", nil))
	if detailResp.Code != http.StatusOK {
		t.Fatalf("get agent status = %d, want %d; body: %s", detailResp.Code, http.StatusOK, detailResp.Body.String())
	}

	var detail struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(detailResp.Body).Decode(&detail); err != nil {
		t.Fatalf("decode agent detail response: %v", err)
	}
	if detail.ID != "research-analyst" {
		t.Fatalf("agent detail id = %q, want research-analyst", detail.ID)
	}

	assertStatus(t, handler, http.MethodGet, "/api/agents/missing-agent", nil, http.StatusNotFound)
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
