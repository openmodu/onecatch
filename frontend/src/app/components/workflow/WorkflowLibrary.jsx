import { useTranslation } from "react-i18next";
import { ChevronLeft, ChevronRight, GitBranch, MoreHorizontal, Pencil, Plus, Trash2, Workflow } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ScrollArea } from "@/components/ui/scroll-area";
import { dagTemplate, loopTemplate } from "../../templates.js";
import { directAgentWorkflowID } from "../../runtimeHarnesses.js";
import AuxWindowCloseButton from "../../AuxWindowCloseButton.jsx";
import { usesCompactAuxiliaryChrome } from "../../platform.js";

function workflowPath(workflow) {
  if (workflow?.mode === "dag") {
    const roots = (workflow.steps || []).filter((step) => !(step.dependsOn || []).length).map((step) => step.name);
    const joins = (workflow.steps || []).filter((step) => (step.dependsOn || []).length).map((step) => step.name);
    return `${roots.join(" ∥ ")}${joins.length ? ` → ${joins.join(" → ")}` : ""}`;
  }
  return `${(workflow?.steps || []).map((step) => step.name).join(" → ")}${workflow?.steps?.length > 1 ? " ↺" : " → $done"}`;
}

function WorkflowListItem({ workflow, selected, busy, onSelect, onEdit, onDelete }) {
  const { t } = useTranslation();
  const summary = workflow.mode === "dag"
    ? t("workflow.nodeSummary", { count: workflow.steps?.length || 0 })
    : t("workflow.stepSummary", { count: workflow.steps?.length || 0, max: workflow.policy?.maxTransitions || 20 });

  return <div className={`group grid grid-cols-[minmax(0,1fr)_auto] items-center rounded-lg px-1 py-1 ${selected ? "bg-accent text-accent-foreground" : "text-muted-foreground hover:bg-background/45 hover:text-foreground"}`}>
    <Button variant="ghost" className="h-auto min-w-0 justify-start rounded-md px-2 py-2 text-left hover:bg-transparent" aria-current={selected ? "page" : undefined} onClick={onSelect}>
      <span className="min-w-0">
        <strong className="block truncate text-[13px] font-medium text-foreground">{workflow.name || workflow.id}</strong>
        <small className="mt-0.5 block truncate text-[11px] font-normal text-muted-foreground">{summary}</small>
      </span>
    </Button>
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon-xs" className={`mr-1 shrink-0 ${selected ? "opacity-100" : "opacity-0 group-hover:opacity-100 group-focus-within:opacity-100"}`} aria-label={t("workflow.editNamed", { name: workflow.name })}>
          <MoreHorizontal aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent side="right" align="start" sideOffset={8} collisionPadding={12}>
        <DropdownMenuItem onSelect={onEdit}><Pencil aria-hidden="true" />{t("common.edit")}</DropdownMenuItem>
        <DropdownMenuItem variant="destructive" disabled={busy === "delete-workflow"} onSelect={onDelete}><Trash2 aria-hidden="true" />{t("common.delete")}</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  </div>;
}

function WorkflowDetail({ workflow, runtimes, onEdit }) {
  const { t } = useTranslation();
  if (!workflow) return <div className="flex h-full select-none items-center justify-center px-8 text-center text-sm text-muted-foreground">{t("workflow.runtimeNotice")}</div>;

  const isDag = workflow.mode === "dag";
  const missingRuntime = (workflow.steps || []).some((step) => {
    const runtime = runtimes.find((item) => item.id === step.runtime);
    return !runtime?.available;
  });

  return <ScrollArea className="min-h-0 flex-1">
    <section className="mx-auto max-w-3xl px-8 pt-4 pb-10">
      <header className="mb-7 flex items-start justify-between gap-6">
        <div className="min-w-0">
          <div className="mb-2 flex items-center gap-2">
            <Badge variant="secondary">{t(isDag ? "workflow.modeDag" : "workflow.modeSerial")}</Badge>
            <code className="text-xs text-muted-foreground">{workflow.id}</code>
          </div>
          <h1 className="m-0 truncate text-2xl font-semibold tracking-tight text-foreground">{workflow.name || workflow.id}</h1>
          <p className="mt-2 mb-0 text-sm leading-relaxed text-muted-foreground">{workflowPath(workflow)}</p>
        </div>
        <Button variant="outline" className="shrink-0" onClick={onEdit}><Pencil aria-hidden="true" />{t("common.edit")}</Button>
      </header>

      <div className="grid gap-6">
        <section aria-labelledby="workflow-flow-heading">
          <div className="mb-3 flex items-center gap-2">
            {isDag ? <GitBranch className="size-4 text-muted-foreground" aria-hidden="true" /> : <Workflow className="size-4 text-muted-foreground" aria-hidden="true" />}
            <h2 className="text-[15px] font-semibold text-foreground" id="workflow-flow-heading">{t("workflow.preview")}</h2>
          </div>
          <div className="grid gap-2">
            {(workflow.steps || []).map((step, index) => <article className="rounded-xl border bg-card px-4 py-3.5" key={`${step.id}-${index}`}>
              <div className="flex items-start gap-3">
                <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">{index + 1}</span>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                    <strong className="text-sm font-semibold text-foreground">{step.name || step.id}</strong>
                    <Badge variant="outline" className="font-mono text-[10px]">{step.runtime || "—"}</Badge>
                  </div>
                  {isDag && <p className="mt-1.5 mb-0 text-xs text-muted-foreground">{(step.dependsOn || []).length ? t("workflow.waitDependencies", { count: step.dependsOn.length }) : t("workflow.rootParallel")}</p>}
                  {!isDag && <div className="mt-2 flex flex-wrap gap-1.5">{Object.entries(step.transitions || {}).map(([signal, target]) => <code className="rounded-md bg-muted px-2 py-1 text-[11px] text-muted-foreground" key={signal}>{signal} → {target}</code>)}</div>}
                  {step.instruction && <p className="mt-2 mb-0 line-clamp-3 text-xs leading-relaxed text-muted-foreground">{step.instruction}</p>}
                </div>
              </div>
            </article>)}
          </div>
        </section>

        <section className="rounded-xl border bg-card p-4">
          <h2 className="mb-3 text-sm font-semibold text-foreground">{t("workflow.policy")}</h2>
          <dl className="grid gap-3 text-xs">
            <div className="flex items-center justify-between gap-3"><dt className="text-muted-foreground">{t("workflow.maxTransitions")}</dt><dd className="font-mono font-medium text-foreground">{workflow.policy?.maxTransitions || 20}</dd></div>
            <div className="flex items-center justify-between gap-3"><dt className="text-muted-foreground">{t("workflow.maxFailures")}</dt><dd className="font-mono font-medium text-foreground">{workflow.policy?.maxConsecutiveFailures || 3}</dd></div>
            <div className="flex items-center justify-between gap-3"><dt className="text-muted-foreground">{t("workflow.stepTimeout")}</dt><dd className="font-mono font-medium text-foreground">{workflow.policy?.stepTimeoutSeconds || 1800}s</dd></div>
          </dl>
        </section>
        <section className={`rounded-xl px-4 py-3 text-xs leading-relaxed ${missingRuntime ? "bg-warning/10 text-warning" : "bg-muted text-muted-foreground"}`}>
          {t("workflow.runtimeNotice")}{missingRuntime && <strong className="mt-1 block">{t("workflow.missingRuntime")}</strong>}
        </section>
      </div>
    </section>
  </ScrollArea>;
}

export default function WorkflowLibrary({ workflows, selectedWorkflow, runtimes, editorContent, busy, onSelect, openEditor, deleteWorkflow, canGoBack = false, canGoForward = false, onGoBack, onGoForward }) {
  const { t } = useTranslation();
  const compactAuxiliaryChrome = usesCompactAuxiliaryChrome();
  const editableWorkflows = workflows.filter((workflow) => workflow.id !== directAgentWorkflowID);
  const activeWorkflow = selectedWorkflow && selectedWorkflow.id !== directAgentWorkflowID ? selectedWorkflow : editableWorkflows[0] || null;
  const newWorkflowMenu = <DropdownMenu>
    <DropdownMenuTrigger asChild><Button variant="outline" size="icon-sm" aria-label={t("workflow.title")}><Plus aria-hidden="true" /></Button></DropdownMenuTrigger>
    <DropdownMenuContent align="end" collisionPadding={12}>
      <DropdownMenuItem onSelect={() => openEditor(loopTemplate, true)}><Workflow aria-hidden="true" />{t("workflow.loop")}</DropdownMenuItem>
      <DropdownMenuItem onSelect={() => openEditor(dagTemplate, true)}><GitBranch aria-hidden="true" />{t("workflow.parallelDag")}</DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>;
  return <div className="workflow-window relative grid h-full min-h-0 select-none grid-cols-[240px_minmax(0,1fr)] overflow-hidden bg-transparent text-foreground">
    {compactAuxiliaryChrome
      ? <div className="workflow-titlebar drag-region absolute inset-x-0 top-0 z-40 grid h-[52px] cursor-default grid-cols-[240px_minmax(0,1fr)] select-none">
        <div className="auxiliary-sidebar-title flex min-w-0 items-center justify-start gap-2 border-r border-border bg-sidebar px-4 text-sm font-bold tracking-[-0.01em] text-foreground/85">
          <span className="pointer-events-none truncate">{t("workflow.title")}</span>
          {(onGoBack || onGoForward) && <div className="no-drag flex items-center gap-0.5">
            <Button variant="ghost" size="icon-sm" disabled={!canGoBack} aria-label={t("common.goBack")} title={t("common.goBack")} onClick={onGoBack}><ChevronLeft strokeWidth={2.5} aria-hidden="true" /></Button>
            <Button variant="ghost" size="icon-sm" disabled={!canGoForward} aria-label={t("common.goForward")} title={t("common.goForward")} onClick={onGoForward}><ChevronRight strokeWidth={2.5} aria-hidden="true" /></Button>
          </div>}
        </div>
        <div className="bg-background/80" />
      </div>
      : <div className="workflow-titlebar drag-region absolute inset-x-0 top-0 z-40 grid h-[52px] cursor-default grid-cols-[240px_minmax(0,1fr)] select-none" aria-hidden="true">
        {/* The rail owns the first column: it draws its own drag strip and puts
            the history controls there, so this caption stays out of its way and
            only claims the content column as draggable window chrome. */}
        <span className="pointer-events-none" />
        <span />
        <strong className="pointer-events-none absolute inset-0 flex items-center justify-center text-sm font-semibold tracking-[-0.01em] text-foreground/85">{t("workflow.title")}</strong>
      </div>}
    <AuxWindowCloseButton />

    <aside className="sidebar workflow-sidebar relative z-30 flex min-h-0 select-none flex-col text-sidebar-foreground [clip-path:inset(8px_4px_8px_8px_round_16px)]" aria-label={t("workflow.title")}>
      {compactAuxiliaryChrome
        ? <>
          <div className="workflow-sidebar-title-spacer h-[60px] shrink-0" aria-hidden="true" />
          <div className="flex shrink-0 items-center justify-end px-4 pb-3">{newWorkflowMenu}</div>
        </>
        : <>
          <div className="drag-region flex h-[52px] shrink-0 cursor-default items-center justify-end px-4">
            {(onGoBack || onGoForward) && <div className="no-drag flex items-center gap-0.5">
              <Button variant="ghost" size="icon-xs" disabled={!canGoBack} aria-label={t("common.goBack")} title={t("common.goBack")} onClick={onGoBack}><ChevronLeft aria-hidden="true" /></Button>
              <Button variant="ghost" size="icon-xs" disabled={!canGoForward} aria-label={t("common.goForward")} title={t("common.goForward")} onClick={onGoForward}><ChevronRight aria-hidden="true" /></Button>
            </div>}
          </div>
          <div className="flex items-center justify-between gap-3 px-4 pt-2 pb-3 pl-5">
            <div className="min-w-0"><strong className="block text-sm font-semibold text-foreground">{t("workflow.title")}</strong><small className="mt-0.5 block truncate text-[11px] text-muted-foreground">{editableWorkflows.length}</small></div>
            {newWorkflowMenu}
          </div>
        </>}
      <ScrollArea className="min-h-0 flex-1 px-3 pb-4">
        <div className="grid gap-0.5">
          {editableWorkflows.map((workflow) => <WorkflowListItem key={workflow.id} workflow={workflow} selected={activeWorkflow?.id === workflow.id} busy={busy} onSelect={() => onSelect ? onSelect(workflow) : openEditor(workflow)} onEdit={() => openEditor(workflow)} onDelete={() => deleteWorkflow(workflow)} />)}
          {!editableWorkflows.length && <p className="px-3 py-6 text-center text-xs leading-relaxed text-muted-foreground">{t("workflow.runtimeNotice")}</p>}
        </div>
      </ScrollArea>
    </aside>

    <main className="flex min-h-0 min-w-0 flex-col bg-background pt-[52px]">
      {editorContent || <WorkflowDetail workflow={activeWorkflow} runtimes={runtimes} onEdit={() => activeWorkflow && openEditor(activeWorkflow)} />}
    </main>
  </div>;
}
