import { selectTaskExecutionTarget } from "./runtimeHarnesses.js";
export const isRemoteWorker = (id) => Boolean(id && id !== "local");

export function workerSupportsHarness(worker, harness) {
  return Boolean(worker?.enabled && (worker.provider !== "volcengine-sandbox" || harness === "codex"));
}

export function selectTaskWorker(form, worker) {
  return {
    ...form,
    workerId: worker?.id || "",
    ...(worker ? { worktree: { mode: "project" } } : {}),
    ...(worker?.provider === "volcengine-sandbox" ? { harness: "codex" } : {}),
    model: "", reasoningEffort: "", serviceTier: "",
  };
}

export function workerLabel(worker, fallback = "") {
  return (worker?.name || fallback).replace(/^(?:Volcengine Sandbox(?:\s*·.*)?|volc sandbox)$/, "codex volc_sandbox");
}

export function selectConversationTarget(form, target, workers = []) {
  if (target.startsWith("remote:")) {
    const [id, harness] = JSON.parse(target.slice(7));
    const worker = workers.find((item) => item.id === id);
    if (!workerSupportsHarness(worker, harness)) return form;
    return { ...selectTaskWorker(form, worker), workflowId: "single_agent", harness };
  }
  return selectTaskExecutionTarget(isRemoteWorker(form.workerId) ? selectTaskWorker(form, null) : form, target);
}
