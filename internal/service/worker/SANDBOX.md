# Hidden Volcengine sandbox worker

Open **Settings → Remote Worker** and press **Ctrl+Alt+S** (Control+Option+S on
macOS) to reveal the hidden controls. Enable remote workers and save, open
**Add environment**, and
paste a Volcengine gateway URL containing `faasInstanceName` and `Authorization`.
The connection dialog defaults to Volcengine sandbox and asks for a working
directory instead of a pairing code. Choose Standard OneCatch worker for the
existing pairing flow. The directory defaults to `/home/gem` and must already exist.
Normal worker URLs keep the existing pairing flow.

In a **new conversation**, choose **codex volc_sandbox** from the Agent menu in the
composer. The sandbox selects Codex automatically and uses the model configuration
installed there. The chosen worker is saved with the task and its run definition;
continuations reuse that worker and remote thread. Existing conversations keep
their execution location; start a new conversation to change workers. Local
attachments and local worktree selection are unavailable for remote conversations.

For workflows, choose **Codex** and this worker in a node. Run that workflow against
a local project using the project directory (remote workers do not support local
worktree tasks). With file sync disabled, the local project only groups task
records and Codex works directly in the configured sandbox directory.

## Local project sync (preview)

Edit the sandbox and enable **Sync local project**, then start a new conversation
using **codex volc_sandbox**. The switch is off by default, so existing sandbox
connections keep their original behavior. The preview supports Volcengine
sandboxes; standard workers retain their Git clone/patch flow.

Each turn uploads local source changes before starting Codex, then returns remote
changes after the turn completes. There is no background file watcher in this
version. Files saved locally during execution are queued for the next turn;
unsaved editor buffers cannot be synchronized.

- A persistent replica lives under `<remotePath>/.onecatch-workspaces/<key>`.
  Different local projects and worker connections have separate replicas.
- A dedicated WSS shell runs an embedded Python 3 file helper, independently of
  Codex. Python 3 and Git must be installed in the sandbox. No incoming port or
  permanent service is required. File-level SHA-256 manifests select changes;
  changed files are transferred completely in 192 KiB chunks, with digest checks
  and atomic per-file replacement. Renames are represented as delete + create.
- Nested `.gitignore` rules are respected. Previously synchronized and tracked
  source files remain included even if later ignored. `.git`, dependency/build
  directories and common credential paths (`.env*`, keys, SSH/cloud configs)
  are always excluded. Symlinks and special files are not copied. Exclusions are
  a conservative filename policy, not a guarantee that source contains no secrets.
- Single files are limited to 64 MiB and manifests to 100,000 files. Exceeding a
  limit fails the sync instead of silently running against incomplete source.
  Dependencies must be installed remotely; the local Git database is not copied.
- Local and remote changes are compared with the last acknowledged baseline.
  Non-conflicting remote changes return automatically. A conflict keeps the local
  version and saves the remote alternative under
  `<dataRoot>/worker-sync/<key>/conflicts/<timestamp>/`. The turn reports the
  recovery location. Inspect/merge these copies before sending again: the next
  explicitly submitted turn uses the retained local version. There is no merge
  editor or automatic text merge in this preview.
- Metadata and owned shell session IDs are persisted before execution. After a
  disconnect/restart, those sessions are stopped before remote changes are
  recovered; prompts are never replayed automatically. A replica generation
  marker prevents a recreated sandbox from being interpreted as local deletions.
- Only one synchronized turn per local project can run at a time. A remote lock
  also protects a replica from concurrent synchronization sessions. Do not run
  independent background writers in the replica: they are outside this lock.

The settings list and composer show **Local project · sync each turn** when this
mode is enabled. The transcript reports upload and return progress. Start a new
conversation when changing the sync mode or remote base directory.

The sandbox must already have an authenticated `codex` executable. Connecting
performs an app-server initialization handshake without making a model request.
No worker binary is installed in the sandbox.

## Transport and persistence

- Connect over WSS to `/v1/shell/ws`, retaining the gateway's instance routing
  query and supplying its authorization token.
- Create a dedicated shell session, disable PTY echo, canonical input buffering
  and output rewriting, and start `codex app-server --listen stdio://`.
- Strip the bootstrap using a random marker. Feed the remaining JSON-RPC stream
  through the same Codex turn/event decoder as local runs.
- Tasks, user messages, streamed runtime events and remote Codex thread IDs use
  OneCatch's existing local workflow stores. Continued turns resume that remote
  thread. Sandbox code and Codex's own session files remain in the sandbox.
- Skip ordinary worker Git baseline checks, clone/patch export and patch
  acknowledgement. Optional sandbox file sync uses the separate protocol above.
- Finish/cancel closes the connection and attempts to delete only the shell
  session created for that turn. A broken connection fails the turn; prompts are
  never automatically replayed. If the gateway is unavailable, remote cleanup
  cannot be guaranteed. Resumption requires the sandbox and its Codex data to
  still exist.

The authorization token is removed from public worker URLs and stored separately
in the existing `0600` worker registry. Re-paste a refreshed URL in Edit Worker to
rotate it. Local environment variables and local Codex credentials are not copied.
The sandbox uses its own Codex configuration and credentials.

## Verification

```sh
go test -race ./internal/service/worker -run Sandbox
go test ./internal/service/desktop ./internal/usecase/agentrun ./internal/usecase/workflows
```

Live tests are skipped by default. Supply `ONECATCH_TEST_SANDBOX_URL` through the
environment for `TestSandboxLiveHandshake`. Additionally set
`ONECATCH_TEST_SANDBOX_TURN=1` for `TestSandboxLiveTurn`, which makes two model calls:
a read-only `pwd` task and a continuation of the same thread. Never commit a real
endpoint or token in a test fixture.

`TestSandboxLiveFileSync` checks binary chunk transfer without a model request.
`TestSandboxLiveSyncedTurn` additionally requires `ONECATCH_TEST_SANDBOX_TURN=1`;
it runs two turns on synthetic fixture files to verify local edits, incremental
upload, remote modification/creation/deletion and continuation. It does not upload
the developer's repository.
