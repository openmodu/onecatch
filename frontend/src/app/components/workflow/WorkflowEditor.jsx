import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Action, TUISelect } from "../../../ui/primitives.jsx";
import { nextWorkflowItemID } from "../../workflowIds.js";
import { assignWorkflowWorker, isRemoteWorker } from "../../workflowWorker.js";
import DAGWorkflowEditor from "./DAGWorkflowEditor.jsx";
import WorkflowIdentityFields from "./WorkflowIdentityFields.jsx";

export default function WorkflowEditor({ editor, setEditor, validation, validateEditor, saveWorkflow, busy, updateStep, updateTransition, removeTransition, runtimes, workers, defaultSandbox, allowFullSandbox, onClose, showBack = false }) {
  const { t } = useTranslation();
  const [previewOpen, setPreviewOpen] = useState(false);
  if (editor.mode === "dag") return <DAGWorkflowEditor editor={editor} setEditor={setEditor} validation={validation} validateEditor={validateEditor} saveWorkflow={saveWorkflow} busy={busy} runtimes={runtimes} workers={workers} defaultSandbox={defaultSandbox} allowFullSandbox={allowFullSandbox} onClose={onClose} embedded={!showBack} />;
  const workerOptions = [{ value: "local", label: t("common.local") }, ...workers.filter((worker) => worker.enabled).map((worker) => ({ value: worker.id, label: worker.name }))];
  const updateWorker = (stepIndex, workerId) => setEditor((current) => ({ ...current, steps: current.steps.map((step, index) => index === stepIndex ? assignWorkflowWorker(step, workerId) : step) }));
  const addStep = () => setEditor((current) => {
    const id = nextWorkflowItemID("step", current.steps);
    return { ...current, steps: [...current.steps, { id, name: t("workflow.newStep"), runtime: "codex", model: "", workerId: "local", sandbox: defaultSandbox, rolePrompt: t("workflow.defaultRole"), instruction: t("workflow.defaultInstruction"), transitions: { completed: "$done" } }] };
  });
  const removeStep = (index) => setEditor((current) => ({ ...current, steps: current.steps.filter((_, itemIndex) => itemIndex !== index) }));
  return <section className="workflow-editor-surface">
    <header className="serial-editor-header">
      <div className={`serial-editor-actions ${showBack ? "" : "serial-editor-actions--compact"}`}>
        {showBack && <Action onClick={onClose}>&lt; {t("common.back")}</Action>}
        <div>
          <Action
            onClick={() => setPreviewOpen((open) => !open)}
          >
            {t("workflow.preview")}
          </Action>
          <Action tone="cyan" onClick={validateEditor}>{t("workflow.validate")}</Action>
          <Action tone="primary" disabled={busy === "workflow"} onClick={saveWorkflow}>{busy === "workflow" ? t("common.saving") : t("common.save")}</Action>
        </div>
      </div>
      <div className="serial-editor-basics">
        <WorkflowIdentityFields editor={editor} setEditor={setEditor} validation={validation} />
      </div>
    </header>
    <div className="editor-main">
      <div className="step-editor-list">{editor.steps.map((step, stepIndex) => <section className="step-editor" key={`${step.id}-${stepIndex}`}>
      <div className="step-editor-head">
        <b>{String(stepIndex + 1).padStart(2, "0")}</b>
        <Action size="compact" tone="danger" disabled={editor.steps.length <= 1} onClick={() => removeStep(stepIndex)}>{t("workflow.deleteStep")}</Action>
      </div>
      <div className="step-editor-fields">
        <label className="step-field"><span>{t("workflow.stepName")}</span><input value={step.name} onChange={(event) => updateStep(stepIndex, "name", event.target.value)} /></label>
        <label className="step-field"><span>{t("common.runtime")}</span><TUISelect ariaLabel={t("common.runtime")} value={step.runtime} onChange={(runtime) => updateStep(stepIndex, "runtime", runtime)} options={runtimes.map((runtime) => ({ value: runtime.id, label: runtime.id, meta: runtime.available ? "" : t("common.missing") }))} /></label>
        <label className="step-field"><span>{t("common.worker")}</span><TUISelect ariaLabel={t("common.worker")} value={step.workerId || "local"} onChange={(workerId) => updateWorker(stepIndex, workerId)} options={workerOptions} /></label>
        <label className="step-field"><span>{t("common.sandbox")}</span><TUISelect ariaLabel={t("common.sandbox")} value={step.sandbox || "workspace-write"} onChange={(sandbox) => updateStep(stepIndex, "sandbox", sandbox)} options={[{ value: "read-only", label: t("workspace.readOnly") }, { value: "workspace-write", label: t("workspace.write") }, { value: "full", label: t("workspace.fullDanger"), disabled: isRemoteWorker(step.workerId) || (!allowFullSandbox && step.sandbox !== "full") }]} /></label>
        <label className="step-field step-field--multiline"><span>{t("workflow.rolePrompt")}</span><textarea value={step.rolePrompt} onChange={(event) => updateStep(stepIndex, "rolePrompt", event.target.value)} /></label>
        <label className="step-field step-field--multiline"><span>{t("workflow.stepInstruction")}</span><textarea value={step.instruction} onChange={(event) => updateStep(stepIndex, "instruction", event.target.value)} /></label>
      </div>
      <div className="transition-editor">
        <span>{t("workflow.transitions")}</span>
        {Object.entries(step.transitions || {}).map(([signal, target], transitionIndex) => <section className="transition-item" key={signal}>
          <div className="transition-item-head"><strong>{t("workflow.transitionNumber", { number: transitionIndex + 1 })}</strong><Action size="compact" tone="danger" onClick={() => removeTransition(stepIndex, signal)}>{t("common.delete")}</Action></div>
          <label className="step-field"><span>{t("workflow.signal")}</span><input value={signal} onChange={(event) => updateTransition(stepIndex, signal, event.target.value, target)} /></label>
          <label className="step-field"><span>{t("workflow.target")}</span><input value={target} onChange={(event) => updateTransition(stepIndex, signal, signal, event.target.value)} /></label>
        </section>)}
        <Action size="compact" onClick={() => updateTransition(stepIndex, `signal_${Object.keys(step.transitions || {}).length + 1}`, `signal_${Object.keys(step.transitions || {}).length + 1}`, "$done")}>+ {t("workflow.addSignal")}</Action>
      </div>
    </section>)}</div>
      <Action className="add-step" onClick={addStep}>+ {t("workflow.addStep")}</Action>
    </div>
    <Dialog open={previewOpen} onOpenChange={setPreviewOpen}>
      <DialogContent className="workflow-preview-dialog w-[440px] max-w-[calc(100vw-2rem)] max-h-[calc(100vh-4rem)] overflow-hidden p-0 sm:max-w-[440px]">
        <DialogHeader className="px-6 pt-6 pr-12">
          <DialogTitle>{t("workflow.preview")}</DialogTitle>
          <DialogDescription>{editor.name} · {editor.id}</DialogDescription>
        </DialogHeader>
        <div className="min-h-0 overflow-y-auto px-6 pb-6">
          <pre className="flow-ascii">{editor.steps.map((step, index) => `${index ? "│\n" : ""}┌─ ${index + 1} ${step.name}      ${step.runtime}\n${Object.entries(step.transitions || {}).map(([signal, target]) => `│  ${signal} → ${target}`).join("\n")}`).join("\n")}</pre>
          <div className="policy-box"><span>{t("workflow.policy")}</span><p>{t("workflow.maxTransitions")} <b>{editor.policy?.maxTransitions || 20}</b></p><p>{t("workflow.maxFailures")} <b>{editor.policy?.maxConsecutiveFailures || 3}</b></p><p>{t("workflow.stepTimeout")} <b>{editor.policy?.stepTimeoutSeconds || 1800}s</b></p></div>
          <div className={`validation-box ${validation.length ? "has-errors" : "valid"}`}><div><strong>{t("workflow.validation")}</strong><b>{validation.length ? t("workflow.errorsCount", { count: validation.length }) : t("workflow.ok")}</b></div>{validation.map((issue, index) => <p key={`${issue.path}-${index}`}><code>{issue.path}</code>{issue.message}</p>)}{!validation.length && <p>{t("workflow.validationHint")}</p>}</div>
        </div>
      </DialogContent>
    </Dialog>
  </section>;
}
