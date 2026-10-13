package desktop

import (
	"context"

	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

// StartRunLoopConfirmed applies the composer's choice to the private run
// definition before dispatch. The shared workflow's default stays unchanged.
func (a *Service) StartRunLoopConfirmed(ctx context.Context, taskID, token string, enabled bool) (workflows.Run, error) {
	if err := a.validateTaskSecurity(ctx, taskID, token); err != nil {
		return workflows.Run{}, err
	}
	return a.startRunAuthorizedMode(ctx, taskID, &enabled)
}
