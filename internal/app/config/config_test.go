package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPathDefaultsWhenMissingDefaultFile(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.Log.Service != "oneshot-server" {
		t.Fatalf("Log.Service = %q, want oneshot-server", cfg.Log.Service)
	}
	if cfg.MySQLDSN != "" {
		t.Fatalf("MySQLDSN = %q, want empty", cfg.MySQLDSN)
	}
}

func TestLoadPathReadsYAML(t *testing.T) {
	clearEnv(t)
	path := writeConfig(t, `
http:
  addr: ":9090"
mysql:
  addr: "mysql.example:3307"
  user: "oneshot"
  password: "secret"
  database: "oneshot"
logger:
  service: "api"
  level: "debug"
  format: "json"
  file: "logs/api.log"
  max_size_mb: 10
  max_backups: 3
  max_age_days: 5
  compress: true
`)

	cfg, err := LoadPath(path)
	if err != nil {
		t.Fatalf("LoadPath() error = %v", err)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Fatalf("HTTPAddr = %q, want :9090", cfg.HTTPAddr)
	}
	if !strings.Contains(cfg.MySQLDSN, "oneshot:secret@tcp(mysql.example:3307)/oneshot") {
		t.Fatalf("MySQLDSN = %q, want composed DSN", cfg.MySQLDSN)
	}
	if cfg.Log.Level != "debug" || cfg.Log.Format != "json" || cfg.Log.File != "logs/api.log" {
		t.Fatalf("Log config = %+v, want YAML logger values", cfg.Log)
	}
	if !cfg.Log.Compress {
		t.Fatal("Log.Compress = false, want true")
	}
}

func TestLoadPathEnvOverridesYAML(t *testing.T) {
	clearEnv(t)
	t.Setenv("ONESHOT_ADDR", ":7070")
	t.Setenv("ONESHOT_MYSQL_DSN", "root:env@tcp(127.0.0.1:3306)/env")
	t.Setenv("ONESHOT_LOG_LEVEL", "warn")
	t.Setenv("ONESHOT_LOG_FILE", "logs/env.log")

	path := writeConfig(t, `
http:
  addr: ":9090"
mysql:
  addr: "mysql.example:3307"
logger:
  level: "debug"
  file: "logs/file.log"
`)

	cfg, err := LoadPath(path)
	if err != nil {
		t.Fatalf("LoadPath() error = %v", err)
	}
	if cfg.HTTPAddr != ":7070" {
		t.Fatalf("HTTPAddr = %q, want env override", cfg.HTTPAddr)
	}
	if cfg.MySQLDSN != "root:env@tcp(127.0.0.1:3306)/env" {
		t.Fatalf("MySQLDSN = %q, want env override", cfg.MySQLDSN)
	}
	if cfg.Log.Level != "warn" || cfg.Log.File != "logs/env.log" {
		t.Fatalf("Log config = %+v, want env override", cfg.Log)
	}
}

func TestLoadExplicitMissingConfigReturnsError(t *testing.T) {
	clearEnv(t)
	t.Setenv("ONESHOT_CONFIG", filepath.Join(t.TempDir(), "missing.yaml"))

	_, err := Load()
	if err == nil {
		t.Fatal("expected missing explicit config error")
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func clearEnv(t *testing.T) {
	t.Helper()

	for _, name := range []string{
		"ONESHOT_CONFIG",
		"ONESHOT_ADDR",
		"ONESHOT_MYSQL_DSN",
		"ONESHOT_MYSQL_ADDR",
		"ONESHOT_MYSQL_USER",
		"ONESHOT_MYSQL_PASSWORD",
		"ONESHOT_MYSQL_DATABASE",
		"ONESHOT_LOG_SERVICE",
		"ONESHOT_LOG_LEVEL",
		"ONESHOT_LOG_FORMAT",
		"ONESHOT_LOG_FILE",
		"ONESHOT_LOG_MAX_SIZE_MB",
		"ONESHOT_LOG_MAX_BACKUPS",
		"ONESHOT_LOG_MAX_AGE_DAYS",
		"ONESHOT_LOG_COMPRESS",
		"ONESHOT_LOG_DEVELOPMENT",
		"ONESHOT_LOG_DISABLE_STDOUT",
	} {
		t.Setenv(name, "")
	}
}
