# OneCatch Keel desktop

The native desktop uses Keel v0.2.1 and the pinned remote Gio/typesetting forks in `go.mod`. All interface code is Go; no local framework checkout or patch launcher is required.

## Run

Requires Go 1.26.1+, and on macOS the Xcode Command Line Tools and macOS 14+.
On Linux, use `go tool keel doctor` to check the required Wayland/X11, EGL and
xkbcommon development packages. Windows requires Go.

```sh
go run ./cmd/keel
```

The default data directory is `~/.onecatch/`. Existing projects, tasks, settings, workflows and transcripts use the shared repositories and desktop service. To use separate data:

```sh
go run ./cmd/keel -data-dir /tmp/onecatch-keel-preview
```

The separate data directory isolates projects, runs, settings and Keel UI
preferences. Agent CLI credentials and the shared `~/.onecatch/skills` library
remain global, as in the existing desktop service.

Configure and sign in to a supported Agent CLI normally before starting a task.
The settings window can change its binary path, default model and enabled state.
Finder launches restore the login shell's PATH, as the existing desktop does.
The integrated `worker`, SSH askpass and shell process roles also remain available.

## Implemented

- Full-height sidebar, one vertical divider, neutral light/dark themes, fill-based
  focus and selection, and the OneCatch application icon.
- Add/edit/pin/remove local or SSH Remote FS projects, use the native folder
  chooser, resize/collapse the sidebar and persist navigation entries. Search
  and filter tasks, pin/rename/delete tasks, read conversations and load older
  entries. Deleting an active task is rejected by the shared service.
- Choose the project directory, a new Worktree or an existing Worktree before
  task creation. Git and file operations follow the selected execution context;
  branch mutations are disabled on a task-bound Worktree.
- Docked/resizable/maximized/detached inspector with status, files, Git, review
  and durable events. Git branch switching/creation and Worker Git sources call
  the existing service. Git defaults to the latest read-only step's registered
  Worker; writable steps, Remote FS and bound Worktrees use the local execution
  context. Manual source changes survive stream/poll updates, and late replies
  cannot replace another context. Worker snapshots expose no local branch or
  review actions. The diff view combines staged, working and untracked
  text with line numbers, horizontal scrolling and a file list in wide views.
- Edit workspace files with per-context buffers, 500 ms autosave and Cmd/Ctrl+S.
  Revision checks retain a conflicting draft; reloading a dirty file requires
  confirmation. Directory refreshes do not overwrite open buffers.
- Select an Agent and model in the Agent popover, a workflow and sandbox;
  create and execute a task.
- Connect, edit, test and remove remote Workers in Settings, including sandbox
  URL/path/sync options, one-time pairing and collapsed TLS certificate controls.
  The original Ctrl+Alt+S reveal and deferred enable/save behavior are preserved.
- The settings window uses Keel's standard `Settings`, `SettingPage`,
  `SettingGroup` and `SettingItem` components for navigation, search and form
  layout. Complex Skills, mobile and Worker actions use its custom content
  slots; settings drafts, revision checks and persistence remain application-owned.
  Enabled Workers appear in the Agent menu; remote tasks persist their execution
  target and reject incompatible attachments, Remote FS, full access and local
  Worktrees. Ordinary Worker pairing retains its TLS pin through later edits.
- Host this desktop for the phone client from the original Mobile connection
  settings section. Start/stop, LAN addresses, single-use pairing, countdown,
  certificate fingerprint and copy actions use the existing HTTPS host service.
  Launch restores only a saved opt-in, and closing the client releases its
  listener. Failed state writes query the live host before updating the switch.
- Choose/paste/remove attachments, preview image thumbnails and send files in a
  new task or continuation. Persisted follow-ups are retained even when resume
  fails, so retrying does not duplicate the instruction or attachment copies.
- Keel v0.2.3 native tray: show/hide the main window, create a task, open settings,
  quit, and open the five most recently active sessions across visible projects.
  Running sessions sort first; project names and titles have separate display
  width limits. Closing the main window retains the session, draft and backend
  while the tray is available. Quit from the tray, application menu, Cmd/Ctrl+Q,
  or updater exits the application. If the tray cannot initialize, closing the
  main window retains the previous shutdown behavior. See [tray lifecycle notes](../../docs/keel-tray.md).
- Stream Markdown, code blocks, tool activity and errors; interrupt execution,
  continue a finished conversation, or queue a follow-up while it is running.
- Pending messages appear above the composer with remove and immediate-steering
  actions. Steering promotes the original instruction and attachments instead
  of copying them. A paused run can resume without new text; running tasks show
  Stop when the composer is empty and Send when it contains a message.
- Respond to permission and user-input requests; preserve the backend's
  confirmation requirement for full access.
- Independent workflow window with back/forward history that retains unsaved
  form and raw-source drafts. Create the original Loop/DAG starters, configure
  implementation/review roles, edit arbitrary steps/signals/dependencies,
  validate and save JSON, or delete a definition. The protected `single_agent`
  definition is read-only. Graph references are repaired on step rename/removal.
- Native DAG canvas with draggable nodes, clickable connection ports and Bézier
  edges, dependency removal, automatic layered layout and a node inspector.
  Coordinates persist in the existing workflow JSON. The inspector edits Agent,
  Worker, sandbox, role, instruction and terminal signals; duplicate identifiers
  and signal names do not overwrite another node/transition. Validation reports
  cycles, unknown dependencies and concurrent writes before saving.
- Prompt templates with search, custom local storage, editable materials and
  arguments, nested references and generated-prompt preview/insertion.
- Skills workspace with a searchable list, Markdown/frontmatter preview, create
  and delete confirmations, a rooted resource tree and an explicit-save editor.
  Drafts remain separate from saved snapshots; typing during save is retained.
  Select receiving apps and sync a Skill or the library through the existing
  rsync service. Settings manages built-in paths and custom sync targets.
- Skill debug runs through the existing Modu SDK in a read-only workspace,
  with streamed reasoning/messages/tool output, stop, history and final output.
  Cmd/Ctrl+Enter starts debug when its prompt has focus; it does not submit the
  task conversation behind the Skills page.
- List/sync available Agent usage, account windows, merged daily totals,
  a 53-week heatmap and 14 calendar days.
- Integrated bottom terminal dock with real PTY sessions, tabs, nested horizontal/vertical
  splits, draggable seams, maximize/collapse, search, selection/copy/paste and links.
  A Go VT emulator handles ANSI, wide Unicode, scrollback, alternate screens,
  mouse modes and bracketed paste. Chinese preedit stays local until committed.
  The existing service resolves local/worktree/task/SSH contexts.
- Independent settings window for appearance, Agent paths/models/integration,
  environment variable allowlists, execution, security, terminal configuration
  and storage settings. Settings writes use the repository revision and surface
  conflicts instead of overwriting a newer save.
- Native application/edit menus and shortcuts:
  Cmd/Ctrl+N (new task), Cmd/Ctrl+O (add project), Cmd/Ctrl+Enter (send),
  Cmd/Ctrl+Shift+Enter (insert a correction into the active turn),
  Cmd/Ctrl+, (settings), Cmd/Ctrl+S (save active file), Cmd/Ctrl+` (terminal),
  Cmd/Ctrl+Q (quit).

Agent requests use the selected harness provider. Visual QA history is recorded in [design-qa.md](../../design-qa.md).
Interrupted-run recovery is not automatically triggered on launch, to avoid
altering a run still owned by another process.

## Build and package

```sh
go build -trimpath -ldflags='-s -w' -o bin/onecatch ./cmd/keel
node build/scripts/package-keel.mjs
```

The release script reads CHANGELOG, injects version metadata, and runs the pinned Keel packager. On macOS it creates `bin/keel/OneCatch Keel.app` and a ZIP under `dist/`. `SIGN_IDENTITY` optionally sets the signing identity; notarization remains a separate step.

## Verify

```sh
go test ./internal/app/keel ./cmd/keel
go test -race ./internal/app/keel
go vet ./internal/app/keel ./cmd/keel
```

To render the production views with sample data, set an **absolute** output path:

```sh
ONECATCH_KEEL_PREVIEWS="$PWD/bin/keel-qa" \
  go test ./internal/app/keel -run TestRenderPreview -count=1
```

Use `-run TestRenderDAGPreview` with the same environment variable for DAG
default, connecting, automatic-layout, narrow-window and dark-theme previews.
Use `-run TestRenderMobileAccessPreview` for hosting off/on, pairing expiry,
error, minimum-window and dark-theme previews. These render public sample data;
they do not enable hosting or pair a device in the user's data directory.
Use `-run TestRenderSkillsPreview` for the Skills library, create dialog,
files/debug inspector, sync targets and target settings. Skills integration
checks use temporary libraries and controlled debug services.

Use `-run TestRenderProjectSelectFocus` for focused permissions in the project
dialog at the reference size, minimum window size and in dark mode. This test
renders several real Gio frames, including keyboard focus.
Use `-run TestRenderConversationReference` for the supplied conversation and
expanded tools; the composer and remaining migration modules are still pending.

The render check needs a working graphics driver. Normal headless CI skips it;
backend integration, transcript reconciliation and native Gio input-router
tests still run. Rendered sample-data previews are not a substitute for testing
AppKit window chrome and native dialogs on an unlocked Mac.
