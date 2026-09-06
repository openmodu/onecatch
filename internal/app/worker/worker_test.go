package worker

import (
	"path/filepath"
	"testing"
)

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
