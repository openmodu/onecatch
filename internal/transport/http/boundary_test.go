package httptransport

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	routing "github.com/openmodu/onecatch/internal/transport/router"
)

// loginWechatForTest logs in a second, distinct user via the wechat callback
// (state + providerSubject), returning that session's token.
func loginWechatForTest(t *testing.T, handler http.Handler, subject string) string {
	t.Helper()

	startResp := doRequest(handler, http.MethodPost, "/api/auth/wechat/start", nil, "")
	if startResp.Code != http.StatusOK {
		t.Fatalf("wechat start status = %d; body: %s", startResp.Code, startResp.Body.String())
	}
	var start struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(startResp.Body).Decode(&start); err != nil {
		t.Fatalf("decode wechat start: %v", err)
	}

	body := bytes.NewBufferString(`{"state":"` + start.State + `","providerSubject":"` + subject + `"}`)
	resp := doRequest(handler, http.MethodPost, "/api/auth/wechat/callback", body, "")
	if resp.Code != http.StatusOK {
		t.Fatalf("wechat callback status = %d; body: %s", resp.Code, resp.Body.String())
	}
	var session struct {
		Token string `json:"token"`
		User  struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("decode wechat session: %v", err)
	}
	if session.Token == "" || session.User.ID == "" {
		t.Fatalf("unexpected wechat session: %+v", session)
	}
	return session.Token
}

func TestCrossUserAccessReturns404(t *testing.T) {
	fixture := newTestFixture()
	handler := fixture.handler

	// User A (dev/google) creates an order.
	tokenA := loginForTest(t, handler)
	orderBody := bytes.NewBufferString(`{"agentId":"research-analyst","requirement":{"prompt":"机密需求"}}`)
	orderResp := doRequest(handler, http.MethodPost, "/api/orders", orderBody, tokenA)
	if orderResp.Code != http.StatusCreated {
		t.Fatalf("create order status = %d; body: %s", orderResp.Code, orderResp.Body.String())
	}
	var order struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(orderResp.Body).Decode(&order); err != nil {
		t.Fatalf("decode order: %v", err)
	}

	// User B must not be able to read A's order or its artifacts, and the
	// rejection must not reveal that the resource exists → 404, never 403/200.
	tokenB := loginWechatForTest(t, handler, "cross-user-b")
	assertStatus(t, handler, http.MethodGet, "/api/orders/"+order.ID, nil, tokenB, http.StatusNotFound)
	assertStatus(t, handler, http.MethodGet, "/api/orders/"+order.ID+"/artifacts", nil, tokenB, http.StatusNotFound)
	assertStatus(t, handler, http.MethodPost, "/api/orders/"+order.ID+"/cancel", nil, tokenB, http.StatusNotFound)

	// User A still sees their own order.
	assertStatus(t, handler, http.MethodGet, "/api/orders/"+order.ID, nil, tokenA, http.StatusOK)
}

func TestUserFacingResponsesOmitPrivateFields(t *testing.T) {
	fixture := newTestFixture()
	handler := fixture.handler
	token := loginForTest(t, handler)

	orderBody := bytes.NewBufferString(`{"agentId":"research-analyst","requirement":{"prompt":"需求内容"}}`)
	if resp := doRequest(handler, http.MethodPost, "/api/orders", orderBody, token); resp.Code != http.StatusCreated {
		t.Fatalf("create order status = %d; body: %s", resp.Code, resp.Body.String())
	}

	// Black-box: raw JSON of every user-facing endpoint must not carry these keys.
	private := []string{"userId", "providerSubject", "paymentId", "storageUri"}
	for _, path := range []string{
		"/api/me",
		"/api/billing/balance",
		"/api/billing/ledger",
		"/api/orders",
	} {
		body := doRequest(handler, http.MethodGet, path, nil, token).Body.String()
		for _, key := range private {
			if strings.Contains(body, key) {
				t.Fatalf("%s leaked private key %q: %s", path, key, body)
			}
		}
	}

	// The order response must still carry the owner's own requirement text.
	ordersBody := doRequest(handler, http.MethodGet, "/api/orders", nil, token).Body.String()
	if !strings.Contains(ordersBody, "需求内容") {
		t.Fatalf("order response dropped the owner's requirement: %s", ordersBody)
	}
}

func TestConversationRoutesAuthAndIsolation(t *testing.T) {
	fixture := newTestFixture()
	handler := fixture.handler

	// Unauthenticated start is rejected.
	assertStatus(t, handler, http.MethodPost, "/api/conversations",
		bytes.NewBufferString(`{"agentId":"research-analyst"}`), "", http.StatusUnauthorized)

	tokenA := loginForTest(t, handler)
	startResp := doRequest(handler, http.MethodPost, "/api/conversations",
		bytes.NewBufferString(`{"agentId":"research-analyst"}`), tokenA)
	if startResp.Code != http.StatusCreated {
		t.Fatalf("start conversation status = %d; body: %s", startResp.Code, startResp.Body.String())
	}
	var conv struct {
		ID       string `json:"id"`
		UserID   string `json:"userId"`
		Status   string `json:"status"`
		Messages []struct {
			Kind string `json:"kind"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(startResp.Body).Decode(&conv); err != nil {
		t.Fatalf("decode conversation: %v", err)
	}
	if conv.ID == "" || conv.Status != "active" || len(conv.Messages) != 1 {
		t.Fatalf("unexpected conversation: %+v", conv)
	}
	// Privacy: conversation response must not leak the owner id.
	if conv.UserID != "" {
		t.Fatalf("conversation leaked userId: %+v", conv)
	}

	// Another user cannot read or post to it → 404 (no existence leak).
	tokenB := loginWechatForTest(t, handler, "conv-user-b")
	assertStatus(t, handler, http.MethodGet, "/api/conversations/"+conv.ID, nil, tokenB, http.StatusNotFound)
	assertStatus(t, handler, http.MethodPost, "/api/conversations/"+conv.ID+"/messages",
		bytes.NewBufferString(`{"text":"偷看"}`), tokenB, http.StatusNotFound)
	assertStatus(t, handler, http.MethodPost, "/api/conversations/"+conv.ID+"/confirm", nil, tokenB, http.StatusNotFound)

	// Confirm with nothing staged → 409.
	assertStatus(t, handler, http.MethodPost, "/api/conversations/"+conv.ID+"/confirm", nil, tokenA, http.StatusConflict)
}

func TestAdminBoundaryIsolation(t *testing.T) {
	handler := newTestFixture().handler

	// No admin token configured in tests → admin subtree fails closed (403),
	// and the route exists (not 404).
	assertStatus(t, handler, http.MethodGet, "/admin/api/whoami", nil, "", http.StatusForbidden)

	// Admin capability must NOT be reachable under the user-facing /api tree.
	assertStatus(t, handler, http.MethodGet, "/api/admin/whoami", nil, "", http.StatusNotFound)
	assertStatus(t, handler, http.MethodGet, "/api/whoami", nil, "", http.StatusNotFound)
}

func TestAdminAuthGate(t *testing.T) {
	router := routing.NewRouter()
	newAdminHandler("admin-secret", slog.Default()).register(router)
	handler := router.Handler()

	// Wrong / missing credential → 403.
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/admin/api/whoami", nil))
	if resp.Code != http.StatusForbidden {
		t.Fatalf("no-token admin status = %d, want 403", resp.Code)
	}

	wrong := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/whoami", nil)
	req.Header.Set(adminTokenHeader, "nope")
	handler.ServeHTTP(wrong, req)
	if wrong.Code != http.StatusForbidden {
		t.Fatalf("wrong-token admin status = %d, want 403", wrong.Code)
	}

	// Correct credential → 200 with admin role.
	ok := httptest.NewRecorder()
	authed := httptest.NewRequest(http.MethodGet, "/admin/api/whoami", nil)
	authed.Header.Set(adminTokenHeader, "admin-secret")
	handler.ServeHTTP(ok, authed)
	if ok.Code != http.StatusOK {
		t.Fatalf("admin status = %d, want 200; body: %s", ok.Code, ok.Body.String())
	}
	if !strings.Contains(ok.Body.String(), `"role":"admin"`) {
		t.Fatalf("admin whoami body = %s", ok.Body.String())
	}
}
