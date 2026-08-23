// The harness catalog the UI renders from.
//
// Capabilities are the backend's answer, not a second copy kept here: the
// desktop's runtime list carries the same catalog the domain validates against,
// and `hydrateRuntimeHarnesses` installs it once the app has loaded it. The
// seed below only covers the window before that first load, and the cases where
// no harness list is available at all (demo mode, tests).
const seedHarnesses = [
  { id: "codex", label: "Codex", supportsReasoning: true, supportsSpeed: true },
  { id: "claude", label: "Claude Code", supportsReasoning: true, supportsSpeed: false },
  { id: "modu", label: "modu_code", supportsReasoning: false, supportsSpeed: false },
  { id: "pi", label: "Pi", supportsReasoning: true, supportsSpeed: false },
  { id: "grok", label: "Grok Build", supportsReasoning: true, supportsSpeed: false },
  { id: "dsh", label: "DeepSeek Harness", supportsReasoning: false, supportsSpeed: false },
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

export function runtimeHarnessOptions(runtimes = [], unavailableLabel = "Unavailable") {
  const statusByID = new Map(runtimes.map((item) => [item.id, item]));
  return runtimeHarnesses.map((item) => {
    const status = statusByID.get(item.id);
    return {
      value: item.id,
      label: item.label,
      disabled: status?.available === false,
      meta: status?.available === false ? unavailableLabel : "",
    };
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
