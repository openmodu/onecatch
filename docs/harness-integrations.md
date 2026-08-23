# Harness integrations

OneCatch normalizes every coding harness behind `agentrun.Runner`. A runner may
use a child process (Codex app-server, Claude stream JSON, Modu CLI, Pi print
mode), a protocol client (Grok over ACP), a log reader (DeepSeek Harness), or an
in-process SDK. Every form publishes the same `agentrun.Event` stream and returns
an `agentrun.Result`, so workflows do not depend on the transport.

| Runtime | Transport | Resume | Sandbox lever |
| --- | --- | --- | --- |
| `codex` | app-server JSON-RPC | `thread/resume` | `--sandbox` / `-c sandbox_mode` |
| `claude` | `claude -p --output-format stream-json` | `--resume` | tool deny list |
| `modu` | native Go SDK, or `modu_code -p … -json` | `--resume` | n/a |
| `pi` | `pi -p --mode json` | `--session <id>` | `--tools` allowlist |
| `grok` | ACP over stdio (`grok agent … stdio`) | `session/load` | `GROK_SANDBOX` profile |
| `dsh` | headless profile + its own JSONL session log | none | `DSH_PERMISSION_MODE` |

## Modu native SDK

Open **Settings → Harness**, expand **Modu Code**, and select **Native Go SDK**. The configuration
source can be either Modu's shared `~/.modu/config.toml` or an isolated file.
When the isolated path is empty, OneCatch uses
`<data-root>/harnesses/modu/config.toml` and keeps sessions and extensions in
the same directory. The first switch to this mode copies an existing shared
configuration with owner-only permissions; an explicitly configured path is
never created or overwritten. Prefer `apiKeyEnv` over an inline API key. CLI mode remains
available as a compatibility fallback and runs `modu_code -p ... -json`.

The embedded flow is equivalent to:

```go
model, getAPIKey, err := provider.ResolveConfigFile(configPath)
if err != nil {
    return err
}
session, err := coding_agent.NewCodingSession(coding_agent.CodingSessionOptions{
    Cwd:             workspace,
    AgentDir:        agentDir,
    Model:           model,
    GetAPIKey:       getAPIKey,
    ResumeSessionID: previousSessionID,
    ToolProvider:    tools.NewProvider(tools.ToolSetCoding),
})
if err != nil {
    return err
}
defer session.Close("host_complete")

unsubscribe := session.Subscribe(func(event types.Event) {
    // Convert Modu's semantic event to the host event model.
})
defer unsubscribe()

if err := session.Prompt(ctx, prompt); err != nil {
    return err
}
session.WaitForIdle()
```

The embedded runner intentionally uses Modu's core coding tool provider without
the optional research/web provider or bundled extensions. OneCatch owns workflow
orchestration itself, and this keeps browser, HTML extraction, workflow VM, cron,
goal, and subagent dependencies out of the desktop binary. It still applies
model/provider overrides, maps token usage and streaming tool/message events,
and strictly filters the tool catalog for read-only workflow nodes. Use Modu CLI
mode when a task needs the full standalone `modu_code` extension set.

The bundled remote worker is built with the `onecatch_worker` build tag and uses
the Modu CLI adapter. This prevents the SDK graph from being duplicated into both
the desktop executable and its packaged worker.

## Agent Client Protocol

`acp.go` is a harness-neutral ACP client: it drives `initialize` → `session/new`
(or `session/load`) → `session/prompt` over stdio and translates `session/update`
notifications into the normalized event stream. An adapter supplies only an
`acpLaunch` — the command to run and how that harness spells model, effort, and
sandbox — so the protocol lives in one place.

An `acpLaunch` supplies the whole argument vector rather than a suffix, because
flag placement is harness-specific: `grok agent stdio` takes only debug and
socket options, so model and reasoning effort belong to the parent `agent`
command and must precede the subcommand. The sandbox has no flag on that path at
all and travels in `GROK_SANDBOX`, which Grok documents as the `--sandbox`
equivalent and which still refuses to start on a missing profile. Getting this
wrong is invisible to stub tests — a shell stub accepts any argv — so
`harness_launch_test.go` checks each adapter's argument vector against the real
CLI using something it can answer locally, with no credentials or quota.

Grok Build is the first harness on it. `grok agent stdio` is the surface xAI's
own SDKs target, and the only one carrying blocking `session/request_permission`
requests, which map onto the same approval card Claude Code raises. Its
`-p --output-format streaming-json` mode emits a flattened, vendor-named
projection of the same updates and is deliberately not used. The `initialize`
result also carries Grok's model catalog and reasoning-effort levels, so
`InspectConfiguration` reads them from a handshake that needs no credentials.

### Tool approvals

A harness that can pause on a tool call and wait for a host decision implements
`agentrun.InteractivePermissionRunner`, and hosts ask
`Engine.SupportsInteractivePermissions(runtime, sandbox)` rather than testing the
runtime id. The answer depends on both:

- **Claude Code** can only ask in a **read-only** run. Its `can_use_tool` control
  channel arrives through `--input-format stream-json`, which changes the whole
  invocation, so a write-capable run passes `--dangerously-skip-permissions`
  instead. Widening this means reworking how the write path is launched.
- **Grok** can ask in **read-only and workspace-write**, because ACP carries
  permission requests in every mode. The sandbox is the backstop and the card is
  the first line. The full sandbox is excluded: choosing it is blanket consent
  for unattended automation, and nobody is watching to answer.

"Always allow" means different things and each adapter folds that into
`PermissionRequest.SuppressAlwaysAllow`, which is the single flag the host and
the card read. Claude persists the provider-authored rules in `Suggestions`, so
a request without them suppresses the option rather than degrading it to a
one-shot allow. ACP has no rule payload at all: the agent simply offers an
`allow_always` option and keeps whatever memory it likes, unobservable to
OneCatch — so the card labels it as the harness's own memory.

`PermissionRequest.RequiresUserInteraction` is Claude's escape hatch for a
request that cannot be answered from the card at all; the UI hides its buttons
and points at Claude's own interface. No other adapter should set it.

## Pi

`pi -p --mode json` writes a session header followed by Pi's own event union
(`agent_start`, `message_update` with an `assistantMessageEvent` delta,
`tool_execution_*`, `agent_end`). Pi has no OS sandbox: its permission model is
the tool catalog, so a read-only run passes a `--tools` allowlist rather than a
sandbox flag. `--list-models` prints a padded table used for model discovery.

## DeepSeek Harness

dsh is the one harness with no machine-readable stdout — its headless profile
prints only the final assistant message. It is instead plugin-composed, and a
`--patch` overlay can reconfigure any row of the composed profile by id. The
adapter uses that documented seam to point the harness's own durable session log
at a per-run directory with `compression: none` and `packChunks: false`, then
reads that log as it is written. The events are the ones dsh's own interfaces
render, not a reconstruction.

Two consequences are worth knowing:

- **No resume.** The headless profile creates one fresh agent per invocation and
  exposes no resume flag, so `ResumeSessionID` is rejected rather than silently
  starting a new conversation.
- **`--expose-internals`.** As of `0.1.1-rc.2` the launcher mounts a hot-reload
  plugin that refuses to start without that Node flag, which `NODE_OPTIONS` will
  not carry. `dshCommand` resolves the published script and runs it under `node`
  explicitly; a non-script binary is launched directly, so a release that drops
  the requirement needs no change here.

Sandbox mode rides `DSH_PERMISSION_MODE`, whose vocabulary
(`read-only` / `workspace-write` / `danger-full-access`) maps one-to-one onto
`agentrun.Sandbox`.

## Adding another harness

Four edits, because everything else derives:

1. **`internal/domain/harnesses`** — one catalog entry: the harness's name,
   default command, effort and provider vocabularies, integrations, and whether
   it can resume. Task validation, settings validation, the engine's notion of a
   valid runtime, the desktop probe list, the worker's availability report and
   its `--<id>-binary` flag, and the whole settings UI all read this.
2. **`internal/usecase/agentrun/<name>.go`** — the adapter, implementing
   `agentrun.Runner`, or an `acpLaunch` if the harness speaks ACP. Register it
   in the `descriptors` table in `registry.go`.
3. **`frontend/public/assets/runtime/<id>.svg`** — the mark. It resolves by id,
   so no map needs editing; record provenance in `SOURCES.md`.
4. **`settings.<id>Description`** in `frontend/src/i18n.js`, both languages.

This shape exists because the earlier one failed. The same facts used to be
restated in task validation, settings validation, the engine registry, the
desktop probe list, and again in the frontend, and `grok` shipped registered in
the engine but missing from task validation — selectable in the picker, then
rejected with "task is invalid" the moment anyone used it.
`TestCatalogAndEngineAgree` now fails if a harness exists in one and not the
other, and the settings-page test fails if the UI special-cases a harness id
instead of reading its capabilities.

Process, protocol, and SDK details stay inside the adapter; orchestration,
persistence, remote execution, and the UI consume only normalized events.

Adapter tests replay captured harness output through the parser rather than
asserting against hand-written shapes. `ONECATCH_LIVE=1` runs the live smoke
tests, which spend real model quota and are excluded from the normal suite.
