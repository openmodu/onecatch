package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openmodu/onecatch/internal/service/skillmanager"
	"github.com/openmodu/onecatch/pkg/localfile"
)

const (
	// A debug transcript is worth keeping because it is the evidence for why a
	// skill was changed, but it is not an archive. Twenty runs covers an
	// afternoon of iteration on one skill.
	skillDebugHistoryLimit = 20
	// Tool output can be enormous. Rather than truncate individual events and
	// hand the user a transcript that quietly lies, drop whole oldest runs
	// until the file fits.
	skillDebugHistoryBytes = 2 << 20
)

// SkillDebugRecord is one persisted debug run, newest first on disk.
type SkillDebugRecord struct {
	RunID     string           `json:"runId"`
	Skill     string           `json:"skill"`
	Prompt    string           `json:"prompt"`
	StartedAt time.Time        `json:"startedAt"`
	Result    SkillDebugResult `json:"result"`
}

// skillDebugHistoryPath resolves ~/.onecatch/skill-debug/<skill>.json. History
// lives beside the library rather than inside it so a synced skill directory
// never carries one machine's debug transcripts to another agent.
func (a *Service) skillDebugHistoryPath(skill string) (string, error) {
	manager, err := a.skillManager()
	if err != nil {
		return "", err
	}
	skill = strings.TrimSpace(skill)
	if !skillmanager.ValidSkillName(skill) {
		return "", coded("skill_not_found", "unknown skill")
	}
	return filepath.Join(filepath.Dir(manager.Root()), "skill-debug", skill+".json"), nil
}

// ListSkillDebugRuns returns the stored transcripts for one skill, newest
// first. A skill that has never been debugged has no file and no error.
func (a *Service) ListSkillDebugRuns(skill string) ([]SkillDebugRecord, error) {
	path, err := a.skillDebugHistoryPath(skill)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return []SkillDebugRecord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read skill debug history: %w", err)
	}
	records := []SkillDebugRecord{}
	if err := json.Unmarshal(data, &records); err != nil {
		// A corrupt history is not worth failing the panel over; the next run
		// rewrites the file.
		return []SkillDebugRecord{}, nil
	}
	return records, nil
}

// ClearSkillDebugRuns forgets every stored transcript for one skill.
func (a *Service) ClearSkillDebugRuns(skill string) error {
	path, err := a.skillDebugHistoryPath(skill)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clear skill debug history: %w", err)
	}
	return nil
}

func (a *Service) appendSkillDebugRun(record SkillDebugRecord) error {
	path, err := a.skillDebugHistoryPath(record.Skill)
	if err != nil {
		return err
	}
	a.skillDebugMu.Lock()
	defer a.skillDebugMu.Unlock()

	existing, err := os.ReadFile(path)
	records := []SkillDebugRecord{}
	if err == nil {
		_ = json.Unmarshal(existing, &records)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read skill debug history: %w", err)
	}
	records = append([]SkillDebugRecord{record}, records...)
	if len(records) > skillDebugHistoryLimit {
		records = records[:skillDebugHistoryLimit]
	}
	encoded, err := json.Marshal(records)
	for err == nil && len(encoded) > skillDebugHistoryBytes && len(records) > 1 {
		records = records[:len(records)-1]
		encoded, err = json.Marshal(records)
	}
	if err != nil {
		return fmt.Errorf("encode skill debug history: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create skill debug history directory: %w", err)
	}
	if err := localfile.WriteTextAtomic(path, string(encoded)); err != nil {
		return fmt.Errorf("write skill debug history: %w", err)
	}
	return nil
}
