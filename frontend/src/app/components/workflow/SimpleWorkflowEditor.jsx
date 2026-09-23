import { useTranslation } from "react-i18next";
import { ArrowLeft, ArrowRight, Check, Settings2, Save } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { ScrollArea } from "@/components/ui/scroll-area";
import { SettingsField, SettingsSelect } from "../settings/SettingsControls.jsx";
import { setReviewBehavior } from "../../simpleWorkflow.js";

export default function SimpleWorkflowEditor({ editor, setEditor, simple, runtimeOptions, updateStep, validation, saveWorkflow, busy, onAdvanced, onClose, showBack }) {
  const { t } = useTranslation();
  const update = (step, field, value) => updateStep(editor.steps.findIndex((item) => item.id === step.id), field, value);
  const behavior = (retry, rounds) => setEditor((current) => setReviewBehavior(current, retry, rounds));
  return <section className="workflow-editor-surface bg-background">
    <header className="flex shrink-0 items-center justify-between gap-3 border-b border-border px-5 py-4">
      <div className="flex items-center gap-2">{showBack && <Button variant="ghost" size="icon-sm" aria-label={t("common.back")} onClick={onClose}><ArrowLeft /></Button>}<strong className="text-sm">{t("workflow.simple.title")}</strong></div>
      <Button variant="ghost" size="sm" onClick={onAdvanced}><Settings2 size={15} />{t("workflow.simple.advanced")}</Button>
    </header>
    <ScrollArea className="min-h-0 flex-1">
      <div className="mx-auto grid max-w-3xl gap-7 px-5 py-7 sm:px-8">
        <div><h2 className="text-xl font-semibold tracking-tight">{t("workflow.simple.heading")}</h2><p className="mt-2 text-sm leading-6 text-muted-foreground">{t("workflow.simple.description")}</p></div>
        <SettingsField label={t("workflow.name")}><Input aria-label={t("workflow.name")} value={editor.name} onChange={(event) => setEditor((current) => ({ ...current, name: event.target.value }))} /></SettingsField>
        <div className="flex flex-wrap items-center gap-3 rounded-xl bg-muted/50 px-4 py-3 text-sm" aria-label={t("workflow.simple.path")}><span>{t("workflow.simple.implement")}</span><ArrowRight size={15} className="text-muted-foreground" /><span>{t("workflow.simple.review")}</span><ArrowRight size={15} className="text-muted-foreground" /><span className="flex items-center gap-1.5"><Check size={15} />{t("workflow.simple.done")}</span></div>
        <div className="grid gap-5 md:grid-cols-2">
          {[simple.implement, simple.review].map((step, index) => <section key={step.id} className="rounded-xl border bg-card p-5">
            <div className="mb-5 flex items-center gap-3"><span className="flex size-7 items-center justify-center rounded-full bg-muted text-xs font-semibold">{index + 1}</span><h3 className="font-semibold text-sm">{t(index ? "workflow.simple.review" : "workflow.simple.implement")}</h3></div>
            <div className="grid gap-4">
              <SettingsField label={t("workflow.simple.who")}><SettingsSelect ariaLabel={t(index ? "workflow.simple.reviewer" : "workflow.simple.implementer")} value={step.runtime} onChange={(value) => update(step, "runtime", value)} options={runtimeOptions.map((runtime) => ({ value: runtime.id, label: runtime.name || runtime.id, meta: runtime.available ? "" : t("common.missing"), disabled: runtime.disabled }))} /></SettingsField>
              <SettingsField label={t(index ? "workflow.simple.reviewInstruction" : "workflow.simple.implementInstruction")}><Textarea aria-label={t(index ? "workflow.simple.reviewInstruction" : "workflow.simple.implementInstruction")} className="min-h-28 resize-y select-text" value={step.instruction} onChange={(event) => update(step, "instruction", event.target.value)} /></SettingsField>
              <p className="text-xs text-muted-foreground">{t(step.sandbox === "read-only" ? "workspace.readOnly" : step.sandbox === "full" ? "workspace.fullDanger" : "workspace.write")} · {step.workerId && step.workerId !== "local" ? step.workerId : t("common.local")}{step.model ? ` · ${step.model}` : ""}</p>
            </div>
          </section>)}
        </div>
        <section className="grid gap-4 rounded-xl border p-5">
          <div><h3 className="text-sm font-semibold">{t("workflow.simple.ifRejected")}</h3><p className="mt-1 text-xs leading-5 text-muted-foreground">{t("workflow.simple.approvedHint")}</p></div>
          <div className="grid gap-4 sm:grid-cols-2">
            <SettingsField label={t("workflow.simple.action")}><SettingsSelect ariaLabel={t("workflow.simple.ifRejected")} value={simple.retry ? "retry" : "pause"} onChange={(value) => behavior(value === "retry", simple.rounds)} options={[{ value: "retry", label: t("workflow.simple.retry") }, { value: "pause", label: t("workflow.simple.pause") }]} /></SettingsField>
            {simple.retry && <SettingsField label={t("workflow.simple.rounds")}><SettingsSelect ariaLabel={t("workflow.simple.rounds")} value={String(simple.rounds)} onChange={(value) => behavior(true, Number(value))} options={[...new Set([1, 2, 3, 5, 10, simple.rounds])].sort((a,b) => a-b).map((rounds) => ({ value: String(rounds), label: t("workflow.simple.roundCount", { count: rounds }) }))} /></SettingsField>}
          </div>
          <p className="text-xs leading-5 text-muted-foreground">{t(simple.retry ? "workflow.simple.limitHint" : "workflow.simple.pauseHint")}</p>
        </section>
        <details className="text-sm"><summary className="cursor-pointer text-muted-foreground">{t("workflow.simple.more")}</summary><div className="mt-4 grid gap-4">{[simple.implement, simple.review].map((step) => <SettingsField key={step.id} label={`${step.name} · ${t("workflow.rolePrompt")}`}><Textarea aria-label={`${step.name} · ${t("workflow.rolePrompt")}`} value={step.rolePrompt} onChange={(event) => update(step, "rolePrompt", event.target.value)} /></SettingsField>)}<p className="text-xs text-muted-foreground">{t("workflow.simple.advancedHint")}</p></div></details>
        {!!validation.length && <div role="alert" className="rounded-lg bg-destructive/10 p-4 text-sm text-destructive">{validation.map((issue, index) => <p key={index}>{issue.path}: {issue.message}</p>)}</div>}
      </div>
    </ScrollArea>
    <footer className="flex shrink-0 items-center justify-between gap-4 border-t border-border px-5 py-4"><p className="text-xs text-muted-foreground">{t("workflow.simple.saveHint")}</p><Button disabled={busy === "workflow"} onClick={saveWorkflow}><Save size={15} />{t(busy === "workflow" ? "common.saving" : "common.save")}</Button></footer>
  </section>;
}
