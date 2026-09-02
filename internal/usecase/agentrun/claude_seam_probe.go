package agentrun

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam"
	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam/seamtest"
)

const claudeSeamProbeTimeout = 90 * time.Second

var claudeSeamProbeMu sync.Mutex

type claudeSeamVerdict struct {
	Version string `json:"version"`
	Pass    bool   `json:"pass"`
}

// verifyRemoteSeam behaviorally checks CLAUDE_CODE_SHELL_PREFIX with the
// installed Claude binary. A version string cannot prove the prefix is still
// honored; an offline mock-model turn can. Measured local execution fails
// closed. An inconclusive probe warns and proceeds, because it is not evidence
// that the seam was bypassed.
func (r *ClaudeRunner) verifyRemoteSeam(ctx context.Context, req Request, environment, remoteArgs []string) error {
	claudeSeamProbeMu.Lock()
	defer claudeSeamProbeMu.Unlock()

	version, err := r.claudeVersion(ctx, environment)
	if err != nil {
		fmt.Fprintf(os.Stderr, "onecatch: WARNING: Claude remote seam could not be versioned: %v\n", err)
		return nil
	}
	cache, err := claudeSeamVerdictPath(r.binary, version)
	if err == nil {
		if verdict, ok := loadClaudeSeamVerdict(cache, version); ok {
			if verdict.Pass {
				return nil
			}
			return fmt.Errorf("claude code %s previously bypassed OneCatch's remote shell seam; refusing an unsafe remote run", version)
		}
	}

	pass, measuredBypass, detail := r.probeRemoteSeam(ctx, req, environment, remoteArgs)
	if pass {
		if cacheErr := saveClaudeSeamVerdict(cache, claudeSeamVerdict{Version: version, Pass: true}); cacheErr != nil {
			fmt.Fprintf(os.Stderr, "onecatch: WARNING: could not cache Claude seam verdict: %v\n", cacheErr)
		}
		return nil
	}
	if measuredBypass {
		_ = saveClaudeSeamVerdict(cache, claudeSeamVerdict{Version: version, Pass: false})
		return fmt.Errorf("claude code %s bypassed OneCatch's remote shell seam (%s); refusing to run commands on the local machine", version, detail)
	}
	fmt.Fprintf(os.Stderr, "onecatch: WARNING: Claude Code %s remote seam probe was inconclusive: %s\n", version, detail)
	return nil
}

func (r *ClaudeRunner) claudeVersion(ctx context.Context, environment []string) (string, error) {
	versionCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(versionCtx, r.binary, "--version")
	cmd.Env = environment
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	version := strings.TrimSpace(string(output))
	if version == "" {
		return "", fmt.Errorf("empty --version output")
	}
	return version, nil
}

func (r *ClaudeRunner) probeRemoteSeam(parent context.Context, req Request, environment, remoteArgs []string) (pass, measuredBypass bool, detail string) {
	probeCtx, cancel := context.WithTimeout(parent, claudeSeamProbeTimeout)
	defer cancel()
	mock := seamtest.StartMock(seamtest.DialectAnthropic, "pwd")
	defer mock.Close()
	home, err := os.MkdirTemp("", "onecatch-claude-probe-")
	if err != nil {
		return false, false, err.Error()
	}
	defer func() { _ = os.RemoveAll(home) }()

	prefix := environmentValue(environment, "CLAUDE_CODE_SHELL_PREFIX")
	session := environmentValue(environment, seam.SessionEnv)
	probeEnvironment := scrubClaudeProbeEnvironment(environment)
	probeEnvironment = mergeEnvironment(probeEnvironment, []string{
		"HOME=" + home,
		"ANTHROPIC_API_KEY=onecatch-offline-probe",
		"ANTHROPIC_BASE_URL=" + mock.BaseURL(),
		"CLAUDE_CODE_SHELL_PREFIX=" + prefix,
		seam.SessionEnv + "=" + session,
		"CLAUDE_TELEMETRY_OPT_OUT=1",
		"DO_NOT_TRACK=1",
	})
	args := []string{
		"-p", "Run the requested shell command exactly once.",
		"--output-format", "stream-json", "--verbose",
		"--dangerously-skip-permissions",
	}
	args = append(args, remoteArgs...)
	cmd := exec.CommandContext(probeCtx, r.binary, args...)
	cmd.Dir = req.Workspace
	cmd.Env = probeEnvironment
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	runErr := cmd.Run()
	mock.Wait(time.Second)
	toolOutput, observed := mock.Result()
	return classifyClaudeSeamProbe(req.Workspace, req.Remote.Root, toolOutput, observed, runErr, probeCtx.Err(), output.String())
}

func classifyClaudeSeamProbe(localWorkspace, targetRoot, toolOutput string, observed bool, runErr, contextErr error, processOutput string) (pass, measuredBypass bool, detail string) {
	if observed {
		lines := strings.Split(strings.ReplaceAll(toolOutput, "\r\n", "\n"), "\n")
		for _, line := range lines {
			cwd := strings.TrimSpace(line)
			switch cwd {
			case filepath.Clean(targetRoot):
				return true, false, ""
			case filepath.Clean(localWorkspace):
				return false, true, "pwd reported the local harness workspace"
			}
		}
		return false, false, "tool output did not identify the target workspace: " + strings.TrimSpace(toolOutput)
	}
	if contextErr != nil {
		return false, false, contextErr.Error()
	}
	detail = strings.TrimSpace(processOutput)
	if len(detail) > 1200 {
		detail = detail[len(detail)-1200:]
	}
	if runErr != nil {
		if detail != "" {
			return false, false, runErr.Error() + ": " + detail
		}
		return false, false, runErr.Error()
	}
	if detail == "" {
		detail = "the harness returned no shell tool result"
	}
	return false, false, detail
}

func scrubClaudeProbeEnvironment(environment []string) []string {
	if environment == nil {
		environment = os.Environ()
	}
	result := make([]string, 0, len(environment))
	for _, value := range environment {
		key, _, found := strings.Cut(value, "=")
		upper := strings.ToUpper(key)
		if !found || key == "HOME" || strings.HasPrefix(upper, "ANTHROPIC_") ||
			strings.HasPrefix(upper, "OPENAI_") || strings.HasPrefix(upper, "AWS_") ||
			strings.HasSuffix(upper, "_API_KEY") || strings.HasSuffix(upper, "_TOKEN") ||
			strings.HasSuffix(upper, "_SECRET") {
			continue
		}
		result = append(result, value)
	}
	return result
}

func claudeSeamVerdictPath(binary, version string) (string, error) {
	directory, err := seam.SessionDir()
	if err != nil {
		return "", err
	}
	resolved, err := exec.LookPath(binary)
	if err != nil {
		resolved = binary
	}
	if absolute, err := filepath.Abs(resolved); err == nil {
		resolved = absolute
	}
	sum := sha256.Sum256([]byte(resolved + "\x00" + version))
	return filepath.Join(directory, "seam-verdicts", "claude-"+hex.EncodeToString(sum[:12])+".json"), nil
}

func loadClaudeSeamVerdict(path, version string) (claudeSeamVerdict, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return claudeSeamVerdict{}, false
	}
	var verdict claudeSeamVerdict
	if json.Unmarshal(data, &verdict) != nil || verdict.Version != version {
		return claudeSeamVerdict{}, false
	}
	return verdict, true
}

func saveClaudeSeamVerdict(path string, verdict claudeSeamVerdict) error {
	if path == "" {
		return fmt.Errorf("verdict cache path is unavailable")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(verdict)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".claude-verdict-")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer func() { _ = os.Remove(name) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
