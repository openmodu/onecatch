package httptransport

import (
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"

	routing "github.com/openmodu/oneshot/internal/transport/router"
	"github.com/openmodu/oneshot/pkg/httpx"
)

// errAdminForbidden is intentionally generic so denials don't reveal whether
// admin is configured or what was wrong with the credential.
var errAdminForbidden = errors.New("forbidden")

// Admin boundary scaffold.
//
// Back-office capabilities live under a SEPARATE route tree (/admin/api) with
// their OWN authentication, distinct from the user-facing /api/* surface. This
// file reserves that boundary; it deliberately does not implement real admin
// features (out of scope for issue 00-015).
//
// Rules this scaffold enforces:
//   - Admin routes are never mounted under /api and are never reachable through
//     the desktop oneshot client / Wails bindings (which only call /api/*).
//   - Admin auth is independent: a shared admin token via the X-Admin-Token
//     header. Missing/empty config means admin is CLOSED — every request 403s.
//   - Insufficient privilege returns 403 (not 401), and admin access is audited.
//   - Admin responses may use richer DTOs than the user-facing ones; those will
//     be defined alongside real admin handlers when they land.

const adminTokenHeader = "X-Admin-Token"

type adminHandler struct {
	token string
	audit *slog.Logger
}

func newAdminHandler(token string, audit *slog.Logger) *adminHandler {
	if audit == nil {
		audit = slog.Default()
	}
	return &adminHandler{token: token, audit: audit}
}

// register mounts the admin routes under their own subtree with admin-only
// middleware. Kept separate from the user-facing /api group on purpose.
func (h *adminHandler) register(router routing.Router) {
	router.Group("/admin/api", func(admin routing.Router) {
		admin.Use(h.requireAdmin)
		admin.Get("/whoami", h.whoami)
	})
}

// requireAdmin gates the admin subtree. It fails closed: with no configured
// token, or any mismatch, it returns 403 without revealing why.
func (h *adminHandler) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		presented := r.Header.Get(adminTokenHeader)
		if h.token == "" || subtle.ConstantTimeCompare([]byte(presented), []byte(h.token)) != 1 {
			h.audit.Info("admin access denied",
				slog.String("path", r.URL.Path),
				slog.String("token", redactToken(presented)),
			)
			httpx.WriteError(w, http.StatusForbidden, errAdminForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *adminHandler) whoami(w http.ResponseWriter, r *http.Request) {
	h.audit.Info("admin action",
		slog.String("action", "whoami"),
		slog.String("path", r.URL.Path),
	)
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"role": "admin"})
}
