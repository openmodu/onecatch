package terminal

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultRows = 24
	defaultCols = 80
)

type terminalProcess interface {
	io.ReadWriteCloser
	Resize(rows, cols uint16) error
	Wait(context.Context) (int, error)
	PID() int
}

type processFactory func(CreateInput) (terminalProcess, string, error)
type eventEmitter func(string, any)

type managedSession struct {
	info    Session
	process terminalProcess
	closing atomic.Bool
}

// Service owns the interactive shell processes used by the desktop terminal.
// It deliberately stays separate from agentrun: agent runtimes keep their
// structured JSON streams while this service provides an explicit manual PTY.
type Service struct {
	mu       sync.RWMutex
	sessions map[string]*managedSession
	factory  processFactory
	emitter  eventEmitter
	nextID   atomic.Uint64
}

func NewService() *Service {
	return newService(startProcess)
}

func newService(factory processFactory) *Service {
	return &Service{sessions: make(map[string]*managedSession), factory: factory}
}

func (s *Service) SetEmitter(emitter eventEmitter) {
	s.mu.Lock()
	s.emitter = emitter
	s.mu.Unlock()
}

func (s *Service) Create(input CreateInput) (Session, error) {
	input.Shell = strings.TrimSpace(input.Shell)
	if strings.ContainsAny(input.Shell, "\r\n\x00") {
		return Session{}, errors.New("terminal shell contains control characters")
	}
	for _, argument := range input.Arguments {
		if strings.ContainsAny(argument, "\r\n\x00") {
			return Session{}, errors.New("terminal argument contains control characters")
		}
	}
	workspace := filepath.Clean(input.Workspace)
	info, err := os.Stat(workspace)
	if err != nil || !info.IsDir() {
		return Session{}, fmt.Errorf("terminal workspace is not an accessible directory: %s", input.Workspace)
	}
	input.Workspace = workspace
	if input.Rows == 0 {
		input.Rows = defaultRows
	}
	if input.Cols == 0 {
		input.Cols = defaultCols
	}

	process, shell, err := s.factory(input)
	if err != nil {
		return Session{}, err
	}
	id := fmt.Sprintf("terminal-%d", s.nextID.Add(1))
	session := &managedSession{
		info: Session{
			ID: id, Workspace: workspace, Shell: shell, PID: process.PID(),
			Rows: input.Rows, Cols: input.Cols, StartedAt: time.Now(),
		},
		process: process,
	}
	s.mu.Lock()
	s.sessions[id] = session
	s.mu.Unlock()
	go s.readUntilExit(session)
	return session.info, nil
}

func (s *Service) Write(sessionID, encoded string) error {
	session, err := s.get(sessionID)
	if err != nil {
		return err
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("decode terminal input: %w", err)
	}
	if len(data) == 0 {
		return nil
	}
	_, err = session.process.Write(data)
	return err
}

func (s *Service) Resize(sessionID string, rows, cols uint16) error {
	if rows == 0 || cols == 0 {
		return errors.New("terminal rows and columns must be greater than zero")
	}
	session, err := s.get(sessionID)
	if err != nil {
		return err
	}
	if err := session.process.Resize(rows, cols); err != nil {
		return err
	}
	s.mu.Lock()
	if current := s.sessions[sessionID]; current != nil {
		current.info.Rows = rows
		current.info.Cols = cols
	}
	s.mu.Unlock()
	return nil
}

func (s *Service) CloseSession(sessionID string) error {
	session, err := s.get(sessionID)
	if err != nil {
		return err
	}
	session.closing.Store(true)
	return session.process.Close()
}

func (s *Service) List() []Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		items = append(items, session.info)
	}
	return items
}

func (s *Service) Close() {
	s.mu.RLock()
	items := make([]*managedSession, 0, len(s.sessions))
	for _, session := range s.sessions {
		items = append(items, session)
	}
	s.mu.RUnlock()
	for _, session := range items {
		session.closing.Store(true)
		_ = session.process.Close()
	}
}

func (s *Service) get(sessionID string) (*managedSession, error) {
	s.mu.RLock()
	session := s.sessions[sessionID]
	s.mu.RUnlock()
	if session == nil {
		return nil, fmt.Errorf("terminal session %q not found", sessionID)
	}
	return session, nil
}

func (s *Service) readUntilExit(session *managedSession) {
	type readMessage struct {
		data []byte
		err  error
	}
	messages := make(chan readMessage, 32)
	go func() {
		buffer := make([]byte, 32*1024)
		for {
			n, err := session.process.Read(buffer)
			if n > 0 {
				chunk := append([]byte(nil), buffer[:n]...)
				messages <- readMessage{data: chunk}
			}
			if err != nil {
				messages <- readMessage{err: err}
				return
			}
		}
	}()

	// A shell can write a character at a time. Batch those reads into one Wails
	// event per animation-scale interval so noisy commands cannot saturate the
	// webview bridge, while keeping the terminal feeling immediate.
	ticker := time.NewTicker(12 * time.Millisecond)
	defer ticker.Stop()
	var pending []byte
	flush := func() {
		if len(pending) == 0 {
			return
		}
		s.emit(OutputEvent, OutputFrame{SessionID: session.info.ID, Data: base64.StdEncoding.EncodeToString(pending)})
		pending = pending[:0]
	}
	var readErr error
readLoop:
	for {
		select {
		case message := <-messages:
			pending = append(pending, message.data...)
			if len(pending) >= 32*1024 {
				flush()
			}
			if message.err != nil {
				flush()
				if !errors.Is(message.err, io.EOF) && !session.closing.Load() {
					readErr = message.err
				}
				break readLoop
			}
		case <-ticker.C:
			flush()
		}
	}
	exitCode, waitErr := session.process.Wait(context.Background())
	s.mu.Lock()
	delete(s.sessions, session.info.ID)
	s.mu.Unlock()

	exit := ExitFrame{SessionID: session.info.ID, ExitCode: exitCode}
	if readErr != nil {
		exit.Error = readErr.Error()
	} else if waitErr != nil && !session.closing.Load() {
		exit.Error = waitErr.Error()
	}
	s.emit(ExitEvent, exit)
}

func (s *Service) emit(name string, payload any) {
	s.mu.RLock()
	emitter := s.emitter
	s.mu.RUnlock()
	if emitter != nil {
		emitter(name, payload)
	}
}
