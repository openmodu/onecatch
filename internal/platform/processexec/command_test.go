package processexec

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestChild(t *testing.T) {
	switch os.Getenv("ONECATCH_PROCESS_TEST") {
	case "echo":
		os.Stdout.WriteString("stdout")
		os.Stderr.WriteString("stderr")
		os.Exit(0)
	case "wait":
		time.Sleep(time.Minute)
		os.Exit(0)
	}
}

func TestBackgroundStreams(t *testing.T) {
	cmd := CommandContext(context.Background(), os.Args[0], "-test.run=^TestChild$")
	cmd.Env = append(os.Environ(), "ONECATCH_PROCESS_TEST=echo")
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "stdout") || !strings.Contains(string(output), "stderr") {
		t.Fatalf("output=%q err=%v", output, err)
	}
}

func TestBackgroundCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cmd := CommandContext(ctx, os.Args[0], "-test.run=^TestChild$")
	cmd.Env = append(os.Environ(), "ONECATCH_PROCESS_TEST=wait")
	if err := cmd.Run(); err == nil || ctx.Err() == nil {
		t.Fatalf("child not cancelled: %v", err)
	}
}
