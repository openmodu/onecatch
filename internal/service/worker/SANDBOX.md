# Hidden Volcengine sandbox worker

Open **Settings → Remote Worker (Coming soon)** and press **Ctrl+Alt+S** (Control+Option+S on
macOS) to reveal the hidden controls. Enable remote workers and save, open
**Add Worker**, and
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
worktree tasks). The local project groups the task records; the configured sandbox
directory is where Codex reads and changes files. Register another worker with a
different directory to target another sandbox project.

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
- Skip local Git baseline checks, remote cloning, patch export, local patch
  application and acknowledgement. The worker page hides Git synchronization.
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
