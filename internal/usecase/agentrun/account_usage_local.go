package agentrun

import (
	"bufio"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxLocalUsageLineBytes = 64 << 20

// localUsageCollector deduplicates provider records before assigning them to
// the user's local calendar day. Session formats commonly repeat a completed
// assistant message while streaming or when a conversation is forked.
type localUsageCollector struct {
	days map[string]int64
	seen map[string]localUsageRecord
}

type localUsageRecord struct {
	day    string
	tokens int64
}

func newLocalUsageCollector() *localUsageCollector {
	return &localUsageCollector{days: make(map[string]int64), seen: make(map[string]localUsageRecord)}
}

func (c *localUsageCollector) add(id string, at time.Time, tokens int64) {
	if at.IsZero() || tokens <= 0 {
		return
	}
	id = strings.TrimSpace(id)
	day := at.In(time.Local).Format("2006-01-02")
	if id != "" {
		if previous, ok := c.seen[id]; ok {
			// Some streaming transcripts write the same message more than once.
			// Keep the largest completed reading rather than whichever copy was
			// encountered first.
			if previous.day == day && tokens > previous.tokens {
				c.days[day] += tokens - previous.tokens
				c.seen[id] = localUsageRecord{day: day, tokens: tokens}
			}
			return
		}
		c.seen[id] = localUsageRecord{day: day, tokens: tokens}
	}
	c.days[day] += tokens
}

func (c *localUsageCollector) merge(day string, tokens int64) {
	if _, err := time.ParseInLocation("2006-01-02", day, time.Local); err != nil || tokens <= 0 {
		return
	}
	c.days[day] += tokens
}

func localAccountUsage(runtime Runtime, source string, fetchedAt time.Time, days map[string]int64) AccountUsage {
	usage := AccountUsage{
		Runtime: runtime, Scope: AccountUsageScopeDevice, Source: source, FetchedAt: fetchedAt,
		DailyUsage: make([]AccountDailyUsage, 0, len(days)),
	}
	for day, tokens := range days {
		if tokens > 0 {
			usage.DailyUsage = append(usage.DailyUsage, AccountDailyUsage{StartDate: day, Tokens: tokens})
		}
	}
	sort.Slice(usage.DailyUsage, func(i, j int) bool {
		return usage.DailyUsage[i].StartDate < usage.DailyUsage[j].StartDate
	})
	usage.Summary = summarizeLocalDailyUsage(usage.DailyUsage, fetchedAt)
	return usage
}

func summarizeLocalDailyUsage(days []AccountDailyUsage, now time.Time) AccountUsageSummary {
	var lifetime, peak int64
	longest, currentRun := int64(0), int64(0)
	var previous time.Time
	for _, bucket := range days {
		date, err := time.ParseInLocation("2006-01-02", bucket.StartDate, time.Local)
		if err != nil || bucket.Tokens <= 0 {
			continue
		}
		lifetime += bucket.Tokens
		if bucket.Tokens > peak {
			peak = bucket.Tokens
		}
		if previous.IsZero() || !date.Equal(previous.AddDate(0, 0, 1)) {
			currentRun = 1
		} else {
			currentRun++
		}
		if currentRun > longest {
			longest = currentRun
		}
		previous = date
	}
	streak := currentRun
	today := time.Date(now.In(time.Local).Year(), now.In(time.Local).Month(), now.In(time.Local).Day(), 0, 0, 0, 0, time.Local)
	if previous.IsZero() || previous.Before(today.AddDate(0, 0, -1)) {
		streak = 0
	}
	return AccountUsageSummary{
		LifetimeTokens: &lifetime, PeakDailyTokens: &peak,
		CurrentStreakDays: &streak, LongestStreakDays: &longest,
	}
}

func usageEnvironmentValue(environment []string, key string) string {
	prefix := key + "="
	for _, item := range environment {
		if strings.HasPrefix(item, prefix) {
			return strings.TrimPrefix(item, prefix)
		}
	}
	return ""
}

func usageHome(environment []string) string {
	if home := strings.TrimSpace(usageEnvironmentValue(environment, "HOME")); home != "" {
		return home
	}
	home, _ := os.UserHomeDir()
	return home
}

func expandUsagePath(value, home string) string {
	value = strings.TrimSpace(value)
	if value == "~" {
		return home
	}
	if strings.HasPrefix(value, "~/") {
		return filepath.Join(home, strings.TrimPrefix(value, "~/"))
	}
	return value
}

// scanUsageJSONL walks only ordinary JSONL files and never follows symlinks.
// Corrupt JSON records are left to the adapter to skip so one interrupted
// session cannot hide every other session's usage.
func scanUsageJSONL(ctx context.Context, root string, minModTime time.Time, fileName string, visit func(path, line string)) error {
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Ext(entry.Name()) != ".jsonl" {
			return nil
		}
		if fileName != "" && entry.Name() != fileName {
			return nil
		}
		if !minModTime.IsZero() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.ModTime().Before(minModTime) {
				return nil
			}
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		reader := bufio.NewReader(file)
		for {
			line, readErr := reader.ReadString('\n')
			if len(line) > 0 && len(line) <= maxLocalUsageLineBytes && strings.Contains(line, "\"usage\"") {
				visit(path, line)
			}
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					return nil
				}
				return readErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
		}
	})
}

func localUsageTokens(total, input, output, cacheRead, cacheWrite int64) int64 {
	if total > 0 {
		return total
	}
	return input + output + cacheRead + cacheWrite
}
