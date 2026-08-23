import { ChevronDown, Workflow } from "lucide-react";
import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  directAgentWorkflowID,
  runtimeHarness,
  runtimeHarnessOptions,
  selectTaskExecutionTarget,
  taskExecutionTarget,
  workflowHarnessesEnabled,
} from "../runtimeHarnesses.js";
import RuntimeHarnessIcon from "./RuntimeHarnessIcon.jsx";

export default function TaskExecutorSelector({ form, workflows = [], runtimes = [], runtimeSettings = {}, remoteFS = false, onChange }) {
  const { t } = useTranslation();
  const directAgent = form.workflowId === directAgentWorkflowID;
  const selectedHarness = runtimeHarness(form.harness || "codex");
  const selectedWorkflow = workflows.find((workflow) => workflow.id === form.workflowId);
  const selection = taskExecutionTarget(form);
  const availableWorkflows = workflows.filter((workflow) => workflow.id !== directAgentWorkflowID && workflowHarnessesEnabled(workflow, runtimes, runtimeSettings, remoteFS));
  const harnessOptions = runtimeHarnessOptions(runtimes, t("task.harnessUnavailable"), runtimeSettings, remoteFS);
  const selectedHarnessEnabled = harnessOptions.some((option) => option.value === selectedHarness.id);
  const label = directAgent
    ? selectedHarnessEnabled ? selectedHarness.label : t("task.noHarnessEnabled")
    : selectedWorkflow?.name || t("task.chooseWorkflow");
  const selectTarget = (target) => onChange?.((current) => selectTaskExecutionTarget(current, target));

  useEffect(() => {
    if (directAgent && harnessOptions.some((option) => option.value === form.harness)) return;
    if (!directAgent && availableWorkflows.some((workflow) => workflow.id === form.workflowId)) return;
    const nextHarness = harnessOptions.find((option) => !option.disabled) || harnessOptions[0];
    const nextTarget = directAgent
      ? nextHarness ? `agent:${nextHarness.value}` : availableWorkflows[0] ? `workflow:${availableWorkflows[0].id}` : ""
      : availableWorkflows[0] ? `workflow:${availableWorkflows[0].id}` : nextHarness ? `agent:${nextHarness.value}` : "";
    if (nextTarget) selectTarget(nextTarget);
  }, [availableWorkflows, directAgent, form.harness, form.workflowId, harnessOptions]);

  return <DropdownMenu>
    <DropdownMenuTrigger asChild>
      <Button type="button" variant="ghost" className="new-task-select executor" aria-label={t("task.executionTarget")} title={t("task.executionTarget")} disabled={!harnessOptions.length && !availableWorkflows.length}>
        {directAgent
          ? selectedHarnessEnabled ? <RuntimeHarnessIcon harness={selectedHarness.id} size={14} aria-hidden="true" /> : null
          : <Workflow size={14} aria-hidden="true" />}
        <span>{label}</span>
        <ChevronDown size={14} aria-hidden="true" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent className="task-executor-menu" side="top" align="start" sideOffset={8}>
      <DropdownMenuRadioGroup value={selection} onValueChange={selectTarget}>
        <DropdownMenuLabel className="task-executor-section">{t("task.agentMode")}</DropdownMenuLabel>
        {harnessOptions.map((option) => <DropdownMenuRadioItem className="task-executor-option agent" value={`agent:${option.value}`} disabled={option.disabled} key={`agent:${option.value}`}>
          <RuntimeHarnessIcon harness={option.value} size={14} aria-hidden="true" />
          <span><strong>{option.label}</strong>{option.meta && <small>{option.meta}</small>}</span>
        </DropdownMenuRadioItem>)}
        {availableWorkflows.length > 0 && <DropdownMenuSeparator />}
        {availableWorkflows.length > 0 && <DropdownMenuLabel className="task-executor-section">{t("task.workflowMode")}</DropdownMenuLabel>}
        {availableWorkflows.map((workflow) => <DropdownMenuRadioItem className="task-executor-option" value={`workflow:${workflow.id}`} key={`workflow:${workflow.id}`}>
          <Workflow size={14} aria-hidden="true" />
          <span><strong>{workflow.name}</strong><small>{workflow.description || t("task.workflowModeDescription")}</small></span>
        </DropdownMenuRadioItem>)}
      </DropdownMenuRadioGroup>
    </DropdownMenuContent>
  </DropdownMenu>;
}
