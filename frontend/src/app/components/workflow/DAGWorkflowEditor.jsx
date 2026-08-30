import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ArrowLeft, CheckCircle2, GitBranch, LayoutGrid, Plus, Save, Trash2 } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { nextWorkflowItemID } from "../../workflowIds.js";
import { assignWorkflowWorker, isRemoteWorker } from "../../workflowWorker.js";
import { enabledRuntimeInfos } from "../../runtimeHarnesses.js";
import { SettingsField, SettingsSelect } from "../settings/SettingsControls.jsx";
import WorkflowIdentityFields from "./WorkflowIdentityFields.jsx";

export default function DAGWorkflowEditor({ editor, setEditor, validation, validateEditor, saveWorkflow, busy, runtimes, workers, defaultSandbox, allowFullSandbox, onClose, embedded = false }) {
  const { t } = useTranslation();
  const [selectedID, setSelectedID] = useState(editor.steps[0]?.id || "");
  const [connectFrom, setConnectFrom] = useState("");
  const drag = useRef(null);
  const canvas = useRef(null);
  const selectedIndex = editor.steps.findIndex((step) => step.id === selectedID);
  const selected = editor.steps[selectedIndex];
  const positions = editor.layout?.nodes || {};
  const workerOptions = [{ value: "local", label: t("common.local") }, ...workers.filter((worker) => worker.enabled).map((worker) => ({ value: worker.id, label: worker.name }))];
  const runtimeOptions = enabledRuntimeInfos(runtimes, {}, editor.steps.map((step) => step.runtime));
  const defaultRuntime = runtimeOptions.find((runtime) => !runtime.disabled)?.id || "";

  const setStep = (field, value) => setEditor((current) => ({ ...current, steps: current.steps.map((step) => step.id === selectedID ? { ...step, [field]: value } : step) }));
  const setWorker = (workerId) => setEditor((current) => ({ ...current, steps: current.steps.map((step) => step.id === selectedID ? assignWorkflowWorker(step, workerId) : step) }));
  const renameStep = (nextID) => setEditor((current) => {
    const oldID = selectedID;
    const nodes = { ...(current.layout?.nodes || {}) }; nodes[nextID] = nodes[oldID] || { x: 80, y: 80 }; delete nodes[oldID];
    const steps = current.steps.map((step) => ({ ...step, id: step.id === oldID ? nextID : step.id, dependsOn: (step.dependsOn || []).map((id) => id === oldID ? nextID : id) }));
    return { ...current, entryStepId: current.entryStepId === oldID ? nextID : current.entryStepId, steps, layout: { nodes } };
  });
  const addNode = () => {
    const id = nextWorkflowItemID("node", editor.steps);
    setEditor((current) => ({ ...current, steps: [...current.steps, { id, name: t("workflow.newNode"), runtime: defaultRuntime, workerId: "local", sandbox: defaultSandbox, dependsOn: [], rolePrompt: t("workflow.defaultDagRole"), instruction: t("workflow.defaultNodeInstruction"), transitions: { completed: "$done", need_human: "$pause" } }], layout: { nodes: { ...(current.layout?.nodes || {}), [id]: { x: 120 + current.steps.length * 34, y: 110 + current.steps.length * 34 } } } }));
    setSelectedID(id);
  };
  const deleteNode = () => {
    if (!selected || editor.steps.length <= 1) return;
    const remaining = editor.steps.filter((step) => step.id !== selected.id).map((step) => ({ ...step, dependsOn: (step.dependsOn || []).filter((id) => id !== selected.id) }));
    const nodes = { ...positions }; delete nodes[selected.id];
    setEditor({ ...editor, steps: remaining, entryStepId: editor.entryStepId === selected.id ? remaining[0].id : editor.entryStepId, layout: { nodes } });
    setSelectedID(remaining[0].id);
  };
  const connect = (targetID) => {
    if (!connectFrom) { setConnectFrom(targetID); return; }
    if (connectFrom !== targetID) setEditor((current) => ({ ...current, steps: current.steps.map((step) => step.id === targetID ? { ...step, dependsOn: [...new Set([...(step.dependsOn || []), connectFrom])] } : step) }));
    setConnectFrom("");
  };
  const deleteEdge = (source, target) => setEditor((current) => ({ ...current, steps: current.steps.map((step) => step.id === target ? { ...step, dependsOn: (step.dependsOn || []).filter((id) => id !== source) } : step) }));
  const autoLayout = () => {
    const levels = {};
    const levelOf = (id, trail = []) => { if (levels[id] != null) return levels[id]; if (trail.includes(id)) return 0; const step = editor.steps.find((item) => item.id === id); return levels[id] = step?.dependsOn?.length ? Math.max(...step.dependsOn.map((dep) => levelOf(dep, [...trail, id]))) + 1 : 0; };
    editor.steps.forEach((step) => levelOf(step.id));
    const rows = {};
    const nodes = {}; editor.steps.forEach((step) => { const level = levels[step.id] || 0; rows[level] = (rows[level] || 0) + 1; nodes[step.id] = { x: 60 + level * 310, y: 55 + (rows[level] - 1) * 155 }; });
    setEditor({ ...editor, layout: { nodes } });
  };
  const moveNode = (event, stepID) => {
    if (!drag.current || drag.current.id !== stepID) return;
    const rect = canvas.current.getBoundingClientRect();
    const width = Math.max(canvas.current.clientWidth, canvas.current.scrollWidth);
    const height = Math.max(canvas.current.clientHeight, canvas.current.scrollHeight);
    const x = Math.max(10, Math.min(width - 224, event.clientX - rect.left + canvas.current.scrollLeft - drag.current.offsetX));
    const y = Math.max(10, Math.min(height - 106, event.clientY - rect.top + canvas.current.scrollTop - drag.current.offsetY));
    setEditor((current) => ({ ...current, layout: { nodes: { ...(current.layout?.nodes || {}), [stepID]: { x, y } } } }));
  };
  const updateSignal = (oldSignal, signal, target) => setStep("transitions", Object.fromEntries(Object.entries(selected.transitions || {}).filter(([key]) => key !== oldSignal).concat([[signal, target]])));

  return <section className="workflow-editor-surface dag-editor-page select-none bg-background">
    <div className="dag-editor-shell flex min-h-0 flex-1 flex-col overflow-hidden">
      <header className="shrink-0 border-b border-border/80 bg-background/95 px-5 py-4">
        <div className="mb-4 flex items-center justify-between gap-4">
          <div className="flex items-center gap-2">
            {!embedded && <Button variant="ghost" size="sm" onClick={onClose}><ArrowLeft aria-hidden="true" />{t("common.back")}</Button>}
            <Badge variant="secondary"><GitBranch aria-hidden="true" />DAG · {t("workflow.allJoin")}</Badge>
          </div>
          <div className="flex flex-wrap items-center justify-end gap-2">
            <Button variant="outline" size="sm" onClick={autoLayout}><LayoutGrid aria-hidden="true" />{t("workflow.autoLayout")}</Button>
            <Button variant="outline" size="sm" onClick={addNode}><Plus aria-hidden="true" />{t("workflow.node")}</Button>
            <Button size="sm" disabled={busy === "workflow"} onClick={saveWorkflow}><Save aria-hidden="true" />{busy === "workflow" ? t("common.saving") : t("workflow.saveDag")}</Button>
          </div>
        </div>
        <div className="rounded-xl border border-border/80 bg-card/72 p-4 shadow-xs">
          <WorkflowIdentityFields editor={editor} setEditor={setEditor} validation={validation} />
        </div>
      </header>

      <div className="dag-workspace grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_minmax(280px,320px)]">
        <div className="dag-canvas" ref={canvas} onPointerUp={() => { drag.current = null; }} onPointerLeave={() => { drag.current = null; }}>
          <svg className="dag-edges" aria-hidden="true">{editor.steps.flatMap((step) => (step.dependsOn || []).map((source) => {
            const from = positions[source] || { x: 30, y: 30 };
            const to = positions[step.id] || { x: 400, y: 200 };
            const x1 = from.x + 204, y1 = from.y + 47, x2 = to.x, y2 = to.y + 47;
            return <g key={`${source}-${step.id}`} className="dag-edge" onClick={() => deleteEdge(source, step.id)}><path d={`M ${x1} ${y1} C ${x1 + 70} ${y1}, ${x2 - 70} ${y2}, ${x2} ${y2}`} /><text x={(x1 + x2) / 2} y={(y1 + y2) / 2 - 7}>×</text></g>;
          }))}</svg>
          {editor.steps.map((step) => {
            const point = positions[step.id] || { x: 50, y: 50 };
            return <div key={step.id} className={`dag-node-card ${selectedID === step.id ? "selected" : ""} ${connectFrom === step.id ? "connecting" : ""}`} style={{ left: point.x, top: point.y }} onClick={() => setSelectedID(step.id)} onPointerDown={(event) => { if (event.target.closest("button")) return; const rect = event.currentTarget.getBoundingClientRect(); drag.current = { id: step.id, offsetX: event.clientX - rect.left, offsetY: event.clientY - rect.top }; event.currentTarget.setPointerCapture(event.pointerId); }} onPointerMove={(event) => moveNode(event, step.id)}>
              <button className="dag-port input" title={t("workflow.connectHere")} aria-label={t("workflow.connectTo", { name: step.name })} onClick={(event) => { event.stopPropagation(); connect(step.id); }} />
              <div className="dag-node-head"><Badge variant="outline" className={`runtime-name ${step.runtime} max-w-[118px] truncate font-mono text-[10px]`}>{step.runtime}</Badge><small>{step.workerId || "local"}</small></div>
              <strong>{step.name}</strong>
              <p>{(step.dependsOn || []).length ? t("workflow.waitDependencies", { count: (step.dependsOn || []).length }) : t("workflow.rootParallel")}</p>
              <button className="dag-port output" title={t("workflow.connectFromHere")} aria-label={t("workflow.connectFrom", { name: step.name })} onClick={(event) => { event.stopPropagation(); connect(step.id); }} />
            </div>;
          })}
          <div className="canvas-hint">{connectFrom ? t("workflow.connectingFrom", { id: connectFrom }) : t("workflow.canvasHint")}</div>
        </div>

        <aside className="min-h-0 overflow-y-auto border-l border-border/80 bg-card/35">
          {selected ? <>
            <div className="sticky top-0 z-10 flex items-start justify-between gap-3 border-b border-border/80 bg-background/95 px-4 py-3.5 backdrop-blur">
              <div className="min-w-0"><span className="text-[11px] font-semibold uppercase tracking-[0.08em] text-muted-foreground">{t("workflow.nodeInspector")}</span><h3 className="mt-1 mb-0 truncate text-sm font-semibold text-foreground">{selected.name}</h3></div>
              <Button variant="ghost" size="icon-sm" className="text-destructive hover:bg-destructive/10 hover:text-destructive" title={t("workflow.deleteNode")} aria-label={t("workflow.deleteNode")} disabled={editor.steps.length <= 1} onClick={deleteNode}><Trash2 aria-hidden="true" /></Button>
            </div>
            <div className="grid gap-4 p-4">
              <SettingsField label={t("common.nodeID")}><Input className="font-mono" value={selected.id} onChange={(event) => { const next = event.target.value; renameStep(next); setSelectedID(next); }} /></SettingsField>
              <SettingsField label={t("worker.name")}><Input value={selected.name} onChange={(event) => setStep("name", event.target.value)} /></SettingsField>
              <SettingsField label={t("common.runtime")}><SettingsSelect ariaLabel={t("common.runtime")} value={selected.runtime} onChange={(runtime) => setStep("runtime", runtime)} options={runtimeOptions.map((runtime) => ({ value: runtime.id, label: runtime.name || runtime.id, meta: runtime.available ? "" : t("common.missing"), disabled: runtime.disabled }))} /></SettingsField>
              <SettingsField label={t("common.worker")}><SettingsSelect ariaLabel={t("common.worker")} value={selected.workerId || "local"} onChange={setWorker} options={workerOptions} /></SettingsField>
              <SettingsField label={t("common.sandbox")}><SettingsSelect ariaLabel={t("common.sandbox")} value={selected.sandbox || "read-only"} onChange={(sandbox) => setStep("sandbox", sandbox)} options={[{ value: "read-only", label: t("workspace.readOnly") }, { value: "workspace-write", label: t("workspace.write") }, { value: "full", label: t("workspace.fullDanger"), disabled: isRemoteWorker(selected.workerId) || (!allowFullSandbox && selected.sandbox !== "full") }]} /></SettingsField>
              <SettingsField label={t("workflow.rolePrompt")}><Textarea className="min-h-20 select-text resize-y" value={selected.rolePrompt} onChange={(event) => setStep("rolePrompt", event.target.value)} /></SettingsField>
              <SettingsField label={t("workflow.nodeInstruction")}><Textarea className="min-h-24 select-text resize-y" value={selected.instruction} onChange={(event) => setStep("instruction", event.target.value)} /></SettingsField>

              <section className="rounded-xl border border-border/75 bg-muted/35 p-3.5">
                <strong className="text-xs font-semibold text-foreground">{t("workflow.terminalSignals")}</strong>
                <div className="mt-3 grid gap-2.5">
                  {Object.entries(selected.transitions || {}).map(([signal, target], index) => <section className="rounded-lg border border-border/75 bg-background/80 p-3" key={signal}>
                    <Badge variant="secondary" className="mb-3">{t("workflow.transitionNumber", { number: index + 1 })}</Badge>
                    <div className="grid gap-3">
                      <SettingsField label={t("workflow.signal")}><Input className="font-mono" value={signal} onChange={(event) => updateSignal(signal, event.target.value, target)} /></SettingsField>
                      <SettingsField label={t("workflow.target")}><SettingsSelect ariaLabel={`${signal} target`} value={target} onChange={(nextTarget) => updateSignal(signal, signal, nextTarget)} options={[{ value: "$done", label: "$done" }, { value: "$pause", label: "$pause" }, { value: "$fail", label: "$fail" }]} /></SettingsField>
                    </div>
                  </section>)}
                </div>
              </section>
            </div>
          </> : null}
        </aside>
      </div>

      <footer className={`flex min-h-11 shrink-0 items-center gap-3 overflow-x-auto border-t border-border/80 px-4 py-2 text-xs ${validation.length ? "bg-destructive/5 text-destructive" : "bg-muted/30 text-muted-foreground"}`}>
        <Button variant="outline" size="sm" onClick={validateEditor}><CheckCircle2 aria-hidden="true" />{t("workflow.validateDag")}</Button>
        {validation.length ? validation.map((issue) => <span className="whitespace-nowrap" key={`${issue.path}-${issue.code}`}><code className="mr-1 font-mono">{issue.path}</code>{issue.message}</span>) : <span className="whitespace-nowrap">{t("workflow.dagValidationHint")}</span>}
      </footer>
    </div>
  </section>;
}
