import { Bot, ChevronDown, Workflow } from "lucide-react";
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
} from "../runtimeHarnesses.js";

export default function TaskExecutorSelector({ form, workflows = [], runtimes = [], onChange }) {
  const { t } = useTranslation();
  const directAgent = form.workflowId === directAgentWorkflowID;
  const selectedHarness = runtimeHarness(form.harness || "codex");
  const selectedWorkflow = workflows.find((workflow) => workflow.id === form.workflowId);
  const selection = taskExecutionTarget(form);
  const label = directAgent
    ? t("task.agentLabel", { name: selectedHarness.label })
    : t("task.workflowTargetLabel", { name: selectedWorkflow?.name || t("task.chooseWorkflow") });
  const availableWorkflows = workflows.filter((workflow) => workflow.id !== directAgentWorkflowID);
  const harnessOptions = runtimeHarnessOptions(runtimes, t("task.harnessUnavailable"));
  const SelectedIcon = directAgent ? Bot : Workflow;

  const selectTarget = (target) => onChange?.((current) => selectTaskExecutionTarget(current, target));

  return <DropdownMenu>
    <DropdownMenuTrigger asChild>
      <Button type="button" variant="ghost" className="new-task-select executor" aria-label={t("task.executionTarget")} title={t("task.executionTarget")}>
        <SelectedIcon size={14} aria-hidden="true" />
        <span>{label}</span>
        <ChevronDown size={14} aria-hidden="true" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent className="task-executor-menu" side="top" align="start" sideOffset={8}>
      <DropdownMenuRadioGroup value={selection} onValueChange={selectTarget}>
        <DropdownMenuLabel className="task-executor-section">{t("task.agentMode")}</DropdownMenuLabel>
        {harnessOptions.map((option) => <DropdownMenuRadioItem className="task-executor-option agent" value={`agent:${option.value}`} disabled={option.disabled} key={`agent:${option.value}`}>
          <Bot size={14} aria-hidden="true" />
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
