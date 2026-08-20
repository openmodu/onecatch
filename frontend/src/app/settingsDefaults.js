// The shape the UI falls back to before (or instead of) a real backend answer:
// demo mode runs on it, and every window uses it as initial state so the first
// render never has to guard every nested field.
//
// It lives apart from SettingsPage.jsx so that windows which only need the
// default value — the workbench, which merely holds settings in state — do not
// drag the whole settings screen and its controls into their first load.
export const demoSettings = {
  schemaVersion: 1, revision: 1,
  runtimes: { codex: { binary: "", defaultModel: "", reasoningEffort: "", serviceTier: "", environmentAllowlist: [] }, claude: { binary: "", defaultModel: "", reasoningEffort: "", environmentAllowlist: [] }, modu: { binary: "", defaultModel: "", provider: "auto", environmentAllowlist: [] } },
  terminal: { shell: "", arguments: [], theme: "system" },
  execution: { maxTransitions: 20, maxConsecutiveFailures: 3, stepTimeoutSeconds: 1800, maxLocalDAGConcurrency: 4, interruptGraceSeconds: 10, defaultSandbox: "workspace-write" },
  security: { allowFullSandbox: false, confirmFullSandboxEveryRun: true, diagnosticsIncludePrompt: false, diagnosticsIncludeRawEvents: false },
  storage: { completedRunRetentionDays: 0, logLevel: "info", logMaxSizeMB: 20, logMaxBackups: 5, logMaxAgeDays: 14 },
  experimental: { remoteWorkersEnabled: false },
};
