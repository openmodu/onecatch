import { useLayoutEffect, useRef, useState } from "react";
import { ArrowUp, CornerDownRight, ListEnd, Paperclip, Square, Trash2, Workflow } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Action } from "../../ui/primitives.jsx";
import { preserveComposerFocus } from "../composerInteraction.js";
import { composerSubmitMode } from "../composerKeyboard.js";
import { autosizeComposerTextarea, WORKBENCH_TEXTAREA_MIN_HEIGHT } from "../composerTextarea.js";
import { fileName } from "../format.js";
import { primaryShortcutLabel } from "../platform.js";
import { directAgentWorkflowID, supportsRuntimeProfile, supportsRuntimeSkills } from "../runtimeHarnesses.js";
import HarnessSelector from "./HarnessSelector.jsx";
import ContextGauge from "./ContextGauge.jsx";
import RuntimeProfileMenu from "./RuntimeProfileMenu.jsx";
import { useSkillPicker } from "./SkillPicker.jsx";
import SkillTextarea from "./SkillTextarea.jsx";
import TaskPermissionSelector from "./TaskPermissionSelector.jsx";
import WorkspaceComposerMeta from "./WorkspaceComposerMeta.jsx";

// Draft text is intentionally local state: keystrokes re-render only this
// subtree instead of the whole workbench + polling tree, which is what made
// typing feel laggy. The parent still owns attachments and the async submit.
export default function Composer({
  runStatus,
  active,
  busy,
  attachments,
  pendingInstructions,
  onChooseAttachments,
  onRemoveAttachment,
  onRemoveInstruction,
  onSteerInstruction,
  onInterrupt,
  onSubmit,
  runtimeProfile,
  onRuntimeProfileChange,
  runtimes,
  runtimeConfiguration,
  runtimeSettings,
  workflowId,
  workflowName,
  permission,
  contextWindow,
  mode,
  workspace,
  onEditWorkspace,
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState("");
  const composing = useRef(false);
  const draftRef = useRef(null);
  const editable = ["running", "paused", "completed"].includes(runStatus);
  const canSend = Boolean(draft.trim() || attachments.length);
  const directAgent = !workflowId || workflowId === directAgentWorkflowID;
  const workflowLabel = workflowName || t("task.workflowMode");
  const showRuntimeProfile = Boolean(runtimeProfile && supportsRuntimeProfile(runtimeProfile.harness));
  const steerShortcut = primaryShortcutLabel("⇧↵");

  useLayoutEffect(() => {
    autosizeComposerTextarea(draftRef.current, WORKBENCH_TEXTAREA_MIN_HEIGHT);
  }, [draft]);

  const send = async (modeName) => {
    const accepted = await onSubmit(modeName, draft.trim(), runtimeProfile);
    if (accepted) setDraft("");
  };
  const submitFromComposer = (event) => {
    const submitMode = composerSubmitMode(event, { running: runStatus === "running" }, composing.current);
    if (!submitMode || (!canSend && runStatus !== "paused")) return;
    event.preventDefault();
    void send(submitMode);
  };
  const skillRuntime = directAgent && supportsRuntimeSkills(runtimeProfile?.harness) ? runtimeProfile.harness : "";
  const skillPicker = useSkillPicker({
    enabled: Boolean(skillRuntime) && editable,
    mode,
    runtime: skillRuntime,
    workspacePath: workspace?.path || "",
    value: draft,
    onValueChange: setDraft,
    textareaRef: draftRef,
    onKeyDown: submitFromComposer,
  });
  const skillHighlight = Boolean(skillRuntime);

  return <div className="workbench-composer shrink-0" onMouseDown={preserveComposerFocus}>
    <div className="workbench-composer-inner">
      <div className={`workbench-composer-stack ${pendingInstructions.length ? "has-followups" : ""}`.trim()}>
        {pendingInstructions.length > 0 && <div className="composer-followups" role="list" aria-label={t("composer.pendingFollowUps")}>{pendingInstructions.map((instruction) => <div className={`composer-followup ${instruction.priority ? "is-steering" : ""}`} role="listitem" key={instruction.id}>
          <ListEnd className="composer-followup-icon" size={16} aria-hidden="true" />
          <span className="composer-followup-message" title={instruction.content}>{instruction.content || t("common.attachmentsCount", { count: instruction.attachments?.length || 0 })}</span>
          <div className="composer-followup-actions">
            {instruction.followUp && <Button type="button" variant="ghost" size="sm" className="composer-followup-steer" disabled={busy === `steer:${instruction.id}` || runStatus !== "running"} onClick={() => onSteerInstruction(instruction.id)}><CornerDownRight aria-hidden="true" />{t("composer.steer")}</Button>}
            <Button type="button" variant="ghost" size="icon-sm" className="composer-followup-remove" disabled={busy === `steer:${instruction.id}`} aria-label={t("composer.removeFollowUp")} title={t("composer.removeFollowUp")} onClick={() => onRemoveInstruction(instruction.id)}><Trash2 size={16} aria-hidden="true" /></Button>
          </div>
        </div>)}</div>}
        <div className={`workbench-composer-shell ${editable ? "" : "disabled"}`.trim()}>
        {attachments.length > 0 && <div className="composer-attachments flex flex-wrap gap-1.5">{attachments.map((path) => <span className="attachment-chip inline-flex items-center gap-1.5 rounded-md bg-muted px-2 py-1 text-xs text-muted-foreground" key={path} title={path}>{fileName(path)}<Action size="compact" tone="danger" onClick={() => onRemoveAttachment(path)}>{t("common.remove")}</Action></span>)}</div>}
        <div className="workbench-composer-input"><div className={`codex-skill-field ${skillHighlight ? "has-skill-highlight" : ""}`.trim()}><SkillTextarea ref={draftRef} highlight={skillHighlight} aria-label={t("composer.aria")} value={draft} disabled={!editable} onCompositionStart={() => { composing.current = true; }} onCompositionEnd={() => { window.setTimeout(() => { composing.current = false; }, 100); }} placeholder={runStatus === "running" ? t("composer.runningPlaceholder") : runStatus === "paused" ? t("composer.pausedPlaceholder") : runStatus === "completed" ? t("composer.continuePlaceholder") : t("composer.finishedPlaceholder")} {...skillPicker.inputProps} />{skillPicker.menu}</div></div>
        <div className={`workbench-composer-actions ${showRuntimeProfile ? "profile-visible" : ""}`.trim()}>
          {onChooseAttachments && <Button type="button" variant="ghost" size="icon-sm" className="attachment-action" disabled={!editable} aria-label={t("composer.attachment")} title={t("composer.attachment")} onClick={onChooseAttachments}><Paperclip size={16} aria-hidden="true" /></Button>}
          {runtimeProfile && <div className="workbench-runtime-controls">
            {directAgent ? <HarnessSelector value={runtimeProfile} runtimes={runtimes} readOnly agentLabel /> : <span className="new-task-select executor is-read-only" aria-label={t("task.workflowTargetLabel", { name: workflowLabel })}><Workflow size={14} aria-hidden="true" /><span>{workflowLabel}</span></span>}
            <TaskPermissionSelector value={permission} readOnly />
            {/* Context pressure belongs where the context is about to be
                spent, not in the inspector: the moment it changes what you do
                is the moment you are deciding what to type next. */}
            {contextWindow?.known && <ContextGauge {...contextWindow} />}
          </div>}
          {showRuntimeProfile && <RuntimeProfileMenu className="workbench-runtime-profile" value={runtimeProfile} onChange={onRuntimeProfileChange} configuration={runtimeConfiguration?.data} runtimeSettings={runtimeSettings} loading={runtimeConfiguration?.loading} error={runtimeConfiguration?.error} readOnly={runStatus === "running"} />}
          <div className="workbench-composer-submit">
            {runStatus === "running" && (canSend
              ? <Button type="button" size="icon-sm" className="composer-send-action" disabled={["insert", "queue", "interrupt"].includes(busy)} aria-label={t("composer.queueFollowUp")} title={t("composer.queueHint", { shortcut: steerShortcut })} onClick={() => send("queue")}><ArrowUp size={15} strokeWidth={2.4} aria-hidden="true" /></Button>
              : <Button type="button" variant="secondary" size="icon-sm" className="composer-pause-action" disabled={busy === "interrupt"} aria-label={t("composer.stopRun")} title={t("composer.stopRun")} onClick={onInterrupt}><Square size={12} fill="currentColor" aria-hidden="true" /></Button>)}
            {directAgent ? <>
            {["paused", "completed"].includes(runStatus) && <Button type="button" size="icon-sm" className="composer-send-action" disabled={busy === "queue" || active || (!canSend && runStatus !== "paused")} aria-label={t("composer.sendMessage")} title={t("composer.sendMessage")} onClick={() => send("queue")}><ArrowUp size={15} strokeWidth={2.4} aria-hidden="true" /></Button>}
          </> : <>
            {runStatus === "paused" && <Button type="button" size="icon-sm" className="composer-send-action" disabled={busy === "queue" || active} aria-label={t("composer.resume")} title={t("composer.resume")} onClick={() => send("queue")}><ArrowUp size={15} strokeWidth={2.4} aria-hidden="true" /></Button>}
            {runStatus === "completed" && <Button type="button" size="icon-sm" className="composer-send-action" disabled={!canSend || busy === "queue" || active} aria-label={t("composer.continue")} title={t("composer.continue")} onClick={() => send("queue")}><ArrowUp size={15} strokeWidth={2.4} aria-hidden="true" /></Button>}
          </>}</div>
        </div>
      </div>
      </div>
      <WorkspaceComposerMeta mode={mode} workspace={workspace} onEdit={onEditWorkspace} />
    </div>
  </div>;
}
