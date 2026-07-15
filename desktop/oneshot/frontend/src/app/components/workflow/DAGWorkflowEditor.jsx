import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Action, TUISelect } from "../../../ui/primitives.jsx";
import { nextWorkflowItemID } from "../../workflowIds.js";
import Modal from "../Modal.jsx";

export default function DAGWorkflowEditor({ editor, setEditor, validation, validateEditor, saveWorkflow, busy, runtimes, workers, defaultSandbox, allowFullSandbox, onClose }) {
  const { t } = useTranslation();
  const [selectedID, setSelectedID] = useState(editor.steps[0]?.id || "");
  const [connectFrom, setConnectFrom] = useState("");
  const drag = useRef(null);
  const canvas = useRef(null);
  const selectedIndex = editor.steps.findIndex((step) => step.id === selectedID);
  const selected = editor.steps[selectedIndex];
  const positions = editor.layout?.nodes || {};
  const workerOptions = [{ id: "local", name: t("common.local") }, ...workers.filter((worker) => worker.enabled)];

  const setStep = (field, value) => setEditor((current) => ({ ...current, steps: current.steps.map((step) => step.id === selectedID ? { ...step, [field]: value } : step) }));
  const renameStep = (nextID) => setEditor((current) => {
    const oldID = selectedID;
    const nodes = { ...(current.layout?.nodes || {}) }; nodes[nextID] = nodes[oldID] || { x: 80, y: 80 }; delete nodes[oldID];
    const steps = current.steps.map((step) => ({ ...step, id: step.id === oldID ? nextID : step.id, dependsOn: (step.dependsOn || []).map((id) => id === oldID ? nextID : id) }));
    return { ...current, entryStepId: current.entryStepId === oldID ? nextID : current.entryStepId, steps, layout: { nodes } };
  });
  const addNode = () => {
    const id = nextWorkflowItemID("node", editor.steps);
    setEditor((current) => ({ ...current, steps: [...current.steps, { id, name: t("workflow.newNode"), runtime: "codex", workerId: "local", sandbox: defaultSandbox, dependsOn: [], rolePrompt: t("workflow.defaultDagRole"), instruction: t("workflow.defaultNodeInstruction"), transitions: { completed: "$done", need_human: "$pause" } }], layout: { nodes: { ...(current.layout?.nodes || {}), [id]: { x: 120 + current.steps.length * 34, y: 110 + current.steps.length * 34 } } } }));
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
    const x = Math.max(10, Math.min(700, event.clientX - rect.left - drag.current.offsetX));
    const y = Math.max(10, Math.min(460, event.clientY - rect.top - drag.current.offsetY));
    setEditor((current) => ({ ...current, layout: { nodes: { ...(current.layout?.nodes || {}), [stepID]: { x, y } } } }));
  };
  const updateSignal = (oldSignal, signal, target) => setStep("transitions", Object.fromEntries(Object.entries(selected.transitions || {}).filter(([key]) => key !== oldSignal).concat([[signal, target]])));

  return <Modal wide title={t("workflow.dagCanvas")} subtitle={t("workflow.dagCanvasSubtitle")} onClose={onClose}>
    <div className="dag-editor-shell">
      <div className="dag-toolbar">
        <div className="dag-meta"><input value={editor.id} onChange={(event) => setEditor({ ...editor, id: event.target.value })} /><input value={editor.name} onChange={(event) => setEditor({ ...editor, name: event.target.value })} /></div>
        <div className="dag-actions"><span className="mode-chip dag">DAG · {t("workflow.allJoin")}</span><Action onClick={autoLayout}>{t("workflow.autoLayout")}</Action><Action tone="primary" onClick={addNode}>{t("workflow.node")}</Action><Action tone="primary" disabled={busy === "workflow"} onClick={saveWorkflow}>{busy === "workflow" ? t("common.saving") : t("workflow.saveDag")}</Action></div>
      </div>
      <div className="dag-workspace">
        <div className="dag-canvas" ref={canvas} onPointerUp={() => { drag.current = null; }} onPointerLeave={() => { drag.current = null; }}>
          <svg className="dag-edges" viewBox="0 0 900 560" preserveAspectRatio="none">{editor.steps.flatMap((step) => (step.dependsOn || []).map((source) => {
            const from = positions[source] || { x: 30, y: 30 };
            const to = positions[step.id] || { x: 400, y: 200 };
            const x1 = from.x + 190, y1 = from.y + 48, x2 = to.x, y2 = to.y + 48;
            return <g key={`${source}-${step.id}`} className="dag-edge" onClick={() => deleteEdge(source, step.id)}><path d={`M ${x1} ${y1} C ${x1 + 70} ${y1}, ${x2 - 70} ${y2}, ${x2} ${y2}`} /><text x={(x1 + x2) / 2} y={(y1 + y2) / 2 - 7}>×</text></g>;
          }))}</svg>
          {editor.steps.map((step) => {
            const point = positions[step.id] || { x: 50, y: 50 };
            return <div key={step.id} className={`dag-node-card ${selectedID === step.id ? "selected" : ""} ${connectFrom === step.id ? "connecting" : ""}`} style={{ left: point.x, top: point.y }} onClick={() => setSelectedID(step.id)} onPointerDown={(event) => { if (event.target.closest("button")) return; const rect = event.currentTarget.getBoundingClientRect(); drag.current = { id: step.id, offsetX: event.clientX - rect.left, offsetY: event.clientY - rect.top }; event.currentTarget.setPointerCapture(event.pointerId); }} onPointerMove={(event) => moveNode(event, step.id)}>
              <button className="dag-port input" title={t("workflow.connectHere")} aria-label={t("workflow.connectTo", { name: step.name })} onClick={(event) => { event.stopPropagation(); connect(step.id); }} />
              <div className="dag-node-head"><span className={`runtime-badge ${step.runtime}`}>{step.runtime}</span><small>{step.workerId || "local"}</small></div><strong>{step.name}</strong><p>{(step.dependsOn || []).length ? t("workflow.waitDependencies", { count: (step.dependsOn || []).length }) : t("workflow.rootParallel")}</p>
              <button className="dag-port output" title={t("workflow.connectFromHere")} aria-label={t("workflow.connectFrom", { name: step.name })} onClick={(event) => { event.stopPropagation(); connect(step.id); }} />
            </div>;
          })}
          <div className="canvas-hint">{connectFrom ? t("workflow.connectingFrom", { id: connectFrom }) : t("workflow.canvasHint")}</div>
        </div>
        <aside className="dag-inspector">{selected ? <>
          <div className="dag-inspector-title"><div><span className="kicker">{t("workflow.nodeInspector")}</span><h3>{selected.name}</h3></div><Action className="dag-delete-node" tone="danger" disabled={editor.steps.length <= 1} onClick={deleteNode}>{t("workflow.deleteNode")}</Action></div>
          <label>{t("common.nodeID")}<input value={selected.id} onChange={(event) => { const next = event.target.value; renameStep(next); setSelectedID(next); }} /></label>
          <label>{t("worker.name")}<input value={selected.name} onChange={(event) => setStep("name", event.target.value)} /></label>
          <div className="two-fields"><label>{t("common.runtime")}<TUISelect ariaLabel={t("common.runtime")} value={selected.runtime} onChange={(runtime) => setStep("runtime", runtime)} options={runtimes.map((runtime) => ({ value: runtime.id, label: runtime.name }))} /></label><label>{t("common.worker")}<TUISelect ariaLabel={t("common.worker")} value={selected.workerId || "local"} onChange={(workerId) => setStep("workerId", workerId)} options={workerOptions.map((worker) => ({ value: worker.id, label: worker.name }))} /></label></div>
          <label>{t("common.sandbox")}<TUISelect ariaLabel={t("common.sandbox")} value={selected.sandbox || "read-only"} onChange={(sandbox) => setStep("sandbox", sandbox)} options={[{ value: "read-only", label: t("workspace.readOnly") }, { value: "workspace-write", label: t("workspace.write") }, { value: "full", label: t("workspace.fullDanger") }]} /></label>
          <label>{t("workflow.rolePrompt")}<textarea value={selected.rolePrompt} onChange={(event) => setStep("rolePrompt", event.target.value)} /></label><label>{t("workflow.nodeInstruction")}<textarea value={selected.instruction} onChange={(event) => setStep("instruction", event.target.value)} /></label>
          <div className="transition-editor"><span>{t("workflow.terminalSignals")}</span>{Object.entries(selected.transitions || {}).map(([signal, target]) => <div className="transition-row" key={signal}><input value={signal} onChange={(event) => updateSignal(signal, event.target.value, target)} /><span>→</span><TUISelect ariaLabel={`${signal} target`} value={target} onChange={(nextTarget) => updateSignal(signal, signal, nextTarget)} options={[{ value: "$done", label: "$done" }, { value: "$pause", label: "$pause" }, { value: "$fail", label: "$fail" }]} /></div>)}</div>
        </> : null}</aside>
      </div>
      <div className={`dag-validation ${validation.length ? "has-errors" : ""}`}><button className="text-button" onClick={validateEditor}>{t("workflow.validateDag")}</button>{validation.length ? validation.map((issue) => <span key={`${issue.path}-${issue.code}`}><code>{issue.path}</code>{issue.message}</span>) : <span>{t("workflow.dagValidationHint")}</span>}</div>
    </div>
  </Modal>;
}
