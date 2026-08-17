package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domainorders "github.com/openmodu/onecatch/internal/domain/orders"
	"github.com/openmodu/onecatch/internal/domain/users"
	repoagents "github.com/openmodu/onecatch/internal/repo/agents"
	repoartifacts "github.com/openmodu/onecatch/internal/repo/artifacts"
	repobilling "github.com/openmodu/onecatch/internal/repo/billing"
	repoconversations "github.com/openmodu/onecatch/internal/repo/conversations"
	repoorders "github.com/openmodu/onecatch/internal/repo/orders"
	repousers "github.com/openmodu/onecatch/internal/repo/users"
	serverservice "github.com/openmodu/onecatch/internal/service/server"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	usecaseagents "github.com/openmodu/onecatch/internal/usecase/agents"
	usecaseartifacts "github.com/openmodu/onecatch/internal/usecase/artifacts"
	usecaseauth "github.com/openmodu/onecatch/internal/usecase/auth"
	usecasebilling "github.com/openmodu/onecatch/internal/usecase/billing"
	usecaseconversations "github.com/openmodu/onecatch/internal/usecase/conversations"
	usecaseexecution "github.com/openmodu/onecatch/internal/usecase/execution"
	usecaseorders "github.com/openmodu/onecatch/internal/usecase/orders"
)

func TestAuthRoutesSessionLifecycle(t *testing.T) {
	handler := newTestHandler()

	assertStatus(t, handler, http.MethodGet, "/api/me", nil, "", http.StatusUnauthorized)

	loginResp := httptest.NewRecorder()
	handler.ServeHTTP(loginResp, httptest.NewRequest(http.MethodPost, "/api/auth/google/callback", nil))
	if loginResp.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d; body: %s", loginResp.Code, http.StatusOK, loginResp.Body.String())
	}

	var session struct {
		Token    string `json:"token"`
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
	if session.User.ID == "" || session.Token == "" {
		t.Fatalf("missing user id or token: %+v", session)
	}

	assertStatus(t, handler, http.MethodGet, "/api/me", nil, session.Token, http.StatusOK)
	assertStatus(t, handler, http.MethodPost, "/api/auth/logout", nil, session.Token, http.StatusOK)
	assertStatus(t, handler, http.MethodGet, "/api/me", nil, session.Token, http.StatusUnauthorized)
}

func TestAuthRequiresBearerToken(t *testing.T) {
	handler := newTestHandler()
	loginForTest(t, handler)

	// A logged-in session must not leak to requests without a token.
	assertStatus(t, handler, http.MethodGet, "/api/me", nil, "", http.StatusUnauthorized)
	assertStatus(t, handler, http.MethodGet, "/api/billing/balance", nil, "", http.StatusUnauthorized)
	assertStatus(t, handler, http.MethodPost, "/api/auth/logout", nil, "", http.StatusUnauthorized)
}

func TestWechatAuthRoutes(t *testing.T) {
	handler := newTestHandler()

	startResp := httptest.NewRecorder()
	handler.ServeHTTP(startResp, httptest.NewRequest(http.MethodPost, "/api/auth/wechat/start", nil))
	if startResp.Code != http.StatusOK {
		t.Fatalf("wechat start status = %d; body: %s", startResp.Code, startResp.Body.String())
	}
	var start struct {
		Provider string `json:"provider"`
		State    string `json:"state"`
		AuthURL  string `json:"authUrl"`
	}
	if err := json.NewDecoder(startResp.Body).Decode(&start); err != nil {
		t.Fatalf("decode wechat start: %v", err)
	}
	if start.Provider != "wechat" || start.State == "" || start.AuthURL == "" {
		t.Fatalf("unexpected wechat start: %+v", start)
	}

	callbackBody := bytes.NewBufferString(`{"state":"` + start.State + `","providerSubject":"wechat-openid-http","displayName":"Wechat User"}`)
	callbackResp := httptest.NewRecorder()
	callbackReq := httptest.NewRequest(http.MethodPost, "/api/auth/wechat/callback", callbackBody)
	callbackReq.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(callbackResp, callbackReq)
	if callbackResp.Code != http.StatusOK {
		t.Fatalf("wechat callback status = %d; body: %s", callbackResp.Code, callbackResp.Body.String())
	}
	var session struct {
		Token    string `json:"token"`
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(callbackResp.Body).Decode(&session); err != nil {
		t.Fatalf("decode wechat callback: %v", err)
	}
	if session.Provider != "wechat" || session.Token == "" {
		t.Fatalf("unexpected wechat session: %+v", session)
	}

	// Replaying an identity-bearing callback without a fresh state must fail.
	replay := bytes.NewBufferString(`{"providerSubject":"wechat-openid-http"}`)
	assertStatus(t, handler, http.MethodPost, "/api/auth/wechat/callback", replay, "", http.StatusUnauthorized)

	assertStatus(t, handler, http.MethodPost, "/api/auth/logout", nil, session.Token, http.StatusOK)
}

func TestCreateOrderRequiresLogin(t *testing.T) {
	handler := newTestFixture().handler
	body := bytes.NewBufferString(`{"agentId":"industry-research","requirement":{"prompt":"需要一份行业研究"}}`)

	assertStatus(t, handler, http.MethodPost, "/api/orders", body, "", http.StatusUnauthorized)
}

func TestBillingPurchaseAndOrderDebit(t *testing.T) {
	fixture := newTestFixture()
	handler := fixture.handler
	token := loginForTest(t, handler)

	purchaseBody := bytes.NewBufferString(`{"planId":"uses_10","paymentId":"pay-idempotent-1"}`)
	assertStatus(t, handler, http.MethodPost, "/api/billing/purchases", purchaseBody, token, http.StatusOK)
	purchaseBody = bytes.NewBufferString(`{"planId":"uses_10","paymentId":"pay-idempotent-1"}`)
	assertStatus(t, handler, http.MethodPost, "/api/billing/purchases", purchaseBody, token, http.StatusOK)

	var balance struct {
		Remaining int `json:"remaining"`
	}
	getJSON(t, handler, http.MethodGet, "/api/billing/balance", nil, token, http.StatusOK, &balance)
	if balance.Remaining != 20 {
		t.Fatalf("balance after idempotent purchase = %d, want 20", balance.Remaining)
	}

	orderBody := bytes.NewBufferString(`{"agentId":"research-analyst","requirement":{"prompt":"需要一份行业研究"}}`)
	orderResp := httptest.NewRecorder()
	orderReq := httptest.NewRequest(http.MethodPost, "/api/orders", orderBody)
	orderReq.Header.Set("Content-Type", "application/json")
	orderReq.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(orderResp, orderReq)
	if orderResp.Code != http.StatusCreated {
		t.Fatalf("create order status = %d, want %d; body: %s", orderResp.Code, http.StatusCreated, orderResp.Body.String())
	}
	var order struct {
		ID          string `json:"id"`
		AgentName   string `json:"agentName"`
		Status      string `json:"status"`
		UsageCost   int    `json:"usageCost"`
		AmountCents int    `json:"amountCents"`
	}
	if err := json.NewDecoder(orderResp.Body).Decode(&order); err != nil {
		t.Fatalf("decode order: %v", err)
	}
	if order.ID == "" || order.AgentName == "" || order.Status != "running" || order.UsageCost != 1 || order.AmountCents == 0 {
		t.Fatalf("unexpected order fields: %+v", order)
	}

	var afterDebit struct {
		Remaining int `json:"remaining"`
	}
	getJSON(t, handler, http.MethodGet, "/api/billing/balance", nil, token, http.StatusOK, &afterDebit)
	if afterDebit.Remaining != 19 {
		t.Fatalf("balance after order debit = %d, want 19", afterDebit.Remaining)
	}

	var ledger []struct {
		Type         string `json:"type"`
		PaymentID    string `json:"paymentId"`
		UserID       string `json:"userId"`
		OrderID      string `json:"orderId"`
		BalanceAfter int    `json:"balanceAfter"`
	}
	getJSON(t, handler, http.MethodGet, "/api/billing/ledger", nil, token, http.StatusOK, &ledger)
	if len(ledger) != 2 {
		t.Fatalf("ledger len = %d, want 2: %+v", len(ledger), ledger)
	}
	if ledger[0].Type != "purchase" || ledger[0].BalanceAfter != 20 {
		t.Fatalf("purchase ledger mismatch: %+v", ledger[0])
	}
	if ledger[1].Type != "debit" || ledger[1].OrderID != order.ID || ledger[1].BalanceAfter != 19 {
		t.Fatalf("debit ledger mismatch: %+v", ledger[1])
	}
	// Privacy: ledger entries must not leak the payment reference or owner id.
	for _, entry := range ledger {
		if entry.PaymentID != "" || entry.UserID != "" {
			t.Fatalf("ledger entry leaked private field: %+v", entry)
		}
	}

	// Cancelling the running order refunds the debited use.
	assertStatus(t, handler, http.MethodPost, "/api/orders/"+order.ID+"/cancel", nil, token, http.StatusOK)
	var afterCancel struct {
		Remaining int `json:"remaining"`
	}
	getJSON(t, handler, http.MethodGet, "/api/billing/balance", nil, token, http.StatusOK, &afterCancel)
	if afterCancel.Remaining != 20 {
		t.Fatalf("balance after cancel refund = %d, want 20", afterCancel.Remaining)
	}
}

func TestOrderValidationAndArtifacts(t *testing.T) {
	fixture := newTestFixture()
	handler := fixture.handler
	token := loginForTest(t, handler)

	emptyRequirement := bytes.NewBufferString(`{"agentId":"research-analyst","requirement":{"prompt":"   "}}`)
	assertStatus(t, handler, http.MethodPost, "/api/orders", emptyRequirement, token, http.StatusBadRequest)

	running := domainorders.Order{
		ID:          "order_running",
		UserID:      users.DevUserID,
		AgentID:     "research-analyst",
		AgentName:   "行业研究分析师",
		Requirement: domainorders.Requirement{Prompt: "运行中订单"},
		Status:      domainorders.StatusRunning,
		UsageCost:   1,
		AmountCents: 1990,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := fixture.orders.SaveOrder(context.Background(), running); err != nil {
		t.Fatalf("save running order: %v", err)
	}
	assertStatus(t, handler, http.MethodGet, "/api/orders/order_running/artifacts", nil, token, http.StatusConflict)

	delivered := running
	delivered.ID = "order_delivered"
	delivered.Requirement = domainorders.Requirement{Prompt: "已交付订单"}
	delivered.Status = domainorders.StatusDelivered
	if err := fixture.orders.SaveOrder(context.Background(), delivered); err != nil {
		t.Fatalf("save delivered order: %v", err)
	}
	// Simulate what the worker does: record the files the agent produced in its
	// workspace as the order's deliverables.
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "report.md"), []byte("# 报告\norder_delivered 的研究结论"), 0o644); err != nil {
		t.Fatalf("seed workspace file: %v", err)
	}
	if _, err := fixture.artifacts.RecordWorkspaceOutput(context.Background(), delivered, workspace, "已完成研究"); err != nil {
		t.Fatalf("record workspace output: %v", err)
	}

	var artifacts []struct {
		ID       string `json:"id"`
		FileName string `json:"fileName"`
		FileType string `json:"fileType"`
	}
	getJSON(t, handler, http.MethodGet, "/api/orders/order_delivered/artifacts", nil, token, http.StatusOK, &artifacts)
	// Expect the agent's report.md plus the always-written SUMMARY.md.
	var reportID string
	names := map[string]string{}
	for _, a := range artifacts {
		names[a.FileName] = a.ID
		if a.FileName == "report.md" {
			reportID = a.ID
			if a.FileType != "Markdown" {
				t.Fatalf("report.md file type = %q, want Markdown", a.FileType)
			}
		}
	}
	if _, ok := names["SUMMARY.md"]; !ok || reportID == "" {
		t.Fatalf("unexpected artifacts: %+v", artifacts)
	}

	downloadResp := httptest.NewRecorder()
	downloadReq := httptest.NewRequest(http.MethodGet, "/api/artifacts/"+reportID+"/download", nil)
	downloadReq.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(downloadResp, downloadReq)
	if downloadResp.Code != http.StatusOK {
		t.Fatalf("download status = %d, want %d; body: %s", downloadResp.Code, http.StatusOK, downloadResp.Body.String())
	}
	if got := downloadResp.Header().Get("Content-Type"); !strings.Contains(got, "text/markdown") {
		t.Fatalf("download content type = %q, want text/markdown", got)
	}
	if !strings.Contains(downloadResp.Body.String(), "order_delivered 的研究结论") {
		t.Fatalf("download body missing real file content: %q", downloadResp.Body.String())
	}

	var share struct {
		Token string `json:"token"`
		URL   string `json:"url"`
	}
	getJSON(t, handler, http.MethodPost, "/api/artifacts/"+reportID+"/share", nil, token, http.StatusOK, &share)
	if share.URL == "" {
		t.Fatalf("unexpected share response: %+v", share)
	}
	// Privacy: the raw share token is embedded in the URL, not exposed separately.
	if share.Token != "" {
		t.Fatalf("share response leaked raw token: %+v", share)
	}
}

func TestOrderRunEndpoint(t *testing.T) {
	fixture := newTestFixture()
	handler := fixture.handler
	token := loginForTest(t, handler)

	// Unauthenticated access is rejected.
	assertStatus(t, handler, http.MethodGet, "/api/orders/whatever/run", nil, "", http.StatusUnauthorized)

	// An order the caller does not own yields 404, never another user's run.
	assertStatus(t, handler, http.MethodGet, "/api/orders/order_missing/run", nil, token, http.StatusNotFound)

	// A real order with no run yet reports a stable empty shape.
	running := domainorders.Order{
		ID:        "order_run_view",
		UserID:    users.DevUserID,
		AgentID:   "research-analyst",
		AgentName: "行业研究分析师",
		Status:    domainorders.StatusRunning,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := fixture.orders.SaveOrder(context.Background(), running); err != nil {
		t.Fatalf("save order: %v", err)
	}
	var run struct {
		OrderID string `json:"orderId"`
		Status  string `json:"status"`
		Events  []struct {
			Kind string `json:"kind"`
		} `json:"events"`
	}
	getJSON(t, handler, http.MethodGet, "/api/orders/order_run_view/run", nil, token, http.StatusOK, &run)
	if run.OrderID != "order_run_view" || run.Status != "pending" {
		t.Fatalf("unexpected run view: %+v", run)
	}
	if run.Events == nil {
		t.Fatal("events should be a non-nil array")
	}
}

func TestAgentCatalogRoutes(t *testing.T) {
	handler := newTestFixture().handler

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
	if len(agents) != 6 {
		t.Fatalf("agent count = %d, want 6", len(agents))
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

	assertStatus(t, handler, http.MethodGet, "/api/agents/missing-agent", nil, "", http.StatusNotFound)
}

type testFixture struct {
	handler   http.Handler
	orders    repoorders.OrdersRepo
	artifacts *usecaseartifacts.Usecase
}

func newTestHandler() http.Handler {
	return newTestFixture().handler
}

func newTestFixture() testFixture {
	agentRepo := repoagents.NewAgentsRepo(nil)
	artifactRepo := repoartifacts.NewArtifactsRepo(nil)
	billingRepo := repobilling.NewBillingRepo(nil)
	conversationRepo := repoconversations.NewConversationsRepo(nil)
	orderRepo := repoorders.NewOrdersRepo(nil)
	userRepo := repousers.NewUsersRepo(nil)
	agentsUsecase := usecaseagents.NewUsecase(agentRepo)
	billingUsecase := usecasebilling.NewUsecase(billingRepo)
	artifactsUsecase := usecaseartifacts.NewUsecase(artifactRepo, orderRepo)
	executionUsecase := usecaseexecution.NewUsecase(orderRepo, agentRepo, artifactsUsecase, agentrun.NewEngineWithRunners(), usecaseexecution.Config{})
	ordersUsecase := usecaseorders.NewUsecase(agentRepo, orderRepo, billingUsecase, executionUsecase)
	conversationsUsecase := usecaseconversations.NewUsecase(conversationRepo, agentsUsecase, ordersUsecase)

	return testFixture{
		handler: NewServer(serverservice.NewServices(
			usecaseauth.NewUsecaseWithOptions(userRepo, usecaseauth.Options{AllowInsecureCallbacks: true}),
			agentsUsecase,
			artifactsUsecase,
			billingUsecase,
			conversationsUsecase,
			executionUsecase,
			ordersUsecase,
		)),
		orders:    orderRepo,
		artifacts: artifactsUsecase,
	}
}

func loginForTest(t *testing.T, handler http.Handler) string {
	t.Helper()

	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/auth/google/callback", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("login status = %d; body: %s", resp.Code, resp.Body.String())
	}
	var session struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if session.Token == "" {
		t.Fatal("login response missing token")
	}
	return session.Token
}

func getJSON(t *testing.T, handler http.Handler, method string, path string, body *bytes.Buffer, token string, want int, out any) {
	t.Helper()

	resp := doRequest(handler, method, path, body, token)
	if resp.Code != want {
		t.Fatalf("%s %s status = %d, want %d; body: %s", method, path, resp.Code, want, resp.Body.String())
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode %s %s response: %v", method, path, err)
	}
}

func assertStatus(t *testing.T, handler http.Handler, method string, path string, body *bytes.Buffer, token string, want int) {
	t.Helper()

	resp := doRequest(handler, method, path, body, token)
	if resp.Code != want {
		t.Fatalf("%s %s status = %d, want %d; body: %s", method, path, resp.Code, want, resp.Body.String())
	}
}

func doRequest(handler http.Handler, method string, path string, body *bytes.Buffer, token string) *httptest.ResponseRecorder {
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
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
}
