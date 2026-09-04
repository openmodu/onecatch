//go:build !onecatch_worker

package agentrun

import "context"

func (r *ModuSDKRunner) ReadAccountUsage(ctx context.Context, _ string, environment []string) (AccountUsage, error) {
	return readModuLocalAccountUsage(ctx, r.agentDir, environment, r.now())
}
