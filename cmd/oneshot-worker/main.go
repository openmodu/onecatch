package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openmodu/oneshot/internal/agentrun"
	"github.com/openmodu/oneshot/internal/gitinspect"
	"github.com/openmodu/oneshot/internal/worker"
)

type workspaceFlags map[string]string

func (f workspaceFlags) String() string { return fmt.Sprintf("%v", map[string]string(f)) }
func (f workspaceFlags) Set(value string) error {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return fmt.Errorf("workspace must be id=/absolute/path")
	}
	f[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	return nil
}

func main() {
	workspaces := workspaceFlags{}
	listen := flag.String("listen", "127.0.0.1:9231", "HTTP listen address")
	id := flag.String("id", "worker", "stable worker ID")
	name := flag.String("name", "Oneshot Worker", "worker display name")
	token := flag.String("token", "", "shared bearer token (prefer --token-env)")
	tokenEnv := flag.String("token-env", "ONESHOT_WORKER_TOKEN", "environment variable containing the token")
	codex := flag.String("codex-binary", "", "Codex binary override")
	claude := flag.String("claude-binary", "", "Claude binary override")
	modu := flag.String("modu-binary", "", "Modu Code binary override")
	maxConcurrency := flag.Int("max-concurrency", 4, "maximum simultaneous runs (<=0 uses the default)")
	flag.Var(workspaces, "workspace", "workspace mapping id=/absolute/path (repeatable)")
	flag.Parse()
	secret := strings.TrimSpace(*token)
	if secret == "" {
		secret = strings.TrimSpace(os.Getenv(*tokenEnv))
	}
	if secret == "" {
		log.Fatal("worker token is required")
	}
	if len(workspaces) == 0 {
		log.Fatal("at least one --workspace mapping is required")
	}
	for workspaceID, path := range workspaces {
		absolute, err := filepath.Abs(path)
		if err != nil {
			log.Fatalf("resolve workspace %s: %v", workspaceID, err)
		}
		info, err := os.Stat(absolute)
		if err != nil || !info.IsDir() {
			log.Fatalf("workspace %s is not an existing directory", workspaceID)
		}
		workspaces[workspaceID] = absolute
	}
	engine := agentrun.NewEngine(agentrun.Config{CodexBinary: *codex, ClaudeBinary: *claude, ModuBinary: *modu})
	service := worker.NewServer(*id, *name, secret, workspaces, engine, *maxConcurrency)
	service.SetGitInspector(gitinspect.New(""))
	server := &http.Server{Addr: *listen, Handler: service.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: time.Minute, WriteTimeout: worker.MaxRunDuration + 2*time.Minute, IdleTimeout: 60 * time.Second}
	log.Printf("oneshot worker %s listening on %s", *id, *listen)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
