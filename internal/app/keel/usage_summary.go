package keeldesktop

import (
	"sort"
	"time"

	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type usageTotals struct {
	Daily                            map[string]int64
	Lifetime, Peak, Current, Longest int64
	Available                        [4]bool
}

// The All view derives peaks and streaks from merged calendar days. Taking the
// largest provider summary misses simultaneous usage and alternating sources.
func summarizeUsage(usages []agentrun.AccountUsage, filter string, now time.Time) usageTotals {
	out := usageTotals{Daily: map[string]int64{}}
	var individual *agentrun.AccountUsageSummary
	for _, usage := range usages {
		if filter != "" && string(usage.Runtime) != filter {
			continue
		}
		individual = &usage.Summary
		if value := usage.Summary.LifetimeTokens; value != nil {
			out.Lifetime += max(0, *value)
			out.Available[0] = true
		}
		for _, day := range usage.DailyUsage {
			if _, err := time.Parse("2006-01-02", day.StartDate); err != nil {
				continue
			}
			out.Daily[day.StartDate] += max(0, day.Tokens)
		}
	}
	var active []string
	var total int64
	for day, tokens := range out.Daily {
		total += tokens
		out.Peak = max(out.Peak, tokens)
		if tokens > 0 {
			active = append(active, day)
		}
	}
	if len(out.Daily) > 0 {
		out.Available = [4]bool{true, true, true, true}
		if out.Lifetime == 0 {
			out.Lifetime = total
		}
	}
	sort.Strings(active)
	var previous time.Time
	var run int64
	for _, day := range active {
		date, _ := time.ParseInLocation("2006-01-02", day, now.Location())
		if date.Equal(previous.AddDate(0, 0, 1)) {
			run++
		} else {
			run = 1
		}
		out.Longest = max(out.Longest, run)
		previous = date
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if !previous.Before(today.AddDate(0, 0, -1)) {
		out.Current = run
	}
	// A single provider may report lifetime statistics beyond its daily series.
	if filter != "" && individual != nil {
		values := []*int64{individual.LifetimeTokens, individual.PeakDailyTokens, individual.CurrentStreakDays, individual.LongestStreakDays}
		targets := []*int64{&out.Lifetime, &out.Peak, &out.Current, &out.Longest}
		for i, value := range values {
			if value != nil {
				*targets[i] = max(0, *value)
				out.Available[i] = true
			}
		}
	}
	return out
}

// Match the existing frontend's quartile-based heatmap; a single outlier should
// not flatten all other active days into the lowest color level.
func usageHeatLevel(tokens int64, sorted []int64) int {
	if tokens <= 0 || len(sorted) == 0 {
		return 0
	}
	for i, ratio := range []float64{0.25, 0.5, 0.75, 1} {
		if tokens <= sorted[int(float64(len(sorted)-1)*ratio)] {
			return i + 1
		}
	}
	return 4
}
