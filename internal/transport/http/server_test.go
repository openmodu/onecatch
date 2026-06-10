package httptransport

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domainorders "github.com/openmodu/oneshot/internal/domain/orders"
	"github.com/openmodu/oneshot/internal/domain/users"
	repoagents "github.com/openmodu/oneshot/internal/repo/agents"
	repoartifacts "github.com/openmodu/oneshot/internal/repo/artifacts"
	repobilling "github.com/openmodu/oneshot/internal/repo/billing"
	repoorders "github.com/openmodu/oneshot/internal/repo/orders"
	repousers "github.com/openmodu/oneshot/internal/repo/users"
	"github.com/openmodu/oneshot/internal/service"
	usecaseagents "github.com/openmodu/oneshot/internal/usecase/agents"
	usecaseartifacts "github.com/openmodu/oneshot/internal/usecase/artifacts"
	usecaseauth "github.com/openmodu/oneshot/internal/usecase/auth"
	usecasebilling "github.com/openmodu/oneshot/internal/usecase/billing"
	usecaseexecution "github.com/openmodu/oneshot/internal/usecase/execution"
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
	handler := newTestFixture().handler
	body := bytes.NewBufferString(`{"agentId":"industry-research","requirement":{"prompt":"需要一份行业研究"}}`)

	assertStatus(t, handler, http.MethodPost, "/api/orders", body, http.StatusUnauthorized)
}

func TestBillingPurchaseAndOrderDebit(t *testing.T) {
	fixture := newTestFixture()
	handler := fixture.handler
	loginForTest(t, handler)

	purchaseBody := bytes.NewBufferString(`{"planId":"uses_10","paymentId":"pay-idempotent-1"}`)
	purchaseResp := httptest.NewRecorder()
	handler.ServeHTTP(purchaseResp, httptest.NewRequest(http.MethodPost, "/api/billing/purchases", purchaseBody))
	if purchaseResp.Code != http.StatusOK {
		t.Fatalf("purchase status = %d, want %d; body: %s", purchaseResp.Code, http.StatusOK, purchaseResp.Body.String())
	}
	purchaseBody = bytes.NewBufferString(`{"planId":"uses_10","paymentId":"pay-idempotent-1"}`)
	assertStatus(t, handler, http.MethodPost, "/api/billing/purchases", purchaseBody, http.StatusOK)

	var balance struct {
		Remaining int `json:"remaining"`
	}
	getJSON(t, handler, http.MethodGet, "/api/billing/balance", nil, http.StatusOK, &balance)
	if balance.Remaining != 20 {
		t.Fatalf("balance after idempotent purchase = %d, want 20", balance.Remaining)
	}

	orderBody := bytes.NewBufferString(`{"agentId":"research-analyst","requirement":{"prompt":"需要一份行业研究"}}`)
	orderResp := httptest.NewRecorder()
	handler.ServeHTTP(orderResp, httptest.NewRequest(http.MethodPost, "/api/orders", orderBody))
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
	getJSON(t, handler, http.MethodGet, "/api/billing/balance", nil, http.StatusOK, &afterDebit)
	if afterDebit.Remaining != 19 {
		t.Fatalf("balance after order debit = %d, want 19", afterDebit.Remaining)
	}

	var ledger []struct {
		Type         string `json:"type"`
		PaymentID    string `json:"paymentId"`
		OrderID      string `json:"orderId"`
		BalanceAfter int    `json:"balanceAfter"`
	}
	getJSON(t, handler, http.MethodGet, "/api/billing/ledger", nil, http.StatusOK, &ledger)
	if len(ledger) != 2 {
		t.Fatalf("ledger len = %d, want 2: %+v", len(ledger), ledger)
	}
	if ledger[0].Type != "purchase" || ledger[0].PaymentID != "pay-idempotent-1" || ledger[0].BalanceAfter != 20 {
		t.Fatalf("purchase ledger mismatch: %+v", ledger[0])
	}
	if ledger[1].Type != "debit" || ledger[1].OrderID != order.ID || ledger[1].BalanceAfter != 19 {
		t.Fatalf("debit ledger mismatch: %+v", ledger[1])
	}
}

func TestOrderValidationAndArtifacts(t *testing.T) {
	fixture := newTestFixture()
	handler := fixture.handler
	loginForTest(t, handler)

	emptyRequirement := bytes.NewBufferString(`{"agentId":"research-analyst","requirement":{"prompt":"   "}}`)
	assertStatus(t, handler, http.MethodPost, "/api/orders", emptyRequirement, http.StatusBadRequest)

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
	if err := fixture.orders.SaveOrder(nil, running); err != nil {
		t.Fatalf("save running order: %v", err)
	}
	assertStatus(t, handler, http.MethodGet, "/api/orders/order_running/artifacts", nil, http.StatusConflict)

	delivered := running
	delivered.ID = "order_delivered"
	delivered.Requirement = domainorders.Requirement{Prompt: "已交付订单"}
	delivered.Status = domainorders.StatusDelivered
	if err := fixture.orders.SaveOrder(nil, delivered); err != nil {
		t.Fatalf("save delivered order: %v", err)
	}
	var artifacts []struct {
		ID       string `json:"id"`
		FileName string `json:"fileName"`
		FileType string `json:"fileType"`
	}
	getJSON(t, handler, http.MethodGet, "/api/orders/order_delivered/artifacts", nil, http.StatusOK, &artifacts)
	if len(artifacts) != 1 || artifacts[0].ID == "" || artifacts[0].FileType != "PDF" {
		t.Fatalf("unexpected artifacts: %+v", artifacts)
	}

	downloadResp := httptest.NewRecorder()
	handler.ServeHTTP(downloadResp, httptest.NewRequest(http.MethodGet, "/api/artifacts/"+artifacts[0].ID+"/download", nil))
	if downloadResp.Code != http.StatusOK {
		t.Fatalf("download status = %d, want %d; body: %s", downloadResp.Code, http.StatusOK, downloadResp.Body.String())
	}
	if got := downloadResp.Header().Get("Content-Type"); !strings.Contains(got, "application/pdf") {
		t.Fatalf("download content type = %q, want application/pdf", got)
	}
	if !strings.Contains(downloadResp.Body.String(), "order_delivered") {
		t.Fatalf("download body missing order id: %q", downloadResp.Body.String())
	}

	var share struct {
		Token string `json:"token"`
		URL   string `json:"url"`
	}
	getJSON(t, handler, http.MethodPost, "/api/artifacts/"+artifacts[0].ID+"/share", nil, http.StatusOK, &share)
	if share.Token == "" || share.URL == "" {
		t.Fatalf("unexpected share response: %+v", share)
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

type testFixture struct {
	handler http.Handler
	orders  repoorders.OrdersRepo
}

func newTestHandler() http.Handler {
	return newTestFixture().handler
}

func newTestFixture() testFixture {
	agentRepo := repoagents.NewAgentsRepo(nil)
	artifactRepo := repoartifacts.NewArtifactsRepo(nil)
	billingRepo := repobilling.NewBillingRepo(nil)
	orderRepo := repoorders.NewOrdersRepo(nil)
	userRepo := repousers.NewUsersRepo(nil)
	billingUsecase := usecasebilling.NewUsecase(billingRepo)
	artifactsUsecase := usecaseartifacts.NewUsecase(artifactRepo, orderRepo)
	executionUsecase := usecaseexecution.NewUsecase(orderRepo, artifactsUsecase)

	return testFixture{
		handler: NewServer(service.NewServices(
			usecaseauth.NewUsecase(userRepo),
			usecaseagents.NewUsecase(agentRepo),
			artifactsUsecase,
			billingUsecase,
			executionUsecase,
			usecaseorders.NewUsecase(agentRepo, orderRepo, billingUsecase),
		)),
		orders: orderRepo,
	}
}

func loginForTest(t *testing.T, handler http.Handler) {
	t.Helper()
	assertStatus(t, handler, http.MethodPost, "/api/auth/google/callback", nil, http.StatusOK)
}

func getJSON(t *testing.T, handler http.Handler, method string, path string, body *bytes.Buffer, want int, out any) {
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
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode %s %s response: %v", method, path, err)
	}
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
