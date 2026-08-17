package auth

import (
	"context"
	"sync"
	"time"

	domainauth "github.com/openmodu/onecatch/internal/domain/auth"
)

// memorySessionStore is the default SessionStore for tests and the in-process
// desktop client. Server deployments inject a persistent store instead.
type memorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]domainauth.Session
}

func newMemorySessionStore() *memorySessionStore {
	return &memorySessionStore{sessions: make(map[string]domainauth.Session)}
}

func (s *memorySessionStore) SaveSession(_ context.Context, session domainauth.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[session.Token] = session
	return nil
}

func (s *memorySessionStore) FindSessionByToken(_ context.Context, token string) (domainauth.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[token]
	if !ok {
		return domainauth.Session{}, domainauth.ErrUnauthenticated
	}
	return session, nil
}

func (s *memorySessionStore) DeleteSessionByToken(_ context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, token)
	return nil
}

func (s *memorySessionStore) DeleteExpiredSessions(_ context.Context, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for token, session := range s.sessions {
		if session.ExpiresAt.Before(now) {
			delete(s.sessions, token)
		}
	}
	return nil
}
