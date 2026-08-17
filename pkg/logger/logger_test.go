package logger

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func TestNewWritesFile(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "onecatch.log")
	log, err := New(Config{
		Service:       "test",
		File:          logFile,
		DisableStdout: true,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	log.Info("file logger ready", zap.String("component", "logger"))
	Sync(log)

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected log file content")
	}
}

func TestNewRejectsInvalidFormat(t *testing.T) {
	_, err := New(Config{Format: "xml"})
	if err == nil {
		t.Fatal("expected invalid format error")
	}
}

func TestNewRejectsNoOutput(t *testing.T) {
	_, err := New(Config{DisableStdout: true})
	if err == nil {
		t.Fatal("expected no output error")
	}
}
