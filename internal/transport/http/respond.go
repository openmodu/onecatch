package httptransport

import (
	"errors"
	"net/http"

	domainagents "github.com/openmodu/oneshot/internal/domain/agents"
	domainartifacts "github.com/openmodu/oneshot/internal/domain/artifacts"
	domainauth "github.com/openmodu/oneshot/internal/domain/auth"
	domainbilling "github.com/openmodu/oneshot/internal/domain/billing"
	domainorders "github.com/openmodu/oneshot/internal/domain/orders"
	"github.com/openmodu/oneshot/pkg/httpx"
)

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, domainagents.ErrNotFound), errors.Is(err, domainorders.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, domainauth.ErrUnauthenticated):
		status = http.StatusUnauthorized
	case errors.Is(err, domainbilling.ErrInsufficientBalance):
		status = http.StatusPaymentRequired
	case errors.Is(err, domainorders.ErrInvalidRequirement):
		status = http.StatusBadRequest
	case errors.Is(err, domainartifacts.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, domainartifacts.ErrNotReady):
		status = http.StatusConflict
	}
	httpx.WriteError(w, status, err)
}
