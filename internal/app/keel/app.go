package keeldesktop

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"sync"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/markdown"
	"github.com/dyike/keel/ui/window"
	domainsettings "github.com/openmodu/onecatch/internal/domain/settings"
	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/service/desktop/runstate"
	"github.com/openmodu/onecatch/internal/service/desktop/runstream"
	workerservice "github.com/openmodu/onecatch/internal/service/worker"
)

type app struct {
	ui                                                                                      uiState
	library                                                                                 libraryState
	skills                                                                                  skillWorkspaceState
	workbench                                                                               workbenchState
	terminal                                                                                terminalState
	attachments                                                                             attachmentState
	project                                                                                 projectState
	backend                                                                                 *backend
	ctx                                                                                     context.Context
	cancel                                                                                  context.CancelFunc
	closed                                                                                  bool
	unsubscribe                                                                             func()
	window, settingsWindow                                                                  *window.Window
	logo                                                                                    *kit.ImageView
	dark                                                                                    bool
	workspaces                                                                              []domainworkspaces.Workspace
	tasks                                                                                   map[string][]domaintasks.Task
	runtimes                                                                                []desktopservice.RuntimeInfo
	workers                                                                                 []workerservice.Info
	workerID                                                                                string
	settings                                                                                domainsettings.Settings
	definitions                                                                             []domainworkflows.Definition
	workspaceID, taskID, runID, page                                                        string
	task                                                                                    *domaintasks.Task
	detail                                                                                  *desktopservice.RunDetail
	stateView                                                                               *runstate.View
	transcript                                                                              *transcript
	entries                                                                                 []*entry
	docs                                                                                    map[string]*markdown.Doc
	conversation                                                                            conversationState
	profile                                                                                 runtimeProfileState
	scroller                                                                                *kit.MessageScrollerView
	prompt, query, model, reasoningEffort, serviceTier, projectPath, projectName, errorText string
	busy, loading                                                                           bool
	selection, refreshVersion                                                               uint64
	projectDialog, confirmation                                                             *kit.DialogView
	runtime, sandbox, workflow                                                              *kit.SelectView
	workflowEditor                                                                          *kit.CodeEditorView
	editingWorkflow                                                                         string
	captionGit                                                                              *domainworkspaces.GitSnapshot
	captionContext                                                                          string
	captionVersion                                                                          uint64
	captionLoading                                                                          bool
	answers                                                                                 map[string]string
	pendingResponses                                                                        map[string]bool
	instructionAction                                                                       instructionAction
	// live holds run-stream frames until the next UI frame applies them.
	live liveFrames
	// postUpdate schedules fn on the UI thread; tests replace it.
	postUpdate func(func())
	// rowSigs records each conversation row's content at its last measure,
	// so a token re-measures only the rows it changed.
	rowSigs map[string]string
}

// liveFrames batches run-stream frames. A provider may deliver dozens of
// small deltas between two frames; applying them one by one rebuilt the
// conversation dozens of times per frame.
type liveFrames struct {
	mu        sync.Mutex
	frames    []runstream.Frame
	scheduled bool
}

func newApp(b *backend) *app {
	ctx, cancel := context.WithCancel(context.Background())
	a := &app{backend: b, ctx: ctx, cancel: cancel, tasks: make(map[string][]domaintasks.Task), transcript: newTranscript(), docs: make(map[string]*markdown.Doc), answers: make(map[string]string), pendingResponses: make(map[string]bool)}
	a.runtime = kit.Select("").Hint("选择 Agent").Appearance(false).MenuWidth(220).OnChange(func(id string) {
		a.resetProfileSelection()
	})
	a.sandbox = newAccessSelect()
	a.sandbox.SetValue("workspace-write")
	a.workflow = kit.Select("").Hint("单 Agent 完成").Appearance(false).MenuWidth(260)
	a.workflow.SetValue("single_agent")
	a.workflowEditor = kit.CodeEditor("").Language("json").Name("工作流定义").Fill()
	// The Wails client's round arrow replaces the built-in latest pill.
	a.scroller = kit.MessageScroller(nil, 140, a.message).LatestButton(false)
	a.projectDialog = kit.Dialog("添加项目").Width(440).CloseButton(true).OnClose(func() { a.project.Password = "" }).BeforeCancel(func() bool { return !a.project.Saving && !a.project.Picking }).Body(el.ViewFunc(a.projectForm)).Footer(
		button("取消", func() {
			if !a.project.Saving {
				a.project.Password = ""
				a.projectDialog.SetValue(false)
			}
		}, kit.ButtonGhost),
		button("保存", a.addWorkspace, kit.ButtonPrimary),
	)
	a.confirmation = kit.Dialog("允许完全访问？").Width(420)
	a.initNavigation()
	a.initLibrary()
	a.initSkillWorkspace()
	a.initWorkbench()
	a.initTerminal()
	a.watchWorkspace()
	a.unsubscribe = b.streams.Subscribe(a.receiveFrame)
	b.states.SetEmitter(func(v runstate.View) {
		core.Update(func() {
			if a.closed || a.runID != v.RunID {
				return
			}
			a.stateView = &v
			if a.detail == nil {
				return
			}
			a.detail.Run, a.detail.StepRuns, a.detail.Instructions, a.detail.Active = v.Run, v.StepRuns, v.Instructions, v.Active
			a.followActiveInspectorSource()
			a.transcript.detail(desktopservice.RunDetail{Task: a.detail.Task, Instructions: v.Instructions})
			a.syncMessages()
		})
	})
	b.lists.SetEmitter(func() {
		core.Update(func() {
			if !a.closed {
				a.refresh()
			}
		})
	})
	if b.runtimes != nil {
		b.runtimes.SetRuntimesChanged(func(items []desktopservice.RuntimeInfo) {
			core.Update(func() {
				if !a.closed {
					a.runtimes = items
					a.updatePickers()
					if a.page == "usage" && len(a.library.Usage) == 0 {
						a.loadUsage(false)
					}
				}
			})
		})
	}
	return a
}

func (a *app) close() {
	a.closed = true
	a.backend.service.SetSkillDebugEmitter(nil)
	a.closeTerminals()
	a.clearAttachments()
	a.cancel()
	a.unsubscribe()
	if a.workbench.DetachedWindow != nil {
		a.workbench.DetachedWindow.Close()
	}
	if a.settingsWindow != nil {
		a.settingsWindow.Close()
	}
	if a.ui.WorkflowWindow != nil {
		a.ui.WorkflowWindow.Close()
	}
	_ = a.backend.close()
}

func (a *app) fail(err error) {
	if err != nil && !a.closed {
		a.errorText = err.Error()
	}
}

// UI callbacks only enqueue I/O; background results always re-enter Keel's
// serialized frame queue. Versions discard stale results after navigation.
func (a *app) refresh() {
	a.refreshVersion++
	version := a.refreshVersion
	go func() {
		workspaces, err := a.backend.service.ListWorkspaces(a.ctx)
		tasks := make(map[string][]domaintasks.Task, len(workspaces))
		for _, w := range workspaces {
			if err != nil {
				break
			}
			tasks[w.ID], err = a.backend.service.ListTasks(a.ctx, w.ID)
		}
		definitions, defsErr := a.backend.service.ListDefinitions(a.ctx)
		settings, settingsErr := a.backend.service.GetSettings(a.ctx)
		workers, workersErr := a.backend.service.ListWorkers(a.ctx)
		runtimes := a.backend.service.ListRuntimes()
		core.Update(func() {
			if a.closed || version != a.refreshVersion {
				return
			}
			a.fail(err)
			a.fail(defsErr)
			a.fail(settingsErr)
			a.fail(workersErr)
			if workersErr == nil {
				a.workers = workers
			}
			a.workspaces, a.tasks, a.runtimes, a.definitions, a.settings = workspaces, tasks, runtimes, definitions, settings
			if a.workspaceID == "" && len(workspaces) > 0 {
				a.workspaceID = workspaces[0].ID
				a.ui.Expanded[a.workspaceID] = true
				a.sandbox.SetValue(workspaces[0].DefaultSandbox)
			}
			a.updatePickers()
			if a.captionContext != a.workspaceContext() {
				a.loadContextGit()
			}
			if task, ok := a.cachedTask(a.taskID); ok {
				a.task = &task
				if a.runID == "" && !a.loading && (task.Status == domaintasks.StatusRunning || task.ExecutionMode == domaintasks.ExecutionQueued && task.Status != domaintasks.StatusQueued) {
					a.attachQueuedRun(task.ID)
				}
			}
		})
	}()
}

func (a *app) updatePickers() {
	a.followActiveInspectorSource()
	a.buildExecutorMenu()
	a.updateWorkerSandbox()
	opts := make([]kit.SelectOption, 0, len(a.runtimes))
	for _, r := range a.runtimes {
		name := r.Name
		if !r.Available {
			name += " · 未安装"
		}
		opts = append(opts, kit.SelectOption{Value: r.ID, Label: name, Disabled: !r.Enabled || !r.Available})
	}
	a.runtime.SetEntries(opts...)
	if a.runtime.Value() == "" {
		for _, r := range a.runtimes {
			if r.Enabled && r.Available {
				a.runtime.SetValue(r.ID)
				a.resetProfileSelection()
				break
			}
		}
	}
	flows := make([]kit.SelectOption, 0, len(a.definitions))
	for _, d := range a.definitions {
		flows = append(flows, kit.SelectOption{Value: d.ID, Label: d.Name})
	}
	a.workflow.SetEntries(flows...)
	if a.workflow.Value() == "" {
		a.workflow.SetValue("single_agent")
	}
}

func (a *app) newTask(workspaceID string) {
	before := a.currentLocation()
	a.resetTask(workspaceID)
	a.recordNavigation(before)
}

func (a *app) resetTask(workspaceID string) {
	a.selection++
	if workspaceID != "" {
		a.workspaceID = workspaceID
		a.ui.Expanded[workspaceID] = true
	}
	for _, w := range a.workspaces {
		if w.ID == a.workspaceID && w.DefaultSandbox != "" {
			a.sandbox.SetValue(w.DefaultSandbox)
			break
		}
	}
	a.taskID, a.runID, a.page, a.prompt, a.errorText = "", "", "", "", ""
	a.resetProfileSelection()
	a.workerID = ""
	a.updateWorkerSandbox()
	a.task, a.detail, a.loading = nil, nil, false
	a.clearAttachments()
	a.workbench.WorktreeMode, a.workbench.Worktree = "", nil
	a.workbench.Worktrees = nil
	a.workbench.WorktreeVersion++
	a.buildWorktreeMenu()
	a.stateView = nil
	a.transcript, a.docs = newTranscript(), make(map[string]*markdown.Doc)
	a.conversation = conversationState{}
	a.answers, a.pendingResponses = make(map[string]string), make(map[string]bool)
	a.runtime.SetDisabled(false)
	a.workflow.SetDisabled(false)
	a.sandbox.SetDisabled(false)
	a.syncMessages()
	a.loadContextGit()
}

func (a *app) openTask(task domaintasks.Task) {
	before := a.currentLocation()
	a.resetTask(task.WorkspaceID)
	a.taskID, a.task, a.loading = task.ID, &task, true
	a.workerID = task.WorkerID
	a.updateWorkerSandbox()
	a.recordNavigation(before)
	a.runtime.SetValue(task.Harness)
	a.model, a.reasoningEffort, a.serviceTier = task.Model, task.ReasoningEffort, task.ServiceTier
	a.workflow.SetValue(task.WorkflowID)
	a.sandbox.SetValue(task.Sandbox)
	a.runtime.SetDisabled(true)
	a.workflow.SetDisabled(true)
	a.sandbox.SetDisabled(true)
	a.loadContextGit()
	version := a.selection
	go func() {
		runs, err := a.backend.service.ListRunsByTask(a.ctx, task.ID)
		core.Update(func() {
			if a.closed || version != a.selection {
				return
			}
			a.loading = false
			a.fail(err)
			if len(runs) > 0 {
				a.openRun(runs[0].ID)
			} else {
				a.transcript.detail(desktopservice.RunDetail{Task: task})
				a.syncMessages()
			}
		})
	}()
}

func (a *app) openRun(id string) {
	a.runID, a.loading = id, true
	version := a.selection
	go func() {
		detail, err := a.backend.service.GetRunDetail(a.ctx, id)
		snapshot := a.backend.service.GetRunStreamSnapshot(id)
		core.Update(func() {
			if a.closed || version != a.selection || id != a.runID {
				return
			}
			a.loading = false
			a.fail(err)
			if err != nil {
				return
			}
			a.applyRunDetail(detail, snapshot)
			a.scroller.ScrollToEnd()
		})
	}()
}

// reloadRun reconciles the open run after an action, in the background, as
// the Wails client does: no loading state, the composer stays usable and the
// conversation does not jump. Live frames and run-state pushes have usually
// shown the change already; this only fills what they do not carry.
func (a *app) reloadRun(id string) {
	if id == "" || id != a.runID || a.detail == nil {
		a.openRun(id)
		return
	}
	version := a.selection
	go func() {
		detail, err := a.backend.service.GetRunDetail(a.ctx, id)
		snapshot := a.backend.service.GetRunStreamSnapshot(id)
		core.Update(func() {
			if a.closed || version != a.selection || id != a.runID {
				return
			}
			a.fail(err)
			if err == nil {
				a.applyRunDetail(detail, snapshot)
			}
		})
	}()
}

// applyRunDetail merges a loaded run with the live snapshot taken after it.
func (a *app) applyRunDetail(detail desktopservice.RunDetail, snapshot []runstream.Frame) {
	a.detail = &detail
	if v := a.stateView; v != nil && v.Run.Revision >= detail.Run.Revision {
		a.detail.Run, a.detail.StepRuns, a.detail.Instructions, a.detail.Active = v.Run, v.StepRuns, v.Instructions, v.Active
	}
	a.transcript.detail(*a.detail)
	for _, f := range snapshot {
		a.transcript.frame(f)
	}
	a.syncMessages()
	a.followActiveInspectorSource()
}

// receiveFrame queues a run-stream frame from the publishing goroutine and
// schedules one UI update for everything queued before the next frame, as
// the Wails client batches frames per animation frame.
func (a *app) receiveFrame(f runstream.Frame) {
	q := &a.live
	q.mu.Lock()
	q.frames = append(q.frames, f)
	schedule := !q.scheduled
	q.scheduled = true
	q.mu.Unlock()
	if schedule {
		post := a.postUpdate
		if post == nil {
			post = core.Update
		}
		post(a.applyLiveFrames)
	}
}

// applyLiveFrames merges the queued frames of the open run and rebuilds the
// conversation once.
func (a *app) applyLiveFrames() {
	q := &a.live
	q.mu.Lock()
	frames := q.frames
	q.frames, q.scheduled = nil, false
	q.mu.Unlock()
	if a.closed {
		return
	}
	changed := false
	for _, f := range frames {
		if f.RunID == a.runID && a.transcript.frame(f) {
			changed = true
		}
	}
	if changed {
		a.syncMessages()
	}
}

func (a *app) syncMessages() {
	a.entries = buildConversationEntries(a.transcript.visible())
	keys := make([]string, len(a.entries))
	for i, e := range a.entries {
		keys[i] = e.key
		if e.kind == "message" || e.user {
			doc := a.docs[e.key]
			if doc == nil {
				doc = newConversationDocument(e.text).TableDividers(!a.ui.Preferences.HideTableDividers)
				a.docs[e.key] = doc
			}
			if doc.Source() != e.text {
				doc.SetSource(e.text)
			}
			doc.SetStreaming(e.streaming && a.detail != nil && a.detail.Active)
		}
	}
	a.scroller.SetKeys(keys)
	// Re-measure only rows whose content changed: a streaming token touches
	// one row, and re-measuring every row's Markdown made long runs stutter.
	if a.rowSigs == nil {
		a.rowSigs = map[string]string{}
	}
	active := a.detail != nil && a.detail.Active
	seen := make(map[string]bool, len(a.entries))
	var changed []string
	for i, e := range a.entries {
		sig := rowSignature(e, active, i == len(a.entries)-1)
		seen[e.key] = true
		if a.rowSigs[e.key] != sig {
			a.rowSigs[e.key] = sig
			changed = append(changed, e.key)
		}
	}
	for key := range a.rowSigs {
		if !seen[key] {
			delete(a.rowSigs, key)
		}
	}
	if len(changed) > 0 {
		a.scroller.Invalidate(changed...)
	}
}

// rowSignature identifies what a conversation row draws: its records'
// revisions and text, streaming and outcome flags, and, for process groups
// and streaming messages, whether the run is active and the row is last.
func rowSignature(e *entry, active, last bool) string {
	var b strings.Builder
	var write func(*entry)
	write = func(e *entry) {
		fmt.Fprintf(&b, "%s|%s|%t|%d|%d|%t|%t|%t|", e.key, e.kind, e.user, e.revision, len(e.text), e.streaming, e.failed, e.settled)
		if e.permission != nil {
			b.WriteString("p:" + e.permission.ID + "|")
		}
		if e.question != nil {
			b.WriteString("q:" + e.question.ID + "|")
		}
		for _, d := range e.details {
			write(d)
		}
		b.WriteString(";")
		for _, p := range e.process {
			write(p)
		}
	}
	write(e)
	// Run activity only changes a process group's label and a streaming
	// message's caret; re-estimating every row when a run ends lost the
	// view's place at the bottom.
	if len(e.process) > 0 || e.streaming {
		fmt.Fprintf(&b, "%t|%t", active, last)
	}
	return b.String()
}

func (a *app) send() {
	if a.page == "skills" {
		if a.skills.Visible && a.skills.Tab == "debug" && a.skills.Debug.PromptFocused {
			a.runSkillDebug()
		}
		return
	}
	if a.page != "" {
		return
	}
	a.sendWithMode(false)
}

func (a *app) sendSteering() {
	if a.page == "" {
		a.sendWithMode(true)
	}
}

func (a *app) sendWithMode(steer bool) {
	if a.busy || a.loading || a.closed || a.instructionBusy() || a.attachments.Loading || a.attachments.Picking {
		return
	}
	if a.task != nil && a.task.Status == domaintasks.StatusQueued && a.runID == "" {
		return
	}
	prompt := strings.TrimSpace(a.prompt)
	attachments := a.attachmentPaths()
	if prompt == "" && len(attachments) > 0 {
		prompt = "请查看附件并继续任务。"
	}
	if a.task == nil && prompt == "" {
		return
	}
	if a.workspaceID == "" {
		a.newProject()
		return
	}
	if a.task == nil && a.workflow.Value() == "single_agent" && a.runtime.Value() == "" {
		a.errorText = "请先在设置中配置一个可用的 Agent。"
		return
	}
	version, runID := a.selection, a.runID
	a.busy, a.errorText = true, ""
	if runID != "" {
		if prompt == "" && (a.detail == nil || a.detail.Active || a.detail.Run.Status != domainworkflows.RunPaused) {
			a.busy = false
			return
		}
		input := desktopservice.ResumeRunInput{Instruction: prompt, Model: a.model, ReasoningEffort: a.reasoningEffort, ServiceTier: a.serviceTier, Harness: a.continuationHarness()}
		active := a.detail != nil && a.detail.Active
		if a.detail != nil && !active && a.detail.Run.Status != domainworkflows.RunPaused && a.detail.Run.Status != domainworkflows.RunCompleted {
			a.busy = false
			a.errorText = "此执行已结束，暂不支持继续。请新建任务。"
			return
		}
		go func() {
			persisted, err := submitContinuationMode(a.ctx, a.backend.service, runID, input, attachments, active, steer)
			core.Update(func() {
				if a.closed {
					return
				}
				a.busy = false
				if version != a.selection {
					return
				}
				a.fail(err)
				if err == nil || persisted {
					a.prompt = ""
					a.clearAttachments()
					// Like Wails: follow the conversation to the new message
					// and reconcile quietly; frames and run-state pushes show it.
					a.scroller.ScrollToEnd()
					a.reloadRun(runID)
				}
			})
		}()
		return
	}
	queued := a.task == nil && a.library.Queue
	input := a.createTaskInput(prompt)
	var existing string
	if a.task != nil {
		existing = a.task.ID
	}
	go func() {
		var task domaintasks.Task
		var err error
		if existing == "" {
			task, err = a.backend.service.CreateTask(a.ctx, input)
		} else {
			task.ID = existing
		}
		var preview desktopservice.RunStartPreview
		if err == nil {
			preview, err = a.backend.service.PreviewRun(a.ctx, task.ID)
		}
		core.Update(func() {
			if a.closed {
				return
			}
			a.busy = false
			if version != a.selection {
				return
			}
			a.fail(err)
			if task.ID != "" && existing == "" {
				a.taskID, a.task = task.ID, &task
				a.clearAttachments()
				a.loadContextGit()
				a.transcript.detail(desktopservice.RunDetail{Task: task})
				a.syncMessages()
				a.refresh()
			}
			if err != nil {
				return
			}
			start := func(token string) {
				if queued {
					a.enqueueTask(task.ID, token)
				} else {
					a.start(task.ID, token)
				}
			}
			if preview.RequiresFullSandboxConfirmation {
				a.confirmation.Confirm("允许完全访问？", "此任务将允许 Agent 访问工作区以外的文件并执行命令。", func() { start(preview.ConfirmationToken) })
			} else {
				start("")
			}
		})
	}()
}

func (a *app) start(taskID, token string) {
	a.busy = true
	version := a.selection
	go func() {
		run, err := a.backend.service.StartRunConfirmed(a.ctx, taskID, token)
		core.Update(func() {
			if a.closed {
				return
			}
			a.busy = false
			if version != a.selection {
				return
			}
			a.fail(err)
			if err == nil {
				a.prompt = ""
				a.runtime.SetDisabled(true)
				a.workflow.SetDisabled(true)
				a.sandbox.SetDisabled(true)
				a.openRun(run.ID)
			}
		})
	}()
}

func (a *app) stop() {
	id, version := a.runID, a.selection
	if id == "" || a.busy || a.closed || a.instructionBusy() {
		return
	}
	a.busy = true
	go func() {
		_, err := a.backend.service.InterruptRun(a.ctx, id)
		core.Update(func() {
			if a.closed {
				return
			}
			a.busy = false
			if version != a.selection {
				return
			}
			a.fail(err)
			a.reloadRun(id)
		})
	}()
}

func (a *app) loadFullHistory() {
	id, version := a.runID, a.selection
	go func() {
		events, err := a.backend.service.GetRunTranscript(a.ctx, id)
		core.Update(func() {
			if a.closed || version != a.selection || a.runID != id {
				return
			}
			a.fail(err)
			if err == nil {
				a.transcript.history(events)
				if a.detail != nil {
					a.detail.RuntimeEventsTotal = len(events)
					a.detail.RuntimeEvents = events
				}
				a.syncMessages()
			}
		})
	}()
}

func (a *app) respondPermission(e *entry, decision string) {
	id := e.permission.ID
	if a.pendingResponses[id] {
		return
	}
	a.pendingResponses[id] = true
	runID, version := a.runID, a.selection
	go func() {
		err := a.backend.service.RespondPermission(desktopservice.PermissionDecisionInput{RunID: runID, RequestID: id, Decision: decision})
		core.Update(func() {
			if a.closed || version != a.selection {
				return
			}
			delete(a.pendingResponses, id)
			a.fail(err)
			if err == nil {
				a.transcript.resolvedPermissions[id] = true
				a.syncMessages()
			}
		})
	}()
}

func (a *app) respondQuestion(e *entry, cancelled bool) {
	id := e.question.ID
	if a.pendingResponses[id] {
		return
	}
	a.pendingResponses[id] = true
	answers := make(map[string]string)
	for _, q := range e.question.Questions {
		answers[q.ID] = a.answers[id+"/"+q.ID]
	}
	runID, version := a.runID, a.selection
	go func() {
		err := a.backend.service.RespondUserInput(desktopservice.UserInputResponseInput{RunID: runID, RequestID: id, Answers: answers, Cancelled: cancelled})
		core.Update(func() {
			if a.closed || version != a.selection {
				return
			}
			delete(a.pendingResponses, id)
			a.fail(err)
			if err == nil {
				a.transcript.resolvedQuestions[id] = true
				a.syncMessages()
			}
		})
	}()
}

func (a *app) editWorkflow(d domainworkflows.Definition) {
	a.editingWorkflow = d.ID
	a.ui.WorkflowDraft = domainworkflows.Normalize(d)
	a.ui.WorkflowRaw, a.ui.WorkflowStep = false, 0
	a.ui.WorkflowAdvanced = false
	a.resetWorkflowForm()
	data, _ := json.MarshalIndent(d, "", "  ")
	a.workflowEditor.SetValue(string(data))
	a.workflowEditor.SetReadOnly(d.ID == "single_agent")
}

func (a *app) saveWorkflow() {
	var definition domainworkflows.Definition
	if err := json.Unmarshal([]byte(a.workflowEditor.Value()), &definition); err != nil {
		a.fail(fmt.Errorf("工作流 JSON 无效：%w", err))
		return
	}
	if err := domainworkflows.Validate(definition); err != nil {
		if definition.Mode == domainworkflows.ModeDAG {
			a.validateDAG()
		}
		a.fail(err)
		return
	}
	if a.editingWorkflow == "single_agent" {
		a.fail(fmt.Errorf("内置单 Agent 工作流不可修改"))
		return
	}
	id := a.editingWorkflow
	if a.busy {
		return
	}
	a.busy = true
	go func() {
		var err error
		if id == "" {
			_, err = a.backend.service.CreateDefinition(a.ctx, definition)
		} else {
			_, err = a.backend.service.UpdateDefinition(a.ctx, id, definition)
		}
		core.Update(func() {
			if a.closed {
				return
			}
			a.busy = false
			a.fail(err)
			if err == nil {
				a.editingWorkflow = definition.ID
				a.ui.SelectedWorkflow = definition.ID
				a.ui.WorkflowSource = false
				a.refresh()
			}
		})
	}()
}

func (a *app) saveRuntime(id, binary, model string, enabled bool, done func(error)) {
	// Persist only this runtime against a freshly read revision. Other settings
	// sections (and credentials) remain in the existing repository.
	go func() {
		current, err := a.backend.service.GetSettings(a.ctx)
		if err == nil {
			runtimes := maps.Clone(current.Runtimes)
			r := runtimes[id]
			r.Binary, r.DefaultModel, r.Enabled = strings.TrimSpace(binary), strings.TrimSpace(model), enabled
			runtimes[id] = r
			_, err = a.backend.service.UpdateRuntimeSettings(a.ctx, runtimes, current.Revision)
		}
		if err == nil {
			_, err = a.backend.service.CheckRuntime(id)
		}
		core.Update(func() {
			if a.closed {
				return
			}
			a.fail(err)
			a.refresh()
			done(err)
		})
	}()
}

func (a *app) enqueueTask(id, token string) {
	a.busy = true
	version := a.selection
	go func() {
		task, err := a.backend.service.EnqueueTask(a.ctx, id, token)
		core.Update(func() {
			if a.closed {
				return
			}
			a.busy = false
			a.fail(err)
			if err == nil {
				a.refresh()
				if version == a.selection {
					a.prompt = ""
					a.openTask(task)
				}
			}
		})
	}()
}
