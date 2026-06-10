package httptransport

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/openmodu/oneshot/internal/api"
	"github.com/openmodu/oneshot/internal/domain/orders"
	"github.com/openmodu/oneshot/internal/domain/users"
	"github.com/openmodu/oneshot/internal/service"
	usecaseauth "github.com/openmodu/oneshot/internal/usecase/auth"
	usecasebilling "github.com/openmodu/oneshot/internal/usecase/billing"
	usecaseorders "github.com/openmodu/oneshot/internal/usecase/orders"
	"github.com/openmodu/oneshot/pkg/httpx"
)

type Server struct {
	services *service.Services
}

func NewServer(services *service.Services) http.Handler {
	server := &Server{services: services}
	router := api.NewRouter()

	router.Use(api.DefaultMiddlewares()...)

	router.Get("/healthz", server.health)
	router.Group("/api", func(router api.Router) {
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
		router.Post("/orders", server.createOrder)
		router.Get("/orders", server.listOrders)
		router.Get("/orders/{orderID}/artifacts", server.listArtifacts)
		router.Get("/orders/{orderID}", server.getOrder)
		router.Post("/orders/{orderID}/cancel", server.cancelOrder)
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
	httpx.WriteJSON(w, http.StatusOK, user)
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
	httpx.WriteJSON(w, http.StatusOK, session)
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
	httpx.WriteJSON(w, http.StatusOK, session)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if token := bearerToken(r); token != "" {
		if err := s.services.Auth.LogoutToken(r.Context(), token); err != nil {
			writeError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if err := s.services.Auth.Logout(r.Context()); err != nil {
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
	id := api.URLParam(r, "agentID")
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
	httpx.WriteJSON(w, http.StatusOK, balance)
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
	httpx.WriteJSON(w, http.StatusOK, ledger)
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
	httpx.WriteJSON(w, http.StatusOK, purchase)
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
	}
	if err := httpx.DecodeJSON(r, &input); err != nil {
		writeError(w, err)
		return
	}

	order, err := s.services.Orders.Create(r.Context(), usecaseorders.CreateInput{
		UserID:      user.ID,
		AgentID:     input.AgentID,
		Requirement: input.Requirement,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, order)
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
	httpx.WriteJSON(w, http.StatusOK, items)
}

func (s *Server) getOrder(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUserFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	id := api.URLParam(r, "orderID")
	order, err := s.services.Orders.Get(r.Context(), user.ID, id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, order)
}

func (s *Server) cancelOrder(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUserFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	id := api.URLParam(r, "orderID")
	order, err := s.services.Orders.Cancel(r.Context(), user.ID, id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, order)
}

func (s *Server) listArtifacts(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUserFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	orderID := api.URLParam(r, "orderID")
	artifacts, err := s.services.Artifacts.ListForOrder(r.Context(), user.ID, orderID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, artifacts)
}

func (s *Server) downloadArtifact(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUserFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	artifactID := api.URLParam(r, "artifactID")
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

	artifactID := api.URLParam(r, "artifactID")
	share, err := s.services.Artifacts.Share(r.Context(), user.ID, artifactID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, share)
}

func (s *Server) currentUserFromRequest(r *http.Request) (users.User, error) {
	if token := bearerToken(r); token != "" {
		return s.services.Auth.CurrentUserByToken(r.Context(), token)
	}
	return s.services.Auth.CurrentUser(r.Context())
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
