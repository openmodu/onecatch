package keeldesktop

import (
	"strings"

	"github.com/openmodu/onecatch/internal/service/worker"
)

func workerLabel(w worker.Info) string {
	name := w.Name
	if name == "" {
		name = w.ID
	}
	if name == "volc sandbox" || name == "Volcengine Sandbox" || strings.HasPrefix(name, "Volcengine Sandbox ·") {
		return "codex volc_sandbox"
	}
	return name
}

func workerSupportsHarness(w worker.Info, harness string) bool {
	return w.Enabled && (w.Provider != worker.ProviderVolcengineSandbox || harness == "codex")
}

func (a *app) remoteConversation() bool { return a.workerID != "" && a.workerID != "local" }

func (a *app) conversationWorkerReady() bool {
	if !a.remoteConversation() {
		return true
	}
	w, found := a.selectedWorker()
	return found && a.settings.Experimental.RemoteWorkersEnabled && a.workspaceForID(a.workspaceID).RemoteFS == nil && workerSupportsHarness(w, a.runtime.Value()) && len(a.attachments.Items) == 0
}

func (a *app) selectedWorker() (worker.Info, bool) {
	for _, w := range a.workers {
		if w.ID == a.workerID {
			return w, true
		}
	}
	return worker.Info{}, false
}

// Execution targets are immutable once a task exists. Remote FS and local
// attachments stay on the local Agent path, as in the original composer.
func (a *app) selectConversationWorker(id, harness string) bool {
	if a.task != nil || a.busy || a.loading || !a.settings.Experimental.RemoteWorkersEnabled || a.workspaceForID(a.workspaceID).RemoteFS != nil || len(a.attachments.Items) > 0 || a.attachments.Loading || a.attachments.Picking {
		return false
	}
	for _, w := range a.workers {
		if w.ID != id || !workerSupportsHarness(w, harness) {
			continue
		}
		for _, r := range a.runtimes {
			if r.ID == harness && r.Enabled && r.Available {
				a.workerID = id
				a.workflow.SetValue("single_agent")
				a.runtime.SetValue(harness)
				a.resetProfileSelection()
				a.workbench.WorktreeMode, a.workbench.Worktree = "project", nil
				a.workbench.WorktreeVersion++
				a.buildWorktreeMenu()
				a.loadContextGit()
				a.updateWorkerSandbox()
				if a.ui.Executor != nil {
					a.ui.Executor.SetValue(false)
				}
				return true
			}
		}
	}
	return false
}

func (a *app) selectLocalConversation(harness, workflow string) {
	if a.task != nil || a.busy || a.loading {
		return
	}
	if a.remoteConversation() {
		a.resetProfileSelection()
	} else if a.runtime.Value() != harness {
		a.resetProfileSelection()
	}
	a.workerID = ""
	a.runtime.SetValue(harness)
	a.workflow.SetValue(workflow)
	a.updateWorkerSandbox()
	if a.ui.Executor != nil {
		a.ui.Executor.SetValue(false)
	}
}

func (a *app) updateWorkerSandbox() {
	a.sandbox.SetOptionDisabled("full", !a.settings.Security.AllowFullSandbox || a.remoteConversation())
	if a.remoteConversation() && a.sandbox.Value() == "full" {
		a.sandbox.SetValue("workspace-write")
	}
}

func (a *app) conversationLabel(local string) string {
	if !a.remoteConversation() {
		return local
	}
	w, ok := a.selectedWorker()
	if !ok {
		return local + " · " + a.workerID
	}
	if w.Provider == worker.ProviderVolcengineSandbox {
		return workerLabel(w)
	}
	return local + " · " + workerLabel(w)
}
