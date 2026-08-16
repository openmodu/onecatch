export const runtimeHarnesses = [
  { id: "codex", label: "Codex", supportsReasoning: true, supportsSpeed: true },
  { id: "claude", label: "Claude Code", supportsReasoning: true, supportsSpeed: false },
  { id: "modu", label: "modu_code", supportsReasoning: false, supportsSpeed: false },
];

export function runtimeHarness(id = "codex") {
  return runtimeHarnesses.find((item) => item.id === id) || { id, label: id || "Codex", supportsReasoning: false, supportsSpeed: false };
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
