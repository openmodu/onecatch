package worker

import (
	"crypto/subtle"
	"strings"
	"sync"
	"time"
)

const maxPairingAttempts = 10

type pairingState struct {
	mu            sync.Mutex
	code          string
	expiresAt     time.Time
	attemptsLeft  int
	allowInsecure bool
}

func newPairingState(code string, expiresAt time.Time, allowInsecure bool) *pairingState {
	return &pairingState{
		code:          strings.TrimSpace(code),
		expiresAt:     expiresAt,
		attemptsLeft:  maxPairingAttempts,
		allowInsecure: allowInsecure,
	}
}

func (p *pairingState) consume(code string, now time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.code == "" || p.attemptsLeft <= 0 || now.After(p.expiresAt) {
		return false
	}
	p.attemptsLeft--
	provided := strings.TrimSpace(code)
	if len(provided) != len(p.code) || subtle.ConstantTimeCompare([]byte(provided), []byte(p.code)) != 1 {
		return false
	}
	p.code = ""
	return true
}
