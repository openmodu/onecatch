package worker

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

// NewPairingCode returns a one-time code in XXXX-XXXX form. The alphabet drops
// the characters people misread when copying a code off one screen onto
// another phone keyboard.
func NewPairingCode() (string, error) {
	const alphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	for index := range buffer {
		buffer[index] = alphabet[int(buffer[index])%len(alphabet)]
	}
	return string(buffer[:4]) + "-" + string(buffer[4:]), nil
}

// LoadOrCreateToken reads the worker's persistent bearer token from root,
// creating it on first use. The second result reports whether this call
// created it, which is how a fresh worker knows to offer pairing unasked.
func LoadOrCreateToken(root string) (string, bool, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", false, err
	}
	path := filepath.Join(root, "token")
	value, err := os.ReadFile(path)
	if err == nil {
		token := strings.TrimSpace(string(value))
		if token == "" {
			return "", false, fmt.Errorf("%s is empty", path)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return "", false, err
		}
		return token, false, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", false, err
	}
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", false, err
	}
	token := hex.EncodeToString(buffer)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		// Another process won the race; read back what it wrote.
		return LoadOrCreateToken(root)
	}
	if err != nil {
		return "", false, err
	}
	if _, err := file.WriteString(token + "\n"); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", false, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", false, err
	}
	return token, true, nil
}
