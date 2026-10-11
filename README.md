# OneCatch

**English | [简体中文](README.zh-CN.md)**

OneCatch is a local-first desktop workbench for coding agents. It runs harnesses such as Codex and Claude Code on your machine, turning one-off agent sessions into queued, observable, interruptible, and resumable tasks and multi-agent workflows.

OneCatch does not broker model accounts or require uploading your repository to a OneCatch server. Your workspace stays in its original directory. OneCatch stores tasks, workflows, run events, and logs under `~/.onecatch/` by default. Whether a model request uses the network and what it sends to a provider depend on the harness and its configuration.

> **Project status:** OneCatch is under active development; the latest section in [`CHANGELOG.md`](CHANGELOG.md) defines the release version and notes. Desktop installers target macOS 12+, Windows 10+, and Linux systems with GTK4/WebKitGTK 6.0. Configuration formats and interactions may change between minor releases. The repository contains the Remote Worker server and scheduler, but its desktop UI is preview-only and cannot yet be enabled or managed.

## What OneCatch does

- **Runs agents directly:** Supports Codex, Claude Code, Modu Code, Pi, Grok Build, and DeepSeek Harness. OneCatch discovers CLIs on `PATH`; Modu Code can also use its embedded SDK. Where supported, you can select the executable, model, reasoning effort, and inherited environment variables.
- **Orchestrates workflows:** A workflow is a graph of steps. Dependency edges decide order, and independent steps run concurrently. A checking step can return `rework` along a rework edge: the feedback goes back to that earlier step in its own resumed session, and only the steps in between run again. Steps can be agents (each with its own harness and model) or shell commands such as an E2E suite. You can pause a run at any time, leave a step a message, or rerun from any step.
- **Lets you intervene:** Inspect messages, tool calls, command output, and token usage. Queue instructions for the next turn, or interrupt the current turn and insert a high-priority instruction. Harnesses that support resume reuse their original sessions.
- **Controls access boundaries:** Each task or node can be read-only, workspace-write, or full access. Full access is disabled by default. If enabled, OneCatch still asks for confirmation before each run by default.
- **Manages local workspaces:** Browse and edit files, use a multi-tab split terminal, inspect Git status and diffs, and ask an agent to review uncommitted changes without modifying them.
- **Connects local agents to remote directories:** Remote FS operates on a remote workspace through SSH/SFTP while keeping the harness and model credentials local. It currently supports Codex, Claude Code, and Modu Code. This mode is not yet available from the Windows desktop app.

Workflows can also live in the project as code: put YAML files in `.onecatch/workflows/` and OneCatch reads them, draws the flowchart, follows edits as you save, and points to the line when a file is invalid. `after` lists dependencies, `rework` the upstream steps a step may send work back to, `agent` is `runtime/model`, and `run` makes a command step:

```yaml
name: Design to PR
max_rework: 3
steps:
  design:    { agent: claude/opus, sandbox: read-only, prompt: Propose a design; do not write code }
  implement: { agent: codex, after: design }
  review:    { agent: claude/sonnet, sandbox: read-only, after: implement, rework: [implement, design] }
  e2e:       { run: npm run e2e, after: review, rework: implement }
  pr:        { agent: modu, after: e2e, prompt: Commit and open a pull request }
```

Any workflow in the Settings library can be written out this way with Export to project.

The first launch includes Single Agent, Implement and Review, Design to PR (Opus designs, Codex implements, Sonnet reviews, Modu runs E2E and opens the PR), and Parallel Review. The canvas editor lets you add steps, draw dependency and rework edges, and set each step's harness, model, sandbox and prompt or command.

## Installation and development

The desktop uses Keel v0.1.10 with a Go-rendered interface. Dependencies come from pinned remote modules, including the Keel Gio and typesetting forks in `go.mod`; no local Keel checkout is needed.

Install Go 1.26.1, Git, and a configured Agent CLI or Modu provider. macOS needs Xcode Command Line Tools. Run `go tool keel doctor` to check native graphics libraries on Linux. Node.js is only needed for packaging and release metadata; desktop development does not require npm.

```bash
go mod download
go run ./cmd/keel
go build -trimpath -ldflags='-s -w' -o bin/onecatch ./cmd/app
go test ./...
```

Configure an Agent under Settings → Harness, add a project, then create a task or select a workflow. Data defaults to `~/.onecatch/`. Use `go run ./cmd/keel -data-dir /tmp/onecatch-preview` for a separate data directory.

With Task installed, `task dev:desktop`, `task build`, and `task test` are equivalent shortcuts. The previous Wails desktop/mobile applications and React frontend have been removed.

See [harness integrations](docs/harness-integrations.md) and [data compatibility](docs/data-compatibility.md).

## Local data and security boundaries

OneCatch writes its main persistent data to `~/.onecatch/` by default:

```text
~/.onecatch/
├── workspaces/     Workspace index
├── tasks/          Tasks
├── workflows/      Workflow definitions
├── runs/           Run snapshots and JSONL event streams
├── locks/          Workspace locks
└── logs/           Application logs
```

Local-first means that OneCatch stores its own data and orchestration state on your machine. It does not mean that an agent runs offline. A harness still connects to its model provider according to its own authentication and privacy policy. Before starting a task, make sure the project contents may be sent to that provider.

Task attachments are copied to `.onecatch/attachments/` inside the project. OneCatch attempts to add `.onecatch/` to the project's `.git/info/exclude`. Diagnostic archives are redacted by default: they exclude Worker tokens, environment-variable values, and complete local paths. Including task prompts or raw events requires separate authorization for each export.

## Remote Worker (under development)

A Remote Worker runs harnesses on another machine and sends `workspace-write` changes back to the desktop app as a Git patch. This is different from Remote FS: Remote FS forwards file and command operations to an SSH target, while a Worker moves the harness process, workspace, and model environment to the remote machine.

**The current desktop release has no usable Worker configuration UI.** The following commands are for development, API integration tests, and future feature validation. They do not describe a released user feature. Once enabled, writable remote runs will still have these constraints:

- Both Git worktrees must be clean and at the same `HEAD`.
- The desktop commit must have been pushed to an `origin` accessible to the Worker.
- A run can return at most 24 MiB of tracked changes and non-ignored untracked files.
- Ignored files are not synchronized, and repositories with Git submodules cannot run writable remote tasks.
- `full` access remains local-only.

Build a development Worker and start it on the loopback interface:

```bash
task build:headless
./bin/headless/onecatch worker --pair
```

A non-loopback listener requires TLS through `--tls-cert` and `--tls-key`; add `--client-ca` for mTLS. Use `--allow-insecure-http` only when a trusted tunnel already provides transport security. Install a per-user background service with `--install-service`. Run `./bin/headless/onecatch worker --help` for the complete option list. See [`internal/app/worker/README.md`](internal/app/worker/README.md) for the Worker entry point and [`deploy/onecatch-worker/`](deploy/onecatch-worker/) for launchd and systemd templates.

## Desktop packaging and releases

```bash
node build/scripts/release-info.mjs --check
node build/scripts/package-keel.mjs
```

Version and notes come from `CHANGELOG.md`. The script runs `go tool keel build` and writes release archives to `dist/`: a macOS application ZIP, a portable Windows EXE ZIP, and a Linux tar.gz with an install script. New builds no longer produce DMG, NSIS, AppImage or mobile packages; existing published releases remain unchanged.

macOS uses ad-hoc signing unless `SIGN_IDENTITY` specifies a Developer ID; notarization is a separate step. macOS updates retain pinned-key Ed25519 verification. Portable Windows and Linux packages require manual replacement. A matching version tag triggers native builds, release archives, checksums and the macOS update feed.

## Repository layout

```text
cmd/
└── app/                Unified native desktop, worker, and helper entry point
internal/
├── app/                Desktop, Mobile, and Worker assembly
├── domain/             Domain models and business rules
├── repo/               File, Git, and historical data access
├── usecase/            Agent adapters and workflow use cases
├── service/            Desktop, Mobile, and Worker services
└── transport/          HTTP adapters
clients/onecatch/        OneCatch Go HTTP SDK
pkg/                    General Go packages exposed outside this repository
build/                  Native desktop build and packaging files
deploy/                 Worker launchd and systemd templates
docs/                   Harness and other development documentation
tools/                  Go tool dependency declarations
```

The repository has one root `go.mod`. Packages under `cmd` contain process entry points only. Shared repository code belongs under `internal`; only packages that must be imported by external projects belong under `pkg` or `clients`. The desktop UI lives in `internal/app/keel`. See [`internal/README.md`](internal/README.md) for layer boundaries and placement rules.

## Contributing

Issues and pull requests are welcome. Before submitting a change, run:

```bash
task test
task build:desktop
```

When adding a harness, treat `internal/domain/harnesses/catalog.go` as the capability catalog and update its adapter, configuration inspection, and tests. Do not maintain a second runtime catalog in the UI.

## License

OneCatch is licensed under the [MIT License](LICENSE). Copyright © 2026 OpenModu.
