import { useTranslation } from "react-i18next";
import { Action, Kicker, ModeBadge } from "../../../ui/primitives.jsx";
import { loopTemplate, dagTemplate } from "../../templates.js";

export default function WorkflowLibrary({ workflows, runtimes, openEditor }) {
  const { t } = useTranslation();
  const path = (workflow) => {
    if (workflow.mode === "dag") {
      const roots = (workflow.steps || []).filter((step) => !(step.dependsOn || []).length).map((step) => step.name);
      const joins = (workflow.steps || []).filter((step) => (step.dependsOn || []).length).map((step) => step.name);
      return `${roots.join(" ∥ ")}${joins.length ? ` → ${joins.join(" → ")}` : ""}`;
    }
    return `${(workflow.steps || []).map((step) => step.name).join(" → ")}${workflow.steps?.length > 1 ? " ↺" : " → $done"}`;
  };
  return <div className="library-page">
    <div className="library-hero"><div><Kicker>{t("workflow.libraryKicker")}</Kicker><h2>{t("workflow.title")}</h2></div><div className="hero-actions"><Action onClick={() => openEditor(loopTemplate)}>+ {t("workflow.loop")}</Action><Action tone="primary" onClick={() => openEditor(dagTemplate)}>+ {t("workflow.parallelDag")}</Action></div></div>
    <div className="workflow-grid">{workflows.map((workflow) => <button className="workflow-card" key={workflow.id} onClick={() => openEditor(workflow)}><ModeBadge mode={workflow.mode || "serial"}>{t(workflow.mode === "dag" ? "workflow.modeDag" : "workflow.modeSerial")}</ModeBadge><strong>{workflow.name}</strong><span className="workflow-path">{path(workflow)}</span><small>{workflow.mode === "dag" ? t("workflow.nodeSummary", { count: workflow.steps?.length || 0 }) : t("workflow.stepSummary", { count: workflow.steps?.length || 0, max: workflow.policy?.maxTransitions || 20 })}</small><b>[ {t("common.edit")} ]</b></button>)}</div>
    <div className="runtime-callout">{t("workflow.runtimeNotice")}{runtimes.some((runtime) => !runtime.available) && <strong>{t("workflow.missingRuntime")}</strong>}</div>
  </div>;
}
