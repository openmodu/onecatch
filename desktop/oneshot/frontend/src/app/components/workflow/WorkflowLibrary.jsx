import { Action, Kicker, ModeBadge } from "../../../ui/primitives.jsx";
import { loopTemplate, dagTemplate } from "../../templates.js";

export default function WorkflowLibrary({ workflows, runtimes, openEditor }) {
  const path = (workflow) => {
    if (workflow.mode === "dag") {
      const roots = (workflow.steps || []).filter((step) => !(step.dependsOn || []).length).map((step) => step.name);
      const joins = (workflow.steps || []).filter((step) => (step.dependsOn || []).length).map((step) => step.name);
      return `${roots.join(" ∥ ")}${joins.length ? ` → ${joins.join(" → ")}` : ""}`;
    }
    return `${(workflow.steps || []).map((step) => step.name).join(" → ")}${workflow.steps?.length > 1 ? " ↺" : " → $done"}`;
  };
  return <div className="library-page">
    <div className="library-hero"><div><Kicker>loops &amp; parallel dag</Kicker><h2>Workflow</h2></div><div className="hero-actions"><Action onClick={() => openEditor(loopTemplate)}>+ loop</Action><Action tone="primary" onClick={() => openEditor(dagTemplate)}>+ 并行 dag</Action></div></div>
    <div className="workflow-grid">{workflows.map((workflow) => <button className="workflow-card" key={workflow.id} onClick={() => openEditor(workflow)}><ModeBadge mode={workflow.mode || "serial"} /><strong>{workflow.name}</strong><span className="workflow-path">{path(workflow)}</span><small>{workflow.steps?.length || 0} {workflow.mode === "dag" ? "nodes · all join" : `${workflow.steps?.length === 1 ? "step" : "steps"} · ${workflow.policy?.maxTransitions || 20} max`}</small><b>[ edit ]</b></button>)}</div>
    <div className="runtime-callout">Workflow 不会自动替换缺失的 runtime，以免破坏角色语义。{runtimes.some((runtime) => !runtime.available) && <strong>存在缺失 Runtime</strong>}</div>
  </div>;
}
