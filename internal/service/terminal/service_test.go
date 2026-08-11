package terminal

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"sync"
	"testing"
	"time"
)

type fakeProcess struct {
	mu        sync.Mutex
	input     bytes.Buffer
	output    chan []byte
	closed    chan struct{}
	closeOnce sync.Once
	rows      uint16
	cols      uint16
}

func newFakeProcess() *fakeProcess {
	return &fakeProcess{output: make(chan []byte, 2), closed: make(chan struct{})}
}

func (p *fakeProcess) Read(buffer []byte) (int, error) {
	select {
	case data := <-p.output:
		return copy(buffer, data), nil
	case <-p.closed:
		return 0, io.EOF
	}
}
func (p *fakeProcess) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.input.Write(data)
}
func (p *fakeProcess) Resize(rows, cols uint16) error {
	p.mu.Lock()
	p.rows, p.cols = rows, cols
	p.mu.Unlock()
	return nil
}
func (p *fakeProcess) Wait(context.Context) (int, error) { return 0, nil }
func (p *fakeProcess) PID() int                          { return 42 }
func (p *fakeProcess) Close() error {
	p.closeOnce.Do(func() { close(p.closed) })
	return nil
}

func TestServiceStreamsRawBytesAndControlsSession(t *testing.T) {
	process := newFakeProcess()
	service := newService(func(input CreateInput) (terminalProcess, string, error) {
		if input.Rows != defaultRows || input.Cols != defaultCols {
			t.Fatalf("default size = %dx%d", input.Rows, input.Cols)
		}
		return process, "zsh", nil
	})
	events := make(chan any, 4)
	service.SetEmitter(func(_ string, payload any) { events <- payload })

	session, err := service.Create(CreateInput{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if session.Shell != "zsh" || session.PID != 42 {
		t.Fatalf("session = %+v", session)
	}

	input := []byte("echo 你好\r")
	if err := service.Write(session.ID, base64.StdEncoding.EncodeToString(input)); err != nil {
		t.Fatal(err)
	}
	if err := service.Resize(session.ID, 31, 112); err != nil {
		t.Fatal(err)
	}
	process.output <- []byte{0x1b, '[', '3', '1', 'm', 0xe4, 0xbd, 0xa0}

	select {
	case value := <-events:
		frame, ok := value.(OutputFrame)
		if !ok {
			t.Fatalf("event = %T", value)
		}
		data, decodeErr := base64.StdEncoding.DecodeString(frame.Data)
		if decodeErr != nil || !bytes.Equal(data, []byte{0x1b, '[', '3', '1', 'm', 0xe4, 0xbd, 0xa0}) {
			t.Fatalf("output = %v, %v", data, decodeErr)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for output")
	}

	process.mu.Lock()
	if !bytes.Equal(process.input.Bytes(), input) || process.rows != 31 || process.cols != 112 {
		t.Fatalf("input=%q size=%dx%d", process.input.Bytes(), process.rows, process.cols)
	}
	process.mu.Unlock()
	if err := service.CloseSession(session.ID); err != nil {
		t.Fatal(err)
	}
}

func TestServiceRejectsMissingWorkspace(t *testing.T) {
	service := newService(func(CreateInput) (terminalProcess, string, error) {
		t.Fatal("factory should not be called")
		return nil, "", nil
	})
	if _, err := service.Create(CreateInput{Workspace: t.TempDir() + "/missing"}); err == nil {
		t.Fatal("expected missing workspace error")
	}
}

func TestServiceRejectsControlCharactersInShellConfiguration(t *testing.T) {
	service := newService(func(CreateInput) (terminalProcess, string, error) {
		t.Fatal("factory should not be called")
		return nil, "", nil
	})
	workspace := t.TempDir()
	for _, input := range []CreateInput{
		{Workspace: workspace, Shell: "zsh\nmalformed"},
		{Workspace: workspace, Arguments: []string{"-l\x00hidden"}},
	} {
		if _, err := service.Create(input); err == nil {
			t.Fatalf("expected input to be rejected: %+v", input)
		}
	}
}
