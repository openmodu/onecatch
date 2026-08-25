import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Events } from "@wailsio/runtime";
import { ChevronDown } from "lucide-react";
import { SettingsBinding } from "../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Switch } from "@/components/ui/switch";
import {
  SettingsButton,
  SettingsField,
  SettingsKicker,
  SettingsNumberField,
  SettingsSection,
  SettingsSelect,
  SettingsSwitchRow,
} from "./components/settings/SettingsControls.jsx";

// Preview swatches for the accent picker. These mirror the light-theme
// --primary values in index.css; the picker is the one place the colour has to
// be shown before it is applied, so it cannot read them off the live token.
const ACCENT_SWATCH = { forest: "#694d1f", ocean: "#1f6475", violet: "#684886", amber: "#87501d" };
import { APPEARANCE_CHANGED_EVENT, accentThemes, chatFontSizes, readAppearance, saveAppearance, themeModes } from "./appearance.js";
import { codexEffortValues, codexServiceTierValues, demoClaudeConfiguration, demoCodexConfiguration, selectedCodexModel } from "./codexRuntimeOptions.js";
import { LANGUAGE_CHANGED_EVENT, normalizeLanguage } from "../i18n.js";
import { ConfirmDialog } from "./components/settings/ConfirmDialog.jsx";
import { demoSettings } from "./settingsDefaults.js";

const sectionMeta = (t) => ["runtime", "harness", "terminal", "execution", "security", "storage", "experimental"].map((id) => ({ id, label: t(`settings.section.${id}`), description: t(`settings.section.${id}Description`) }));
const clone = (value) => JSON.parse(JSON.stringify(value));
const message = (error, t) => String(error?.message || error || t("common.unknownError")).replace(/^Error:\s*/, "");
const bytes = (value = 0) => value < 1024 ? `${value} B` : value < 1048576 ? `${(value / 1024).toFixed(1)} KB` : value < 1073741824 ? `${(value / 1048576).toFixed(1)} MB` : `${(value / 1073741824).toFixed(1)} GB`;
const sectionKey = (section) => section === "harness" ? "runtimes" : section;
const harnessFields = ["enabled", "remoteFsEnabled", "integration", "configSource", "configPath", "binary", "environmentAllowlist", "defaultModel", "reasoningEffort", "serviceTier", "maxContextWindow", "provider"];

const resetRuntimeFields = (current, defaults, fields) => Object.fromEntries(Object.keys(defaults).map((id) => {
  const next = { ...current[id] };
  fields.forEach((field) => { next[field] = clone(defaults[id]?.[field] ?? (field === "environmentAllowlist" ? [] : "")); });
  return [id, next];
}));

export default function SettingsPage({ mode, value, runtimes, onChange, notify, workersPanel }) {
  const { t, i18n } = useTranslation();
  const [section, setSection] = useState("runtime");
  const [draft, setDraft] = useState(() => clone(value || demoSettings));
  const [saving, setSaving] = useState(false);
  const [runtimeStatus, setRuntimeStatus] = useState({});
  const [codexConfiguration, setCodexConfiguration] = useState({ loading: false, data: null, error: "" });
  // Codex and Claude Code predate the shared configuration shape and keep their
  // own slots; every other harness reports through this one, so a new adapter
  // needs no state of its own here.
  const [harnessConfigurations, setHarnessConfigurations] = useState({});
  const [claudeConfiguration, setClaudeConfiguration] = useState({ loading: false, data: null, error: "" });
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
      else if (section === "harness") saved = await SettingsBinding.UpdateRuntimeSettings(draft.runtimes, value.revision);
      else if (section === "terminal") saved = await SettingsBinding.UpdateTerminalSettings(draft.terminal, value.revision);
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
      let saved;
      if (section === "harness") {
        const runtimes = resetRuntimeFields(value.runtimes, demoSettings.runtimes, harnessFields);
        saved = mode === "demo" ? { ...value, runtimes, revision: (value?.revision || 1) + 1 } : await SettingsBinding.UpdateRuntimeSettings(runtimes, value.revision);
      } else {
        saved = mode === "demo" ? { ...value, [resetKey]: clone(demoSettings[resetKey]), revision: (value?.revision || 1) + 1 } : await SettingsBinding.ResetSettingsSection(section, value.revision);
      }
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
    } catch (error) {
      setRuntimeStatus((current) => ({ ...current, [id]: { available: false, error: message(error, t) } }));
      if (id === "codex") setCodexConfiguration((current) => ({ ...current, loading: false, error: message(error, t) }));
      if (id === "claude") setClaudeConfiguration((current) => ({ ...current, loading: false, error: message(error, t) }));
      return;
    }
    if (id === "codex") {
      setCodexConfiguration((current) => ({ ...current, loading: true, error: "" }));
      try {
        const data = mode === "demo" ? demoCodexConfiguration : await SettingsBinding.InspectCodexConfiguration(draft.runtimes.codex);
        setCodexConfiguration({ loading: false, data, error: "" });
      } catch (error) {
        setCodexConfiguration((current) => ({ ...current, loading: false, error: message(error, t) }));
      }
      return;
    }
    if (id === "claude") {
      setClaudeConfiguration((current) => ({ ...current, loading: true, error: "" }));
      try {
        const data = mode === "demo" ? demoClaudeConfiguration : await SettingsBinding.InspectClaudeConfiguration(draft.runtimes.claude);
        setClaudeConfiguration({ loading: false, data, error: "" });
      } catch (error) {
        setClaudeConfiguration((current) => ({ ...current, loading: false, error: message(error, t) }));
      }
      return;
    }
    if (mode === "demo") return;
    setHarnessConfigurations((current) => ({ ...current, [id]: { loading: true, data: null, error: "" } }));
    try {
      const data = await SettingsBinding.InspectHarnessConfiguration(id, draft.runtimes[id]);
      setHarnessConfigurations((current) => ({ ...current, [id]: { loading: false, data, error: "" } }));
    } catch (error) {
      // A harness that cannot report its models is not broken; the fields fall
      // back to free-text entry.
      setHarnessConfigurations((current) => ({ ...current, [id]: { loading: false, data: null, error: message(error, t) } }));
    }
  };
  const refreshUsage = async () => {
    setUsageLoading(true);
    try {
      setUsage(mode === "demo" ? { totalBytes: 7340032, root: "~/.onecatch", calculatedAt: new Date().toISOString(), categories: [{ name: "workflows", bytes: 163840, files: 4 }, { name: "runs", bytes: 4980736, files: 12 }, { name: "events", bytes: 491520, files: 12 }, { name: "logs", bytes: 1703936, files: 3 }] } : await SettingsBinding.GetStorageUsage());
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
      const result = mode === "demo" ? { path: diagnosticPath || "~/.onecatch/onecatch-diagnostics.zip" } : await SettingsBinding.ExportDiagnostics({ destination: diagnosticPath, runIds: [], ...diagnosticOptions });
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

  return <div className="settings-page relative grid min-h-0 flex-1 select-none grid-cols-[216px_minmax(0,1fr)] overflow-hidden bg-transparent text-foreground">
    <div className="settings-titlebar drag-region absolute inset-x-0 top-0 z-40 grid h-[52px] cursor-default grid-cols-[216px_minmax(0,1fr)] select-none" aria-hidden="true">
      <span className="settings-titlebar-sidebar pointer-events-none" />
      <span className="pointer-events-none bg-background" />
      <strong className="pointer-events-none absolute inset-0 flex items-center justify-center text-sm font-semibold tracking-[-0.01em] text-foreground/85">{t("sidebar.settings")}</strong>
    </div>
    <ScrollArea className="sidebar settings-sidebar relative z-30 min-h-0 select-none text-sidebar-foreground [clip-path:inset(8px_4px_8px_8px_round_16px)]">
      <aside className="flex min-h-full flex-col gap-1 px-3 pt-[52px] pb-4" aria-label={t("settings.sectionsAria")}>
        <SettingsKicker className="mb-2 px-2 text-[13px] tracking-[0.04em]">{t("settings.preferences")}</SettingsKicker>
        {sections.map((item) => <Button key={item.id} variant="ghost" className={`h-auto w-full justify-start rounded-lg px-3 py-2.5 text-left ${section === item.id ? "bg-accent text-accent-foreground hover:bg-accent" : "text-muted-foreground hover:bg-background/45"}`} aria-current={section === item.id ? "page" : undefined} onClick={() => switchSection(item.id)}>
          <span className="min-w-0"><strong className="flex items-center gap-2 text-[13px] font-medium text-foreground">{item.label}{section === item.id && dirty && <i className="size-1.5 rounded-full bg-primary" aria-label={t("settings.unsaved")} />}</strong><small className="mt-0.5 block whitespace-normal text-[11px] font-normal leading-snug text-muted-foreground">{item.description}</small></span>
        </Button>)}
      </aside>
    </ScrollArea>

    <ScrollArea className="settings-content min-h-0 min-w-0 bg-background">
      <section className="px-7 pt-[64px] pb-10">
        <header className="drag-region mb-6 flex items-start justify-between gap-4">
          <div className="min-w-0"><SettingsKicker>{t("settings.localSettings")}</SettingsKicker><h1 className="mt-1 mb-1 text-xl font-semibold text-foreground">{activeMeta.label}</h1><p className="m-0 text-sm text-muted-foreground">{activeMeta.description}</p></div>
          <div className="no-drag flex shrink-0 items-center gap-2.5"><span className="text-xs text-muted-foreground">{dirty ? t("settings.waitingSave") : t("settings.synced", { revision: value?.revision || 1 })}</span>{section !== "runtime" && <SettingsButton tone="muted" onClick={reset}>{t("settings.reset")}</SettingsButton>}</div>
        </header>
        {conflict && <div className="mb-4 flex items-center justify-between gap-4 rounded-lg border border-warning/30 bg-warning/8 px-4 py-3" role="alert"><div><strong className="block text-sm font-semibold text-foreground">{t("settings.conflictTitle")}</strong><span className="mt-0.5 block text-xs text-muted-foreground">{t("settings.conflictDescription")}</span></div><SettingsButton tone="muted" onClick={reload}>{t("settings.reload")}</SettingsButton></div>}
        {validationErrors.length > 0 && <div className="mb-4 rounded-lg border border-destructive/30 bg-destructive/7 px-4 py-3" role="alert"><strong className="block text-sm font-semibold text-destructive">{t("settings.validationCount", { count: validationErrors.length })}</strong><span className="mt-0.5 block text-xs text-muted-foreground">{t("settings.validationDescription")}</span></div>}
        {section === "runtime" && <InterfaceSettings i18n={i18n} />}
        {section === "harness" && <HarnessSettings value={draft.runtimes} setValue={(next) => setSectionValue("runtimes", next)} status={runtimeStatus} runtimes={runtimes} check={checkRuntime} errors={errorsByField} codexConfiguration={codexConfiguration} claudeConfiguration={claudeConfiguration} harnessConfigurations={harnessConfigurations} />}
        {section === "terminal" && <TerminalSettings value={draft.terminal || demoSettings.terminal} setValue={(next) => setSectionValue("terminal", next)} errors={errorsByField} />}
        {section === "execution" && <ExecutionSettings value={draft.execution} setValue={(next) => setSectionValue("execution", next)} errors={errorsByField} />}
        {section === "security" && <SecuritySettings value={draft.security} setValue={(next) => setSectionValue("security", next)} confirmFullAccess={confirmFullAccess} />}
        {section === "storage" && <StorageSettings value={draft.storage} setValue={(next) => setSectionValue("storage", next)} errors={errorsByField} security={draft.security} diagnosticOptions={diagnosticOptions} setDiagnosticOptions={setDiagnosticOptions} usage={usage} usageLoading={usageLoading} refreshUsage={refreshUsage} preview={preview} previewCleanup={previewCleanup} executeCleanup={executeCleanup} reveal={() => mode === "wails" && SettingsBinding.RevealDataRoot()} diagnosticPath={diagnosticPath} setDiagnosticPath={setDiagnosticPath} exportDiagnostics={exportDiagnostics} />}
        {section === "experimental" && <ExperimentalSettings draft={draft.experimental} saved={value?.experimental || demoSettings.experimental} setValue={(next) => setSectionValue("experimental", next)} workersPanel={workersPanel} />}
        {dirty && <div className="sticky bottom-0 mt-6 flex items-center justify-between gap-4 rounded-xl border bg-card px-4 py-3 shadow-lg" role="region" aria-label={t("settings.unsavedSettings")}><div className="min-w-0"><strong className="block text-sm font-semibold text-foreground">{t("settings.unsavedTitle")}</strong><span className="mt-0.5 block text-xs text-muted-foreground">{t("settings.unsavedDescription")}</span></div><div className="flex shrink-0 items-center gap-2"><SettingsButton tone="muted" disabled={saving} onClick={discard}>{t("settings.discard")}</SettingsButton><SettingsButton tone="primary" disabled={saving || validationErrors.length > 0} onClick={save}>{saving ? t("common.saving") : t("settings.save")}</SettingsButton></div></div>}
      </section>
    </ScrollArea>
    <ConfirmDialog dialog={dialog} busy={confirming} onCancel={closeDialog} onConfirm={acceptDialog} />
  </div>;
}

function InterfaceSettings({ i18n }) {
  const { t } = useTranslation();
  const [appearance, setAppearance] = useState(readAppearance);
  const updateLanguage = (language) => {
    const next = normalizeLanguage(language);
    void i18n.changeLanguage(next);
    void Events.Emit(LANGUAGE_CHANGED_EVENT, next);
  };
  const updateAppearance = (change) => {
    const next = saveAppearance({ ...appearance, ...change });
    setAppearance(next);
    void Events.Emit(APPEARANCE_CHANGED_EVENT, next);
  };
  return <SettingsSection title={t("settings.interface")} description={t("settings.interfaceDescription")}>
    <div className="grid gap-2">
      <div className="flex items-center justify-between gap-6 rounded-lg bg-muted/35 px-4 py-3.5"><div className="min-w-0"><h4 className="m-0 text-sm font-medium text-foreground">{t("settings.language")}</h4><p className="mt-0.5 mb-0 text-xs leading-relaxed text-muted-foreground">{t("settings.languageDescription")}</p></div><SettingsSelect className="w-44 shrink-0" ariaLabel={t("language.label")} value={i18n.resolvedLanguage} onChange={updateLanguage} options={[{ value: "zh-CN", label: t("language.chinese") }, { value: "en", label: t("language.english") }]} /></div>
      <div className="flex items-center justify-between gap-6 rounded-lg bg-muted/35 px-4 py-3.5"><div className="min-w-0"><h4 className="m-0 text-sm font-medium text-foreground">{t("settings.colorMode")}</h4><p className="mt-0.5 mb-0 text-xs leading-relaxed text-muted-foreground">{t("settings.colorModeDescription")}</p></div><div className="appearance-mode-picker inline-flex shrink-0 rounded-md border bg-muted p-0.5" role="radiogroup" aria-label={t("settings.colorMode")}>{themeModes.map((mode) => <Button type="button" variant="ghost" size="xs" role="radio" aria-checked={appearance.theme === mode} className={`rounded-sm px-3 ${appearance.theme === mode ? "bg-background text-foreground shadow-xs hover:bg-background" : "text-muted-foreground"}`} key={mode} onClick={() => updateAppearance({ theme: mode })}>{t(`settings.colorMode.${mode}`)}</Button>)}</div></div>
      <div className="flex items-center justify-between gap-6 rounded-lg bg-muted/35 px-4 py-3.5"><div className="min-w-0"><h4 className="m-0 text-sm font-medium text-foreground">{t("settings.chatFontSize")}</h4><p className="mt-0.5 mb-0 text-xs leading-relaxed text-muted-foreground">{t("settings.chatFontSizeDescription")}</p></div><div className="appearance-font-size-picker inline-flex shrink-0 rounded-md border bg-muted p-0.5" role="radiogroup" aria-label={t("settings.chatFontSize")}>{chatFontSizes.map((size) => <Button type="button" variant="ghost" size="xs" role="radio" aria-checked={appearance.chatFontSize === size} aria-label={t(`settings.chatFontSize.${size}`)} title={t(`settings.chatFontSize.${size}`)} className={`min-w-9 rounded-sm px-2 ${appearance.chatFontSize === size ? "bg-background text-foreground shadow-xs hover:bg-background" : "text-muted-foreground"}`} key={size} onClick={() => updateAppearance({ chatFontSize: size })}><span className="leading-none" style={{ fontSize: { small: 11, standard: 13, large: 15, "extra-large": 17 }[size] }}>A</span><span className="sr-only">{t(`settings.chatFontSize.${size}`)}</span></Button>)}</div></div>
      <div className="flex items-center justify-between gap-6 rounded-lg bg-muted/35 px-4 py-3.5"><div className="min-w-0"><h4 className="m-0 text-sm font-medium text-foreground">{t("settings.themeColor")}</h4><p className="mt-0.5 mb-0 text-xs leading-relaxed text-muted-foreground">{t("settings.themeColorDescription")}</p></div><div className="appearance-accent-picker inline-flex shrink-0 gap-1.5" role="radiogroup" aria-label={t("settings.themeColor")}>{accentThemes.map((accent) => <Button type="button" variant="outline" size="xs" role="radio" aria-checked={appearance.accent === accent} className={appearance.accent === accent ? "border-ring bg-accent text-foreground" : "text-muted-foreground"} key={accent} onClick={() => updateAppearance({ accent })}><i className="size-2.5 rounded-full" style={{ background: ACCENT_SWATCH[accent] }} aria-hidden="true" />{t(`settings.themeColor.${accent}`)}</Button>)}</div></div>
    </div>
  </SettingsSection>;
}

function TerminalSettings({ value, setValue, errors }) {
  const { t } = useTranslation();
  return <SettingsSection title={t("settings.terminalConfiguration")} description={t("settings.terminalConfigurationDescription")} contentClassName="p-4">
    <div className="grid gap-4">
      <SettingsField label={t("settings.terminalShell")} hint={t("settings.terminalShellHint")} error={errors.shell}>
        <Input className="font-mono text-[13px]" value={value.shell || ""} onChange={(event) => setValue({ ...value, shell: event.target.value })} placeholder={t("settings.terminalShellPlaceholder")} />
      </SettingsField>
      <SettingsField label={t("settings.terminalArguments")} hint={t("settings.terminalArgumentsHint")} error={errors.arguments}>
        <Textarea className="min-h-24 resize-y font-mono text-[13px]" value={(value.arguments || []).join("\n")} onChange={(event) => setValue({ ...value, arguments: event.target.value.split("\n") })} placeholder={t("settings.terminalArgumentsPlaceholder")} />
      </SettingsField>
      <SettingsField label={t("settings.terminalTheme")} hint={t("settings.terminalThemeHint")}>
        <SettingsSelect ariaLabel={t("settings.terminalTheme")} value={value.theme || "system"} onChange={(theme) => setValue({ ...value, theme })} options={["system", "paper", "midnight", "contrast"].map((theme) => ({ value: theme, label: t(`settings.terminalTheme.${theme}`) }))} />
      </SettingsField>
    </div>
    <p className="mt-4 mb-0 rounded-lg bg-muted/45 px-3.5 py-3 text-xs leading-relaxed text-muted-foreground">{t("settings.terminalRestartNote")}</p>
  </SettingsSection>;
}

// Every per-harness fact — display name, default command, which controls to
// show — comes from the backend's runtime list, which serves the same catalog
// the domain validates against. Keeping a second copy here is what let the UI
// offer a harness the backend then rejected.
function HarnessSettings({ value, setValue, status, runtimes, check, errors, codexConfiguration, claudeConfiguration, harnessConfigurations = {} }) {
  const catalog = runtimes.length ? runtimes : Object.keys(value).map((id) => ({ id, name: id, integrations: ["cli"] }));
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState({});
  const update = (id, field, next) => setValue({ ...value, [id]: { ...value[id], [field]: next } });
  const harnessByID = new Map(catalog.map((item) => [item.id, item]));
  // A harness whose credentials live in its own configuration advertises no
  // environment variables, so the field is described generically instead.
  const meta = Object.fromEntries(catalog.map((item) => [item.id, {
    name: item.name || item.id,
    command: item.command || item.id,
    env: item.environmentHint || t("settings.optionalEnv"),
  }]));
  return <section aria-labelledby="harness-list-title">
    <h2 id="harness-list-title" className="mb-3 text-[15px] font-semibold text-foreground">{t("settings.harnessList")}</h2>
    <div className="divide-y divide-border/70 border-y border-border/70">
    {catalog.map(({ id }) => {
      const harness = harnessByID.get(id) || {};
      // A harness with a single integration has no choice to offer.
      const integrations = harness.integrations || ["cli"];
      const integration = value[id]?.integration || integrations[0];
      const nativeModu = integration === "sdk";
      const moduConfigSource = value.modu?.configSource || "shared";
      const current = status[id] || runtimes.find((item) => item.id === id) || {};
      const statusText = current.checking ? t("settings.checkingCommand") : current.available ? current.version || t("common.available") : current.error || t("settings.notDetected");
      const codexData = codexConfiguration.data;
      const claudeData = claudeConfiguration.data;
      const reported = harnessConfigurations[id] || {};
      const reportedData = reported.data;
      const selectedReportedModel = (reportedData?.models || []).find((model) => model.model === value[id]?.defaultModel);
      // Effort levels can belong to the model rather than to the harness — Grok
      // offers xhigh on 4.6 but not on 4.5 — so narrow to the selected model
      // and fall back to the catalog only while nothing has been reported.
      const reportedEfforts = selectedReportedModel?.efforts?.length ? selectedReportedModel.efforts
        : reportedData?.efforts?.length ? reportedData.efforts
        : harness.efforts || [];
      const codexModel = id === "codex" ? selectedCodexModel(codexData, value.codex?.defaultModel) : null;
      const effortValues = id === "codex" ? codexEffortValues(codexData, value.codex?.defaultModel, value.codex?.reasoningEffort) : [];
      const claudeEffortValues = id === "claude" ? [...new Set([...(claudeData?.efforts?.length ? claudeData.efforts : ["low", "medium", "high", "xhigh", "max"]), value.claude?.reasoningEffort].filter(Boolean))] : [];
      const serviceTierValues = id === "codex" ? codexServiceTierValues(codexData, value.codex?.defaultModel, value.codex?.serviceTier) : [];
      let modelOptions = id === "codex" ? [{ value: "", label: t("settings.useCodexConfig"), meta: codexData?.model || t("settings.runtimeDefault") }, ...(codexData?.models || []).map((model) => ({ value: model.model, label: model.displayName || model.model, meta: model.model }))] : id === "claude" ? [{ value: "", label: t("settings.useClaudeConfig"), meta: t("settings.runtimeDefault") }, ...(claudeData?.models || []).map((model) => ({ value: model.model, label: model.displayName || model.model, meta: model.alias ? t("settings.claudeModelAlias") : model.model }))] : [];
      if (!modelOptions.length && (reportedData?.models || []).length) {
        modelOptions = [{ value: "", label: t("settings.runtimeDefault"), meta: reportedData.model || t("settings.runtimeDefault") },
          ...reportedData.models.map((model) => ({ value: model.model, label: model.displayName || model.model, meta: model.description || model.model }))];
      }
      if (value[id]?.defaultModel && !modelOptions.some((option) => option.value === value[id].defaultModel)) modelOptions.push({ value: value[id].defaultModel, label: value[id].defaultModel, meta: t("settings.savedCustomValue") });
      const detectedEffort = codexData?.reasoningEffort || codexModel?.defaultReasoningEffort || t("settings.runtimeDefault");
      const detectedTier = codexData?.serviceTier || t("settings.speed.standard");
      const tierDetails = new Map((codexModel?.serviceTiers || []).map((tier) => [tier.id, tier]));
      const updateModel = id === "codex" ? (defaultModel) => {
        const nextModel = selectedCodexModel(codexData, defaultModel);
        const supportedEfforts = nextModel?.reasoningEfforts || [];
        const supportedTiers = ["standard", ...(nextModel?.serviceTiers || []).map((tier) => tier.id)];
        setValue({ ...value, codex: { ...value.codex, defaultModel, reasoningEffort: value.codex?.reasoningEffort && supportedEfforts.length && !supportedEfforts.includes(value.codex.reasoningEffort) ? "" : value.codex?.reasoningEffort || "", serviceTier: value.codex?.serviceTier && supportedTiers.length > 1 && !supportedTiers.includes(value.codex.serviceTier) ? "" : value.codex?.serviceTier || "" } });
      } : (defaultModel) => {
        const nextModel = (reportedData?.models || []).find((model) => model.model === defaultModel);
        const supported = nextModel?.efforts || [];
        const current = value[id]?.reasoningEffort || "";
        // Carrying a level the newly selected model does not offer would save a
        // setting the run then rejects.
        const reasoningEffort = current && supported.length && !supported.includes(current) ? "" : current;
        setValue({ ...value, [id]: { ...value[id], defaultModel, reasoningEffort } });
      };
      const configurationLoading = id === "codex" ? codexConfiguration.loading : id === "claude" ? claudeConfiguration.loading : Boolean(reported.loading);
      const configurationError = id === "codex" ? codexConfiguration.error : id === "claude" ? claudeConfiguration.error : reported.error || "";
      const modelHint = id === "codex" && codexData ? t("settings.codexDetectedValue", { value: codexData.model || t("settings.runtimeDefault") }) : id === "claude" && claudeData ? t("settings.claudeModelsDetected", { count: claudeData.models?.length || 0 }) : t("settings.runtimeDecides");
      const configurationMessage = id === "codex" ? codexConfiguration.loading ? t("settings.readingCodexConfig") : codexConfiguration.error || (codexData && t("settings.codexConfigDetected", { model: codexData.model || t("settings.runtimeDefault"), effort: codexData.reasoningEffort || t("settings.runtimeDefault"), speed: codexData.serviceTier || t("settings.speed.standard") })) : id === "claude" ? claudeConfiguration.loading ? t("settings.readingClaudeModels") : claudeConfiguration.error || (claudeData && t("settings.claudeModelsReady", { count: claudeData.models?.length || 0, effortCount: claudeData.efforts?.length || 0 })) : reported.loading ? t("settings.readingHarnessModels") : reported.error || (reportedData && t("settings.harnessModelsReady", { count: reportedData.models?.length || 0 })) || "";
      const description = id === "modu" ? t(nativeModu ? "settings.moduSDKDescription" : "settings.moduDescription")
        : t(`settings.${id}Description`, { defaultValue: t("settings.harnessAgentDescription", { harness: meta[id].name }) });
      const enabled = value[id]?.enabled !== false;
      const supportsRemoteFS = Boolean(harness.supportsRemoteFs);
      const remoteFSEnabled = supportsRemoteFS && (value[id]?.remoteFsEnabled ?? true);
      const isExpanded = expanded[id];
      const panelID = `harness-${id}-settings`;
      return <section key={id}>
        <div className="group flex w-full items-center gap-3 rounded-lg px-1 py-4 transition-colors hover:bg-muted/35">
          <button type="button" className="min-w-0 flex-1 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50" aria-expanded={isExpanded} aria-controls={panelID} onClick={() => setExpanded((current) => ({ ...current, [id]: !current[id] }))}>
          <span className="min-w-0 flex-1">
            <span className="flex min-w-0 items-center gap-2"><strong className="shrink-0 text-sm font-semibold text-foreground">{meta[id].name}</strong><Badge variant="outline" title={statusText} className={`min-w-0 max-w-64 shrink truncate ${current.available ? "border-success/25 bg-success/8 text-success" : current.error ? "border-destructive/25 bg-destructive/8 text-destructive" : "text-muted-foreground"}`}><i className="size-1.5 shrink-0 rounded-full bg-current" /><span className="truncate">{statusText}</span></Badge></span>
            <span className="mt-1 block text-xs leading-relaxed text-muted-foreground">{description}</span>
          </span>
          </button>
          <div className="flex shrink-0 items-center border-l border-border/60 pl-3">
            <label className="flex cursor-pointer items-center gap-1.5 text-[11px] font-medium text-muted-foreground" htmlFor={`${panelID}-enabled`}>
              <span>{t(enabled ? "settings.harnessOn" : "settings.harnessOff")}</span>
              <Switch id={`${panelID}-enabled`} checked={enabled} aria-label={t("settings.harnessEnabledAria", { harness: meta[id].name })} onCheckedChange={(checked) => update(id, "enabled", checked)} />
            </label>
          </div>
          <button type="button" className="grid size-7 shrink-0 place-items-center rounded-md text-muted-foreground hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50" aria-label={t(isExpanded ? "settings.collapseHarness" : "settings.expandHarness", { harness: meta[id].name })} aria-expanded={isExpanded} aria-controls={panelID} onClick={() => setExpanded((current) => ({ ...current, [id]: !current[id] }))}><ChevronDown size={16} className={`transition-transform ${isExpanded ? "rotate-180" : ""}`} aria-hidden="true" /></button>
        </div>
        {isExpanded && <div id={panelID} className="px-1 pb-6">
          <div className="mb-4"><SettingsSwitchRow checked={remoteFSEnabled} disabled={!supportsRemoteFS || !enabled} label="Remote FS" description={t(supportsRemoteFS ? "settings.remoteFSSupported" : "settings.remoteFSUnsupported")} onChange={(checked) => update(id, "remoteFsEnabled", checked)} /></div>
          {/* Codex only: it is the one harness that defaults a model below the
              window the model accepts, and the one with a config override to
              raise it. Claude reports the largest window its auth can reach
              already, so there is nothing here to turn on. */}
          {id === "codex" && <div className="mb-4"><SettingsSwitchRow checked={value.codex?.maxContextWindow ?? false} disabled={!enabled} label={t("settings.maxContextWindow")} description={t("settings.maxContextWindowDescription")} onChange={(checked) => update(id, "maxContextWindow", checked)} /></div>}
          {configurationMessage && <div className={`mb-4 rounded-lg px-3 py-2 text-xs leading-relaxed ${configurationError ? "select-text bg-destructive/8 text-destructive" : "bg-primary/6 text-muted-foreground"}`} role={configurationError ? "alert" : "status"}>{configurationMessage}</div>}
          <div className="grid grid-cols-2 gap-4">
          {integrations.length > 1 && <SettingsField className="col-span-2" label={t("settings.integrationMode")} hint={t("settings.integrationModeHint")}><SettingsSelect ariaLabel={t("settings.integrationMode")} value={integration} onChange={(next) => update(id, "integration", next)} options={[{ value: "sdk", label: t("settings.integration.sdk"), meta: t("settings.integration.sdkMeta") }, { value: "cli", label: t("settings.integration.cli"), meta: t("settings.integration.cliMeta") }]} /></SettingsField>}
          {nativeModu && <SettingsField className="col-span-2" label={t("settings.moduConfigSource")} hint={t("settings.moduConfigSourceHint")}><SettingsSelect ariaLabel={t("settings.moduConfigSource")} value={moduConfigSource} onChange={(next) => update(id, "configSource", next)} options={[{ value: "onecatch", label: t("settings.moduConfig.onecatch"), meta: t("settings.moduConfig.onecatchMeta") }, { value: "shared", label: t("settings.moduConfig.shared"), meta: "~/.modu/config.toml" }]} /></SettingsField>}
          {nativeModu && moduConfigSource === "onecatch" && <SettingsField className="col-span-2" label={t("settings.moduConfigPath")} hint={t("settings.moduConfigPathHint")} error={errors[`${id}.configPath`]}><Input value={value[id]?.configPath || ""} aria-invalid={Boolean(errors[`${id}.configPath`])} onChange={(event) => update(id, "configPath", event.target.value)} placeholder={t("settings.moduConfigDefaultPath")} /></SettingsField>}
          {nativeModu && <div className="col-span-2 rounded-lg bg-primary/6 px-3.5 py-3 text-xs leading-relaxed text-muted-foreground"><strong className="block text-foreground">{t("settings.moduNativeConfig")}</strong><span className="mt-1 block">{t("settings.moduNativeConfigDescription")}</span><code className="mt-2 block select-text text-info">{moduConfigSource === "shared" ? "~/.modu/config.toml" : value[id]?.configPath || t("settings.moduConfigDefaultPath")}</code></div>}
          {!nativeModu && <SettingsField className="col-span-2" label={t("settings.binaryPath")} hint={t("settings.useCommand", { command: meta[id].command })} error={errors[`${id}.binary`]}><Input value={value[id]?.binary || ""} aria-invalid={Boolean(errors[`${id}.binary`])} onChange={(event) => update(id, "binary", event.target.value)} placeholder={meta[id].command} /></SettingsField>}
          {!nativeModu && <SettingsField className="col-span-2" label={t("settings.envAllowlist")} hint={t("settings.envAllowlistHint")} error={errors[`${id}.environmentAllowlist`]}><Input value={(value[id]?.environmentAllowlist || []).join(", ")} aria-invalid={Boolean(errors[`${id}.environmentAllowlist`])} onChange={(event) => update(id, "environmentAllowlist", event.target.value.toUpperCase().split(",").map((item) => item.trim()).filter(Boolean))} placeholder={meta[id].env} /></SettingsField>}
          <div className="col-span-2 border-t border-border/60" />
          <SettingsField className={id === "claude" || id === "modu" ? "col-span-2" : ""} label={t("settings.defaultModel")} hint={modelHint} error={errors[`${id}.defaultModel`]}>{modelOptions.length > 1 ? <SettingsSelect ariaLabel={t("settings.defaultModel")} value={value[id]?.defaultModel || ""} onChange={updateModel} options={modelOptions} /> : <Input value={value[id]?.defaultModel || ""} aria-invalid={Boolean(errors[`${id}.defaultModel`])} onChange={(event) => update(id, "defaultModel", event.target.value)} placeholder={t("settings.runtimeDefault")} />}</SettingsField>
          {id === "codex" && <SettingsField label={t("settings.reasoningEffort")} hint={t("settings.codexDetectedValue", { value: detectedEffort })} error={errors[`${id}.reasoningEffort`]}><SettingsSelect ariaLabel={t("settings.reasoningEffort")} value={value.codex?.reasoningEffort || ""} onChange={(reasoningEffort) => update("codex", "reasoningEffort", reasoningEffort)} options={[{ value: "", label: t("settings.useCodexConfig"), meta: detectedEffort }, ...(codexData && effortValues.length ? effortValues : ["minimal", "low", "medium", "high", "xhigh", "max", "ultra"]).map((effort) => ({ value: effort, label: t(`settings.reasoningEffort.${effort}`), meta: effort }))]} /></SettingsField>}
          {id === "claude" && <SettingsField className="col-span-2" label={t("settings.reasoningEffort")} hint={t("settings.claudeEffortsDetected", { count: claudeEffortValues.length })} error={errors[`${id}.reasoningEffort`]}><SettingsSelect ariaLabel={t("settings.reasoningEffort")} value={value.claude?.reasoningEffort || ""} onChange={(reasoningEffort) => update("claude", "reasoningEffort", reasoningEffort)} options={[{ value: "", label: t("settings.useClaudeConfig"), meta: t("settings.runtimeDefault") }, ...claudeEffortValues.map((effort) => ({ value: effort, label: t(`settings.reasoningEffort.${effort}`), meta: effort }))]} /></SettingsField>}
          {id !== "codex" && id !== "claude" && reportedEfforts.length > 0 && <SettingsField className="col-span-2" label={t("settings.reasoningEffort")} hint={selectedReportedModel ? t("settings.effortsForModel", { model: selectedReportedModel.displayName || selectedReportedModel.model }) : t("settings.runtimeDecides")} error={errors[`${id}.reasoningEffort`]}><SettingsSelect ariaLabel={t("settings.reasoningEffort")} value={value[id]?.reasoningEffort || ""} onChange={(reasoningEffort) => update(id, "reasoningEffort", reasoningEffort)} options={[{ value: "", label: t("settings.runtimeDefault"), meta: selectedReportedModel?.defaultEffort || "" }, ...reportedEfforts.map((effort) => ({ value: effort, label: t(`settings.reasoningEffort.${effort}`, { defaultValue: effort }), meta: effort }))]} /></SettingsField>}
          {id === "codex" && <SettingsField className="col-span-2" label={t("settings.speed")} hint={t("settings.codexDetectedValue", { value: detectedTier })} error={errors[`${id}.serviceTier`]}><SettingsSelect ariaLabel={t("settings.speed")} value={value.codex?.serviceTier || ""} onChange={(serviceTier) => update("codex", "serviceTier", serviceTier)} options={[{ value: "", label: t("settings.useCodexConfig"), meta: detectedTier }, ...(codexData ? serviceTierValues : ["standard", "fast", "priority", "flex"]).map((tier) => ({ value: tier, label: t(`settings.speed.${tier}`, { defaultValue: tierDetails.get(tier)?.name || tier }), meta: tierDetails.get(tier)?.description || tier }))]} /></SettingsField>}
          {(harness.providers || []).length > 0 && <SettingsField className="col-span-2" label={t("common.provider")} hint={t(`settings.providerHint.${id}`, { defaultValue: t("settings.providerHint") })} error={errors[`${id}.provider`]}><SettingsSelect ariaLabel={t("common.provider")} value={value[id]?.provider || harness.providers[0]} onChange={(provider) => update(id, "provider", provider)} options={harness.providers.map((provider) => ({ value: provider, label: t(`settings.provider.${provider}`, { defaultValue: provider }) }))} /></SettingsField>}
          </div>
          <div className="mt-4 flex justify-end"><SettingsButton tone="cyan" disabled={current.checking || configurationLoading} onClick={() => check(id)}>{current.checking || configurationLoading ? t("settings.checking") : id === "codex" ? t("settings.refreshCodexConfig") : id === "claude" ? t("settings.refreshClaudeModels") : t("settings.testConfig")}</SettingsButton></div>
        </div>}
      </section>;
    })}
    </div>
  </section>;
}

function ExecutionSettings({ value, setValue, errors }) {
  const { t } = useTranslation();
  const number = (key, next) => setValue({ ...value, [key]: Number(next) });
  return <SettingsSection title={t("settings.executionPolicy")} description={t("settings.executionPolicyDescription")} contentClassName="p-4">
    <div className="grid grid-cols-1 gap-4">
      <SettingsNumberField field="maxTransitions" label={t("settings.maxTransitions")} hint="1–10000" value={value.maxTransitions} error={errors.maxTransitions} onChange={(next) => number("maxTransitions", next)} />
      <SettingsNumberField field="maxConsecutiveFailures" label={t("settings.maxFailures")} hint="1–100" value={value.maxConsecutiveFailures} error={errors.maxConsecutiveFailures} onChange={(next) => number("maxConsecutiveFailures", next)} />
      <SettingsNumberField field="stepTimeoutSeconds" label={t("settings.nodeTimeout")} hint={t("settings.secondsRange", { min: 30, max: 86400 })} value={value.stepTimeoutSeconds} error={errors.stepTimeoutSeconds} onChange={(next) => number("stepTimeoutSeconds", next)} />
      <SettingsNumberField field="maxLocalDAGConcurrency" label={t("settings.dagConcurrency")} hint={t("settings.readonlyNodesRange")} value={value.maxLocalDAGConcurrency} error={errors.maxLocalDAGConcurrency} onChange={(next) => number("maxLocalDAGConcurrency", next)} />
      <SettingsNumberField field="interruptGraceSeconds" label={t("settings.interruptGrace")} hint={t("settings.secondsRange", { min: 1, max: 60 })} value={value.interruptGraceSeconds} error={errors.interruptGraceSeconds} onChange={(next) => number("interruptGraceSeconds", next)} />
      <SettingsField label={t("workspace.defaultSandbox")} hint={t("settings.noGlobalFullAccess")}><SettingsSelect ariaLabel={t("workspace.defaultSandbox")} value={value.defaultSandbox} onChange={(defaultSandbox) => setValue({ ...value, defaultSandbox })} options={[{ value: "read-only", label: t("workspace.readOnly") }, { value: "workspace-write", label: t("workspace.write") }]} /></SettingsField>
    </div>
  </SettingsSection>;
}

function SecuritySettings({ value, setValue, confirmFullAccess }) {
  const { t } = useTranslation();
  const toggle = (key) => (checked) => setValue({ ...value, [key]: checked });
  return <>
    <SettingsSection title={t("settings.executionAuth")} description={t("settings.executionAuthDescription")} contentClassName="p-4">
      <SettingsSwitchRow checked={value.allowFullSandbox} onChange={confirmFullAccess} label={t("settings.allowFullAccess")} description={t("settings.fullAccessDanger")} dangerous />
      <SettingsSwitchRow checked={value.confirmFullSandboxEveryRun} onChange={toggle("confirmFullSandboxEveryRun")} label={t("settings.confirmEveryRun")} description={value.allowFullSandbox ? t("settings.confirmEveryRunEnabled") : t("settings.confirmEveryRunDisabled")} disabled={!value.allowFullSandbox} />
    </SettingsSection>
    <SettingsSection title={t("settings.diagnosticPrivacy")} description={t("settings.diagnosticPrivacyDescription")} contentClassName="p-4">
      <SettingsSwitchRow checked={value.diagnosticsIncludePrompt} onChange={toggle("diagnosticsIncludePrompt")} label={t("settings.allowPrompt")} description={t("settings.confirmOnExport")} />
      <SettingsSwitchRow checked={value.diagnosticsIncludeRawEvents} onChange={toggle("diagnosticsIncludeRawEvents")} label={t("settings.allowRawEvents")} description={t("settings.rawEventsDescription")} />
    </SettingsSection>
  </>;
}

function StorageSettings({ value, setValue, errors, security, diagnosticOptions, setDiagnosticOptions, usage, usageLoading, refreshUsage, preview, previewCleanup, executeCleanup, reveal, diagnosticPath, setDiagnosticPath, exportDiagnostics }) {
  const { t, i18n } = useTranslation();
  const number = (key, next) => setValue({ ...value, [key]: Number(next) });
  return <>
    <SettingsSection title={t("settings.localData")} description={t("settings.localDataDescription")} aside={<SettingsButton tone="cyan" onClick={refreshUsage} disabled={usageLoading}>{usageLoading ? t("settings.calculating") : t("settings.recalculate")}</SettingsButton>} contentClassName="p-4">
      <div className="flex items-center gap-3"><div className="min-w-0 flex-1"><span className="mb-1 block text-xs text-muted-foreground">{t("settings.dataRoot")}</span><code className="block select-text truncate rounded-md bg-muted px-2 py-1.5 text-xs text-muted-foreground">~/.onecatch/</code></div><SettingsButton tone="muted" onClick={reveal}>{t("settings.revealFinder")}</SettingsButton></div>
      {usage ? <><div className="mt-5 text-xl font-semibold text-foreground">{bytes(usage.totalBytes)} <small className="text-xs font-normal text-muted-foreground">{t("settings.totalUsage")}</small></div><div className="mt-3 grid gap-2">{(usage.categories || []).map((item) => <div className="grid gap-1 text-xs text-muted-foreground" key={item.name}><span>{item.name}</span><b>{bytes(item.bytes)}</b><i className="block h-1.5 rounded-full bg-primary" style={{ width: `${Math.max(3, item.bytes / Math.max(usage.totalBytes, 1) * 100)}%` }} /></div>)}</div><p className="mt-3 mb-0 text-xs text-muted-foreground">{t("settings.lastCalculated", { time: usage.calculatedAt ? new Date(usage.calculatedAt).toLocaleString(i18n.resolvedLanguage === "en" ? "en-US" : "zh-CN") : t("settings.justNow") })}</p></> : <div className="mt-4 rounded-lg bg-muted p-3 text-xs text-muted-foreground">{usageLoading ? t("settings.calculatingUsage") : t("settings.noUsage")}</div>}
    </SettingsSection>
    <SettingsSection title={t("settings.historyCleanup")} description={t("settings.historyCleanupDescription")} contentClassName="p-4">
      <SettingsField label={t("settings.retention")} hint={t("settings.keepForeverDefault")}><SettingsSelect ariaLabel={t("settings.retention")} value={value.completedRunRetentionDays} onChange={(retentionDays) => { number("completedRunRetentionDays", retentionDays); setPreview(null); }} options={[{ value: 0, label: t("settings.keepForever") }, { value: 30, label: t("common.daysCount", { count: 30 }) }, { value: 90, label: t("common.daysCount", { count: 90 }) }, { value: 180, label: t("common.daysCount", { count: 180 }) }]} /></SettingsField>
      <div className="mt-4 flex flex-wrap items-center gap-2"><SettingsButton tone="muted" disabled={!value.completedRunRetentionDays} onClick={previewCleanup}>{t("settings.previewCleanup")}</SettingsButton></div>
      {preview?.token && <div className="mt-4 flex items-center justify-between gap-4 rounded-lg bg-muted p-4" role="status"><div><SettingsKicker>{t("settings.cleanupPreview")}</SettingsKicker><strong className="mt-1 block text-sm text-foreground">{t("settings.historicalRuns", { count: preview.count })}</strong><p className="mt-1 mb-0 text-xs text-muted-foreground">{t("settings.estimatedRelease", { size: bytes(preview.estimatedBytes) })}</p></div><SettingsButton tone="danger" disabled={!preview.count} onClick={executeCleanup}>{t("settings.irreversibleCleanup")}</SettingsButton></div>}
    </SettingsSection>
    <SettingsSection title={t("settings.logRotation")} description={t("settings.logRotationDescription")} contentClassName="p-4">
      <div className="grid grid-cols-[repeat(auto-fit,minmax(220px,1fr))] gap-4">
        <SettingsField label={t("settings.logLevel")} hint={t("settings.recommendedInfo")}><SettingsSelect ariaLabel={t("settings.logLevel")} value={value.logLevel} onChange={(logLevel) => setValue({ ...value, logLevel })} options={["error", "warn", "info", "debug"]} /></SettingsField>
        <SettingsNumberField field="logMaxSizeMB" label={t("settings.logFileSize")} hint="1–1024 MB" value={value.logMaxSizeMB} error={errors.logMaxSizeMB} onChange={(next) => number("logMaxSizeMB", next)} />
        <SettingsNumberField field="logMaxBackups" label={t("settings.backupCount")} hint="1–50" value={value.logMaxBackups} error={errors.logMaxBackups} onChange={(next) => number("logMaxBackups", next)} />
        <SettingsNumberField field="logMaxAgeDays" label={t("settings.logRetention")} hint={t("settings.daysRange", { min: 1, max: 365 })} value={value.logMaxAgeDays} error={errors.logMaxAgeDays} onChange={(next) => number("logMaxAgeDays", next)} />
      </div>
      {value.logLevel === "debug" && <div className="mt-4 rounded-lg bg-warning/10 p-3 text-xs text-warning"><strong className="block">{t("settings.debugWarning")}</strong><span>{t("settings.debugAdvice")}</span></div>}
    </SettingsSection>
    <SettingsSection title={t("settings.exportDiagnostics")} description={t("settings.exportDiagnosticsDescription")} contentClassName="p-4">
      <SettingsField label={t("settings.zipPath")} hint={t("settings.zipPathHint")}><Input value={diagnosticPath} onChange={(event) => setDiagnosticPath(event.target.value)} placeholder="/Users/me/Desktop/onecatch-diagnostics.zip" /></SettingsField>
      <SettingsSwitchRow checked={diagnosticOptions.includePrompt} onChange={(checked) => setDiagnosticOptions({ ...diagnosticOptions, includePrompt: checked })} label={t("settings.includePrompt")} description={security.diagnosticsIncludePrompt ? t("settings.confirmOnExport") : t("settings.authorizeSecurity")} disabled={!security.diagnosticsIncludePrompt} />
      <SettingsSwitchRow checked={diagnosticOptions.includeRawEvents} onChange={(checked) => setDiagnosticOptions({ ...diagnosticOptions, includeRawEvents: checked })} label={t("settings.includeRawEvents")} description={security.diagnosticsIncludeRawEvents ? t("settings.confirmOnExport") : t("settings.authorizeSecurity")} disabled={!security.diagnosticsIncludeRawEvents} />
      <div className="mt-4 flex flex-wrap items-center gap-2"><SettingsButton tone="primary" onClick={exportDiagnostics}>{t("settings.exportZip")}</SettingsButton></div>
    </SettingsSection>
  </>;
}

function ExperimentalSettings({ draft, saved, setValue, workersPanel }) {
  const { t } = useTranslation();
  return <>
    <div className="mb-7 rounded-lg border bg-card p-4 text-xs leading-relaxed text-muted-foreground" role="note"><SettingsKicker>{t("settings.experimentalBoundary")}</SettingsKicker><strong className="mt-1 block text-sm text-foreground">{t("settings.remoteTrustedOnly")}</strong><p className="mt-1 mb-0">{t("settings.remoteBoundaryDescription")}</p></div>
    <SettingsSection title={t("settings.remoteScheduling")} description={t("settings.remoteSchedulingDescription")} contentClassName="p-4">
      <SettingsSwitchRow checked={draft.remoteWorkersEnabled} onChange={(checked) => setValue({ ...draft, remoteWorkersEnabled: checked })} label={t("settings.enableRemoteWorker")} description={t("settings.enableRemoteDescription")} />
    </SettingsSection>
    {draft.remoteWorkersEnabled && saved.remoteWorkersEnabled && workersPanel}
    {draft.remoteWorkersEnabled && !saved.remoteWorkersEnabled && <div className="rounded-lg border bg-card p-4"><strong className="block text-sm text-foreground">{t("settings.enableAfterSave")}</strong><span className="mt-1 block text-xs text-muted-foreground">{t("settings.enableAfterSaveDescription")}</span></div>}
    {!draft.remoteWorkersEnabled && saved.remoteWorkersEnabled && <div className="rounded-lg border bg-card p-4"><strong className="block text-sm text-foreground">{t("settings.disableAfterSave")}</strong><span className="mt-1 block text-xs text-muted-foreground">{t("settings.disableAfterSaveDescription")}</span></div>}
  </>;
}

function validateSection(section, draft, t) {
  const errors = [];
  const add = (field, messageText) => errors.push({ field, message: messageText });
  if (section === "harness") {
    const forbidden = (key) => ["PATH", "HOME", "SHELL", "BASH_ENV", "ENV", "ZDOTDIR"].includes(key) || key.startsWith("DYLD_") || key.startsWith("LD_");
    for (const id of ["codex", "claude", "modu"]) {
      const runtime = draft.runtimes[id];
      if (id === "modu" && !["sdk", "cli"].includes(runtime.integration || "sdk")) add(`${id}.integration`, t("settings.validation.invalidIntegration"));
      if (/[\r\n\0]/.test(runtime.binary || "")) add(`${id}.binary`, t("settings.validation.pathCharacters"));
      const invalid = (runtime.environmentAllowlist || []).find((key) => !/^[A-Z_][A-Z0-9_]{0,127}$/.test(key) || forbidden(key));
      if (invalid) add(`${id}.environmentAllowlist`, t("settings.validation.invalidEnv", { key: invalid }));
      if (/[\r\n\0]/.test(runtime.defaultModel || "")) add(`${id}.defaultModel`, t("settings.validation.modelCharacters"));
      if (id === "codex" && !["", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"].includes(runtime.reasoningEffort || "")) add(`${id}.reasoningEffort`, t("settings.validation.invalidReasoningEffort"));
      if (id === "claude" && !["", "low", "medium", "high", "xhigh", "max"].includes(runtime.reasoningEffort || "")) add(`${id}.reasoningEffort`, t("settings.validation.invalidReasoningEffort"));
      if (id === "codex" && runtime.serviceTier && !/^[a-z0-9][a-z0-9_-]{0,63}$/.test(runtime.serviceTier)) add(`${id}.serviceTier`, t("settings.validation.invalidServiceTier"));
      if (id === "modu" && !["", "auto", "openai", "anthropic", "gemini"].includes(runtime.provider || "")) add(`${id}.provider`, t("settings.validation.invalidProvider"));
    }
  }
  if (section === "terminal") {
    if (/[\r\n\0]/.test(draft.terminal?.shell || "")) add("shell", t("settings.validation.pathCharacters"));
    if ((draft.terminal?.arguments || []).some((argument) => /[\r\0]/.test(argument))) add("arguments", t("settings.validation.argumentCharacters"));
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
