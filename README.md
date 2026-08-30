# OneCatch

**English | [简体中文](README.zh-CN.md)**

OneCatch is a local-first desktop workbench for coding agents. It runs harnesses such as Codex and Claude Code on your machine, turning one-off agent sessions into queued, observable, interruptible, and resumable tasks, serial loops, and parallel DAGs.

OneCatch does not broker model accounts or require uploading your repository to a OneCatch server. Your workspace stays in its original directory. OneCatch stores tasks, workflows, run events, and logs under `~/.onecatch/` by default. Whether a model request uses the network and what it sends to a provider depend on the harness and its configuration.

> **Project status:** OneCatch is under active development; release versions come from [`VERSION`](VERSION). Desktop installers target macOS 12+, Windows 10+, and Linux systems with GTK4/WebKitGTK 6.0. Configuration formats and interactions may change between minor releases. The repository contains the Remote Worker server and scheduler, but its desktop UI is preview-only and cannot yet be enabled or managed.

## What OneCatch does

- **Runs agents directly:** Supports Codex, Claude Code, Modu Code, Pi, Grok Build, and DeepSeek Harness. OneCatch discovers CLIs on `PATH`; Modu Code can also use its embedded SDK. Where supported, you can select the executable, model, reasoning effort, and inherited environment variables.
- **Orchestrates workflows:** Serial workflows route between steps using signals returned by agents, enabling loops such as implement → review → revise. DAG workflows run independent nodes concurrently and pass their results to downstream nodes.
- **Lets you intervene:** Inspect messages, tool calls, command output, and token usage. Queue instructions for the next turn, or interrupt the current turn and insert a high-priority instruction. Harnesses that support resume reuse their original sessions.
- **Controls access boundaries:** Each task or node can be read-only, workspace-write, or full access. Full access is disabled by default. If enabled, OneCatch still asks for confirmation before each run by default.
- **Manages local workspaces:** Browse and edit files, use a multi-tab split terminal, inspect Git status and diffs, and ask an agent to review uncommitted changes without modifying them.
- **Connects local agents to remote directories:** Remote FS operates on a remote workspace through SSH/SFTP while keeping the harness and model credentials local. It currently supports Codex, Claude Code, and Modu Code. This mode is not yet available from the Windows desktop app.

The first launch includes three workflows: Single Agent, Implement and Review Loop, and Parallel Review DAG. The visual editor lets you change roles, instructions, dependencies, signals, sandboxes, and execution limits.

## Installation

### Install a release

Download installers from [GitHub Releases](https://github.com/openmodu/onecatch/releases):

- macOS: `OneCatch-<version>-macOS-<arch>.dmg`
- Windows: `OneCatch-<version>-Windows-<arch>-Setup.exe`
- Linux: `OneCatch-<version>-Linux-<arch>.deb` or `.AppImage`

Each installer has a matching `.sha256` checksum file. The current macOS package uses ad-hoc signing, and the Windows package is not Authenticode-signed, so the operating system may show a security warning on first launch. Public releases still need production signing and macOS notarization.

### Run from source

Prerequisites:

- Go `1.26.1`, as declared in [`go.mod`](go.mod)
- Node.js `24.14.0` and npm `11.9.0`, as declared in [`frontend/package.json`](frontend/package.json)
- Git
- The [platform dependencies required by Wails 3](https://v3.wails.io/quick-start/installation/)
- At least one usable harness: CLI integrations require the matching command to be installed and authenticated; the embedded Modu Code SDK requires a valid provider configuration

The Go `tool` directive pins the Wails CLI, so you do not need a separate `go install wails3` step. Run these commands from a terminal:

```bash
git clone https://github.com/openmodu/onecatch.git
cd onecatch
go tool wails3 task deps
go tool wails3 task dev:desktop
```

The `deps` task runs `go mod download` and `npm ci`. It does not use `go mod tidy` or `npm install`, so it does not rewrite either lockfile.

After OneCatch opens:

1. Open **Settings → Harness** and confirm that at least one agent is available.
2. Add a local project and choose its default file-access level.
3. Create a task and select either one agent or a workflow.
4. Start the task, then inspect, interrupt, or resume it from the timeline.

See [`docs/harness-integrations.md`](docs/harness-integrations.md) for protocol, resume, and sandbox differences between harnesses.

## Common development commands

Run these commands from the repository root:

```bash
go tool wails3 task deps              # Install Go and frontend dependencies
go tool wails3 task dev:desktop       # Start the desktop development environment
go tool wails3 task build:desktop     # Build the desktop app with development settings
go tool wails3 task package:desktop   # Package the desktop app for the current OS
go tool wails3 task build:worker      # Build bin/onecatch-worker
go tool wails3 task test              # Run Go and frontend tests
```

The iOS and Android clients are experimental Remote Worker consoles. They do not run local agent CLIs on the mobile device:

```bash
go tool wails3 task run:ios
go tool wails3 task run:android
```

Mobile development also requires Xcode or the Android SDK and NDK.

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
go tool wails3 task build:worker
./bin/onecatch-worker --pair
```

A non-loopback listener requires TLS through `--tls-cert` and `--tls-key`; add `--client-ca` for mTLS. Use `--allow-insecure-http` only when a trusted tunnel already provides transport security. Install a per-user background service with `--install-service`. Run `./bin/onecatch-worker --help` for the complete option list. See [`cmd/worker/README.md`](cmd/worker/README.md) for the Worker entry point and [`deploy/onecatch-worker/`](deploy/onecatch-worker/) for launchd and systemd templates.

## Desktop packaging and releases

The root [`VERSION`](VERSION) file is the source of the installer version and accepts only a three-part `X.Y.Z` value:

```bash
go tool wails3 task package:desktop
```

Windows packaging requires [NSIS](https://nsis.sourceforge.io/):

```powershell
winget install NSIS.NSIS
```

Linux packaging uses GTK4 and WebKitGTK 6.0. The `.deb` and AppImage include the worker, shell, and SSH askpass helpers used by the desktop app.

For a release, update and commit `VERSION`, then push the matching tag:

```bash
git tag v0.2.0
git push origin v0.2.0
```

The tag must equal `v` followed by the contents of `VERSION`. GitHub Actions builds the macOS DMG, Windows Setup package, Linux `.deb`, and Linux AppImage, then publishes the installers and their SHA-256 files to the matching GitHub Release.

To sign the macOS package with a Developer ID, set `SIGN_IDENTITY`. If notarization credentials have already been saved with `notarytool store-credentials`, also set `NOTARY_PROFILE`:

```bash
SIGN_IDENTITY="Developer ID Application: Example Corp (TEAMID)" \
NOTARY_PROFILE="onecatch-notary" \
  go tool wails3 task package:desktop
```

## Repository layout

```text
cmd/
├── app/                Shared Wails entry point for desktop and mobile
├── worker/             Remote execution service entry point
├── onecatchsh/         Remote FS command proxy
└── onecatch-askpass/   SSH password helper
frontend/               React, Vite, tests, and generated Wails bindings
internal/
├── app/                Desktop, Mobile, and Worker assembly
├── domain/             Domain models and business rules
├── repo/               File, Git, and historical data access
├── usecase/            Agent adapters and workflow use cases
├── service/            Desktop, Mobile, and Worker services
└── transport/          Wails and HTTP adapters
clients/onecatch/        OneCatch Go HTTP SDK
pkg/                    General Go packages exposed outside this repository
build/                  Desktop, iOS, and Android build and packaging files
deploy/                 Worker launchd and systemd templates
docs/                   Harness and other development documentation
tools/                  Go tool dependency declarations
```

The repository has one root `go.mod`. Packages under `cmd` contain process entry points only. Shared repository code belongs under `internal`; only packages that must be imported by external projects belong under `pkg` or `clients`. Desktop and mobile builds share the React source, lockfile, bindings, and build output under `frontend`. See [`internal/README.md`](internal/README.md) for layer boundaries and placement rules.

## Contributing

Issues and pull requests are welcome. Before submitting a change, run:

```bash
go tool wails3 task test
go tool wails3 task build:desktop
```

When adding a harness, treat `internal/domain/harnesses/catalog.go` as the capability catalog and update its adapter, configuration inspection, and tests. Do not maintain a second runtime catalog in the frontend.

## License

OneCatch is licensed under the [MIT License](LICENSE). Copyright © 2026 OpenModu.
