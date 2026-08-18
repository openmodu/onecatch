import { useRef } from "react";
import { ArrowUp, ChevronDown, ListPlus, Paperclip, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Textarea } from "@/components/ui/textarea";
import { shouldSubmitComposer } from "../composerKeyboard.js";
import { fileName } from "../format.js";
import { supportsRuntimeProfile } from "../runtimeHarnesses.js";
import RuntimeProfileMenu from "./RuntimeProfileMenu.jsx";
import TaskExecutorSelector from "./TaskExecutorSelector.jsx";
import TaskPermissionSelector from "./TaskPermissionSelector.jsx";

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
  allowFullSandbox,
}) {
  const { t } = useTranslation();
  const composing = useRef(false);
  const directAgent = form.workflowId === "single_agent";
  const showRuntimeProfile = directAgent && supportsRuntimeProfile(form.harness);
  const ready = Boolean(workspaceID && form.prompt.trim() && form.workflowId && form.sandbox && (!directAgent || form.harness));
  const executionMode = form.executionMode === "queued" ? "queued" : "immediate";
  const executionLabel = executionMode === "queued" ? t("task.joinQueue") : t("task.runNow");
  const submitLabel = busy === "run"
    ? t("task.creating")
    : executionLabel;

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

  // overflow-y alone would compute overflow-x to `auto`, so a single control
  // that cannot shrink turns the whole screen into a horizontal scroller.
  return <div className="new-task-screen min-h-0 min-w-0 flex-1 overflow-x-hidden overflow-y-auto">
    <form className="new-task-layout" aria-busy={busy === "run"} onSubmit={submit}>
      <header className="new-task-intro select-none">
        <h1>{t("task.promptTitle")}</h1>
        <p>{t("task.promptDescription")}</p>
      </header>

      <div className="new-task-composer">
        <Textarea
          id="task-create-goal"
          className="new-task-prompt"
          autoFocus
          value={form.prompt}
          aria-label={t("task.goal")}
          placeholder={t("task.goalPlaceholder")}
          onChange={(event) => onChange((current) => ({ ...current, prompt: event.target.value }))}
          onCompositionStart={() => { composing.current = true; }}
          onCompositionEnd={() => { window.setTimeout(() => { composing.current = false; }, 100); }}
          onKeyDown={submitFromComposer}
        />

        {form.attachmentPaths?.length > 0 && <div className="new-task-attachments">
          {form.attachmentPaths.map((path) => <span className="new-task-attachment" key={path} title={path}>
            <span>{fileName(path)}</span>
            <button type="button" aria-label={`${t("common.remove")} ${fileName(path)}`} title={t("common.remove")} onClick={() => onChange((current) => ({ ...current, attachmentPaths: current.attachmentPaths.filter((item) => item !== path) }))}><X size={12} aria-hidden="true" /></button>
          </span>)}
        </div>}

        <div className={`new-task-toolbar ${directAgent ? "agent-mode" : "workflow-mode"} ${showRuntimeProfile ? "has-runtime-profile" : "no-runtime-profile"}`}>
          <Button type="button" variant="ghost" size="icon-sm" className="new-task-attach" aria-label={t("task.chooseFiles")} title={`${t("task.chooseFiles")} · ${t("task.attachmentsLimit")}`} onClick={onChooseAttachments}><Paperclip size={16} aria-hidden="true" /></Button>
          <TaskExecutorSelector form={form} workflows={workflows} runtimes={runtimes} onChange={onChange} />
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
            <Button type="submit" size="sm" className="new-task-submit-action" disabled={!ready || busy === "run"} aria-label={submitLabel} title={submitLabel}>
              {executionMode === "queued" ? <ListPlus size={15} strokeWidth={2.2} aria-hidden="true" /> : <ArrowUp size={15} strokeWidth={2.4} aria-hidden="true" />}
              <span>{submitLabel}</span>
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button type="button" size="icon-sm" className="new-task-submit-mode" aria-label={`${t("task.executionMode")}: ${executionLabel}`} title={`${t("task.executionMode")}: ${executionLabel}`}><ChevronDown size={13} aria-hidden="true" /></Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent className="new-task-execution-menu" side="top" align="end" sideOffset={8}>
                <DropdownMenuRadioGroup value={executionMode} onValueChange={(nextMode) => onChange((current) => ({ ...current, executionMode: nextMode }))}>
                  <DropdownMenuRadioItem className="new-task-execution-option" value="immediate">{t("task.runNow")}</DropdownMenuRadioItem>
                  <DropdownMenuRadioItem className="new-task-execution-option" value="queued">{t("task.joinQueue")}</DropdownMenuRadioItem>
                </DropdownMenuRadioGroup>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
      </div>
    </form>
  </div>;
}
