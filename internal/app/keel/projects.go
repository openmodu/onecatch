package keeldesktop

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"github.com/openmodu/onecatch/internal/app/keel/filepicker"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
)

type projectState struct {
	ID, Host, Username, SSHOptions, Password string
	Kind, Access                             *kit.SelectView
	Auto                                     *filledToggle
	Menus                                    map[string]*kit.MenuView
	Version                                  uint64
	Saving, Picking                          bool
	Error                                    string
}

func (a *app) initProjects() {
	a.project.Kind = kit.Select("").Hint("项目位置").Appearance(false).Size(32)
	a.project.Kind.SetEntries(kit.SelectOption{Value: "local", Label: "本地目录"}, kit.SelectOption{Value: "remote", Label: "远端文件系统"})
	a.project.Access = kit.Select("").Hint("工作区访问").Appearance(false).Size(32)
	a.project.Access.SetEntries(kit.SelectOption{Value: "read-only", Label: "只读访问"}, kit.SelectOption{Value: "workspace-write", Label: "工作区访问"}, kit.SelectOption{Value: "full", Label: "完全访问"})
	a.project.Kind.SetValue("local")
	a.project.Access.SetValue("workspace-write")
	a.project.Auto = newFilledToggle("新任务默认创建 Worktree", false, true)
	a.project.Menus = map[string]*kit.MenuView{}
}
func (a *app) workspaceForID(id string) domainworkspaces.Workspace {
	if strings.HasPrefix(id, "task:") || strings.HasPrefix(id, "worktree:") {
		id = a.workspaceID
	}
	for _, w := range a.workspaces {
		if w.ID == id {
			return w
		}
	}
	return domainworkspaces.Workspace{}
}
func (a *app) newProject() {
	if a.project.Saving {
		return
	}
	a.project.Version++
	a.project.ID, a.project.Host, a.project.Username, a.project.SSHOptions, a.project.Error = "", "", "", "", ""
	a.projectPath, a.projectName = "", ""
	a.project.Password = ""
	a.project.Kind.SetValue("local")
	a.project.Access.SetValue("workspace-write")
	a.project.Auto.SetValue(false)
	a.projectDialog.SetTitle("添加项目")
	a.projectDialog.SetValue(true)
}
func (a *app) editProject(id string) {
	if a.project.Saving {
		return
	}
	w := a.workspaceForID(id)
	if w.ID == "" {
		a.newProject()
		return
	}
	a.project.Version++
	a.project.ID, a.projectPath, a.projectName, a.project.Error = w.ID, w.Path, w.Name, ""
	a.project.Host, a.project.Username, a.project.SSHOptions = "", "", ""
	a.project.Password = ""
	a.project.Kind.SetValue("local")
	access := w.DefaultSandbox
	if access == "" {
		access = "workspace-write"
	}
	a.project.Access.SetValue(access)
	a.project.Auto.SetValue(w.AutoWorktree)
	if w.RemoteFS != nil {
		a.project.Kind.SetValue("remote")
		a.project.Host = w.RemoteFS.Host
		a.project.Username = w.RemoteFS.Username
		a.project.SSHOptions = strings.Join(w.RemoteFS.SSHOptions, "\n")
	}
	a.projectDialog.SetTitle("编辑项目")
	a.projectDialog.SetValue(true)
}
func (a *app) projectForm(cx *el.Context) el.Element {
	p := &a.project
	p.Kind.SetDisabled(p.Saving || p.ID != "")
	p.Access.SetDisabled(p.Saving)
	p.Access.SetOptionDisabled("full", !a.settings.Security.AllowFullSandbox)
	p.Auto.SetDisabled(p.Saving || p.Kind.Value() == "remote")
	column := el.Div().Gap(12).Child(el.Text("设置工作目录和新任务的默认执行方式。").TextSize(13).TextColor(theme.Muted), p.Kind.Render(cx), field("project-name", "项目名称（可选）", &a.projectName).ReadOnly(p.Saving))
	if p.Kind.Value() == "remote" {
		column.Child(field("project-host", "SSH Host，例如 my-server", &p.Host).ReadOnly(p.Saving), field("project-user", "SSH 用户名（可选）", &p.Username).ReadOnly(p.Saving), field("project-password", "SSH 密码（可选，保存在系统钥匙串）", &p.Password).Password().ReadOnly(p.Saving), field("project-path", "远端目录，例如 /srv/project", &a.projectPath).ReadOnly(p.Saving), el.TextArea().ID("project-ssh-options").Name("SSH 参数").Bind(&p.SSHOptions).Placeholder("SSH 配置参数，每行一个（可选）").AutoGrow(2, 3).TextSize(12).Bg(theme.Subtle).Rounded(6))
	} else {
		column.Child(el.Div().Row().Items(el.Center).Gap(6).Child(field("project-path", "文件夹路径，例如 ~/Code/my-project", &a.projectPath).Grow().W(el.Dp(0)).ReadOnly(p.Saving), iconAction(cx, "project-browse", "选择工作目录", "folder", a.chooseProjectDirectory, false)))
	}
	column.Child(p.Access.Render(cx), p.Auto.Render(cx))
	if p.Error != "" {
		column.Child(el.Text(p.Error).TextSize(12).TextColor(theme.DangerText))
	}
	return column
}
func (a *app) projectInput() (desktopservice.AddWorkspaceInput, error) {
	path := strings.TrimSpace(a.projectPath)
	if path == "" {
		return desktopservice.AddWorkspaceInput{}, fmt.Errorf("请选择工作目录")
	}
	auto := a.project.Auto.Value()
	input := desktopservice.AddWorkspaceInput{Path: path, Name: strings.TrimSpace(a.projectName), DefaultSandbox: a.project.Access.Value(), AutoWorktree: &auto, Password: a.project.Password}
	if a.project.Kind.Value() == "remote" {
		input.RemoteFS = &domainworkspaces.RemoteFS{Host: strings.TrimSpace(a.project.Host), Root: path, Username: strings.TrimSpace(a.project.Username)}
		for _, option := range strings.Split(a.project.SSHOptions, "\n") {
			if value := strings.TrimSpace(option); value != "" {
				input.RemoteFS.SSHOptions = append(input.RemoteFS.SSHOptions, value)
			}
		}
	} else {
		var err error
		input.Path, err = expandProjectPath(path)
		if err != nil {
			return input, err
		}
	}
	return input, nil
}
func (a *app) addWorkspace() {
	if a.project.Saving {
		return
	}
	input, err := a.projectInput()
	if err != nil {
		a.project.Error = err.Error()
		return
	}
	id, version := a.project.ID, a.project.Version
	a.project.Saving, a.project.Error = true, ""
	go func() {
		var workspace domainworkspaces.Workspace
		var err error
		if id == "" {
			workspace, err = a.backend.service.AddWorkspace(a.ctx, input)
		} else {
			workspace, err = a.backend.service.UpdateWorkspace(a.ctx, desktopservice.UpdateWorkspaceInput{ID: id, Path: input.Path, Name: input.Name, DefaultSandbox: input.DefaultSandbox, AutoWorktree: input.AutoWorktree, RemoteFS: input.RemoteFS, Password: input.Password})
		}
		core.Update(func() {
			if a.closed {
				return
			}
			a.project.Saving = false
			if version != a.project.Version {
				return
			}
			if err != nil {
				a.project.Error = err.Error()
				return
			}
			a.project.Password = ""
			a.projectDialog.SetValue(false)
			if id == "" {
				a.newTask(workspace.ID)
			} else {
				for i := range a.workspaces {
					if a.workspaces[i].ID == id {
						a.workspaces[i] = workspace
					}
				}
				a.loadContextGit()
			}
			a.refresh()
		})
	}()
}
func (a *app) projectMenu(w domainworkspaces.Workspace) *kit.MenuView {
	menu := a.project.Menus[w.ID]
	if menu == nil {
		menu = kit.Menu().Width(180).Placement(el.Right, el.Start)
		menu.ContentItem("编辑项目…", "", menuIcon("square-pen", "编辑项目…"), func() { a.editProject(w.ID) }).CheckItem("置顶项目", "", w.Pinned, func(on bool) {
			go func() {
				_, err := a.backend.service.SetWorkspacePinned(a.ctx, w.ID, on)
				core.Update(func() {
					if !a.closed {
						a.fail(err)
						a.refresh()
					}
				})
			}()
		}).Separator().ContentItem("移除项目", "", menuIcon("trash-2", "移除项目"), func() {
			a.confirmation.ConfirmDanger("移除项目？", "项目会从列表中移除，磁盘上的工作目录会保留。", "移除项目", func() {
				go func() {
					err := a.backend.service.RemoveWorkspace(a.ctx, w.ID)
					core.Update(func() {
						if a.closed {
							return
						}
						a.fail(err)
						if err == nil && a.workspaceID == w.ID {
							a.workspaceID = ""
							a.newTask("")
						}
						a.refresh()
					})
				}()
			})
		})
		menu.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
			return iconAction(cx, "project-actions/"+w.ID, "项目操作", "ellipsis", menu.Toggle, menu.Value())
		}))
		a.project.Menus[w.ID] = menu
	}
	menu.SetItemChecked("置顶项目", w.Pinned)
	return menu
}
func (a *app) selectWorktree(mode string, entry *domainworkspaces.Worktree) {
	if a.task != nil || a.busy || a.loading || a.remoteConversation() || entry != nil && entry.Unavailable {
		return
	}
	a.workbench.WorktreeMode = mode
	a.workbench.Worktree = nil
	if entry != nil {
		copy := *entry
		a.workbench.Worktree = &copy
	}
	a.loadContextGit()
	a.buildWorktreeMenu()
}
func (a *app) effectiveWorktreeMode() string {
	if a.workbench.WorktreeMode != "" {
		return a.workbench.WorktreeMode
	}
	if a.workspaceForID(a.workspaceID).AutoWorktree {
		return "new"
	}
	return "project"
}
func (a *app) worktreeLabel() string {
	if a.task != nil && a.task.Worktree != nil {
		return "Worktree"
	}
	switch a.effectiveWorktreeMode() {
	case "new":
		return "新建 Worktree"
	case "existing":
		if a.workbench.Worktree != nil {
			if a.workbench.Worktree.Main {
				return "主工作目录"
			}
			return filepath.Base(a.workbench.Worktree.Root)
		}
	}
	return "项目目录"
}
func (a *app) buildWorktreeMenu() {
	w := &a.workbench
	wasOpen := w.WorktreeMenu != nil && w.WorktreeMenu.Value()
	menu := kit.Menu().Width(300).Placement(el.Top, el.Start).Label("选择工作目录")
	item := func(label, description, mode string, entry *domainworkspaces.Worktree, checked bool) {
		content := el.ViewFunc(func(cx *el.Context) el.Element {
			row := el.Div().Row().Items(el.Center).Gap(10).Child(lucide(cx, "git-fork", 15, theme.Muted), el.Div().Gap(3).Grow().W(el.Dp(0)).Child(el.Text(label).TextSize(12).Medium().MaxLines(1), el.Text(description).TextSize(11).TextColor(theme.Muted).MaxLines(1)))
			if checked {
				row.Child(lucide(cx, "check", 14, theme.Text))
			}
			return row
		})
		menu.ContentItem(label, "", content, func() { a.selectWorktree(mode, entry) })
	}
	workspace := a.workspaceForID(a.workspaceID)
	defaultDescription := "使用项目目录"
	if workspace.AutoWorktree {
		defaultDescription = "每个新任务创建一个 Worktree"
	}
	item("跟随项目设置", defaultDescription, "", nil, w.WorktreeMode == "")
	item("新建 Worktree", "从当前分支创建独立工作目录", "new", nil, w.WorktreeMode == "new")
	menu.SetItemDisabled("新建 Worktree", a.captionGit == nil || a.captionGit.Head == "")
	item("项目目录", "在项目的当前工作目录中执行", "project", nil, w.WorktreeMode == "project")
	menu.ContentItem("项目设置…", "", menuIcon("folder", "项目设置…"), func() { a.editProject(a.workspaceID) }).Separator().Label("已有 Worktree")
	for _, entry := range w.Worktrees {
		e := entry
		label := e.Branch
		if label == "" {
			label = gitBranchLabel(&domainworkspaces.GitSnapshot{Head: e.Head})
		}
		if e.Main {
			label += " · 主目录"
		}
		item(label, e.Path, "existing", &e, w.Worktree != nil && w.Worktree.ContextID == e.ContextID)
		menu.SetItemDisabled(label, e.Unavailable)
	}
	menu.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
		return chip(cx, "worktree-picker", a.worktreeLabel(), "git-fork", func() {
			menu.Toggle()
			if menu.Value() {
				a.loadWorktrees()
			}
		}, false)
	}))
	menu.SetDisabled(a.task != nil || a.busy || a.loading)
	if wasOpen {
		menu.SetValue(true)
	}
	w.WorktreeMenu = menu
}
func (a *app) loadWorktrees() {
	id := a.workspaceID
	a.workbench.WorktreeVersion++
	version := a.workbench.WorktreeVersion
	go func() {
		entries, err := a.backend.service.GitListWorktrees(a.ctx, id)
		core.Update(func() {
			if a.closed || id != a.workspaceID || version != a.workbench.WorktreeVersion {
				return
			}
			a.fail(err)
			a.workbench.Worktrees = entries
			a.buildWorktreeMenu()
		})
	}()
}
func (a *app) createTaskInput(prompt string) desktopservice.CreateTaskInput {
	input := desktopservice.CreateTaskInput{WorkspaceID: a.workspaceID, Prompt: prompt, WorkflowID: a.workflow.Value(), Harness: a.runtime.Value(), Model: strings.TrimSpace(a.model), Sandbox: a.sandbox.Value(), WorktreeMode: a.workbench.WorktreeMode, AttachmentPaths: a.attachmentPaths(), WorkerID: a.workerID}
	if a.remoteConversation() {
		input.WorktreeMode = "project"
	}
	if input.WorktreeMode == "existing" && a.workbench.Worktree != nil {
		input.WorktreeID = a.workbench.Worktree.ContextID
	}
	return input
}
func (a *app) workspaceCaption(cx *el.Context) el.Element {
	workspace := a.workspaceForID(a.workspaceID)
	row := el.Div().WFull().Row().Wrap().Items(el.Center).Gap(6).Px(8)
	if workspace.ID == "" {
		return row.Child(button("选择项目", a.newProject, kit.ButtonGhost).Compact(true).Size(28).Render(cx))
	}
	project := iconButton(workspace.Name, func() { a.editProject(workspace.ID) }, kit.ButtonGhost, kit.IconFolder).Name(workspace.Path).Compact(true).Size(28)
	row.Child(project.Render(cx))
	location := "本地"
	if workspace.RemoteFS != nil {
		location = "远端"
	}
	row.Child(el.Div().Row().Items(el.Center).Gap(5).Px(8).Py(4).Rounded(14).Bg(theme.Subtle).Child(lucide(cx, "hard-drive", 12, theme.Success), el.Text(location).TextSize(12).TextColor(theme.Success)))
	if w, ok := a.selectedWorker(); a.remoteConversation() && ok && w.SyncLocal {
		row.Child(el.Text("按轮同步").TextSize(12).TextColor(theme.Muted))
	}
	snapshot := a.captionGit
	if snapshot == nil || !snapshot.IsRepo {
		return row
	}
	branch := gitBranchLabel(snapshot)
	if a.task == nil && a.effectiveWorktreeMode() == "new" {
		branch = "基于 " + branch
	}
	row.Child(el.Div().W(el.Dp(1)).H(el.Dp(14)).Bg(seamColor), lucide(cx, "git-branch", 13, theme.Muted), el.Text(branch).MaxLines(1).TextSize(12).TextColor(theme.Muted))
	if workspace.RemoteFS == nil && !a.remoteConversation() {
		if a.task == nil {
			a.workbench.WorktreeMenu.SetDisabled(a.busy || a.loading)
			row.Child(a.workbench.WorktreeMenu.Render(cx))
		} else if a.task.Worktree != nil {
			row.Child(lucide(cx, "git-fork", 13, theme.Muted), el.Text(a.worktreeLabel()).TextSize(12).TextColor(theme.Muted))
		}
	}
	if a.task != nil || a.effectiveWorktreeMode() != "new" {
		label := "干净"
		if len(snapshot.Files) > 0 {
			label = fmt.Sprintf("%d 项改动", len(snapshot.Files))
		}
		row.Child(el.Div().Grow(), button(label, func() { a.toggleInspector("review") }, kit.ButtonGhost).Compact(true).Size(26).Render(cx))
	}
	return row
}

func (a *app) chooseProjectDirectory() {
	if a.project.Saving || a.project.Picking || a.project.Kind.Value() != "local" {
		return
	}
	a.project.Picking = true
	version := a.project.Version
	filepicker.Choose(filepicker.Options{Title: "选择 Agent 工作目录", Directory: true}, func(result filepicker.Result) {
		core.Update(func() {
			if a.closed {
				return
			}
			a.project.Picking = false
			if version != a.project.Version || !a.projectDialog.Value() {
				return
			}
			if result.Err != nil {
				a.project.Error = result.Err.Error()
				return
			}
			if len(result.Paths) > 0 {
				a.projectPath = result.Paths[0]
			}
		})
	})
}
