package workerapp

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestWorkerTokenIsCreatedPrivatelyAndReused(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), "state")
	first, created, err := loadOrCreateWorkerToken(stateRoot)
	if err != nil || !created || len(first) != 64 {
		t.Fatalf("first token = %q, created=%v, err=%v", first, created, err)
	}
	stat, err := os.Stat(filepath.Join(stateRoot, "token"))
	if err != nil || stat.Mode().Perm() != 0o600 {
		t.Fatalf("token mode = %v, err=%v", stat.Mode().Perm(), err)
	}
	second, created, err := loadOrCreateWorkerToken(stateRoot)
	if err != nil || created || second != first {
		t.Fatalf("second token = %q, created=%v, err=%v", second, created, err)
	}
}

func TestPairingCodeUsesReadableFixedShape(t *testing.T) {
	code, err := newPairingCode()
	if err != nil || !regexp.MustCompile(`^[23456789A-HJ-NP-Z]{4}-[23456789A-HJ-NP-Z]{4}$`).MatchString(code) {
		t.Fatalf("pairing code = %q, err=%v", code, err)
	}
}

func TestServicePairingRequestIsConsumedOnce(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), "state")
	if err := requestServicePairing(stateRoot); err != nil {
		t.Fatal(err)
	}
	requested, err := consumeServicePairingRequest(stateRoot)
	if err != nil || !requested {
		t.Fatalf("first consume = %v, %v", requested, err)
	}
	requested, err = consumeServicePairingRequest(stateRoot)
	if err != nil || requested {
		t.Fatalf("second consume = %v, %v", requested, err)
	}
}
