// The harness catalog the UI renders from.
//
// Capabilities are the backend's answer, not a second copy kept here: the
// desktop's runtime list carries the same catalog the domain validates against,
// and `hydrateRuntimeHarnesses` installs it once the app has loaded it. The
// seed below only covers the window before that first load, and the cases where
// no harness list is available at all (demo mode, tests).
const seedHarnesses = [
  { id: "codex", label: "Codex", supportsReasoning: true, supportsSpeed: true, supportsRemoteFs: true },
  { id: "claude", label: "Claude Code", supportsReasoning: true, supportsSpeed: false, supportsRemoteFs: true },
  { id: "modu", label: "modu_code", supportsReasoning: false, supportsSpeed: false, supportsRemoteFs: true },
  { id: "pi", label: "Pi", supportsReasoning: true, supportsSpeed: false, supportsRemoteFs: false },
  { id: "grok", label: "Grok Build", supportsReasoning: true, supportsSpeed: false, supportsRemoteFs: false },
  { id: "dsh", label: "DeepSeek Harness", supportsReasoning: false, supportsSpeed: false, supportsRemoteFs: false },
];

export let runtimeHarnesses = seedHarnesses;

// hydrateRuntimeHarnesses replaces the seed with the backend's catalog and
// returns the runtime list unchanged, so callers can wrap it around whatever
// they were already storing. An empty list leaves the seed in place, so a
// failed probe does not empty the harness picker.
export function hydrateRuntimeHarnesses(runtimes = []) {
  if (runtimes.length) {
    runtimeHarnesses = runtimes.map((runtime) => ({
      id: runtime.id,
      label: runtime.name || runtime.id,
      supportsReasoning: (runtime.efforts || []).length > 0,
      supportsSpeed: Boolean(runtime.serviceTiers),
      supportsRemoteFs: Boolean(runtime.supportsRemoteFs),
    }));
  }
  return runtimes;
}

export const directAgentWorkflowID = "single_agent";

export function runtimeHarness(id = "codex") {
  return runtimeHarnesses.find((item) => item.id === id) || { id, label: id || "Codex", supportsReasoning: false, supportsSpeed: false };
}

export function supportsRuntimeProfile(id = "codex") {
  const capability = runtimeHarness(id);
  return capability.supportsReasoning || capability.supportsSpeed;
}

export function runtimeHarnessEnabled(id, runtimes = [], runtimeSettings = {}, remoteFS = false) {
  const capability = runtimeHarness(id);
  const status = runtimes.find((item) => item.id === id);
  const preference = runtimeSettings?.[id] || {};
  const enabled = preference.enabled ?? status?.enabled ?? true;
  if (!enabled) return false;
  if (!remoteFS) return true;
  const supportsRemoteFs = status?.supportsRemoteFs ?? capability.supportsRemoteFs ?? false;
  const remoteFsEnabled = preference.remoteFsEnabled ?? status?.remoteFsEnabled ?? supportsRemoteFs;
  return supportsRemoteFs && remoteFsEnabled;
}

export function hasRemoteFSHarness(runtimes = [], runtimeSettings = {}) {
  return runtimeHarnesses.some((item) => runtimeHarnessEnabled(item.id, runtimes, runtimeSettings, true));
}

export function workflowHarnessesEnabled(workflow, runtimes = [], runtimeSettings = {}, remoteFS = false) {
  return (workflow?.steps || []).every((step) => runtimeHarnessEnabled(step.runtime, runtimes, runtimeSettings, remoteFS));
}

export function enabledRuntimeInfos(runtimes = [], runtimeSettings = {}, preserveIDs = []) {
  const preserve = new Set(preserveIDs);
  return runtimes
    .filter((item) => runtimeHarnessEnabled(item.id, runtimes, runtimeSettings) || preserve.has(item.id))
    .map((item) => runtimeHarnessEnabled(item.id, runtimes, runtimeSettings) ? item : { ...item, disabled: true });
}

export function runtimeHarnessOptions(runtimes = [], unavailableLabel = "Unavailable", runtimeSettings = {}, remoteFS = false) {
  const statusByID = new Map(runtimes.map((item) => [item.id, item]));
  return runtimeHarnesses.flatMap((item) => {
    if (!runtimeHarnessEnabled(item.id, runtimes, runtimeSettings, remoteFS)) return [];
    const status = statusByID.get(item.id);
    return [{
      value: item.id,
      label: item.label,
      disabled: status?.available === false,
      meta: status?.available === false ? unavailableLabel : "",
    }];
  });
}

export function selectRuntimeHarness(current, harness) {
  if (current.harness === harness) return current;
  return { ...current, harness, model: "", reasoningEffort: "", serviceTier: "" };
}

export function taskExecutionTarget(value = {}) {
  if (value.workflowId === directAgentWorkflowID) return `agent:${value.harness || "codex"}`;
  return `workflow:${value.workflowId || ""}`;
}

export function selectTaskExecutionTarget(current, target) {
  if (target.startsWith("agent:")) {
    const harness = target.slice("agent:".length) || "codex";
    return selectRuntimeHarness({ ...current, workflowId: directAgentWorkflowID }, harness);
  }
  return {
    ...current,
    workflowId: target.slice("workflow:".length),
    harness: "",
    model: "",
    reasoningEffort: "",
    serviceTier: "",
  };
}
