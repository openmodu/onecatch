package wailstransport

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	desktopservice "github.com/openmodu/oneshot/internal/service/desktop"
	"github.com/openmodu/oneshot/internal/usecase/agentrun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const WhiteboardRuntimeEventName = "whiteboard:runtime-event"

type WhiteboardRuntimeEvent struct {
	RequestID string                           `json:"requestId"`
	Seq       uint64                           `json:"seq"`
	Kind      string                           `json:"kind"`
	Text      string                           `json:"text,omitempty"`
	Failed    bool                             `json:"failed,omitempty"`
	At        string                           `json:"at"`
	Change    *desktopservice.WhiteboardChange `json:"change,omitempty"`
}

type WhiteboardBinding struct {
	service           *desktopservice.Service
	applicationSource func() *application.App
}

func NewWhiteboardBinding(service *desktopservice.Service, applicationSource func() *application.App) *WhiteboardBinding {
	return &WhiteboardBinding{service: service, applicationSource: applicationSource}
}

func (b *WhiteboardBinding) ProposeChanges(input desktopservice.WhiteboardProposalInput) (desktopservice.WhiteboardProposal, error) {
	var seq atomic.Uint64
	changeStream := newWhiteboardChangeStream()
	emit := func(event agentrun.Event) {
		if b.applicationSource == nil || b.applicationSource() == nil {
			return
		}
		frame, ok := whiteboardRuntimeEvent(input.RequestID, seq.Add(1), event)
		if !ok {
			// Message deltas still feed the change stream below even when they do
			// not have a standalone progress frame.
		} else {
			b.applicationSource().Event.Emit(WhiteboardRuntimeEventName, frame)
		}
		for _, change := range changeStream.Push(event) {
			change := change
			b.applicationSource().Event.Emit(WhiteboardRuntimeEventName, WhiteboardRuntimeEvent{
				RequestID: strings.TrimSpace(input.RequestID),
				Seq:       seq.Add(1),
				Kind:      "canvas_change",
				Text:      whiteboardChangeOperation(change),
				At:        event.At.Format(time.RFC3339Nano),
				Change:    &change,
			})
		}
	}
	return b.service.ProposeWhiteboardChangesWithSink(input, emit)
}

func (b *WhiteboardBinding) Cancel(requestID string) {
	b.service.CancelWhiteboardRequest(requestID)
}

func whiteboardRuntimeEvent(requestID string, seq uint64, event agentrun.Event) (WhiteboardRuntimeEvent, bool) {
	frame := WhiteboardRuntimeEvent{RequestID: strings.TrimSpace(requestID), Seq: seq, Kind: string(event.Kind), Failed: event.Failed, At: event.At.Format(time.RFC3339Nano)}
	switch event.Kind {
	case agentrun.KindStarted:
		frame.Text = "Agent 已进入白板会话"
	case agentrun.KindReasoning:
		frame.Text = "正在分析画布结构和工作区上下文"
	case agentrun.KindMessage:
		if event.Phase == agentrun.StreamDelta {
			return WhiteboardRuntimeEvent{}, false
		}
		frame.Text = "正在生成可执行的白板操作"
	case agentrun.KindToolUse, agentrun.KindToolResult, agentrun.KindFileChange, agentrun.KindError:
		frame.Text = compactWhiteboardEventText(event.Text)
	default:
		return WhiteboardRuntimeEvent{}, false
	}
	return frame, true
}

type whiteboardChangeStream struct {
	buffers map[string]string
	emitted map[string]bool
}

func newWhiteboardChangeStream() *whiteboardChangeStream {
	return &whiteboardChangeStream{buffers: make(map[string]string), emitted: make(map[string]bool)}
}

// Push extracts complete change objects from a still-growing JSON response.
// The scanner understands JSON string escaping, so braces in card content do
// not make an incomplete object look complete.
func (s *whiteboardChangeStream) Push(event agentrun.Event) []desktopservice.WhiteboardChange {
	if event.Kind != agentrun.KindMessage {
		return nil
	}
	streamID := strings.TrimSpace(event.StreamID)
	if streamID == "" {
		streamID = "message"
	}
	switch event.Phase {
	case agentrun.StreamStart:
		s.buffers[streamID] = ""
	case agentrun.StreamDelta:
		s.buffers[streamID] += event.Text
	case agentrun.StreamSnapshot, agentrun.StreamEnd:
		s.buffers[streamID] = event.Text
	default:
		s.buffers[streamID] = event.Text
	}
	const maxMessageBytes = 1024 * 1024
	if len(s.buffers[streamID]) > maxMessageBytes {
		delete(s.buffers, streamID)
		return nil
	}
	objects := completedWhiteboardChangeObjects(s.buffers[streamID])
	changes := make([]desktopservice.WhiteboardChange, 0, len(objects))
	for index, object := range objects {
		var change desktopservice.WhiteboardChange
		if json.Unmarshal([]byte(object), &change) != nil {
			continue
		}
		change = desktopservice.NormalizeWhiteboardChange(change, index)
		if s.emitted[change.ID] {
			continue
		}
		s.emitted[change.ID] = true
		changes = append(changes, change)
	}
	return changes
}

func completedWhiteboardChangeObjects(message string) []string {
	key := strings.Index(message, `"changes"`)
	if key < 0 {
		return nil
	}
	arrayOffset := strings.Index(message[key+len(`"changes"`):], "[")
	if arrayOffset < 0 {
		return nil
	}
	arrayStart := key + len(`"changes"`) + arrayOffset
	depth, objectStart := 0, -1
	inString, escaped := false, false
	var objects []string
	for index := arrayStart + 1; index < len(message); index++ {
		character := message[index]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' {
				escaped = true
			} else if character == '"' {
				inString = false
			}
			continue
		}
		switch character {
		case '"':
			inString = true
		case '{':
			if depth == 0 {
				objectStart = index
			}
			depth++
		case '}':
			if depth == 0 {
				continue
			}
			depth--
			if depth == 0 && objectStart >= 0 {
				objects = append(objects, message[objectStart:index+1])
				objectStart = -1
			}
		case ']':
			if depth == 0 {
				return objects
			}
		}
	}
	return objects
}

func whiteboardChangeOperation(change desktopservice.WhiteboardChange) string {
	verb := "新增"
	if change.Action == "update" {
		verb = "更新"
	} else if change.Action == "link" {
		verb = "连接"
	}
	return fmt.Sprintf("%s：%s", verb, change.Title)
}

func compactWhiteboardEventText(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	const maxRunes = 180
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "…"
}
