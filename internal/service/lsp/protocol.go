package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

const maxMessageBytes = 64 << 20

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcResult struct {
	message rpcMessage
	err     error
}

type rpcClient struct {
	reader *bufio.Reader
	writer io.Writer

	writeMu          sync.Mutex
	mu               sync.Mutex
	nextID           int64
	pending          map[int64]chan rpcResult
	done             chan struct{}
	err              error
	once             sync.Once
	workspaceFolders []map[string]string
}

func newRPCClient(reader io.Reader, writer io.Writer) *rpcClient {
	client := &rpcClient{
		reader:  bufio.NewReader(reader),
		writer:  writer,
		pending: make(map[int64]chan rpcResult),
		done:    make(chan struct{}),
	}
	go client.readLoop()
	return client
}

func (c *rpcClient) Request(ctx context.Context, method string, params, result any) error {
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return err
	}
	c.nextID++
	id := c.nextID
	response := make(chan rpcResult, 1)
	c.pending[id] = response
	c.mu.Unlock()

	if err := c.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		c.removePending(id)
		return err
	}

	select {
	case received := <-response:
		if received.err != nil {
			return received.err
		}
		if received.message.Error != nil {
			return fmt.Errorf("LSP %s failed (%d): %s", method, received.message.Error.Code, received.message.Error.Message)
		}
		if result == nil || len(received.message.Result) == 0 || string(received.message.Result) == "null" {
			return nil
		}
		if err := json.Unmarshal(received.message.Result, result); err != nil {
			return fmt.Errorf("decode LSP %s result: %w", method, err)
		}
		return nil
	case <-ctx.Done():
		c.removePending(id)
		return ctx.Err()
	case <-c.done:
		c.removePending(id)
		return c.Err()
	}
}

func (c *rpcClient) Notify(method string, params any) error {
	return c.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (c *rpcClient) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *rpcClient) removePending(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *rpcClient) SetWorkspaceFolders(folders []map[string]string) {
	c.mu.Lock()
	c.workspaceFolders = append([]map[string]string(nil), folders...)
	c.mu.Unlock()
}

func (c *rpcClient) write(message any) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := fmt.Fprintf(c.writer, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
		return err
	}
	_, err = c.writer.Write(payload)
	return err
}

func (c *rpcClient) readLoop() {
	for {
		payload, err := readFrame(c.reader)
		if err != nil {
			c.fail(err)
			return
		}
		var message rpcMessage
		if err := json.Unmarshal(payload, &message); err != nil {
			c.fail(fmt.Errorf("decode LSP message: %w", err))
			return
		}
		if message.Method != "" {
			if len(message.ID) != 0 && string(message.ID) != "null" {
				c.replyToServer(message)
			}
			continue
		}
		id, err := parseResponseID(message.ID)
		if err != nil {
			continue
		}
		c.mu.Lock()
		response := c.pending[id]
		delete(c.pending, id)
		c.mu.Unlock()
		if response != nil {
			response <- rpcResult{message: message}
		}
	}
}

func (c *rpcClient) replyToServer(message rpcMessage) {
	var result any
	switch message.Method {
	case "workspace/configuration":
		var params struct {
			Items []json.RawMessage `json:"items"`
		}
		_ = json.Unmarshal(message.Params, &params)
		result = make([]any, len(params.Items))
	case "workspace/workspaceFolders":
		c.mu.Lock()
		result = append([]map[string]string(nil), c.workspaceFolders...)
		c.mu.Unlock()
	case "workspace/applyEdit":
		result = map[string]any{"applied": false, "failureReason": "OneCatch does not apply server edits during navigation"}
	default:
		result = nil
	}
	_ = c.write(map[string]any{"jsonrpc": "2.0", "id": message.ID, "result": result})
}

func (c *rpcClient) fail(err error) {
	if errors.Is(err, io.EOF) {
		err = errors.New("language server exited")
	}
	c.once.Do(func() {
		c.mu.Lock()
		c.err = err
		pending := c.pending
		c.pending = make(map[int64]chan rpcResult)
		c.mu.Unlock()
		for _, response := range pending {
			response <- rpcResult{err: err}
		}
		close(c.done)
	})
}

func parseResponseID(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 {
		return 0, errors.New("missing response id")
	}
	var number int64
	if err := json.Unmarshal(raw, &number); err == nil {
		return number, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, err
	}
	return strconv.ParseInt(text, 10, 64)
}

func readFrame(reader *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			continue
		}
		contentLength, err = strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("invalid LSP content length: %w", err)
		}
	}
	if contentLength < 0 || contentLength > maxMessageBytes {
		return nil, fmt.Errorf("invalid LSP content length %d", contentLength)
	}
	payload := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}
