package config

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/openmodu/oneshot/pkg/logger"
	"gopkg.in/yaml.v3"
)

const DefaultPath = "config/oneshot-server.yaml"

type Config struct {
	HTTPAddr string
	MySQLDSN string
	Log      logger.Config
	// WorkspaceRoot is where each order's agent workspace is created.
	WorkspaceRoot string
	// CodexBinary / ClaudeBinary override the agent CLIs; empty resolves from PATH.
	CodexBinary  string
	ClaudeBinary string
}

type fileConfig struct {
	HTTP      httpConfig      `yaml:"http"`
	MySQL     mysqlConfig     `yaml:"mysql"`
	Logger    logger.Config   `yaml:"logger"`
	Execution executionConfig `yaml:"execution"`
}

type executionConfig struct {
	WorkspaceRoot string `yaml:"workspace_root"`
	CodexBinary   string `yaml:"codex_binary"`
	ClaudeBinary  string `yaml:"claude_binary"`
}

type httpConfig struct {
	Addr string `yaml:"addr"`
}

type mysqlConfig struct {
	DSN      string `yaml:"dsn"`
	Addr     string `yaml:"addr"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
}

func Load() (Config, error) {
	path, required := configPath()
	return loadPath(path, required)
}

func LoadPath(path string) (Config, error) {
	return loadPath(path, true)
}

func loadPath(path string, required bool) (Config, error) {
	cfg := defaultConfig()
	if path != "" {
		if err := loadFile(path, required, &cfg); err != nil {
			return Config{}, err
		}
	}

	applyEnv(&cfg)
	return cfg, nil
}

func defaultConfig() Config {
	return Config{
		HTTPAddr:      ":8080",
		WorkspaceRoot: defaultWorkspaceRoot(),
		Log: logger.Config{
			Service:       "oneshot-server",
			Level:         logger.DefaultLevel,
			Format:        logger.DefaultFormat,
			MaxSizeMB:     logger.DefaultMaxSizeMB,
			MaxBackups:    logger.DefaultMaxBackups,
			MaxAgeDays:    logger.DefaultMaxAgeDays,
			Compress:      false,
			Development:   false,
			DisableStdout: false,
		},
	}
}

func configPath() (string, bool) {
	if path := os.Getenv("ONESHOT_CONFIG"); path != "" {
		return path, true
	}
	return DefaultPath, false
}

func loadFile(path string, required bool, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !required {
			return nil
		}
		return err
	}

	var file fileConfig
	if err := yaml.Unmarshal(data, &file); err != nil {
		return err
	}
	applyFile(cfg, file)
	return nil
}

func applyFile(cfg *Config, file fileConfig) {
	if file.HTTP.Addr != "" {
		cfg.HTTPAddr = file.HTTP.Addr
	}

	if file.MySQL.DSN != "" {
		cfg.MySQLDSN = file.MySQL.DSN
	} else {
		cfg.MySQLDSN = mysqlDSN(file.MySQL)
	}

	applyLoggerFile(&cfg.Log, file.Logger)

	if file.Execution.WorkspaceRoot != "" {
		cfg.WorkspaceRoot = file.Execution.WorkspaceRoot
	}
	if file.Execution.CodexBinary != "" {
		cfg.CodexBinary = file.Execution.CodexBinary
	}
	if file.Execution.ClaudeBinary != "" {
		cfg.ClaudeBinary = file.Execution.ClaudeBinary
	}
}

func applyLoggerFile(cfg *logger.Config, file logger.Config) {
	if file.Service != "" {
		cfg.Service = file.Service
	}
	if file.Level != "" {
		cfg.Level = file.Level
	}
	if file.Format != "" {
		cfg.Format = file.Format
	}
	if file.File != "" {
		cfg.File = file.File
	}
	if file.MaxSizeMB > 0 {
		cfg.MaxSizeMB = file.MaxSizeMB
	}
	if file.MaxBackups > 0 {
		cfg.MaxBackups = file.MaxBackups
	}
	if file.MaxAgeDays > 0 {
		cfg.MaxAgeDays = file.MaxAgeDays
	}
	cfg.Compress = file.Compress
	cfg.Development = file.Development
	cfg.DisableStdout = file.DisableStdout
}

func applyEnv(cfg *Config) {
	cfg.HTTPAddr = envString("ONESHOT_ADDR", cfg.HTTPAddr)

	mysql := mysqlConfig{
		Addr:     os.Getenv("ONESHOT_MYSQL_ADDR"),
		User:     os.Getenv("ONESHOT_MYSQL_USER"),
		Password: os.Getenv("ONESHOT_MYSQL_PASSWORD"),
		Database: os.Getenv("ONESHOT_MYSQL_DATABASE"),
	}
	if dsn := os.Getenv("ONESHOT_MYSQL_DSN"); dsn != "" {
		cfg.MySQLDSN = dsn
	} else if mysql.hasValue() {
		cfg.MySQLDSN = mysqlDSN(mysql)
	}

	cfg.Log.Service = envString("ONESHOT_LOG_SERVICE", cfg.Log.Service)
	cfg.Log.Level = envString("ONESHOT_LOG_LEVEL", cfg.Log.Level)
	cfg.Log.Format = envString("ONESHOT_LOG_FORMAT", cfg.Log.Format)
	cfg.Log.File = envString("ONESHOT_LOG_FILE", cfg.Log.File)
	cfg.Log.MaxSizeMB = envInt("ONESHOT_LOG_MAX_SIZE_MB", cfg.Log.MaxSizeMB)
	cfg.Log.MaxBackups = envInt("ONESHOT_LOG_MAX_BACKUPS", cfg.Log.MaxBackups)
	cfg.Log.MaxAgeDays = envInt("ONESHOT_LOG_MAX_AGE_DAYS", cfg.Log.MaxAgeDays)
	cfg.Log.Compress = envBool("ONESHOT_LOG_COMPRESS", cfg.Log.Compress)
	cfg.Log.Development = envBool("ONESHOT_LOG_DEVELOPMENT", cfg.Log.Development)
	cfg.Log.DisableStdout = envBool("ONESHOT_LOG_DISABLE_STDOUT", cfg.Log.DisableStdout)

	cfg.WorkspaceRoot = envString("ONESHOT_WORKSPACE_ROOT", cfg.WorkspaceRoot)
	cfg.CodexBinary = envString("ONESHOT_CODEX_BIN", cfg.CodexBinary)
	cfg.ClaudeBinary = envString("ONESHOT_CLAUDE_BIN", cfg.ClaudeBinary)
}

// defaultWorkspaceRoot puts agent workspaces under the user's home directory so
// deliverables survive across runs, falling back to a local ./workspaces dir
// when the home directory cannot be determined.
func defaultWorkspaceRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "workspaces"
	}
	return filepath.Join(home, ".oneshot", "workspaces")
}

func mysqlDSN(mysql mysqlConfig) string {
	if !mysql.hasValue() {
		return ""
	}

	driverCfg := drivermysql.Config{
		User:                 defaultString(mysql.User, "root"),
		Passwd:               mysql.Password,
		Net:                  "tcp",
		Addr:                 defaultString(mysql.Addr, "127.0.0.1:3306"),
		DBName:               mysql.Database,
		AllowNativePasswords: true,
		ParseTime:            true,
		Loc:                  time.Local,
		Params: map[string]string{
			"charset": "utf8mb4",
		},
	}
	return driverCfg.FormatDSN()
}

func (cfg mysqlConfig) hasValue() bool {
	return cfg.Addr != "" || cfg.User != "" || cfg.Password != "" || cfg.Database != ""
}

func envString(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func envInt(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

func envBool(name string, fallback bool) bool {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return b
}
