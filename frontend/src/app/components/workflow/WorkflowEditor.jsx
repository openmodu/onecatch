import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ArrowLeft, CheckCircle2, Eye, Plus, Save, Trash2 } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Textarea } from "@/components/ui/textarea";
import { nextWorkflowItemID } from "../../workflowIds.js";
import { assignWorkflowWorker, isRemoteWorker } from "../../workflowWorker.js";
import { enabledRuntimeInfos } from "../../runtimeHarnesses.js";
import { SettingsField, SettingsSelect } from "../settings/SettingsControls.jsx";
import DAGWorkflowEditor from "./DAGWorkflowEditor.jsx";
import WorkflowIdentityFields from "./WorkflowIdentityFields.jsx";

export default function WorkflowEditor({ editor, setEditor, validation, validateEditor, saveWorkflow, busy, updateStep, updateTransition, removeTransition, runtimes, workers, defaultSandbox, allowFullSandbox, onClose, showBack = false }) {
  const { t } = useTranslation();
  const [previewOpen, setPreviewOpen] = useState(false);
  const runtimeOptions = enabledRuntimeInfos(runtimes, {}, editor.steps.map((step) => step.runtime));
  const defaultRuntime = runtimeOptions.find((runtime) => !runtime.disabled)?.id || "";
  if (editor.mode === "dag") return <DAGWorkflowEditor editor={editor} setEditor={setEditor} validation={validation} validateEditor={validateEditor} saveWorkflow={saveWorkflow} busy={busy} runtimes={runtimes} workers={workers} defaultSandbox={defaultSandbox} allowFullSandbox={allowFullSandbox} onClose={onClose} embedded={!showBack} />;
  const workerOptions = [{ value: "local", label: t("common.local") }, ...workers.filter((worker) => worker.enabled).map((worker) => ({ value: worker.id, label: worker.name }))];
  const updateWorker = (stepIndex, workerId) => setEditor((current) => ({ ...current, steps: current.steps.map((step, index) => index === stepIndex ? assignWorkflowWorker(step, workerId) : step) }));
  const addStep = () => setEditor((current) => {
    const id = nextWorkflowItemID("step", current.steps);
    return { ...current, steps: [...current.steps, { id, name: t("workflow.newStep"), runtime: defaultRuntime, model: "", workerId: "local", sandbox: defaultSandbox, rolePrompt: t("workflow.defaultRole"), instruction: t("workflow.defaultInstruction"), transitions: { completed: "$done" } }] };
  });
  const removeStep = (index) => setEditor((current) => ({ ...current, steps: current.steps.filter((_, itemIndex) => itemIndex !== index) }));

  return <section className="workflow-editor-surface select-none bg-background">
    <header className="shrink-0 border-b border-border/80 bg-background/95 px-5 py-4">
      <div className="mb-4 flex items-center justify-between gap-4">
        <div>{showBack && <Button variant="ghost" size="sm" onClick={onClose}><ArrowLeft aria-hidden="true" />{t("common.back")}</Button>}</div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          <Button variant="outline" size="sm" onClick={() => setPreviewOpen(true)}><Eye aria-hidden="true" />{t("workflow.preview")}</Button>
          <Button variant="outline" size="sm" onClick={validateEditor}><CheckCircle2 aria-hidden="true" />{t("workflow.validate")}</Button>
          <Button size="sm" disabled={busy === "workflow"} onClick={saveWorkflow}><Save aria-hidden="true" />{busy === "workflow" ? t("common.saving") : t("common.save")}</Button>
        </div>
      </div>
      <div className="rounded-xl border border-border/80 bg-card/72 p-4 shadow-xs">
        <WorkflowIdentityFields editor={editor} setEditor={setEditor} validation={validation} />
      </div>
    </header>

    <ScrollArea className="min-h-0 flex-1">
      <div className="mx-auto grid max-w-4xl gap-4 px-5 py-5">
        {editor.steps.map((step, stepIndex) => <Card className="gap-0 overflow-hidden py-0 shadow-none" key={`${step.id}-${stepIndex}`}>
          <div className="flex items-center justify-between gap-4 border-b border-border/75 bg-muted/35 px-4 py-3">
            <div className="flex min-w-0 items-center gap-3">
              <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground">{stepIndex + 1}</span>
              <div className="min-w-0">
                <strong className="block truncate text-sm font-semibold text-foreground">{step.name || step.id}</strong>
                <code className="block truncate text-[11px] text-muted-foreground">{step.id}</code>
              </div>
            </div>
            <Button variant="ghost" size="sm" className="text-destructive hover:bg-destructive/10 hover:text-destructive" disabled={editor.steps.length <= 1} onClick={() => removeStep(stepIndex)}><Trash2 aria-hidden="true" />{t("workflow.deleteStep")}</Button>
          </div>
          <CardContent className="grid gap-5 p-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <SettingsField label={t("workflow.stepName")}><Input value={step.name} onChange={(event) => updateStep(stepIndex, "name", event.target.value)} /></SettingsField>
              <SettingsField label={t("common.runtime")}><SettingsSelect ariaLabel={t("common.runtime")} value={step.runtime} onChange={(runtime) => updateStep(stepIndex, "runtime", runtime)} options={runtimeOptions.map((runtime) => ({ value: runtime.id, label: runtime.name || runtime.id, meta: runtime.available ? "" : t("common.missing"), disabled: runtime.disabled }))} /></SettingsField>
              <SettingsField label={t("common.worker")}><SettingsSelect ariaLabel={t("common.worker")} value={step.workerId || "local"} onChange={(workerId) => updateWorker(stepIndex, workerId)} options={workerOptions} /></SettingsField>
              <SettingsField label={t("common.sandbox")}><SettingsSelect ariaLabel={t("common.sandbox")} value={step.sandbox || "workspace-write"} onChange={(sandbox) => updateStep(stepIndex, "sandbox", sandbox)} options={[{ value: "read-only", label: t("workspace.readOnly") }, { value: "workspace-write", label: t("workspace.write") }, { value: "full", label: t("workspace.fullDanger"), disabled: isRemoteWorker(step.workerId) || (!allowFullSandbox && step.sandbox !== "full") }]} /></SettingsField>
              <SettingsField className="sm:col-span-2" label={t("workflow.rolePrompt")}><Textarea className="min-h-20 select-text resize-y" value={step.rolePrompt} onChange={(event) => updateStep(stepIndex, "rolePrompt", event.target.value)} /></SettingsField>
              <SettingsField className="sm:col-span-2" label={t("workflow.stepInstruction")}><Textarea className="min-h-24 select-text resize-y" value={step.instruction} onChange={(event) => updateStep(stepIndex, "instruction", event.target.value)} /></SettingsField>
            </div>

            <section className="rounded-xl border border-border/75 bg-muted/35 p-3.5">
              <div className="mb-3 flex items-center justify-between gap-3">
                <strong className="text-xs font-semibold text-foreground">{t("workflow.transitions")}</strong>
                <Button variant="outline" size="xs" onClick={() => updateTransition(stepIndex, `signal_${Object.keys(step.transitions || {}).length + 1}`, `signal_${Object.keys(step.transitions || {}).length + 1}`, "$done")}><Plus aria-hidden="true" />{t("workflow.addSignal")}</Button>
              </div>
              <div className="grid gap-2.5">
                {Object.entries(step.transitions || {}).map(([signal, target], transitionIndex) => <section className="rounded-lg border border-border/75 bg-background/80 p-3" key={signal}>
                  <div className="mb-3 flex items-center justify-between gap-3">
                    <Badge variant="secondary">{t("workflow.transitionNumber", { number: transitionIndex + 1 })}</Badge>
                    <Button variant="ghost" size="xs" className="text-destructive hover:bg-destructive/10 hover:text-destructive" onClick={() => removeTransition(stepIndex, signal)}><Trash2 aria-hidden="true" />{t("common.delete")}</Button>
                  </div>
                  <div className="grid gap-3 sm:grid-cols-2">
                    <SettingsField label={t("workflow.signal")}><Input className="font-mono" value={signal} onChange={(event) => updateTransition(stepIndex, signal, event.target.value, target)} /></SettingsField>
                    <SettingsField label={t("workflow.target")}><Input className="font-mono" value={target} onChange={(event) => updateTransition(stepIndex, signal, signal, event.target.value)} /></SettingsField>
                  </div>
                </section>)}
              </div>
            </section>
          </CardContent>
        </Card>)}
        <Button variant="outline" className="h-11 border-dashed" onClick={addStep}><Plus aria-hidden="true" />{t("workflow.addStep")}</Button>
      </div>
    </ScrollArea>

    <Dialog open={previewOpen} onOpenChange={setPreviewOpen}>
      <DialogContent className="workflow-preview-dialog max-h-[calc(100vh-4rem)] select-none overflow-hidden p-0 sm:max-w-lg">
        <DialogHeader className="px-6 pt-6 pr-12">
          <DialogTitle>{t("workflow.preview")}</DialogTitle>
          <DialogDescription>{editor.name} · {editor.id}</DialogDescription>
        </DialogHeader>
        <div className="grid min-h-0 gap-4 overflow-y-auto px-6 pb-6">
          <pre className="m-0 overflow-x-auto rounded-xl border bg-muted/45 p-4 font-mono text-xs leading-7 text-muted-foreground">{editor.steps.map((step, index) => `${index ? "│\n" : ""}┌─ ${index + 1} ${step.name}      ${step.runtime}\n${Object.entries(step.transitions || {}).map(([signal, target]) => `│  ${signal} → ${target}`).join("\n")}`).join("\n")}</pre>
          <section className="rounded-xl border bg-card p-4">
            <h3 className="mb-3 text-sm font-semibold text-foreground">{t("workflow.policy")}</h3>
            <dl className="grid gap-2.5 text-xs">
              <div className="flex justify-between gap-4"><dt className="text-muted-foreground">{t("workflow.maxTransitions")}</dt><dd className="font-mono font-medium">{editor.policy?.maxTransitions || 20}</dd></div>
              <div className="flex justify-between gap-4"><dt className="text-muted-foreground">{t("workflow.maxFailures")}</dt><dd className="font-mono font-medium">{editor.policy?.maxConsecutiveFailures || 3}</dd></div>
              <div className="flex justify-between gap-4"><dt className="text-muted-foreground">{t("workflow.stepTimeout")}</dt><dd className="font-mono font-medium">{editor.policy?.stepTimeoutSeconds || 1800}s</dd></div>
            </dl>
          </section>
          <section className={`rounded-xl border p-4 text-xs ${validation.length ? "border-destructive/30 bg-destructive/5 text-destructive" : "border-success/30 bg-success/5 text-muted-foreground"}`}>
            <div className="flex items-center justify-between gap-3"><strong className="text-foreground">{t("workflow.validation")}</strong><Badge variant={validation.length ? "destructive" : "secondary"}>{validation.length ? t("workflow.errorsCount", { count: validation.length }) : t("workflow.ok")}</Badge></div>
            {validation.map((issue, index) => <p className="mt-2 mb-0" key={`${issue.path}-${index}`}><code className="mr-1 font-mono">{issue.path}</code>{issue.message}</p>)}
            {!validation.length && <p className="mt-2 mb-0">{t("workflow.validationHint")}</p>}
          </section>
        </div>
      </DialogContent>
    </Dialog>
  </section>;
}
