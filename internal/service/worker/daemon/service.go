package workerdaemon

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

const (
	launchdLabel = "app.onecatch.worker"
	systemdUnit  = "onecatch-worker.service"
)

type Config struct {
	Binary   string
	Listen   string
	ID       string
	Name     string
	DataDir  string
	TLSCert  string
	TLSKey   string
	ClientCA string
	// Binaries holds each harness's executable override by runtime id, and is
	// forwarded as one --<id>-binary flag per entry.
	Binaries          map[string]string
	MaxConcurrency    int
	AllowInsecureHTTP bool
	PathEnvironment   string
}

type Result struct {
	ServiceFile string
	LogHint     string
}

func Install(ctx context.Context, config Config) (Result, error) {
	if err := validate(config); err != nil {
		return Result{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Result{}, err
	}
	switch runtime.GOOS {
	case "darwin":
		path := filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")
		logPath := filepath.Join(home, "Library", "Logs", "onecatch-worker.log")
		payload, err := RenderLaunchd(config, logPath)
		if err != nil {
			return Result{}, err
		}
		if err := writeAtomic(path, payload, 0o600); err != nil {
			return Result{}, err
		}
		domain := "gui/" + strconv.Itoa(os.Getuid())
		_ = exec.CommandContext(ctx, "launchctl", "bootout", domain+"/"+launchdLabel).Run()
		if output, err := exec.CommandContext(ctx, "launchctl", "bootstrap", domain, path).CombinedOutput(); err != nil {
			return Result{}, fmt.Errorf("launchctl bootstrap: %w: %s", err, strings.TrimSpace(string(output)))
		}
		return Result{ServiceFile: path, LogHint: "tail -f " + shellQuote(logPath)}, nil
	case "linux":
		path := filepath.Join(home, ".config", "systemd", "user", systemdUnit)
		if err := writeAtomic(path, RenderSystemd(config), 0o600); err != nil {
			return Result{}, err
		}
		if output, err := exec.CommandContext(ctx, "systemctl", "--user", "daemon-reload").CombinedOutput(); err != nil {
			return Result{}, fmt.Errorf("systemctl daemon-reload: %w: %s", err, strings.TrimSpace(string(output)))
		}
		if output, err := exec.CommandContext(ctx, "systemctl", "--user", "enable", "--now", systemdUnit).CombinedOutput(); err != nil {
			return Result{}, fmt.Errorf("systemctl enable: %w: %s", err, strings.TrimSpace(string(output)))
		}
		return Result{ServiceFile: path, LogHint: "journalctl --user -u " + systemdUnit + " -f"}, nil
	default:
		return Result{}, fmt.Errorf("service installation is not supported on %s", runtime.GOOS)
	}
}

func RenderLaunchd(config Config, logPath string) ([]byte, error) {
	if err := validate(config); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	output.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	output.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	output.WriteString("<plist version=\"1.0\">\n<dict>\n")
	plistKeyString(&output, "Label", launchdLabel)
	output.WriteString("  <key>ProgramArguments</key>\n  <array>\n")
	for _, argument := range arguments(config) {
		output.WriteString("    <string>")
		if err := xml.EscapeText(&output, []byte(argument)); err != nil {
			return nil, err
		}
		output.WriteString("</string>\n")
	}
	output.WriteString("  </array>\n")
	if config.PathEnvironment != "" {
		output.WriteString("  <key>EnvironmentVariables</key>\n  <dict>\n")
		plistKeyString(&output, "PATH", config.PathEnvironment)
		output.WriteString("  </dict>\n")
	}
	output.WriteString("  <key>RunAtLoad</key>\n  <true/>\n")
	output.WriteString("  <key>KeepAlive</key>\n  <true/>\n")
	plistKeyString(&output, "StandardOutPath", logPath)
	plistKeyString(&output, "StandardErrorPath", logPath)
	output.WriteString("</dict>\n</plist>\n")
	return output.Bytes(), nil
}

func RenderSystemd(config Config) []byte {
	var output strings.Builder
	output.WriteString("[Unit]\nDescription=OneCatch remote agent worker\nAfter=network-online.target\nWants=network-online.target\n\n")
	output.WriteString("[Service]\nType=simple\n")
	if config.PathEnvironment != "" {
		output.WriteString("Environment=" + systemdQuote("PATH="+config.PathEnvironment) + "\n")
	}
	output.WriteString("ExecStart=")
	for index, argument := range arguments(config) {
		if index > 0 {
			output.WriteByte(' ')
		}
		output.WriteString(systemdQuote(argument))
	}
	output.WriteString("\nRestart=on-failure\nRestartSec=5s\nTimeoutStopSec=35s\nNoNewPrivileges=true\nPrivateTmp=true\n\n")
	output.WriteString("[Install]\nWantedBy=default.target\n")
	return []byte(output.String())
}

func arguments(config Config) []string {
	arguments := []string{
		config.Binary,
		"--listen", config.Listen,
		"--id", config.ID,
		"--name", config.Name,
		"--data-dir", config.DataDir,
		"--max-concurrency", strconv.Itoa(config.MaxConcurrency),
	}
	appendPair := func(flag, value string) {
		if strings.TrimSpace(value) != "" {
			arguments = append(arguments, flag, value)
		}
	}
	appendPair("--tls-cert", config.TLSCert)
	appendPair("--tls-key", config.TLSKey)
	appendPair("--client-ca", config.ClientCA)
	// Sorted so an installed service file is byte-identical between installs
	// with the same configuration.
	for _, id := range slices.Sorted(maps.Keys(config.Binaries)) {
		appendPair("--"+id+"-binary", config.Binaries[id])
	}
	if config.AllowInsecureHTTP {
		arguments = append(arguments, "--allow-insecure-http")
	}
	return arguments
}

func validate(config Config) error {
	if strings.TrimSpace(config.Binary) == "" || !filepath.IsAbs(config.Binary) {
		return errors.New("worker binary path must be absolute")
	}
	if strings.TrimSpace(config.DataDir) == "" || !filepath.IsAbs(config.DataDir) {
		return errors.New("worker data directory must be absolute")
	}
	if (config.TLSCert != "" && !filepath.IsAbs(config.TLSCert)) ||
		(config.TLSKey != "" && !filepath.IsAbs(config.TLSKey)) ||
		(config.ClientCA != "" && !filepath.IsAbs(config.ClientCA)) {
		return errors.New("worker certificate paths must be absolute")
	}
	if strings.TrimSpace(config.Listen) == "" || strings.TrimSpace(config.ID) == "" || strings.TrimSpace(config.Name) == "" {
		return errors.New("worker listen address, id, and name are required")
	}
	return nil
}

func writeAtomic(path string, payload []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".onecatch-service-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func plistKeyString(output *bytes.Buffer, key, value string) {
	output.WriteString("  <key>")
	_ = xml.EscapeText(output, []byte(key))
	output.WriteString("</key>\n  <string>")
	_ = xml.EscapeText(output, []byte(value))
	output.WriteString("</string>\n")
}

func systemdQuote(value string) string {
	return strconv.Quote(strings.ReplaceAll(value, "%", "%%"))
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
