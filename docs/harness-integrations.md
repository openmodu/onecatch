# Harness integrations

OneCatch normalizes every coding harness behind `agentrun.Runner`. A runner may
use a child process (Codex app-server, Claude stream JSON, Modu CLI) or an
in-process SDK. Both forms publish the same `agentrun.Event` stream and return
an `agentrun.Result`, so workflows do not depend on the transport.

## Modu native SDK

Select **Settings → Runtime → Modu Code → Native Go SDK**. The configuration
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

OneCatch additionally loads Modu's enabled extensions for writable nodes,
applies model/provider overrides, maps token usage and streaming tool/message
events, and strictly filters the tool catalog for read-only workflow nodes.

## Adding Grok Build or Pi

Add one adapter implementing `agentrun.Runner`, register its runtime ID in the
engine, and add its `RuntimeSettings` metadata and settings Tab. Process and SDK
integrations should stay inside the adapter; orchestration, persistence, remote
execution, and the UI consume only normalized events.
