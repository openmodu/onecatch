import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { SettingsBinding } from "../../bindings/github.com/openmodu/oneshot/desktop/oneshot/bindings/index.js";
import { Action, Field, NumberField, SettingPanel as SettingCard, TUISelect, ToggleRow as Toggle } from "../ui/primitives.jsx";

const sectionMeta = (t) => ["runtime", "execution", "security", "storage", "experimental"].map((id) => ({ id, label: t(`settings.section.${id}`), description: t(`settings.section.${id}Description`) }));
const clone = (value) => JSON.parse(JSON.stringify(value));
const message = (error, t) => String(error?.message || error || t("common.unknownError")).replace(/^Error:\s*/, "");
const bytes = (value = 0) => value < 1024 ? `${value} B` : value < 1048576 ? `${(value / 1024).toFixed(1)} KB` : value < 1073741824 ? `${(value / 1048576).toFixed(1)} MB` : `${(value / 1073741824).toFixed(1)} GB`;
const sectionKey = (section) => section === "runtime" ? "runtimes" : section;

export const demoSettings = {
  schemaVersion: 1, revision: 1,
  runtimes: { codex: { binary: "", defaultModel: "", environmentAllowlist: [] }, claude: { binary: "", defaultModel: "", environmentAllowlist: [] }, modu: { binary: "", defaultModel: "", provider: "auto", environmentAllowlist: [] } },
  execution: { maxTransitions: 20, maxConsecutiveFailures: 3, stepTimeoutSeconds: 1800, maxLocalDAGConcurrency: 4, interruptGraceSeconds: 10, defaultSandbox: "workspace-write" },
  security: { allowFullSandbox: false, confirmFullSandboxEveryRun: true, diagnosticsIncludePrompt: false, diagnosticsIncludeRawEvents: false },
  storage: { completedRunRetentionDays: 0, logLevel: "info", logMaxSizeMB: 20, logMaxBackups: 5, logMaxAgeDays: 14 },
  experimental: { remoteWorkersEnabled: false },
};

export function ConfirmDialog({ dialog, busy = false, onCancel, onConfirm }) {
  const { t } = useTranslation();
  const dialogRef = useRef(null);
  const cancelRef = useRef(onCancel);

  useEffect(() => { cancelRef.current = onCancel; }, [onCancel]);
  useEffect(() => {
    if (!dialog) return undefined;
    const previousFocus = document.activeElement;
    const handleKeyDown = (event) => {
      if (event.key === "Escape" && !busy) {
        event.preventDefault();
        cancelRef.current?.();
        return;
      }
      if (event.key !== "Tab") return;
      const focusable = [...(dialogRef.current?.querySelectorAll("button:not(:disabled), [href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex='-1'])") || [])];
      if (!focusable.length) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      previousFocus?.focus?.();
    };
  }, [busy, dialog]);

  if (!dialog) return null;
  return <div className="modal-backdrop confirm-backdrop" onMouseDown={(event) => event.target === event.currentTarget && !busy && onCancel()}>
    <section ref={dialogRef} className={`modal confirm-dialog ${dialog.dangerous ? "dangerous" : ""}`} role="alertdialog" aria-modal="true" aria-labelledby="confirm-title" aria-describedby="confirm-description">
      <div className="confirm-copy">
        <span className="kicker">{dialog.eyebrow || t(dialog.dangerous ? "modal.dangerous" : "modal.confirmChange")}</span>
        <h2 id="confirm-title">{dialog.title}</h2>
        <p id="confirm-description">{dialog.description}</p>
        {dialog.detail && <div className="confirm-detail">{dialog.detail}</div>}
      </div>
      <div className="confirm-actions">
        <Action disabled={busy} onClick={onCancel}>{dialog.cancelLabel || t("common.cancel")}</Action>
        <Action autoFocus tone={dialog.dangerous ? "danger" : "primary"} disabled={busy} onClick={onConfirm}>{busy ? t("common.processing") : dialog.confirmLabel || t("common.confirm")}</Action>
      </div>
    </section>
  </div>;
}

export default function SettingsPage({ mode, value, runtimes, onChange, notify, workersPanel }) {
  const { t, i18n } = useTranslation();
  const [section, setSection] = useState("runtime");
  const [draft, setDraft] = useState(() => clone(value || demoSettings));
  const [saving, setSaving] = useState(false);
  const [runtimeStatus, setRuntimeStatus] = useState({});
  const [usage, setUsage] = useState(null);
  const [usageLoading, setUsageLoading] = useState(false);
  const [preview, setPreview] = useState(null);
  const [diagnosticPath, setDiagnosticPath] = useState("");
  const [diagnosticOptions, setDiagnosticOptions] = useState({ includePrompt: false, includeRawEvents: false });
  const [conflict, setConflict] = useState(false);
  const [dialog, setDialog] = useState(null);
  const [confirming, setConfirming] = useState(false);
  const key = sectionKey(section);
  const dirty = useMemo(() => JSON.stringify(draft?.[key]) !== JSON.stringify(value?.[key]), [draft, key, value]);
  const validationErrors = useMemo(() => validateSection(section, draft, t), [draft, section, t]);
  const errorsByField = useMemo(() => Object.fromEntries(validationErrors.map((error) => [error.field, error.message])), [validationErrors]);
  const sections = sectionMeta(t);
  const activeMeta = sections.find((item) => item.id === section);

  useEffect(() => setDraft(clone(value || demoSettings)), [value]);

  const ask = (options, action) => setDialog({ ...options, action });
  const closeDialog = () => { if (!confirming) setDialog(null); };
  const acceptDialog = async () => {
    const action = dialog?.action;
    if (!action) return;
    setConfirming(true);
    try { await action(); setDialog(null); } finally { setConfirming(false); }
  };
  const switchSectionNow = (next) => { setDraft(clone(value || demoSettings)); setSection(next); setPreview(null); setConflict(false); };
  const switchSection = (next) => {
    if (next === section) return;
    if (!dirty) { switchSectionNow(next); return; }
    ask({ title: t("settings.discardSectionTitle"), description: t("settings.discardSectionDescription", { section: activeMeta.label }), confirmLabel: t("settings.discardAndSwitch"), dangerous: true }, () => switchSectionNow(next));
  };
  const setSectionValue = (name, next) => setDraft((current) => ({ ...current, [name]: next }));
  const save = async () => {
    if (!dirty || validationErrors.length) return;
    setSaving(true);
    try {
      let saved;
      if (mode === "demo") saved = { ...value, [key]: clone(draft[key]), revision: (value?.revision || 1) + 1 };
      else if (section === "runtime") saved = await SettingsBinding.UpdateRuntimeSettings(draft.runtimes, value.revision);
      else if (section === "execution") saved = await SettingsBinding.UpdateExecutionSettings(draft.execution, value.revision);
      else if (section === "security") saved = await SettingsBinding.UpdateSecuritySettings(draft.security, value.revision);
      else if (section === "storage") saved = await SettingsBinding.UpdateStorageSettings(draft.storage, value.revision);
      else saved = await SettingsBinding.UpdateExperimentalSettings(draft.experimental, value.revision);
      onChange(saved);
      notify("success", t("settings.saved", { section: activeMeta.label }));
      setConflict(false);
    } catch (error) {
      const text = message(error, t);
      if (text.includes("settings_state_conflict")) setConflict(true);
      notify("error", text);
    } finally { setSaving(false); }
  };
  const reset = () => ask({ title: t("settings.resetTitle", { section: activeMeta.label }), description: t("settings.resetDescription"), detail: t("settings.resetDetail"), confirmLabel: t("settings.reset"), dangerous: true }, async () => {
    try {
      const resetKey = sectionKey(section);
      const saved = mode === "demo" ? { ...value, [resetKey]: clone(demoSettings[resetKey]), revision: (value?.revision || 1) + 1 } : await SettingsBinding.ResetSettingsSection(section, value.revision);
      onChange(clone(saved));
      setPreview(null);
      notify("success", t("settings.resetSuccess", { section: activeMeta.label }));
    } catch (error) { notify("error", message(error, t)); }
  });
  const checkRuntime = async (id) => {
    setRuntimeStatus((current) => ({ ...current, [id]: { checking: true } }));
    try {
      const result = mode === "demo" ? { available: true, version: "preview", checkedAt: new Date().toISOString() } : await SettingsBinding.CheckRuntimeDraft({ runtime: id, settings: draft.runtimes[id] });
      setRuntimeStatus((current) => ({ ...current, [id]: result }));
    } catch (error) { setRuntimeStatus((current) => ({ ...current, [id]: { available: false, error: message(error, t) } })); }
  };
  const refreshUsage = async () => {
    setUsageLoading(true);
    try {
      setUsage(mode === "demo" ? { totalBytes: 7340032, root: "~/.oneshot", calculatedAt: new Date().toISOString(), categories: [{ name: "workflows", bytes: 163840, files: 4 }, { name: "runs", bytes: 4980736, files: 12 }, { name: "events", bytes: 491520, files: 12 }, { name: "logs", bytes: 1703936, files: 3 }] } : await SettingsBinding.GetStorageUsage());
    } catch (error) { notify("error", message(error, t)); } finally { setUsageLoading(false); }
  };
  useEffect(() => { if (section === "storage" && !usage && !usageLoading) refreshUsage(); }, [section]);
  const previewCleanup = async () => {
    try { setPreview(mode === "demo" ? { token: "demo", count: 3, estimatedBytes: 1048576, runIds: ["run-a", "run-b", "run-c"] } : await SettingsBinding.PreviewCleanup({ retentionDays: draft.storage.completedRunRetentionDays })); }
    catch (error) { notify("error", message(error, t)); }
  };
  const executeCleanup = () => {
    if (!preview?.token) return;
    ask({ title: t("settings.cleanupTitle", { count: preview.count }), description: t("settings.cleanupDescription", { size: bytes(preview.estimatedBytes) }), detail: t("settings.cleanupDetail"), confirmLabel: t("settings.confirmCleanup"), dangerous: true }, async () => {
      try {
        const result = mode === "demo" ? { removedRunIds: preview.runIds } : await SettingsBinding.ExecuteCleanup(preview.token);
        setPreview(null);
        await refreshUsage();
        notify("success", t("settings.cleanupSuccess", { count: (result.removedRunIds || []).length }));
      } catch (error) { notify("error", message(error, t)); }
    });
  };
  const doExportDiagnostics = async () => {
    try {
      const result = mode === "demo" ? { path: diagnosticPath || "~/.oneshot/oneshot-diagnostics.zip" } : await SettingsBinding.ExportDiagnostics({ destination: diagnosticPath, runIds: [], ...diagnosticOptions });
      notify("success", t("settings.exportSuccess", { path: result.path }));
    } catch (error) { notify("error", message(error, t)); }
  };
  const exportDiagnostics = () => {
    if (!diagnosticOptions.includePrompt && !diagnosticOptions.includeRawEvents) { doExportDiagnostics(); return; }
    ask({ title: t("settings.sensitiveExportTitle"), description: t("settings.sensitiveExportDescription"), confirmLabel: t("settings.confirmExport"), dangerous: true }, doExportDiagnostics);
  };
  const discard = () => setDraft(clone(value || demoSettings));
  const reload = async () => {
    try { const current = mode === "demo" ? demoSettings : await SettingsBinding.GetSettings(); onChange(current); setConflict(false); }
    catch (error) { notify("error", message(error, t)); }
  };
  const confirmFullAccess = (checked) => {
    if (!checked) { setSectionValue("security", { ...draft.security, allowFullSandbox: false }); return; }
    ask({ title: t("settings.fullAccessTitle"), description: t("settings.fullAccessDescription"), detail: t("settings.fullAccessDetail"), confirmLabel: t("settings.enableRisk"), dangerous: true }, () => setSectionValue("security", { ...draft.security, allowFullSandbox: true }));
  };

  return <div className="settings-page">
    <aside className="settings-rail" aria-label={t("settings.sectionsAria")}>
      <span className="kicker">{t("settings.preferences")}</span>
      {sections.map((item) => <button key={item.id} className={section === item.id ? "active" : ""} aria-current={section === item.id ? "page" : undefined} onClick={() => switchSection(item.id)}>
        <strong>{item.label}{section === item.id && dirty && <i className="dirty-dot" aria-label={t("settings.unsaved")} />}</strong>
        <small>{item.description}</small>
      </button>)}
    </aside>
    <section className="settings-content">
      <div className="settings-title">
        <div><span className="kicker">{t("settings.localSettings")}</span><h2>{activeMeta.label}</h2><p>{activeMeta.description} · {t("settings.revision", { revision: value?.revision || 1 })}</p></div>
        <div className="settings-title-actions"><TUISelect className="settings-language-select" ariaLabel={t("language.label")} value={i18n.resolvedLanguage} onChange={(language) => i18n.changeLanguage(language)} options={[{ value: "zh-CN", label: t("language.chinese") }, { value: "en", label: t("language.english") }]} /><span className="settings-sync-state">{dirty ? t("settings.waitingSave") : t("settings.synced", { revision: value?.revision || 1 })}</span><Action onClick={reset}>{t("settings.reset")}</Action></div>
      </div>
      {conflict && <div className="settings-banner conflict" role="alert"><div><strong>{t("settings.conflictTitle")}</strong><span>{t("settings.conflictDescription")}</span></div><Action onClick={reload}>{t("settings.reload")}</Action></div>}
      {validationErrors.length > 0 && <div className="settings-banner invalid" role="alert"><div><strong>{t("settings.validationCount", { count: validationErrors.length })}</strong><span>{t("settings.validationDescription")}</span></div></div>}
      {section === "runtime" && <RuntimeSettings value={draft.runtimes} setValue={(next) => setSectionValue("runtimes", next)} status={runtimeStatus} runtimes={runtimes} check={checkRuntime} errors={errorsByField} />}
      {section === "execution" && <ExecutionSettings value={draft.execution} setValue={(next) => setSectionValue("execution", next)} errors={errorsByField} />}
      {section === "security" && <SecuritySettings value={draft.security} setValue={(next) => setSectionValue("security", next)} confirmFullAccess={confirmFullAccess} />}
      {section === "storage" && <StorageSettings value={draft.storage} setValue={(next) => setSectionValue("storage", next)} errors={errorsByField} security={draft.security} diagnosticOptions={diagnosticOptions} setDiagnosticOptions={setDiagnosticOptions} usage={usage} usageLoading={usageLoading} refreshUsage={refreshUsage} preview={preview} previewCleanup={previewCleanup} executeCleanup={executeCleanup} reveal={() => mode === "wails" && SettingsBinding.RevealDataRoot()} diagnosticPath={diagnosticPath} setDiagnosticPath={setDiagnosticPath} exportDiagnostics={exportDiagnostics} />}
      {section === "experimental" && <ExperimentalSettings draft={draft.experimental} saved={value?.experimental || demoSettings.experimental} setValue={(next) => setSectionValue("experimental", next)} workersPanel={workersPanel} />}
      {dirty && <div className="settings-savebar" role="region" aria-label={t("settings.unsavedSettings")}>
        <div><strong>{t("settings.unsavedTitle")}</strong><span>{t("settings.unsavedDescription")}</span></div>
        <div><Action disabled={saving} onClick={discard}>{t("settings.discard")}</Action><Action tone="primary" disabled={saving || validationErrors.length > 0} onClick={save}>{saving ? t("common.saving") : t("settings.save")}</Action></div>
      </div>}
    </section>
    <ConfirmDialog dialog={dialog} busy={confirming} onCancel={closeDialog} onConfirm={acceptDialog} />
  </div>;
}

function RuntimeSettings({ value, setValue, status, runtimes, check, errors }) {
  const { t, i18n } = useTranslation();
  const update = (id, field, next) => setValue({ ...value, [id]: { ...value[id], [field]: next } });
  const meta = {
    codex: { name: "Codex", command: "codex", description: t("settings.runtimeDescription"), env: "OPENAI_API_KEY, HTTPS_PROXY" },
    claude: { name: "Claude Code", command: "claude", description: t("settings.runtimeDescription"), env: "ANTHROPIC_API_KEY, HTTPS_PROXY" },
    modu: { name: "Modu Code", command: "modu_code", description: t("settings.moduDescription"), env: t("settings.optionalEnv") },
  };
  return <>{["codex", "claude", "modu"].map((id) => {
    const current = status[id] || runtimes.find((item) => item.id === id) || {};
    const statusText = current.checking ? t("settings.checkingCommand") : current.available ? `${current.version || t("common.available")}${current.checkedAt ? ` · ${new Date(current.checkedAt).toLocaleTimeString(i18n.resolvedLanguage === "en" ? "en-US" : "zh-CN")}` : ""}` : current.error || t("settings.notDetected");
    return <SettingCard className="runtime-setting-card" key={id} title={meta[id].name} description={meta[id].description} aside={<span className={`setting-status ${current.available ? "ok" : current.error ? "bad" : ""}`}><i />{statusText}</span>}>
      <div className="settings-grid">
        <Field label={t("settings.binaryPath")} hint={t("settings.useCommand", { command: meta[id].command })} error={errors[`${id}.binary`]}><input value={value[id]?.binary || ""} aria-invalid={Boolean(errors[`${id}.binary`])} onChange={(event) => update(id, "binary", event.target.value)} placeholder={meta[id].command} /></Field>
        <Field label={t("settings.defaultModel")} hint={t("settings.runtimeDecides")} error={errors[`${id}.defaultModel`]}><input value={value[id]?.defaultModel || ""} aria-invalid={Boolean(errors[`${id}.defaultModel`])} onChange={(event) => update(id, "defaultModel", event.target.value)} placeholder={t("settings.runtimeDefault")} /></Field>
        {id === "modu" && <Field label={t("common.provider")} hint={t("settings.providerHint")} error={errors[`${id}.provider`]}><TUISelect ariaLabel={t("common.provider")} value={value[id]?.provider || "auto"} onChange={(provider) => update(id, "provider", provider)} options={[{ value: "auto", label: t("settings.autoDetect") }, { value: "openai", label: "OpenAI / Compatible" }, { value: "anthropic", label: "Anthropic" }, { value: "gemini", label: "Gemini" }]} /></Field>}
        <Field className="full" label={t("settings.envAllowlist")} hint={t("settings.envAllowlistHint")} error={errors[`${id}.environmentAllowlist`]}><input value={(value[id]?.environmentAllowlist || []).join(", ")} aria-invalid={Boolean(errors[`${id}.environmentAllowlist`])} onChange={(event) => update(id, "environmentAllowlist", event.target.value.toUpperCase().split(",").map((item) => item.trim()).filter(Boolean))} placeholder={meta[id].env} /></Field>
      </div>
      <div className="settings-actions"><Action tone="cyan" disabled={current.checking} onClick={() => check(id)}>{current.checking ? t("settings.checking") : t("settings.testConfig")}</Action></div>
    </SettingCard>;
  })}</>;
}

function ExecutionSettings({ value, setValue, errors }) {
  const { t } = useTranslation();
  const number = (key, next) => setValue({ ...value, [key]: Number(next) });
  return <SettingCard title={t("settings.executionPolicy")} description={t("settings.executionPolicyDescription")}>
    <div className="settings-grid execution-grid">
      <NumberField field="maxTransitions" label={t("settings.maxTransitions")} hint="1–10000" value={value.maxTransitions} error={errors.maxTransitions} onChange={(next) => number("maxTransitions", next)} />
      <NumberField field="maxConsecutiveFailures" label={t("settings.maxFailures")} hint="1–100" value={value.maxConsecutiveFailures} error={errors.maxConsecutiveFailures} onChange={(next) => number("maxConsecutiveFailures", next)} />
      <NumberField field="stepTimeoutSeconds" label={t("settings.nodeTimeout")} hint={t("settings.secondsRange", { min: 30, max: 86400 })} value={value.stepTimeoutSeconds} error={errors.stepTimeoutSeconds} onChange={(next) => number("stepTimeoutSeconds", next)} />
      <NumberField field="maxLocalDAGConcurrency" label={t("settings.dagConcurrency")} hint={t("settings.readonlyNodesRange")} value={value.maxLocalDAGConcurrency} error={errors.maxLocalDAGConcurrency} onChange={(next) => number("maxLocalDAGConcurrency", next)} />
      <NumberField field="interruptGraceSeconds" label={t("settings.interruptGrace")} hint={t("settings.secondsRange", { min: 1, max: 60 })} value={value.interruptGraceSeconds} error={errors.interruptGraceSeconds} onChange={(next) => number("interruptGraceSeconds", next)} />
      <Field label={t("workspace.defaultSandbox")} hint={t("settings.noGlobalFullAccess")}><TUISelect ariaLabel={t("workspace.defaultSandbox")} value={value.defaultSandbox} onChange={(defaultSandbox) => setValue({ ...value, defaultSandbox })} options={[{ value: "read-only", label: t("workspace.readOnly") }, { value: "workspace-write", label: t("workspace.write") }]} /></Field>
    </div>
  </SettingCard>;
}

function SecuritySettings({ value, setValue, confirmFullAccess }) {
  const { t } = useTranslation();
  const toggle = (key) => (checked) => setValue({ ...value, [key]: checked });
  return <>
    <SettingCard title={t("settings.executionAuth")} description={t("settings.executionAuthDescription")}>
      <Toggle checked={value.allowFullSandbox} onChange={confirmFullAccess} label={t("settings.allowFullAccess")} description={t("settings.fullAccessDanger")} dangerous />
      <Toggle checked={value.confirmFullSandboxEveryRun} onChange={toggle("confirmFullSandboxEveryRun")} label={t("settings.confirmEveryRun")} description={value.allowFullSandbox ? t("settings.confirmEveryRunEnabled") : t("settings.confirmEveryRunDisabled")} disabled={!value.allowFullSandbox} />
    </SettingCard>
    <SettingCard title={t("settings.diagnosticPrivacy")} description={t("settings.diagnosticPrivacyDescription")}>
      <Toggle checked={value.diagnosticsIncludePrompt} onChange={toggle("diagnosticsIncludePrompt")} label={t("settings.allowPrompt")} description={t("settings.confirmOnExport")} />
      <Toggle checked={value.diagnosticsIncludeRawEvents} onChange={toggle("diagnosticsIncludeRawEvents")} label={t("settings.allowRawEvents")} description={t("settings.rawEventsDescription")} />
    </SettingCard>
  </>;
}

function StorageSettings({ value, setValue, errors, security, diagnosticOptions, setDiagnosticOptions, usage, usageLoading, refreshUsage, preview, previewCleanup, executeCleanup, reveal, diagnosticPath, setDiagnosticPath, exportDiagnostics }) {
  const { t, i18n } = useTranslation();
  const number = (key, next) => setValue({ ...value, [key]: Number(next) });
  return <>
    <SettingCard title={t("settings.localData")} description={t("settings.localDataDescription")} aside={<Action tone="cyan" onClick={refreshUsage} disabled={usageLoading}>{usageLoading ? t("settings.calculating") : t("settings.recalculate")}</Action>}>
      <div className="data-root-row"><div><span>{t("settings.dataRoot")}</span><code>~/.oneshot/</code></div><Action onClick={reveal}>{t("settings.revealFinder")}</Action></div>
      {usage ? <><div className="usage-total">{bytes(usage.totalBytes)} <small>{t("settings.totalUsage")}</small></div><div className="usage-bars">{(usage.categories || []).map((item) => <div key={item.name}><span>{item.name}</span><b>{bytes(item.bytes)}</b><i style={{ width: `${Math.max(3, item.bytes / Math.max(usage.totalBytes, 1) * 100)}%` }} /></div>)}</div><p className="storage-calculated-at">{t("settings.lastCalculated", { time: usage.calculatedAt ? new Date(usage.calculatedAt).toLocaleString(i18n.resolvedLanguage === "en" ? "en-US" : "zh-CN") : t("settings.justNow") })}</p></> : <div className="settings-inline-empty">{usageLoading ? t("settings.calculatingUsage") : t("settings.noUsage")}</div>}
    </SettingCard>
    <SettingCard title={t("settings.historyCleanup")} description={t("settings.historyCleanupDescription")}>
      <Field label={t("settings.retention")} hint={t("settings.keepForeverDefault")}><TUISelect ariaLabel={t("settings.retention")} value={value.completedRunRetentionDays} onChange={(retentionDays) => { number("completedRunRetentionDays", retentionDays); setPreview(null); }} options={[{ value: 0, label: t("settings.keepForever") }, { value: 30, label: t("common.daysCount", { count: 30 }) }, { value: 90, label: t("common.daysCount", { count: 90 }) }, { value: 180, label: t("common.daysCount", { count: 180 }) }]} /></Field>
      <div className="settings-actions"><Action tone="muted" disabled={!value.completedRunRetentionDays} onClick={previewCleanup}>{t("settings.previewCleanup")}</Action></div>
      {preview?.token && <div className="cleanup-preview" role="status"><div><span className="kicker">{t("settings.cleanupPreview")}</span><strong>{t("settings.historicalRuns", { count: preview.count })}</strong><p>{t("settings.estimatedRelease", { size: bytes(preview.estimatedBytes) })}</p></div><Action tone="danger" disabled={!preview.count} onClick={executeCleanup}>{t("settings.irreversibleCleanup")}</Action></div>}
    </SettingCard>
    <SettingCard title={t("settings.logRotation")} description={t("settings.logRotationDescription")}>
      <div className="settings-grid">
        <Field label={t("settings.logLevel")} hint={t("settings.recommendedInfo")}><TUISelect ariaLabel={t("settings.logLevel")} value={value.logLevel} onChange={(logLevel) => setValue({ ...value, logLevel })} options={["error", "warn", "info", "debug"]} /></Field>
        <NumberField field="logMaxSizeMB" label={t("settings.logFileSize")} hint="1–1024 MB" value={value.logMaxSizeMB} error={errors.logMaxSizeMB} onChange={(next) => number("logMaxSizeMB", next)} />
        <NumberField field="logMaxBackups" label={t("settings.backupCount")} hint="1–50" value={value.logMaxBackups} error={errors.logMaxBackups} onChange={(next) => number("logMaxBackups", next)} />
        <NumberField field="logMaxAgeDays" label={t("settings.logRetention")} hint={t("settings.daysRange", { min: 1, max: 365 })} value={value.logMaxAgeDays} error={errors.logMaxAgeDays} onChange={(next) => number("logMaxAgeDays", next)} />
      </div>
      {value.logLevel === "debug" && <div className="settings-note warning"><strong>{t("settings.debugWarning")}</strong><span>{t("settings.debugAdvice")}</span></div>}
    </SettingCard>
    <SettingCard title={t("settings.exportDiagnostics")} description={t("settings.exportDiagnosticsDescription")}>
      <Field label={t("settings.zipPath")} hint={t("settings.zipPathHint")}><input value={diagnosticPath} onChange={(event) => setDiagnosticPath(event.target.value)} placeholder="/Users/me/Desktop/oneshot-diagnostics.zip" /></Field>
      <Toggle checked={diagnosticOptions.includePrompt} onChange={(checked) => setDiagnosticOptions({ ...diagnosticOptions, includePrompt: checked })} label={t("settings.includePrompt")} description={security.diagnosticsIncludePrompt ? t("settings.confirmOnExport") : t("settings.authorizeSecurity")} disabled={!security.diagnosticsIncludePrompt} />
      <Toggle checked={diagnosticOptions.includeRawEvents} onChange={(checked) => setDiagnosticOptions({ ...diagnosticOptions, includeRawEvents: checked })} label={t("settings.includeRawEvents")} description={security.diagnosticsIncludeRawEvents ? t("settings.confirmOnExport") : t("settings.authorizeSecurity")} disabled={!security.diagnosticsIncludeRawEvents} />
      <div className="settings-actions"><Action onClick={exportDiagnostics}>{t("settings.exportZip")}</Action></div>
    </SettingCard>
  </>;
}

function ExperimentalSettings({ draft, saved, setValue, workersPanel }) {
  const { t } = useTranslation();
  return <>
    <div className="settings-boundary" role="note"><span className="kicker">{t("settings.experimentalBoundary")}</span><strong>{t("settings.remoteTrustedOnly")}</strong><p>{t("settings.remoteBoundaryDescription")}</p></div>
    <SettingCard title={t("settings.remoteScheduling")} description={t("settings.remoteSchedulingDescription")}>
      <Toggle checked={draft.remoteWorkersEnabled} onChange={(checked) => setValue({ ...draft, remoteWorkersEnabled: checked })} label={t("settings.enableRemoteWorker")} description={t("settings.enableRemoteDescription")} />
    </SettingCard>
    {draft.remoteWorkersEnabled && saved.remoteWorkersEnabled && workersPanel}
    {draft.remoteWorkersEnabled && !saved.remoteWorkersEnabled && <div className="settings-pending panel"><strong>{t("settings.enableAfterSave")}</strong><span>{t("settings.enableAfterSaveDescription")}</span></div>}
    {!draft.remoteWorkersEnabled && saved.remoteWorkersEnabled && <div className="settings-pending panel"><strong>{t("settings.disableAfterSave")}</strong><span>{t("settings.disableAfterSaveDescription")}</span></div>}
    {!draft.remoteWorkersEnabled && !saved.remoteWorkersEnabled && <div className="settings-disabled panel"><strong>{t("settings.remoteDisabled")}</strong><span>{t("settings.remoteDisabledDescription")}</span></div>}
  </>;
}

function validateSection(section, draft, t) {
  const errors = [];
  const add = (field, messageText) => errors.push({ field, message: messageText });
  if (section === "runtime") {
    const forbidden = (key) => ["PATH", "HOME", "SHELL", "BASH_ENV", "ENV", "ZDOTDIR"].includes(key) || key.startsWith("DYLD_") || key.startsWith("LD_");
    for (const id of ["codex", "claude", "modu"]) {
      const runtime = draft.runtimes[id];
      if (/[\r\n\0]/.test(runtime.binary || "")) add(`${id}.binary`, t("settings.validation.pathCharacters"));
      if (/[\r\n\0]/.test(runtime.defaultModel || "")) add(`${id}.defaultModel`, t("settings.validation.modelCharacters"));
      const invalid = (runtime.environmentAllowlist || []).find((key) => !/^[A-Z_][A-Z0-9_]{0,127}$/.test(key) || forbidden(key));
      if (invalid) add(`${id}.environmentAllowlist`, t("settings.validation.invalidEnv", { key: invalid }));
      if (id === "modu" && !["", "auto", "openai", "anthropic", "gemini"].includes(runtime.provider || "")) add(`${id}.provider`, t("settings.validation.invalidProvider"));
    }
  }
  if (section === "execution") {
    const rules = [["maxTransitions", t("settings.maxTransitions"), 1, 10000], ["maxConsecutiveFailures", t("settings.maxFailures"), 1, 100], ["stepTimeoutSeconds", t("settings.nodeTimeout"), 30, 86400], ["maxLocalDAGConcurrency", t("settings.dagConcurrency"), 1, 16], ["interruptGraceSeconds", t("settings.interruptGrace"), 1, 60]];
    rules.forEach(([field, label, min, max]) => { const current = draft.execution[field]; if (!Number.isInteger(current) || current < min || current > max) add(field, t("settings.validation.range", { label, min, max })); });
  }
  if (section === "storage") {
    [["logMaxSizeMB", t("settings.logFileSize"), 1, 1024], ["logMaxBackups", t("settings.backupCount"), 1, 50], ["logMaxAgeDays", t("settings.logRetention"), 1, 365]].forEach(([field, label, min, max]) => { const current = draft.storage[field]; if (!Number.isInteger(current) || current < min || current > max) add(field, t("settings.validation.range", { label, min, max })); });
  }
  return errors;
}
