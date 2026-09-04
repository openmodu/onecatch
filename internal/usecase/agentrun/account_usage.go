package agentrun

import (
	"context"
	"fmt"
	"time"
)

// AccountUsage is the daily activity and quota snapshot reported by one
// harness. It is deliberately separate from Usage: Usage counts tokens
// consumed by one workflow step, while AccountUsage describes a longer-lived
// account or device history according to Scope.
type AccountUsage struct {
	Runtime      Runtime                       `json:"runtime"`
	Scope        AccountUsageScope             `json:"scope"`
	Source       string                        `json:"source,omitempty"`
	FetchedAt    time.Time                     `json:"fetchedAt"`
	RateLimits   []AccountRateLimit            `json:"rateLimits"`
	DailyUsage   []AccountDailyUsage           `json:"dailyUsage"`
	Summary      AccountUsageSummary           `json:"summary"`
	ResetCredits *AccountRateLimitResetCredits `json:"resetCredits,omitempty"`
}

// AccountUsageScope says which population a snapshot covers. Account data can
// span other machines signed into the same provider, while device data is
// reconstructed from the harness session files visible on this computer.
type AccountUsageScope string

const (
	AccountUsageScopeAccount AccountUsageScope = "account"
	AccountUsageScopeDevice  AccountUsageScope = "device"
)

// AccountDailyUsage is one source-defined calendar day of token activity.
// StartDate is kept as YYYY-MM-DD: turning it into a timestamp in the adapter
// would make the source's day drift across local time zones.
type AccountDailyUsage struct {
	StartDate string `json:"startDate"`
	Tokens    int64  `json:"tokens"`
}

// AccountUsageSummary contains optional lifetime statistics reported alongside
// the daily series. Pointers preserve the difference between a real zero and a
// field an older app-server did not report.
type AccountUsageSummary struct {
	LifetimeTokens        *int64 `json:"lifetimeTokens,omitempty"`
	PeakDailyTokens       *int64 `json:"peakDailyTokens,omitempty"`
	LongestRunningTurnSec *int64 `json:"longestRunningTurnSec,omitempty"`
	CurrentStreakDays     *int64 `json:"currentStreakDays,omitempty"`
	LongestStreakDays     *int64 `json:"longestStreakDays,omitempty"`
}

// AccountRateLimit is one independently metered quota bucket. Providers can
// expose more than one bucket (for example a general Codex limit and a
// model-specific limit), so callers must not assume there is exactly one.
type AccountRateLimit struct {
	ID                   string                  `json:"id"`
	Name                 string                  `json:"name,omitempty"`
	PlanType             string                  `json:"planType,omitempty"`
	Primary              *AccountRateLimitWindow `json:"primary,omitempty"`
	Secondary            *AccountRateLimitWindow `json:"secondary,omitempty"`
	Credits              *AccountCredits         `json:"credits,omitempty"`
	IndividualLimit      *AccountSpendControl    `json:"individualLimit,omitempty"`
	SpendControlReached  *bool                   `json:"spendControlReached,omitempty"`
	RateLimitReachedType string                  `json:"rateLimitReachedType,omitempty"`
}

type AccountRateLimitWindow struct {
	UsedPercent        int   `json:"usedPercent"`
	WindowDurationMins int64 `json:"windowDurationMins,omitempty"`
	ResetsAt           int64 `json:"resetsAt,omitempty"`
}

type AccountCredits struct {
	HasCredits bool   `json:"hasCredits"`
	Unlimited  bool   `json:"unlimited"`
	Balance    string `json:"balance,omitempty"`
}

type AccountSpendControl struct {
	Limit            string `json:"limit"`
	Used             string `json:"used"`
	RemainingPercent int    `json:"remainingPercent"`
	ResetsAt         int64  `json:"resetsAt"`
}

type AccountRateLimitResetCredits struct {
	AvailableCount int                           `json:"availableCount"`
	Credits        []AccountRateLimitResetCredit `json:"credits,omitempty"`
}

type AccountRateLimitResetCredit struct {
	ID          string `json:"id"`
	ResetType   string `json:"resetType"`
	Status      string `json:"status"`
	GrantedAt   int64  `json:"grantedAt"`
	ExpiresAt   int64  `json:"expiresAt,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

// AccountUsageReader is implemented by a harness that can inspect daily usage
// without starting a model turn. Rolling account limits are optional.
type AccountUsageReader interface {
	ReadAccountUsage(ctx context.Context, cwd string, environment []string) (AccountUsage, error)
}

// ErrAccountUsageUnsupported lets the UI distinguish an unsupported harness
// from a failed account or CLI request.
type ErrAccountUsageUnsupported struct{ Runtime Runtime }

func (e ErrAccountUsageUnsupported) Error() string {
	return fmt.Sprintf("agentrun: runtime %q cannot report account usage", e.Runtime)
}

// ReadAccountUsage asks one runtime for daily activity and any quota its source
// exposes. The interface keeps harness-specific storage and APIs out of the
// desktop service and UI plumbing.
func (e *Engine) ReadAccountUsage(ctx context.Context, rt Runtime, cwd string, environment []string) (AccountUsage, error) {
	runner := e.runners[rt]
	if runner == nil {
		return AccountUsage{}, ErrUnknownRuntime{Runtime: rt}
	}
	if !runner.Available() {
		return AccountUsage{}, ErrRuntimeUnavailable{Runtime: rt}
	}
	reader, ok := runner.(AccountUsageReader)
	if !ok {
		return AccountUsage{}, ErrAccountUsageUnsupported{Runtime: rt}
	}
	return reader.ReadAccountUsage(ctx, cwd, environment)
}
