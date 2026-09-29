package agentrun

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// TRAE reports weekly basic quota through its own model endpoint. This is
// account quota, not token consumption; no daily token history is inferred.
func (r *TraeRunner) ReadAccountUsage(ctx context.Context, cwd string, environment []string) (AccountUsage, error) {

	now := time.Now()
	local, err := readTraeLocalUsage(ctx, environment, now)
	if err != nil {
		return AccountUsage{}, err
	}
	quota, err := r.readTraeQuota(ctx, cwd, environment, now)
	if err != nil {
		if ctx.Err() != nil {
			return AccountUsage{}, ctx.Err()
		}
		local.Warning = err.Error()
		return local, nil
	}
	local.Scope = AccountUsageScopeMixed
	local.Source = "local-sessions + model/internalUsage/read"
	local.RateLimits = quota.RateLimits
	return local, nil
}

func (r *TraeRunner) readTraeQuota(ctx context.Context, cwd string, environment []string, now time.Time) (AccountUsage, error) {
	p, err := r.connect(ctx, cwd, environment)
	if err != nil {
		return AccountUsage{}, err
	}
	defer p.close()
	raw, err := p.call("model/internalUsage/read", map[string]any{})
	if err != nil {
		return AccountUsage{}, err
	}
	return decodeTraeAccountUsage(raw, now)
}

func decodeTraeAccountUsage(raw json.RawMessage, now time.Time) (AccountUsage, error) {
	var response struct {
		Usage *struct {
			Percent   *int    `json:"usage_percent"`
			Remaining *uint32 `json:"quota_remaining"`
			Limit     *uint32 `json:"quota_limit"`
			Depleted  *bool   `json:"is_depleted"`
			ResetTime int64   `json:"reset_time"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return AccountUsage{}, fmt.Errorf("decode TRAE usage: %w", err)
	}
	if response.Usage == nil {
		return AccountUsage{}, fmt.Errorf("TRAE weekly usage is currently unavailable")
	}
	usage := response.Usage
	limit := AccountRateLimit{ID: "trae_weekly", Name: "TRAE", QuotaLimit: usage.Limit, QuotaRemaining: usage.Remaining, SpendControlReached: usage.Depleted}
	if usage.Percent != nil {
		limit.Primary = &AccountRateLimitWindow{UsedPercent: max(0, min(100, *usage.Percent)), WindowDurationMins: 7 * 24 * 60, ResetsAt: usage.ResetTime}
	}
	return AccountUsage{
		Runtime: RuntimeTrae, Scope: AccountUsageScopeAccount, Source: "model/internalUsage/read", FetchedAt: now,
		RateLimits: []AccountRateLimit{limit}, DailyUsage: []AccountDailyUsage{},
	}, nil
}
