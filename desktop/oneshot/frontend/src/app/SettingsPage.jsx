import { useEffect, useMemo, useRef, useState } from "react";
import { SettingsBinding } from "../../bindings/github.com/openmodu/oneshot/desktop/oneshot/bindings/index.js";
import { Action, Field, NumberField, SettingPanel as SettingCard, TUISelect, ToggleRow as Toggle } from "../ui/primitives.jsx";

const sectionMeta = [
  { id: "runtime", label: "Runtime", description: "命令与模型" },
  { id: "execution", label: "执行", description: "新流程默认值" },
  { id: "security", label: "安全", description: "授权与诊断" },
  { id: "storage", label: "存储与日志", description: "本机数据管理" },
  { id: "experimental", label: "远端 Worker", description: "实验功能" },
];
const clone = (value) => JSON.parse(JSON.stringify(value));
const message = (error) => String(error?.message || error || "未知错误").replace(/^Error:\s*/, "");
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
        <span className="kicker">{dialog.eyebrow || (dialog.dangerous ? "DANGEROUS ACTION" : "CONFIRM CHANGE")}</span>
        <h2 id="confirm-title">{dialog.title}</h2>
        <p id="confirm-description">{dialog.description}</p>
        {dialog.detail && <div className="confirm-detail">{dialog.detail}</div>}
      </div>
      <div className="confirm-actions">
        <Action disabled={busy} onClick={onCancel}>{dialog.cancelLabel || "取消"}</Action>
        <Action autoFocus tone={dialog.dangerous ? "danger" : "primary"} disabled={busy} onClick={onConfirm}>{busy ? "处理中…" : dialog.confirmLabel || "确认"}</Action>
      </div>
    </section>
  </div>;
}

export default function SettingsPage({ mode, value, runtimes, onChange, notify, workersPanel }) {
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
  const validationErrors = useMemo(() => validateSection(section, draft), [draft, section]);
  const errorsByField = useMemo(() => Object.fromEntries(validationErrors.map((error) => [error.field, error.message])), [validationErrors]);
  const activeMeta = sectionMeta.find((item) => item.id === section);

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
    ask({ title: "放弃当前分区的修改？", description: `你在“${activeMeta.label}”中的修改还没有保存。切换后无法恢复。`, confirmLabel: "放弃并切换", dangerous: true }, () => switchSectionNow(next));
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
      notify("success", `${activeMeta.label}设置已保存并应用`);
      setConflict(false);
    } catch (error) {
      const text = message(error);
      if (text.includes("settings_state_conflict")) setConflict(true);
      notify("error", text);
    } finally { setSaving(false); }
  };
  const reset = () => ask({ title: `恢复“${activeMeta.label}”默认值？`, description: "当前分区会立即恢复为安全默认值，其他分区不会受到影响。", detail: "这个操作会直接保存，并覆盖当前分区尚未保存的草稿。", confirmLabel: "恢复默认", dangerous: true }, async () => {
    try {
      const resetKey = sectionKey(section);
      const saved = mode === "demo" ? { ...value, [resetKey]: clone(demoSettings[resetKey]), revision: (value?.revision || 1) + 1 } : await SettingsBinding.ResetSettingsSection(section, value.revision);
      onChange(clone(saved));
      setPreview(null);
      notify("success", `${activeMeta.label}已恢复默认值`);
    } catch (error) { notify("error", message(error)); }
  });
  const checkRuntime = async (id) => {
    setRuntimeStatus((current) => ({ ...current, [id]: { checking: true } }));
    try {
      const result = mode === "demo" ? { available: true, version: "preview", checkedAt: new Date().toISOString() } : await SettingsBinding.CheckRuntimeDraft({ runtime: id, settings: draft.runtimes[id] });
      setRuntimeStatus((current) => ({ ...current, [id]: result }));
    } catch (error) { setRuntimeStatus((current) => ({ ...current, [id]: { available: false, error: message(error) } })); }
  };
  const refreshUsage = async () => {
    setUsageLoading(true);
    try {
      setUsage(mode === "demo" ? { totalBytes: 7340032, root: "~/.oneshot", calculatedAt: new Date().toISOString(), categories: [{ name: "workflows", bytes: 163840, files: 4 }, { name: "runs", bytes: 4980736, files: 12 }, { name: "events", bytes: 491520, files: 12 }, { name: "logs", bytes: 1703936, files: 3 }] } : await SettingsBinding.GetStorageUsage());
    } catch (error) { notify("error", message(error)); } finally { setUsageLoading(false); }
  };
  useEffect(() => { if (section === "storage" && !usage && !usageLoading) refreshUsage(); }, [section]);
  const previewCleanup = async () => {
    try { setPreview(mode === "demo" ? { token: "demo", count: 3, estimatedBytes: 1048576, runIds: ["run-a", "run-b", "run-c"] } : await SettingsBinding.PreviewCleanup({ retentionDays: draft.storage.completedRunRetentionDays })); }
    catch (error) { notify("error", message(error)); }
  };
  const executeCleanup = () => {
    if (!preview?.token) return;
    ask({ title: `清理 ${preview.count} 个历史 Run？`, description: `预计释放 ${bytes(preview.estimatedBytes)}。active、running 和 paused Run 会继续保留。`, detail: "历史记录删除后无法恢复。", confirmLabel: "确认清理", dangerous: true }, async () => {
      try {
        const result = mode === "demo" ? { removedRunIds: preview.runIds } : await SettingsBinding.ExecuteCleanup(preview.token);
        setPreview(null);
        await refreshUsage();
        notify("success", `已清理 ${(result.removedRunIds || []).length} 个 Run`);
      } catch (error) { notify("error", message(error)); }
    });
  };
  const doExportDiagnostics = async () => {
    try {
      const result = mode === "demo" ? { path: diagnosticPath || "~/.oneshot/oneshot-diagnostics.zip" } : await SettingsBinding.ExportDiagnostics({ destination: diagnosticPath, runIds: [], ...diagnosticOptions });
      notify("success", `诊断包已导出：${result.path}`);
    } catch (error) { notify("error", message(error)); }
  };
  const exportDiagnostics = () => {
    if (!diagnosticOptions.includePrompt && !diagnosticOptions.includeRawEvents) { doExportDiagnostics(); return; }
    ask({ title: "导出包含敏感内容的诊断包？", description: "本次导出包含你已授权的任务 Prompt 或原始事件。环境变量值和 Worker token 仍不会导出。", confirmLabel: "确认导出", dangerous: true }, doExportDiagnostics);
  };
  const discard = () => setDraft(clone(value || demoSettings));
  const reload = async () => {
    try { const current = mode === "demo" ? demoSettings : await SettingsBinding.GetSettings(); onChange(current); setConflict(false); }
    catch (error) { notify("error", message(error)); }
  };
  const confirmFullAccess = (checked) => {
    if (!checked) { setSectionValue("security", { ...draft.security, allowFullSandbox: false }); return; }
    ask({ title: "启用 Full access sandbox？", description: "Full access 允许 Agent 在工作目录之外读取和写入。只应在你完全信任 Workflow 与指令时启用。", detail: "启用后，包含 Full access 的 Workflow 才能保存和启动。", confirmLabel: "我了解风险，启用", dangerous: true }, () => setSectionValue("security", { ...draft.security, allowFullSandbox: true }));
  };

  return <div className="settings-page">
    <aside className="settings-rail" aria-label="设置分区">
      <span className="kicker">PREFERENCES</span>
      {sectionMeta.map((item) => <button key={item.id} className={section === item.id ? "active" : ""} aria-current={section === item.id ? "page" : undefined} onClick={() => switchSection(item.id)}>
        <strong>{item.label}{section === item.id && dirty && <i className="dirty-dot" aria-label="有未保存修改" />}</strong>
        <small>{item.description}</small>
      </button>)}
    </aside>
    <section className="settings-content">
      <div className="settings-title">
        <div><span className="kicker">LOCAL SETTINGS</span><h2>{activeMeta.label}</h2><p>{activeMeta.description} · revision {value?.revision || 1}</p></div>
        <div className="settings-title-actions"><span className="settings-sync-state">{dirty ? "等待保存" : `已同步 · revision ${value?.revision || 1}`}</span><Action onClick={reset}>恢复默认</Action></div>
      </div>
      {conflict && <div className="settings-banner conflict" role="alert"><div><strong>设置已在另一个窗口变化</strong><span>当前草稿没有覆盖新值，请重新加载后再修改。</span></div><Action onClick={reload}>重新加载设置</Action></div>}
      {validationErrors.length > 0 && <div className="settings-banner invalid" role="alert"><div><strong>还有 {validationErrors.length} 个字段需要修正</strong><span>错误已标在对应字段下方，修正后才能保存。</span></div></div>}
      {section === "runtime" && <RuntimeSettings value={draft.runtimes} setValue={(next) => setSectionValue("runtimes", next)} status={runtimeStatus} runtimes={runtimes} check={checkRuntime} errors={errorsByField} />}
      {section === "execution" && <ExecutionSettings value={draft.execution} setValue={(next) => setSectionValue("execution", next)} errors={errorsByField} />}
      {section === "security" && <SecuritySettings value={draft.security} setValue={(next) => setSectionValue("security", next)} confirmFullAccess={confirmFullAccess} />}
      {section === "storage" && <StorageSettings value={draft.storage} setValue={(next) => setSectionValue("storage", next)} errors={errorsByField} security={draft.security} diagnosticOptions={diagnosticOptions} setDiagnosticOptions={setDiagnosticOptions} usage={usage} usageLoading={usageLoading} refreshUsage={refreshUsage} preview={preview} previewCleanup={previewCleanup} executeCleanup={executeCleanup} reveal={() => mode === "wails" && SettingsBinding.RevealDataRoot()} diagnosticPath={diagnosticPath} setDiagnosticPath={setDiagnosticPath} exportDiagnostics={exportDiagnostics} />}
      {section === "experimental" && <ExperimentalSettings draft={draft.experimental} saved={value?.experimental || demoSettings.experimental} setValue={(next) => setSectionValue("experimental", next)} workersPanel={workersPanel} />}
      {dirty && <div className="settings-savebar" role="region" aria-label="未保存设置">
        <div><strong>当前分区有未保存的修改</strong><span>保存后立即应用；不会追溯修改已启动的 Run。</span></div>
        <div><Action disabled={saving} onClick={discard}>放弃更改</Action><Action tone="primary" disabled={saving || validationErrors.length > 0} onClick={save}>{saving ? "保存中…" : "保存设置"}</Action></div>
      </div>}
    </section>
    <ConfirmDialog dialog={dialog} busy={confirming} onCancel={closeDialog} onConfirm={acceptDialog} />
  </div>;
}

function RuntimeSettings({ value, setValue, status, runtimes, check, errors }) {
  const update = (id, field, next) => setValue({ ...value, [id]: { ...value[id], [field]: next } });
  const meta = {
    codex: { name: "Codex", command: "codex", description: "留空时从 PATH 自动发现；测试配置只执行 version check，不启动 Agent，也不消耗模型额度。", env: "OPENAI_API_KEY, HTTPS_PROXY" },
    claude: { name: "Claude Code", command: "claude", description: "留空时从 PATH 自动发现；测试配置只执行 version check，不启动 Agent，也不消耗模型额度。", env: "ANTHROPIC_API_KEY, HTTPS_PROXY" },
    modu: { name: "Modu Code", command: "modu_code", description: "默认复用 ~/.modu/config.toml；任务通过 modu_code -p \"…\" -json 非交互执行，并用 --resume 恢复历史会话。旧 modu-code 仍可手动填写兼容。", env: "可选：仅在需要额外继承环境变量时填写" },
  };
  return <>{["codex", "claude", "modu"].map((id) => {
    const current = status[id] || runtimes.find((item) => item.id === id) || {};
    const statusText = current.checking ? "正在检测命令…" : current.available ? `${current.version || "可用"}${current.checkedAt ? ` · ${new Date(current.checkedAt).toLocaleTimeString("zh-CN")}` : ""}` : current.error || "未安装或尚未检测";
    return <SettingCard className="runtime-setting-card" key={id} title={meta[id].name} description={meta[id].description} aside={<span className={`setting-status ${current.available ? "ok" : current.error ? "bad" : ""}`}><i />{statusText}</span>}>
      <div className="settings-grid">
        <Field label="Binary 路径" hint={`留空使用 ${meta[id].command}`} error={errors[`${id}.binary`]}><input value={value[id]?.binary || ""} aria-invalid={Boolean(errors[`${id}.binary`])} onChange={(event) => update(id, "binary", event.target.value)} placeholder={meta[id].command} /></Field>
        <Field label="默认模型" hint="留空由 Runtime 决定" error={errors[`${id}.defaultModel`]}><input value={value[id]?.defaultModel || ""} aria-invalid={Boolean(errors[`${id}.defaultModel`])} onChange={(event) => update(id, "defaultModel", event.target.value)} placeholder="Runtime 默认" /></Field>
        {id === "modu" && <Field label="Provider" hint="自动会根据允许继承的 API Key 检测" error={errors[`${id}.provider`]}><TUISelect ariaLabel="Provider" value={value[id]?.provider || "auto"} onChange={(provider) => update(id, "provider", provider)} options={[{ value: "auto", label: "自动检测" }, { value: "openai", label: "OpenAI / Compatible" }, { value: "anthropic", label: "Anthropic" }, { value: "gemini", label: "Gemini" }]} /></Field>}
        <Field className="full" label="环境变量 Key 白名单" hint="只保存变量名，不保存值；使用逗号分隔" error={errors[`${id}.environmentAllowlist`]}><input value={(value[id]?.environmentAllowlist || []).join(", ")} aria-invalid={Boolean(errors[`${id}.environmentAllowlist`])} onChange={(event) => update(id, "environmentAllowlist", event.target.value.toUpperCase().split(",").map((item) => item.trim()).filter(Boolean))} placeholder={meta[id].env} /></Field>
      </div>
      <div className="settings-actions"><Action tone="cyan" disabled={current.checking} onClick={() => check(id)}>{current.checking ? "检测中…" : "测试配置"}</Action></div>
    </SettingCard>;
  })}</>;
}

function ExecutionSettings({ value, setValue, errors }) {
  const number = (key, next) => setValue({ ...value, [key]: Number(next) });
  return <SettingCard title="默认执行策略" description="节点显式配置优先于 Workflow，Workflow 又优先于这里的全局默认值。只影响新 Workflow 或没有显式值的新 Run。">
    <div className="settings-grid execution-grid">
      <NumberField field="maxTransitions" label="最大转移次数" hint="1–10000" value={value.maxTransitions} error={errors.maxTransitions} onChange={(next) => number("maxTransitions", next)} />
      <NumberField field="maxConsecutiveFailures" label="连续失败上限" hint="1–100" value={value.maxConsecutiveFailures} error={errors.maxConsecutiveFailures} onChange={(next) => number("maxConsecutiveFailures", next)} />
      <NumberField field="stepTimeoutSeconds" label="单节点超时" hint="30–86400 秒" value={value.stepTimeoutSeconds} error={errors.stepTimeoutSeconds} onChange={(next) => number("stepTimeoutSeconds", next)} />
      <NumberField field="maxLocalDAGConcurrency" label="DAG 本地并发" hint="1–16 个只读节点" value={value.maxLocalDAGConcurrency} error={errors.maxLocalDAGConcurrency} onChange={(next) => number("maxLocalDAGConcurrency", next)} />
      <NumberField field="interruptGraceSeconds" label="中断宽限时间" hint="1–60 秒" value={value.interruptGraceSeconds} error={errors.interruptGraceSeconds} onChange={(next) => number("interruptGraceSeconds", next)} />
      <Field label="默认 Sandbox" hint="Full access 不能设为全局默认"><TUISelect ariaLabel="默认 Sandbox" value={value.defaultSandbox} onChange={(defaultSandbox) => setValue({ ...value, defaultSandbox })} options={[{ value: "read-only", label: "Read only" }, { value: "workspace-write", label: "Workspace write" }]} /></Field>
    </div>
  </SettingCard>;
}

function SecuritySettings({ value, setValue, confirmFullAccess }) {
  const toggle = (key) => (checked) => setValue({ ...value, [key]: checked });
  return <>
    <SettingCard title="执行授权" description="关闭 Full access 后，旧 Workflow 仍可查看和编辑，但保存、启动和恢复都会被安全检查阻止。">
      <Toggle checked={value.allowFullSandbox} onChange={confirmFullAccess} label="允许 Full access sandbox" description="危险：Agent 可在工作目录之外执行读写操作。" dangerous />
      <Toggle checked={value.confirmFullSandboxEveryRun} onChange={toggle("confirmFullSandboxEveryRun")} label="每次运行前再次确认" description={value.allowFullSandbox ? "启动包含 Full access 的 Run 前要求一次性确认。" : "启用 Full access 后才能修改此项。"} disabled={!value.allowFullSandbox} />
    </SettingCard>
    <SettingCard title="诊断隐私" description="环境变量值和 Worker token 永不导出；这里仅授权导出器在单次导出时显示敏感选项。">
      <Toggle checked={value.diagnosticsIncludePrompt} onChange={toggle("diagnosticsIncludePrompt")} label="允许诊断包包含任务 Prompt" description="每次实际导出时仍会再次确认。" />
      <Toggle checked={value.diagnosticsIncludeRawEvents} onChange={toggle("diagnosticsIncludeRawEvents")} label="允许诊断包包含原始 Runtime event" description="可能包含本地命令、模型输出和工具调用摘要。" />
    </SettingCard>
  </>;
}

function StorageSettings({ value, setValue, errors, security, diagnosticOptions, setDiagnosticOptions, usage, usageLoading, refreshUsage, preview, previewCleanup, executeCleanup, reveal, diagnosticPath, setDiagnosticPath, exportDiagnostics }) {
  const number = (key, next) => setValue({ ...value, [key]: Number(next) });
  return <>
    <SettingCard title="本地数据" description="数据根目录固定为 ~/.oneshot/，统计只扫描已知子目录且不会跟随符号链接。" aside={<Action tone="cyan" onClick={refreshUsage} disabled={usageLoading}>{usageLoading ? "计算中…" : "重新计算"}</Action>}>
      <div className="data-root-row"><div><span>数据根目录</span><code>~/.oneshot/</code></div><Action onClick={reveal}>在 Finder 中显示</Action></div>
      {usage ? <><div className="usage-total">{bytes(usage.totalBytes)} <small>总占用</small></div><div className="usage-bars">{(usage.categories || []).map((item) => <div key={item.name}><span>{item.name}</span><b>{bytes(item.bytes)}</b><i style={{ width: `${Math.max(3, item.bytes / Math.max(usage.totalBytes, 1) * 100)}%` }} /></div>)}</div><p className="storage-calculated-at">最后计算：{usage.calculatedAt ? new Date(usage.calculatedAt).toLocaleString("zh-CN") : "刚刚"}</p></> : <div className="settings-inline-empty">{usageLoading ? "正在计算本机空间用量…" : "还没有空间用量数据。"}</div>}
    </SettingCard>
    <SettingCard title="历史 Run 清理" description="只清理超过保留期的 completed / cancelled Run；active、running、paused 永远跳过。">
      <Field label="完成记录保留期" hint="默认永久保留"><TUISelect ariaLabel="完成记录保留期" value={value.completedRunRetentionDays} onChange={(retentionDays) => { number("completedRunRetentionDays", retentionDays); setPreview(null); }} options={[{ value: 0, label: "永久保留" }, { value: 30, label: "30 天" }, { value: 90, label: "90 天" }, { value: 180, label: "180 天" }]} /></Field>
      <div className="settings-actions"><Action tone="muted" disabled={!value.completedRunRetentionDays} onClick={previewCleanup}>预览清理</Action></div>
      {preview?.token && <div className="cleanup-preview" role="status"><div><span className="kicker">CLEANUP PREVIEW</span><strong>{preview.count} 个历史 Run</strong><p>预计释放 {bytes(preview.estimatedBytes)}，执行前会再次检查运行状态。</p></div><Action tone="danger" disabled={!preview.count} onClick={executeCleanup}>不可逆清理</Action></div>}
    </SettingCard>
    <SettingCard title="日志轮转" description="保存后立即应用新的日志级别和轮转限制。">
      <div className="settings-grid">
        <Field label="日志级别" hint="推荐 info"><TUISelect ariaLabel="日志级别" value={value.logLevel} onChange={(logLevel) => setValue({ ...value, logLevel })} options={["error", "warn", "info", "debug"]} /></Field>
        <NumberField field="logMaxSizeMB" label="单文件大小" hint="1–1024 MB" value={value.logMaxSizeMB} error={errors.logMaxSizeMB} onChange={(next) => number("logMaxSizeMB", next)} />
        <NumberField field="logMaxBackups" label="备份文件数" hint="1–50" value={value.logMaxBackups} error={errors.logMaxBackups} onChange={(next) => number("logMaxBackups", next)} />
        <NumberField field="logMaxAgeDays" label="日志保留期" hint="1–365 天" value={value.logMaxAgeDays} error={errors.logMaxAgeDays} onChange={(next) => number("logMaxAgeDays", next)} />
      </div>
      {value.logLevel === "debug" && <div className="settings-note warning"><strong>Debug 会明显增加本地磁盘使用</strong><span>仅在排障期间开启，完成后建议切回 info。</span></div>}
    </SettingCard>
    <SettingCard title="导出诊断" description="默认包含版本、脱敏设置、Run 状态、错误码和日志；不包含 token、环境变量值或完整本地路径。">
      <Field label="ZIP 保存路径" hint="留空时保存到数据目录"><input value={diagnosticPath} onChange={(event) => setDiagnosticPath(event.target.value)} placeholder="/Users/me/Desktop/oneshot-diagnostics.zip" /></Field>
      <Toggle checked={diagnosticOptions.includePrompt} onChange={(checked) => setDiagnosticOptions({ ...diagnosticOptions, includePrompt: checked })} label="本次包含 Prompt" description={security.diagnosticsIncludePrompt ? "导出时会再次确认。" : "需要先在“安全”中授权。"} disabled={!security.diagnosticsIncludePrompt} />
      <Toggle checked={diagnosticOptions.includeRawEvents} onChange={(checked) => setDiagnosticOptions({ ...diagnosticOptions, includeRawEvents: checked })} label="本次包含原始事件" description={security.diagnosticsIncludeRawEvents ? "导出时会再次确认。" : "需要先在“安全”中授权。"} disabled={!security.diagnosticsIncludeRawEvents} />
      <div className="settings-actions"><Action onClick={exportDiagnostics}>导出诊断 ZIP</Action></div>
    </SettingCard>
  </>;
}

function ExperimentalSettings({ draft, saved, setValue, workersPanel }) {
  return <>
    <div className="settings-boundary" role="note"><span className="kicker">EXPERIMENTAL BOUNDARY</span><strong>远端 Worker 仅适用于受信任 LAN / VPN</strong><p>Oneshot 不同步项目文件；远端机器需要相同 Workspace ID 的本地 clone。首版远端节点只建议运行 read-only 工作。</p></div>
    <SettingCard title="远端 Worker 调度" description="默认关闭。关闭不会删除已有 Worker token 或配置，只会从新 DAG 和启动路径中移除远端选项。">
      <Toggle checked={draft.remoteWorkersEnabled} onChange={(checked) => setValue({ ...draft, remoteWorkersEnabled: checked })} label="启用远端 Worker" description="保存后在本分区显示 Worker 管理，并允许新 DAG 选择远端节点。" />
    </SettingCard>
    {draft.remoteWorkersEnabled && saved.remoteWorkersEnabled && workersPanel}
    {draft.remoteWorkersEnabled && !saved.remoteWorkersEnabled && <div className="settings-pending panel"><strong>保存后启用 Worker 管理</strong><span>当前只是草稿，远端 Worker 尚未进入调度选项。</span></div>}
    {!draft.remoteWorkersEnabled && saved.remoteWorkersEnabled && <div className="settings-pending panel"><strong>保存后停止远端调度</strong><span>Worker token 与配置会保留，重新启用后可继续使用。</span></div>}
    {!draft.remoteWorkersEnabled && !saved.remoteWorkersEnabled && <div className="settings-disabled panel"><strong>远端调度当前关闭</strong><span>本地串行 Loop 和并行 DAG 不受影响。</span></div>}
  </>;
}

function validateSection(section, draft) {
  const errors = [];
  const add = (field, messageText) => errors.push({ field, message: messageText });
  if (section === "runtime") {
    const forbidden = (key) => ["PATH", "HOME", "SHELL", "BASH_ENV", "ENV", "ZDOTDIR"].includes(key) || key.startsWith("DYLD_") || key.startsWith("LD_");
    for (const id of ["codex", "claude", "modu"]) {
      const runtime = draft.runtimes[id];
      if (/[\r\n\0]/.test(runtime.binary || "")) add(`${id}.binary`, "路径不能包含换行或控制字符");
      if (/[\r\n\0]/.test(runtime.defaultModel || "")) add(`${id}.defaultModel`, "模型名不能包含换行或控制字符");
      const invalid = (runtime.environmentAllowlist || []).find((key) => !/^[A-Z_][A-Z0-9_]{0,127}$/.test(key) || forbidden(key));
      if (invalid) add(`${id}.environmentAllowlist`, `${invalid} 不是允许继承的环境变量 Key`);
      if (id === "modu" && !["", "auto", "openai", "anthropic", "gemini"].includes(runtime.provider || "")) add(`${id}.provider`, "Provider 配置无效");
    }
  }
  if (section === "execution") {
    const rules = [["maxTransitions", "最大转移次数", 1, 10000], ["maxConsecutiveFailures", "连续失败上限", 1, 100], ["stepTimeoutSeconds", "单节点超时", 30, 86400], ["maxLocalDAGConcurrency", "DAG 本地并发", 1, 16], ["interruptGraceSeconds", "中断宽限时间", 1, 60]];
    rules.forEach(([field, label, min, max]) => { const current = draft.execution[field]; if (!Number.isInteger(current) || current < min || current > max) add(field, `${label}必须在 ${min}–${max} 之间`); });
  }
  if (section === "storage") {
    [["logMaxSizeMB", "单文件大小", 1, 1024], ["logMaxBackups", "备份文件数", 1, 50], ["logMaxAgeDays", "日志保留期", 1, 365]].forEach(([field, label, min, max]) => { const current = draft.storage[field]; if (!Number.isInteger(current) || current < min || current > max) add(field, `${label}必须在 ${min}–${max} 之间`); });
  }
  return errors;
}
