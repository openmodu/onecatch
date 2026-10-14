package keeldesktop

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/kit"
	"github.com/openmodu/onecatch/internal/app/keel/terminalview"
	terminalservice "github.com/openmodu/onecatch/internal/service/terminal"
)

type terminalPane struct {
	ID, Context, Title, Error  string
	Session                    terminalservice.Session
	Screen                     *terminalview.Screen
	Starting, Running, Removed bool
	ExitCode                   int
	Rows, Cols                 int
	ResizeVersion              uint64
}
type terminalTab struct {
	ID   string
	Root *terminalSplit
}
type terminalPending struct {
	Data  [][]byte
	Bytes int
	Exit  *terminalservice.ExitFrame
	Error string
}
type terminalState struct {
	Open, Maximized, SearchOpen, SearchFound bool
	SearchFocus                              bool
	Height, DragY                            float32
	Tabs                                     []*terminalTab
	Panes                                    map[string]*terminalPane
	Sessions                                 map[string]string
	Pending                                  map[string]*terminalPending
	Retired                                  map[string]bool
	Active, Focused, Query, LastQuery        string
	Next                                     uint64
	Creating                                 int
	SplitMenu                                *kit.MenuView
	MoreMenu                                 *kit.MenuView
}

func (a *app) initTerminal() {
	a.terminal = terminalState{Height: 286, SearchFound: true, Panes: map[string]*terminalPane{}, Sessions: map[string]string{}, Pending: map[string]*terminalPending{}, Retired: map[string]bool{}}
	a.buildTerminalSplitMenu()
	a.backend.terminal.SetEmitter(func(name string, payload any) {
		core.Update(func() {
			if !a.closed {
				a.terminalEvent(name, payload)
			}
		})
	})
}
func (a *app) terminalEvent(name string, payload any) {
	s := &a.terminal
	var id string
	var data []byte
	var exit *terminalservice.ExitFrame
	switch frame := payload.(type) {
	case terminalservice.OutputFrame:
		if name != terminalservice.OutputEvent {
			return
		}
		id = frame.SessionID
		var err error
		data, err = base64.StdEncoding.DecodeString(frame.Data)
		if err != nil {
			a.fail(err)
			return
		}
	case terminalservice.ExitFrame:
		if name != terminalservice.ExitEvent {
			return
		}
		id = frame.SessionID
		exit = &frame
	default:
		return
	}
	if p := s.Panes[s.Sessions[id]]; p != nil {
		if len(data) > 0 {
			p.Screen.Write(data)
		}
		if exit != nil {
			p.Starting, p.Running = false, false
			p.ExitCode = exit.ExitCode
			p.Error = exit.Error
			p.Screen.Writable = false
		}
		return
	}
	// The reader can emit output before Create returns. Retain these frames
	// until binding the session, including a very fast process's exit status.
	if s.Creating == 0 || s.Retired[id] {
		return
	}
	pending := s.Pending[id]
	if pending == nil {
		pending = &terminalPending{}
		s.Pending[id] = pending
	}
	if pending.Bytes+len(data) > 4<<20 {
		pending.Error = "终端启动输出超过 4 MB，已停止此终端"
		pending.Data = nil
	} else if pending.Error == "" && len(data) > 0 {
		pending.Data = append(pending.Data, data)
		pending.Bytes += len(data)
	}
	if exit != nil {
		pending.Exit = exit
	}
}
func (a *app) toggleTerminal() {
	if a.terminal.Open {
		a.terminal.Open, a.terminal.Maximized = false, false
		return
	}
	if a.page == "" {
		a.workbench.Visible = false
		a.workbench.Maximized = false
	}
	a.terminal.Open = true
	if len(a.terminal.Tabs) == 0 {
		a.createTerminal("")
	} else {
		a.focusTerminal(a.terminal.Focused)
	}
}
func (a *app) terminalTab() *terminalTab {
	for _, tab := range a.terminal.Tabs {
		if tab.ID == a.terminal.Active {
			return tab
		}
	}
	return nil
}
func (a *app) activeTerminal() *terminalPane { return a.terminal.Panes[a.terminal.Focused] }
func (a *app) focusTerminal(id string) {
	if p := a.terminal.Panes[id]; p != nil {
		a.terminal.Focused = id
		p.Screen.RequestFocus()
	}
}
func (a *app) selectTerminalTab(tab *terminalTab) {
	a.terminal.Active = tab.ID
	ids := tab.Root.panes()
	if len(ids) > 0 {
		a.focusTerminal(ids[0])
	}
	a.terminal.SearchOpen = false
}
func (a *app) createTerminal(direction string) { a.createTerminalCommand(direction, "") }

func (a *app) createTerminalCommand(direction, command string) {
	if a.closed {
		return
	}
	a.terminal.Open = true
	contextID := a.workspaceContext()
	if contextID == "" {
		a.errorText = "请先选择一个项目，再打开终端。"
		return
	}
	s := &a.terminal
	s.Next++
	id := fmt.Sprintf("pane-%d", s.Next)
	parent := a.terminalTab()
	focused := s.Focused
	if direction != "" && (parent == nil || s.Panes[focused] == nil) {
		direction = ""
	}
	p := &terminalPane{ID: id, Context: contextID, Title: "终端", Starting: true, Rows: 24, Cols: 80}
	p.Screen = terminalview.New(func(sessionID string, data []byte) error {
		return a.backend.terminal.Write(sessionID, base64.StdEncoding.EncodeToString(data))
	})
	p.Screen.OnActivate = func() { s.Focused = id }
	p.Screen.OnSearch = a.openTerminalSearch
	p.Screen.OnLink = func(raw string) { a.fail(core.OpenURL(raw)) }
	p.Screen.OnTitle = func(title string) {
		title = strings.TrimSpace(strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return -1
			}
			return r
		}, title))
		r := []rune(title)
		if len(r) > 80 {
			r = r[:80]
		}
		if len(r) > 0 {
			p.Title = string(r)
		}
	}
	p.Screen.OnResize = func(rows, cols int) { p.Rows, p.Cols = rows, cols; a.resizeTerminal(p) }
	p.Screen.OnError = func(err error) {
		if p.Removed || !p.Running && !p.Starting {
			return
		}
		p.Error = err.Error()
		p.Starting, p.Running = false, false
		p.Screen.Writable = false
		if p.Session.ID != "" {
			go a.backend.terminal.CloseSession(p.Session.ID)
		}
	}
	s.Panes[id] = p
	if direction == "" {
		tab := &terminalTab{ID: id, Root: terminalLeaf(id)}
		s.Tabs = append(s.Tabs, tab)
		s.Active = tab.ID
	} else {
		parent.Root.split(focused, id, direction, fmt.Sprintf("split-%d", s.Next))
	}
	s.Focused, s.SearchOpen = id, false
	p.Screen.RequestFocus()
	s.Creating++
	config := a.settings.Terminal
	go func() {
		workspace, err := a.backend.service.GetWorkspace(a.ctx, contextID)
		var session terminalservice.Session
		if err == nil {
			input := terminalservice.CreateInput{Workspace: workspace.Path, RemoteFS: workspace.RemoteFS, Rows: 24, Cols: 80}
			if workspace.RemoteFS == nil {
				input.Shell = config.Shell
				input.Arguments = append([]string(nil), config.Arguments...)
			}
			session, err = a.backend.createTerminal(input)
		}
		core.Update(func() {
			s.Creating--
			if a.closed || p.Removed {
				if session.ID != "" {
					s.Retired[session.ID] = true
					delete(s.Pending, session.ID)
					go a.backend.terminal.CloseSession(session.ID)
				}
				return
			}
			p.Starting = false
			if err != nil {
				p.Error = err.Error()
				p.Screen.Writable = false
				p.Screen.AttachSession("")
				return
			}
			p.Session, p.Running = session, true
			if p.Error != "" {
				p.Running = false
				p.Screen.Writable = false
				s.Retired[session.ID] = true
				delete(s.Pending, session.ID)
				go a.backend.terminal.CloseSession(session.ID)
				return
			}
			s.Sessions[session.ID] = id
			p.Title = filepath.Base(session.Shell)
			if p.Title == "." || p.Title == "" {
				p.Title = "终端"
			}
			p.Screen.AttachSession(session.ID)
			if pending := s.Pending[session.ID]; pending != nil {
				for _, data := range pending.Data {
					p.Screen.Write(data)
				}
				if pending.Exit != nil {
					a.terminalEvent(terminalservice.ExitEvent, *pending.Exit)
				}
				if pending.Error != "" {
					p.Error = pending.Error
					p.Screen.OnError(fmt.Errorf("%s", pending.Error))
				}
				delete(s.Pending, session.ID)
			}
			a.resizeTerminal(p)
			if command != "" && p.Running {
				p.Screen.Paste(command)
			}
		})
	}()
}
func (a *app) resizeTerminal(p *terminalPane) {
	if !p.Running || p.Session.ID == "" || p.Removed {
		return
	}
	p.ResizeVersion++
	version := p.ResizeVersion
	id, rows, cols := p.Session.ID, p.Rows, p.Cols
	time.AfterFunc(140*time.Millisecond, func() {
		core.Update(func() {
			if a.closed || p.Removed || !p.Running || p.ResizeVersion != version {
				return
			}
			go func() { _ = a.backend.terminal.Resize(id, uint16(rows), uint16(cols)) }()
		})
	})
}
func (a *app) disposeTerminal(p *terminalPane) {
	if p == nil || p.Removed {
		return
	}
	p.Removed = true
	p.ResizeVersion++
	p.Screen.Close()
	delete(a.terminal.Panes, p.ID)
	if id := p.Session.ID; id != "" {
		delete(a.terminal.Sessions, id)
		a.terminal.Retired[id] = true
		delete(a.terminal.Pending, id)
		go a.backend.terminal.CloseSession(id)
	}
}
func (a *app) removeTerminalPane(id string) {
	for _, tab := range a.terminal.Tabs {
		for _, pane := range tab.Root.panes() {
			if pane == id {
				a.disposeTerminal(a.terminal.Panes[id])
				tab.Root = tab.Root.remove(id)
				if tab.Root == nil {
					a.closeTerminalTab(tab.ID)
				} else if a.terminal.Focused == id {
					a.focusTerminal(tab.Root.panes()[0])
				}
				return
			}
		}
	}
}
func (a *app) closeTerminalTab(id string) {
	s := &a.terminal
	for i, tab := range s.Tabs {
		if tab.ID != id {
			continue
		}
		for _, pane := range tab.Root.panes() {
			a.disposeTerminal(s.Panes[pane])
		}
		s.Tabs = append(s.Tabs[:i], s.Tabs[i+1:]...)
		if s.Active == id {
			s.Active, s.Focused = "", ""
			if len(s.Tabs) > 0 {
				a.selectTerminalTab(s.Tabs[min(i, len(s.Tabs)-1)])
			}
		}
		break
	}
	if len(s.Tabs) == 0 {
		s.Open, s.Maximized, s.SearchOpen = false, false, false
	}
}
func (a *app) terminateTerminal() {
	p := a.activeTerminal()
	if p != nil && p.Running {
		p.Running = false
		p.Screen.Writable = false
		go a.backend.terminal.CloseSession(p.Session.ID)
	}
}
func (a *app) closeTerminals() {
	a.backend.terminal.SetEmitter(nil)
	for _, p := range a.terminal.Panes {
		a.disposeTerminal(p)
	}
}
func (a *app) openTerminalSearch() {
	a.terminal.SearchOpen = true
	a.terminal.SearchFocus = true
	a.terminal.SearchFound = true
}
func (a *app) searchTerminal(backward bool) {
	if p := a.activeTerminal(); p != nil {
		a.terminal.SearchFound = p.Screen.Search(a.terminal.Query, backward)
	}
}

// closeTerminalPanel disposes every tab and its PTY; toolbar toggling only hides it.
func (a *app) closeTerminalPanel() {
	for len(a.terminal.Tabs) > 0 {
		a.closeTerminalTab(a.terminal.Tabs[0].ID)
	}
	a.terminal.Open, a.terminal.Maximized, a.terminal.SearchOpen = false, false, false
}
