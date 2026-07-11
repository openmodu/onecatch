package logger

import (
	"errors"
	"io"
	"os"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	DefaultLevel         = "info"
	DefaultFormat        = "console"
	DefaultMaxSizeMB     = 100
	DefaultMaxBackups    = 7
	DefaultMaxAgeDays    = 30
	DefaultStacktraceLvl = "error"
)

type Config struct {
	Service       string `yaml:"service"`
	Level         string `yaml:"level"`
	Format        string `yaml:"format"`
	File          string `yaml:"file"`
	MaxSizeMB     int    `yaml:"max_size_mb"`
	MaxBackups    int    `yaml:"max_backups"`
	MaxAgeDays    int    `yaml:"max_age_days"`
	Compress      bool   `yaml:"compress"`
	Development   bool   `yaml:"development"`
	DisableStdout bool   `yaml:"disable_stdout"`
}

// Managed keeps the logger pointer stable while allowing Settings to update
// its level and rotating file destination without restarting the desktop app.
type Managed struct {
	Logger *zap.Logger
	level  zap.AtomicLevel
	file   *switchWriter
	mu     sync.Mutex
}

func NewManaged(cfg Config) (*Managed, error) {
	level, err := parseLevel(defaultString(cfg.Level, DefaultLevel))
	if err != nil {
		return nil, err
	}
	stacktraceLevel, err := parseLevel(DefaultStacktraceLvl)
	if err != nil {
		return nil, err
	}
	cores := make([]zapcore.Core, 0, 2)
	if !cfg.DisableStdout {
		output, err := encoder(defaultString(cfg.Format, DefaultFormat), encoderConfig(cfg.Development))
		if err != nil {
			return nil, err
		}
		cores = append(cores, zapcore.NewCore(output, zapcore.Lock(os.Stdout), level))
	}
	var file *switchWriter
	if cfg.File != "" {
		output, err := encoder(defaultString(cfg.Format, DefaultFormat), encoderConfig(cfg.Development))
		if err != nil {
			return nil, err
		}
		file = &switchWriter{current: rotationWriter(cfg)}
		cores = append(cores, zapcore.NewCore(output, zapcore.AddSync(file), level))
	}
	if len(cores) == 0 {
		return nil, errors.New("logger requires stdout or file output")
	}
	options := []zap.Option{zap.AddCaller(), zap.AddStacktrace(stacktraceLevel)}
	if cfg.Service != "" {
		options = append(options, zap.Fields(zap.String("service", cfg.Service)))
	}
	if cfg.Development {
		options = append(options, zap.Development())
	}
	return &Managed{Logger: zap.New(zapcore.NewTee(cores...), options...), level: level, file: file}, nil
}

func (m *Managed) Reconfigure(cfg Config) error {
	level, err := parseLevel(defaultString(cfg.Level, DefaultLevel))
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.level.SetLevel(level.Level())
	if m.file != nil && cfg.File != "" {
		m.file.replace(rotationWriter(cfg))
	}
	return nil
}

type switchWriter struct {
	mu      sync.Mutex
	current io.WriteCloser
}

func (w *switchWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.current.Write(data)
}
func (w *switchWriter) Sync() error { return nil }
func (w *switchWriter) replace(next io.WriteCloser) {
	w.mu.Lock()
	previous := w.current
	w.current = next
	w.mu.Unlock()
	_ = previous.Close()
}

func New(cfg Config) (*zap.Logger, error) {
	level, err := parseLevel(defaultString(cfg.Level, DefaultLevel))
	if err != nil {
		return nil, err
	}
	stacktraceLevel, err := parseLevel(DefaultStacktraceLvl)
	if err != nil {
		return nil, err
	}

	cores := make([]zapcore.Core, 0, 2)
	if !cfg.DisableStdout {
		stdoutEncoder, err := encoder(defaultString(cfg.Format, DefaultFormat), encoderConfig(cfg.Development))
		if err != nil {
			return nil, err
		}
		cores = append(cores, zapcore.NewCore(stdoutEncoder, zapcore.Lock(os.Stdout), level))
	}
	if cfg.File != "" {
		fileEncoder, err := encoder(defaultString(cfg.Format, DefaultFormat), encoderConfig(cfg.Development))
		if err != nil {
			return nil, err
		}
		cores = append(cores, zapcore.NewCore(fileEncoder, zapcore.AddSync(rotationWriter(cfg)), level))
	}
	if len(cores) == 0 {
		return nil, errors.New("logger requires stdout or file output")
	}

	options := []zap.Option{
		zap.AddCaller(),
		zap.AddStacktrace(stacktraceLevel),
	}
	if cfg.Service != "" {
		options = append(options, zap.Fields(zap.String("service", cfg.Service)))
	}
	if cfg.Development {
		options = append(options, zap.Development())
	}

	return zap.New(zapcore.NewTee(cores...), options...), nil
}

func MustNew(cfg Config) *zap.Logger {
	log, err := New(cfg)
	if err != nil {
		panic(err)
	}
	return log
}

func Sync(log *zap.Logger) {
	if log == nil {
		return
	}
	_ = log.Sync()
}

func encoderConfig(development bool) zapcore.EncoderConfig {
	if development {
		cfg := zap.NewDevelopmentEncoderConfig()
		cfg.EncodeTime = zapcore.ISO8601TimeEncoder
		return cfg
	}

	cfg := zap.NewProductionEncoderConfig()
	cfg.EncodeTime = zapcore.ISO8601TimeEncoder
	return cfg
}

func encoder(format string, cfg zapcore.EncoderConfig) (zapcore.Encoder, error) {
	switch strings.ToLower(format) {
	case "", "console":
		return zapcore.NewConsoleEncoder(cfg), nil
	case "json":
		return zapcore.NewJSONEncoder(cfg), nil
	default:
		return nil, errors.New("logger format must be console or json")
	}
}

func parseLevel(value string) (zap.AtomicLevel, error) {
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(strings.ToLower(value))); err != nil {
		return zap.AtomicLevel{}, err
	}
	return zap.NewAtomicLevelAt(level), nil
}

func rotationWriter(cfg Config) *lumberjack.Logger {
	maxSize := cfg.MaxSizeMB
	if maxSize <= 0 {
		maxSize = DefaultMaxSizeMB
	}
	maxBackups := cfg.MaxBackups
	if maxBackups <= 0 {
		maxBackups = DefaultMaxBackups
	}
	maxAge := cfg.MaxAgeDays
	if maxAge <= 0 {
		maxAge = DefaultMaxAgeDays
	}

	return &lumberjack.Logger{
		Filename:   cfg.File,
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		MaxAge:     maxAge,
		Compress:   cfg.Compress,
	}
}

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
