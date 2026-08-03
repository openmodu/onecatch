package httptransport

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	domainauth "github.com/openmodu/oneshot/internal/domain/auth"
	"github.com/openmodu/oneshot/internal/domain/orders"
	"github.com/openmodu/oneshot/internal/domain/users"
	serverservice "github.com/openmodu/oneshot/internal/service/server"
	routing "github.com/openmodu/oneshot/internal/transport/router"
	usecaseauth "github.com/openmodu/oneshot/internal/usecase/auth"
	usecasebilling "github.com/openmodu/oneshot/internal/usecase/billing"
	usecaseorders "github.com/openmodu/oneshot/internal/usecase/orders"
	"github.com/openmodu/oneshot/pkg/httpx"
)

type Server struct {
	services *serverservice.Services
	log      *slog.Logger
}

func NewServer(services *serverservice.Services) http.Handler {
	server := &Server{services: services, log: slog.Default()}
	router := routing.NewRouter()

	router.Use(routing.DefaultMiddlewares()...)

	// Admin lives under a separate route tree with its own auth, never under
	// /api and never reachable through the desktop client.
	newAdminHandler(os.Getenv("ONESHOT_ADMIN_TOKEN"), server.log).register(router)

	router.Get("/healthz", server.health)
	router.Group("/api", func(router routing.Router) {
		router.Get("/me", server.currentUser)
		router.Post("/auth/wechat/start", server.startWechat)
		router.Post("/auth/wechat/callback", server.loginWithWechat)
		router.Post("/auth/google/callback", server.loginWithGoogle)
		router.Post("/auth/logout", server.logout)
		router.Get("/agents", server.listAgents)
		router.Get("/agents/{agentID}", server.getAgent)
		router.Get("/billing/balance", server.getBalance)
		router.Get("/billing/ledger", server.listLedger)
		router.Post("/billing/purchases", server.startPurchase)
		router.Post("/conversations", server.startConversation)
		router.Get("/conversations/{conversationID}", server.getConversation)
		router.Post("/conversations/{conversationID}/messages", server.postConversationMessage)
		router.Post("/conversations/{conversationID}/confirm", server.confirmConversation)
		router.Post("/orders", server.createOrder)
		router.Get("/orders", server.listOrders)
		router.Get("/orders/{orderID}/artifacts", server.listArtifacts)
		router.Get("/orders/{orderID}/run", server.getOrderRun)
		router.Get("/orders/{orderID}", server.getOrder)
		router.Post("/orders/{orderID}/cancel", server.cancelOrder)
		router.Post("/orders/{orderID}/continue", server.continueOrder)
		router.Get("/artifacts/{artifactID}/download", server.downloadArtifact)
		router.Post("/artifacts/{artifactID}/share", server.shareArtifact)
	})

	return router.Handler()
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) currentUser(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUserFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toUserDTO(user))
}

func (s *Server) startWechat(w http.ResponseWriter, r *http.Request) {
	session, err := s.services.Auth.StartWechat(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, session)
}

func (s *Server) loginWithWechat(w http.ResponseWriter, r *http.Request) {
	input, err := decodeOAuthCallback(r)
	if err != nil {
		writeError(w, err)
		return
	}
	session, err := s.services.Auth.LoginWithWechatCallback(r.Context(), input)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toSessionDTO(session))
}

func (s *Server) loginWithGoogle(w http.ResponseWriter, r *http.Request) {
	input, err := decodeOAuthCallback(r)
	if err != nil {
		writeError(w, err)
		return
	}
	session, err := s.services.Auth.LoginWithGoogleCallback(r.Context(), input)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toSessionDTO(session))
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" {
		writeError(w, domainauth.ErrUnauthenticated)
		return
	}
	if err := s.services.Auth.LogoutToken(r.Context(), token); err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.services.Agents.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, agents)
}

func (s *Server) getAgent(w http.ResponseWriter, r *http.Request) {
	id := routing.URLParam(r, "agentID")
	agent, err := s.services.Agents.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, agent)
}

func (s *Server) getBalance(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUserFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	balance, err := s.services.Billing.GetBalance(r.Context(), user.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toBalanceDTO(balance))
}

func (s *Server) listLedger(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUserFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	ledger, err := s.services.Billing.ListLedger(r.Context(), user.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toLedgerDTOs(ledger))
}

func (s *Server) startPurchase(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUserFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	var input struct {
		PlanID    string `json:"planId"`
		PaymentID string `json:"paymentId"`
	}
	if err := httpx.DecodeJSON(r, &input); err != nil {
		writeError(w, err)
		return
	}

	purchase, err := s.services.Billing.StartPurchase(r.Context(), usecasebilling.PurchaseInput{
		UserID:    user.ID,
		PlanID:    input.PlanID,
		PaymentID: input.PaymentID,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toPurchaseDTO(purchase))
}

func (s *Server) startConversation(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUserFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	var input struct {
		AgentID   string `json:"agentId"`
		Workspace string `json:"workspace"`
	}
	if err := httpx.DecodeJSON(r, &input); err != nil {
		writeError(w, err)
		return
	}

	conv, err := s.services.Conversations.Start(r.Context(), user.ID, input.AgentID, input.Workspace)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toConversationDTO(conv))
}

func (s *Server) getConversation(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUserFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	conv, err := s.services.Conversations.Get(r.Context(), user.ID, routing.URLParam(r, "conversationID"))
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toConversationDTO(conv))
}

func (s *Server) postConversationMessage(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUserFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	var input struct {
		Text string `json:"text"`
	}
	if err := httpx.DecodeJSON(r, &input); err != nil {
		writeError(w, err)
		return
	}

	conv, err := s.services.Conversations.PostMessage(r.Context(), user.ID, routing.URLParam(r, "conversationID"), input.Text)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toConversationDTO(conv))
}

func (s *Server) confirmConversation(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUserFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	conv, err := s.services.Conversations.Confirm(r.Context(), user.ID, routing.URLParam(r, "conversationID"))
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toConversationDTO(conv))
}

func (s *Server) createOrder(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUserFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	var input struct {
		AgentID     string             `json:"agentId"`
		Requirement orders.Requirement `json:"requirement"`
		Workspace   string             `json:"workspace"`
	}
	if err := httpx.DecodeJSON(r, &input); err != nil {
		writeError(w, err)
		return
	}

	order, err := s.services.Orders.Create(r.Context(), usecaseorders.CreateInput{
		UserID:      user.ID,
		AgentID:     input.AgentID,
		Requirement: input.Requirement,
		Workspace:   input.Workspace,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	// Audit order creation without logging the requirement text in full.
	s.log.Info("order created",
		slog.String("orderId", order.ID),
		slog.String("agentId", order.AgentID),
		slog.String("requirement", promptDigest(order.Requirement.Prompt)),
	)
	httpx.WriteJSON(w, http.StatusCreated, toOrderDTO(order))
}

func (s *Server) listOrders(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUserFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	items, err := s.services.Orders.List(r.Context(), usecaseorders.ListInput{
		UserID: user.ID,
		Status: orders.Status(r.URL.Query().Get("status")),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toOrderDTOs(items))
}

func (s *Server) getOrder(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUserFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	id := routing.URLParam(r, "orderID")
	order, err := s.services.Orders.Get(r.Context(), user.ID, id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toOrderDTO(order))
}

// getOrderRun returns the live agent run log for an order. Ownership is
// enforced by first loading the order through the orders usecase, which scopes
// reads to the caller; only then is the worker's in-memory run log exposed.
func (s *Server) getOrderRun(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUserFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	id := routing.URLParam(r, "orderID")
	if _, err := s.services.Orders.Get(r.Context(), user.ID, id); err != nil {
		writeError(w, err)
		return
	}
	log, found := s.services.Execution.Snapshot(id)
	httpx.WriteJSON(w, http.StatusOK, toRunDTO(id, log, found))
}

// continueOrder resumes a finished task with a follow-up instruction (a
// multi-turn continuation), reusing the prior agent session and workspace.
func (s *Server) continueOrder(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUserFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	var input struct {
		Prompt string `json:"prompt"`
	}
	if err := httpx.DecodeJSON(r, &input); err != nil {
		writeError(w, err)
		return
	}

	order, err := s.services.Orders.Continue(r.Context(), user.ID, routing.URLParam(r, "orderID"), input.Prompt)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toOrderDTO(order))
}

func (s *Server) cancelOrder(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUserFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	id := routing.URLParam(r, "orderID")
	order, err := s.services.Orders.Cancel(r.Context(), user.ID, id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toOrderDTO(order))
}

func (s *Server) listArtifacts(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUserFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	orderID := routing.URLParam(r, "orderID")
	artifacts, err := s.services.Artifacts.ListForOrder(r.Context(), user.ID, orderID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toArtifactDTOs(artifacts))
}

func (s *Server) downloadArtifact(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUserFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	artifactID := routing.URLParam(r, "artifactID")
	download, err := s.services.Artifacts.Download(r.Context(), user.ID, artifactID)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", download.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, download.Artifact.FileName))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(download.Content)
}

func (s *Server) shareArtifact(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUserFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	artifactID := routing.URLParam(r, "artifactID")
	share, err := s.services.Artifacts.Share(r.Context(), user.ID, artifactID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toShareDTO(share))
}

// currentUserFromRequest resolves the user strictly from the Bearer token.
// The server never falls back to the in-process "current session" — that
// shortcut is only meaningful inside the single-user desktop process.
func (s *Server) currentUserFromRequest(r *http.Request) (users.User, error) {
	token := bearerToken(r)
	if token == "" {
		return users.User{}, domainauth.ErrUnauthenticated
	}
	return s.services.Auth.CurrentUserByToken(r.Context(), token)
}

func bearerToken(r *http.Request) string {
	value := r.Header.Get("Authorization")
	if value == "" {
		return ""
	}
	prefix := "Bearer "
	if !strings.HasPrefix(value, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(value, prefix))
}

func decodeOAuthCallback(r *http.Request) (usecaseauth.OAuthCallbackInput, error) {
	if r.Body == nil || r.ContentLength == 0 {
		return usecaseauth.OAuthCallbackInput{}, nil
	}
	var input usecaseauth.OAuthCallbackInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		return usecaseauth.OAuthCallbackInput{}, err
	}
	return input, nil
}
