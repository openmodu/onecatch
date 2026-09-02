import { useLayoutEffect, useRef } from "react";
import { ArrowUp, ListPlus, Paperclip, Plus, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Textarea } from "@/components/ui/textarea";
import { shouldSubmitComposer } from "../composerKeyboard.js";
import { autosizeComposerTextarea, NEW_TASK_TEXTAREA_MIN_HEIGHT } from "../composerTextarea.js";
import { fileName } from "../format.js";
import { runtimeHarnessEnabled, supportsRuntimeProfile, workflowHarnessesEnabled } from "../runtimeHarnesses.js";
import RuntimeProfileMenu from "./RuntimeProfileMenu.jsx";
import { useSkillPicker } from "./SkillPicker.jsx";
import SkillTextarea from "./SkillTextarea.jsx";
import TaskExecutorSelector from "./TaskExecutorSelector.jsx";
import TaskPermissionSelector from "./TaskPermissionSelector.jsx";
import WorkspaceComposerMeta from "./WorkspaceComposerMeta.jsx";

export default function NewTaskView({
  workspaceID,
  workflows,
  form,
  busy,
  onChange,
  onChooseAttachments,
  onSubmit,
  runtimes,
  runtimeConfiguration,
  runtimeSettings,
  runtimeSettingsByHarness,
  remoteFS = false,
  allowFullSandbox,
  mode,
  workspace,
  onEditWorkspace,
}) {
  const { t } = useTranslation();
  const composing = useRef(false);
  const promptRef = useRef(null);
  const directAgent = form.workflowId === "single_agent";
  const showRuntimeProfile = directAgent && supportsRuntimeProfile(form.harness);
  const targetEnabled = directAgent
    ? runtimeHarnessEnabled(form.harness, runtimes, runtimeSettingsByHarness, remoteFS)
    : workflowHarnessesEnabled(workflows.find((workflow) => workflow.id === form.workflowId), runtimes, runtimeSettingsByHarness, remoteFS);
  const ready = Boolean(workspaceID && form.prompt.trim() && form.workflowId && form.sandbox && (!directAgent || form.harness) && targetEnabled);
  const executionMode = form.executionMode === "queued" ? "queued" : "immediate";
  const executionLabel = executionMode === "queued" ? t("task.joinQueue") : t("task.runNow");
  const submitLabel = busy === "run"
    ? t("task.creating")
    : executionLabel;

  useLayoutEffect(() => {
    autosizeComposerTextarea(promptRef.current, NEW_TASK_TEXTAREA_MIN_HEIGHT);
  }, [form.prompt]);

  const submit = (event) => {
    event.preventDefault();
    if (!event.nativeEvent.isComposing && busy !== "run") void onSubmit();
  };
  const submitFromComposer = (event) => {
    if (shouldSubmitComposer(event, composing.current) && ready && busy !== "run") {
      event.preventDefault();
      void onSubmit();
    }
  };
  const skillRuntime = directAgent && ["codex", "claude"].includes(form.harness) ? form.harness : "";
  const skillPicker = useSkillPicker({
    enabled: Boolean(skillRuntime),
    mode,
    runtime: skillRuntime,
    workspacePath: workspace?.path || "",
    value: form.prompt,
    onValueChange: (prompt) => onChange((current) => ({ ...current, prompt })),
    textareaRef: promptRef,
    onKeyDown: submitFromComposer,
  });

  // overflow-y alone would compute overflow-x to `auto`, so a single control
  // that cannot shrink turns the whole screen into a horizontal scroller.
  return <div className="new-task-screen min-h-0 min-w-0 flex-1 overflow-x-hidden overflow-y-auto">
    <form className="new-task-layout" aria-busy={busy === "run"} onSubmit={submit}>
      <header className="new-task-intro select-none">
        <h1>{t("task.promptTitle")}</h1>
        <p>{t("task.promptDescription")}</p>
      </header>

      <div className="new-task-composer-stack">
        <div className="new-task-composer">
          <div className={`codex-skill-field ${skillRuntime ? "has-skill-highlight" : ""}`.trim()}>
            <SkillTextarea
              ref={promptRef}
              textareaComponent={Textarea}
              highlight={Boolean(skillRuntime)}
              id="task-create-goal"
              className="new-task-prompt"
              autoFocus
              value={form.prompt}
              aria-label={t("task.goal")}
              placeholder={t("task.goalPlaceholder")}
              onCompositionStart={() => { composing.current = true; }}
              onCompositionEnd={() => { window.setTimeout(() => { composing.current = false; }, 100); }}
              {...skillPicker.inputProps}
            />
            {skillPicker.menu}
          </div>

          {form.attachmentPaths?.length > 0 && <div className="new-task-attachments">
            {form.attachmentPaths.map((path) => <span className="new-task-attachment" key={path} title={path}>
              <span>{fileName(path)}</span>
              <button type="button" aria-label={`${t("common.remove")} ${fileName(path)}`} title={t("common.remove")} onClick={() => onChange((current) => ({ ...current, attachmentPaths: current.attachmentPaths.filter((item) => item !== path) }))}><X size={12} aria-hidden="true" /></button>
            </span>)}
          </div>}

          <div className={`new-task-toolbar ${directAgent ? "agent-mode" : "workflow-mode"} ${showRuntimeProfile ? "has-runtime-profile" : "no-runtime-profile"}`}>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button type="button" variant="ghost" size="icon-sm" className="new-task-add" aria-label={t("task.addAndConfigure")} title={t("task.addAndConfigure")}><Plus size={18} aria-hidden="true" /></Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent className="task-executor-menu new-task-add-menu" side="top" align="start" sideOffset={8}>
                {onChooseAttachments && <><DropdownMenuItem onSelect={() => onChooseAttachments()}><Paperclip size={14} aria-hidden="true" /><span>{t("task.chooseFiles")}</span></DropdownMenuItem><DropdownMenuSeparator /></>}
                <DropdownMenuLabel className="task-executor-section">{t("task.executionMode")}</DropdownMenuLabel>
                <DropdownMenuRadioGroup value={executionMode} onValueChange={(nextMode) => onChange((current) => ({ ...current, executionMode: nextMode }))}>
                  <DropdownMenuRadioItem className="task-executor-option agent" value="immediate"><ArrowUp size={14} aria-hidden="true" /><span><strong>{t("task.runNow")}</strong></span></DropdownMenuRadioItem>
                  <DropdownMenuRadioItem className="task-executor-option agent" value="queued"><ListPlus size={14} aria-hidden="true" /><span><strong>{t("task.joinQueue")}</strong></span></DropdownMenuRadioItem>
                </DropdownMenuRadioGroup>
              </DropdownMenuContent>
            </DropdownMenu>
            <TaskExecutorSelector form={form} workflows={workflows} runtimes={runtimes} runtimeSettings={runtimeSettingsByHarness} remoteFS={remoteFS} onChange={onChange} />
            <TaskPermissionSelector value={form.sandbox} allowFull={allowFullSandbox} onChange={onChange} />
            {showRuntimeProfile && <RuntimeProfileMenu
              className="new-task-runtime"
              value={form}
              onChange={onChange}
              configuration={runtimeConfiguration?.data}
              runtimeSettings={runtimeSettings}
              loading={runtimeConfiguration?.loading}
              error={runtimeConfiguration?.error}
            />}
            <div className={`new-task-submit-group ${executionMode}`}>
              <Button type="submit" size="icon-sm" className="new-task-submit-action" disabled={!ready || busy === "run"} aria-label={submitLabel} title={submitLabel}>
                {executionMode === "queued" ? <ListPlus size={15} strokeWidth={2.2} aria-hidden="true" /> : <ArrowUp size={15} strokeWidth={2.4} aria-hidden="true" />}
              </Button>
            </div>
          </div>
        </div>
        <WorkspaceComposerMeta mode={mode} workspace={workspace} onEdit={onEditWorkspace} />
      </div>
    </form>
  </div>;
}
