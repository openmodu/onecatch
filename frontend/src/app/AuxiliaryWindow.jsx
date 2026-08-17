import { lazy, Suspense, useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Events } from "@wailsio/runtime";
import {
  RuntimeBinding,
  SettingsBinding,
  WorkflowBinding,
  WorkspaceBinding,
  WorkerBinding,
} from "../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js";
import SettingsPage, { ConfirmDialog, demoSettings } from "./SettingsPage.jsx";
import { copy, errorMessage } from "./format.js";
import { loopTemplate } from "./templates.js";
import { nextWorkflowDefinitionID } from "./workflowIds.js";
import { demoRuntimes, demoWorkers, demoWorkflows, demoWorkspaces } from "./demoData.js";

const WorkflowLibrary = lazy(() => import("./components/workflow/WorkflowLibrary.jsx"));
const WorkflowEditor = lazy(() => import("./components/workflow/WorkflowEditor.jsx"));
const WorkerPage = lazy(() => import("./components/WorkerPage.jsx"));
const WorkerModal = lazy(() => import("./components/WorkerModal.jsx"));

export const settingsChangedEvent = "onecatch:settings-changed";
export const workflowsChangedEvent = "onecatch:workflows-changed";

const emptyWorkerForm = () => ({ id: "", name: "", baseUrl: "https://", caFile: "", clientCertFile: "", clientKeyFile: "", serverName: "", serverCertificateSha256: "", enabled: true });
const editWorkerForm = (worker) => ({ id: worker.id, name: worker.name, baseUrl: worker.baseUrl, caFile: worker.caFile || "", clientCertFile: worker.clientCertFile || "", clientKeyFile: worker.clientKeyFile || "", serverName: worker.serverName || "", serverCertificateSha256: worker.serverCertificateSha256 || "", enabled: worker.enabled });
const loadingRuntimes = [
  { id: "codex", name: "Codex", checking: true },
  { id: "claude", name: "Claude Code", checking: true },
  { id: "modu", name: "Modu Code", checking: true },
];

let settingsBootstrapPromise;
let settingsSupportPromise;

function loadSettingsBootstrap() {
  settingsBootstrapPromise ||= SettingsBinding.GetSettings();
  return settingsBootstrapPromise;
}

function loadSettingsSupport() {
  settingsSupportPromise ||= Promise.allSettled([
    RuntimeBinding.ListRuntimes(),
    WorkspaceBinding.ListWorkspaces(),
    WorkerBinding.ListWorkers(),
  ]);
  return settingsSupportPromise;
}

function scheduleIdle(callback) {
  if (typeof window.requestIdleCallback === "function") {
    const id = window.requestIdleCallback(callback, { timeout: 600 });
    return () => window.cancelIdleCallback(id);
  }
  const id = window.setTimeout(callback, 0);
  return () => window.clearTimeout(id);
}

function LoadingWindow() {
  const { t } = useTranslation();
  return <div className="flex h-full items-center justify-center bg-background text-sm text-muted-foreground">{t("task.opening")}</div>;
}

function useNotice() {
  const [notice, setNotice] = useState(null);
  const notify = useCallback((type, text) => {
    setNotice({ type, text });
    window.setTimeout(() => setNotice(null), 4200);
  }, []);
  return { notice, notify };
}

function useConfirmDialog() {
  const [dialog, setDialog] = useState(null);
  const resolveRef = useRef(null);
  const requestConfirm = useCallback((options) => new Promise((resolve) => {
    resolveRef.current?.(false);
    resolveRef.current = resolve;
    setDialog(options);
  }), []);
  const resolveConfirm = useCallback((accepted) => {
    const resolve = resolveRef.current;
    resolveRef.current = null;
    setDialog(null);
    resolve?.(accepted);
  }, []);
  return { dialog, requestConfirm, resolveConfirm };
}

export function SettingsWindow() {
  const { t } = useTranslation();
  const [mode, setMode] = useState("loading");
  const [runtimes, setRuntimes] = useState(loadingRuntimes);
  const [workspaces, setWorkspaces] = useState([]);
  const [workers, setWorkers] = useState([]);
  const [workerHealth, setWorkerHealth] = useState({});
  const [settings, setSettings] = useState(demoSettings);
  const [workerModal, setWorkerModal] = useState(false);
  const [workerForm, setWorkerForm] = useState(emptyWorkerForm);
  const [busy, setBusy] = useState("");
  const { notice, notify } = useNotice();
  const { dialog, requestConfirm, resolveConfirm } = useConfirmDialog();

  useEffect(() => {
    let active = true;
    loadSettingsBootstrap()
      .then((settingsValue) => {
        if (!active) return;
        setSettings(settingsValue || demoSettings);
        setMode("wails");
      })
      .catch(() => {
        if (!active) return;
        setRuntimes(demoRuntimes);
        setWorkspaces(demoWorkspaces);
        setWorkers(demoWorkers);
        setSettings(demoSettings);
        setMode("demo");
      });
    return () => { active = false; };
  }, []);

  useEffect(() => {
    if (mode !== "wails") return undefined;
    let active = true;
    const cancelIdle = scheduleIdle(() => {
      loadSettingsSupport().then(([runtimeResult, workspaceResult, workerResult]) => {
        if (!active) return;
        setRuntimes(runtimeResult.status === "fulfilled" ? runtimeResult.value || [] : demoRuntimes);
        if (workspaceResult.status === "fulfilled") setWorkspaces(workspaceResult.value || []);
        if (workerResult.status === "fulfilled") setWorkers(workerResult.value || []);
      });
    });
    return () => { active = false; cancelIdle(); };
  }, [mode]);

  useEffect(() => {
    const nativeSidebar = globalThis.webkit?.messageHandlers?.onecatchSidebar;
    if (!nativeSidebar || mode === "loading") return;
    document.documentElement.dataset.nativeSidebarMaterial = "true";
    nativeSidebar.postMessage({ width: document.querySelector(".settings-sidebar")?.getBoundingClientRect().width || 216 });
  }, [mode]);

  const updateSettings = useCallback((value) => {
    setSettings(value);
    void Events.Emit(settingsChangedEvent, { revision: value?.revision || 0 });
  }, []);

  const openWorker = useCallback((worker) => {
    setWorkerForm(worker ? editWorkerForm(worker) : emptyWorkerForm());
    setWorkerModal(true);
  }, []);

  const updateWorker = async () => {
    if (!workerForm.id.trim() || !workerForm.name.trim() || !workerForm.baseUrl.trim()) {
      notify("error", t("app.workerFieldsRequired"));
      return;
    }
    setBusy("worker");
    try {
      if (mode === "demo") setWorkers((items) => [...items.filter((item) => item.id !== workerForm.id), { ...workerForm, hasToken: true }]);
      else {
        await WorkerBinding.UpdateWorker(workerForm);
        setWorkers(await WorkerBinding.ListWorkers());
      }
      setWorkerModal(false);
      notify("success", t("app.workerSaved"));
    } catch (error) {
      notify("error", errorMessage(error));
    } finally {
      setBusy("");
    }
  };

  const pairWorker = async (baseURL, code) => {
    if (!baseURL.trim() || !code.trim()) {
      notify("error", t("app.workerPairingFieldsRequired"));
      return;
    }
    setBusy("worker-pair");
    try {
      if (mode === "demo") setWorkers((items) => [...items, { id: "paired-worker", name: "Paired Worker", baseUrl: baseURL, hasToken: true, enabled: true }]);
      else {
        await WorkerBinding.PairWorker(baseURL, code);
        setWorkers(await WorkerBinding.ListWorkers());
      }
      setWorkerModal(false);
      notify("success", t("app.workerPaired"));
    } catch (error) {
      notify("error", t("app.workerPairingFailed", { error: errorMessage(error) }));
    } finally {
      setBusy("");
    }
  };

  const checkWorker = useCallback(async (worker) => {
    setWorkerHealth((current) => ({ ...current, [worker.id]: { ...current[worker.id], checking: true } }));
    try {
      const status = mode === "demo" ? { health: { workerId: worker.id, name: worker.name, runtimes: { codex: true, claude: true, modu: true } } } : await WorkerBinding.CheckWorker(worker.id);
      setWorkerHealth((current) => ({ ...current, [worker.id]: { ok: true, checking: false, ...status.health, checkedAt: status.checkedAt, latencyMilliseconds: status.latencyMilliseconds } }));
    } catch (error) {
      setWorkerHealth((current) => ({ ...current, [worker.id]: { ok: false, checking: false, error: errorMessage(error) } }));
    }
  }, [mode]);

  const deleteWorker = useCallback(async (id) => {
    const worker = workers.find((item) => item.id === id);
    if (!await requestConfirm({ title: t("app.deleteWorkerTitle", { name: worker?.name || id }), description: t("app.deleteWorkerDescription"), confirmLabel: t("app.deleteWorker"), dangerous: true })) return;
    try {
      if (mode === "demo") setWorkers((items) => items.filter((item) => item.id !== id));
      else {
        await WorkerBinding.DeleteWorker(id);
        setWorkers(await WorkerBinding.ListWorkers());
      }
    } catch (error) {
      notify("error", errorMessage(error));
    }
  }, [mode, notify, requestConfirm, t, workers]);

  if (mode === "loading") return <LoadingWindow />;
  return <div className="flex h-full min-h-0 flex-col overflow-hidden bg-transparent text-foreground">
    <SettingsPage mode={mode} value={settings} runtimes={runtimes} onChange={updateSettings} notify={notify} workersPanel={<Suspense fallback={<div className="p-4 text-sm text-muted-foreground">{t("task.opening")}</div>}><WorkerPage mode={mode} workspace={workspaces[0]} workers={workers} health={workerHealth} checkWorker={checkWorker} deleteWorker={deleteWorker} notify={notify} openWorker={openWorker} /></Suspense>} />
    {workerModal && <Suspense fallback={null}><WorkerModal form={workerForm} setForm={setWorkerForm} busy={busy} onClose={() => setWorkerModal(false)} onUpdate={updateWorker} onPair={pairWorker} /></Suspense>}
    <ConfirmDialog dialog={dialog} onCancel={() => resolveConfirm(false)} onConfirm={() => resolveConfirm(true)} />
    {notice && <div className={`toast ${notice.type}`}><span>{notice.text}</span></div>}
  </div>;
}

export function WorkflowsWindow() {
  const { t } = useTranslation();
  const [mode, setMode] = useState("loading");
  const [runtimes, setRuntimes] = useState([]);
  const [workflows, setWorkflows] = useState([]);
  const [selectedWorkflowID, setSelectedWorkflowID] = useState("");
  const [workers, setWorkers] = useState([]);
  const [settings, setSettings] = useState(demoSettings);
  const [editor, setEditor] = useState(null);
  const [editorSourceID, setEditorSourceID] = useState("");
  const [validation, setValidation] = useState([]);
  const [busy, setBusy] = useState("");
  const navigationRef = useRef({ back: [], forward: [] });
  const [, setNavigationVersion] = useState(0);
  const { notice, notify } = useNotice();
  const { dialog, requestConfirm, resolveConfirm } = useConfirmDialog();

  const load = useCallback(async () => {
    try {
      const [runtimeItems, workflowItems, workerItems, settingsValue] = await Promise.all([RuntimeBinding.ListRuntimes(), WorkflowBinding.ListDefinitions(), WorkerBinding.ListWorkers(), SettingsBinding.GetSettings()]);
      setRuntimes(runtimeItems || []);
      const nextWorkflows = workflowItems || [];
      setWorkflows(nextWorkflows);
      setSelectedWorkflowID((current) => nextWorkflows.some((item) => item.id === current) ? current : nextWorkflows[0]?.id || "");
      setWorkers(workerItems || []);
      setSettings(settingsValue || demoSettings);
      setMode("wails");
    } catch {
      setRuntimes(demoRuntimes);
      setWorkflows(demoWorkflows);
      setSelectedWorkflowID((current) => demoWorkflows.some((item) => item.id === current) ? current : demoWorkflows[0]?.id || "");
      setWorkers(demoWorkers);
      setSettings(demoSettings);
      setMode("demo");
    }
  }, []);

  useEffect(() => { void load(); }, [load]);
  useEffect(() => {
    const nativeSidebar = globalThis.webkit?.messageHandlers?.onecatchSidebar;
    if (!nativeSidebar || mode === "loading") return;
    document.documentElement.dataset.nativeSidebarMaterial = "true";
    nativeSidebar.postMessage({ width: document.querySelector(".workflow-sidebar")?.getBoundingClientRect().width || 240 });
  }, [mode]);
  useEffect(() => Events.On(settingsChangedEvent, async () => {
    if (mode !== "wails") return;
    try {
      const [value, items] = await Promise.all([SettingsBinding.GetSettings(), WorkerBinding.ListWorkers()]);
      setSettings(value || demoSettings);
      setWorkers(items || []);
    } catch {
      // A later edit or window reopen will retry; the current editor remains usable.
    }
  }), [mode]);

  const announceChange = () => { void Events.Emit(workflowsChangedEvent, {}); };
  const currentPage = () => ({
    selectedWorkflowID,
    editor: editor ? copy(editor) : null,
    editorSourceID,
    validation: copy(validation),
  });
  const pageKey = (page) => page.editor
    ? `editor:${page.editorSourceID || "new"}:${page.editor.id}`
    : `detail:${page.selectedWorkflowID}`;
  const applyPage = (page) => {
    setSelectedWorkflowID(page.selectedWorkflowID || "");
    setEditor(page.editor ? copy(page.editor) : null);
    setEditorSourceID(page.editorSourceID || "");
    setValidation(copy(page.validation || []));
  };
  const visitPage = (page) => {
    const current = currentPage();
    if (pageKey(current) !== pageKey(page)) {
      navigationRef.current.back = [...navigationRef.current.back.slice(-49), current];
      navigationRef.current.forward = [];
      setNavigationVersion((version) => version + 1);
    }
    applyPage(page);
  };
  const goBack = () => {
    const history = navigationRef.current;
    const page = history.back.pop();
    if (!page) return;
    history.forward.push(currentPage());
    applyPage(page);
    setNavigationVersion((version) => version + 1);
  };
  const goForward = () => {
    const history = navigationRef.current;
    const page = history.forward.pop();
    if (!page) return;
    history.back.push(currentPage());
    applyPage(page);
    setNavigationVersion((version) => version + 1);
  };
  const openEditor = (definition, isNew = false) => {
    const next = copy(definition || loopTemplate);
    if (isNew) next.id = nextWorkflowDefinitionID(next.id, workflows);
    if (isNew || !workflows.some((item) => item.id === next.id)) next.policy = { maxTransitions: settings.execution.maxTransitions, maxConsecutiveFailures: settings.execution.maxConsecutiveFailures, stepTimeoutSeconds: settings.execution.stepTimeoutSeconds };
    visitPage({ selectedWorkflowID: isNew ? selectedWorkflowID : next.id, editor: next, editorSourceID: isNew ? "" : next.id, validation: [] });
  };
  const updateStep = (index, field, value) => setEditor((current) => ({ ...current, steps: current.steps.map((step, itemIndex) => itemIndex === index ? { ...step, [field]: value } : step) }));
  const updateTransition = (stepIndex, oldSignal, signal, target) => setEditor((current) => ({ ...current, steps: current.steps.map((step, index) => {
    if (index !== stepIndex) return step;
    const transitions = { ...step.transitions };
    delete transitions[oldSignal];
    transitions[signal] = target;
    return { ...step, transitions };
  }) }));
  const removeTransition = (stepIndex, signal) => setEditor((current) => ({ ...current, steps: current.steps.map((step, index) => {
    if (index !== stepIndex) return step;
    const transitions = { ...step.transitions };
    delete transitions[signal];
    return { ...step, transitions };
  }) }));

  const validateEditor = async () => {
    if (!editor) return [];
    const duplicateID = workflows.some((item) => item.id === editor.id && item.id !== editorSourceID);
    let issues;
    if (mode === "demo") {
      issues = [];
      if (!/^[a-z][a-z0-9_-]*$/.test(editor.id)) issues.push({ path: "id", message: t("workflow.lowercaseID") });
      if (!editor.steps?.length) issues.push({ path: "steps", message: t("workflow.stepRequired") });
    } else issues = await WorkflowBinding.ValidateDefinition(editor) || [];
    if (duplicateID) issues = [...issues, { path: "id", code: "duplicate", message: t("workflow.idExists") }];
    setValidation(issues);
    return issues;
  };

  const saveWorkflow = async () => {
    const issues = await validateEditor();
    if (issues.length) {
      notify("error", t("workflow.configIssues", { count: issues.length }));
      return;
    }
    if (editor.steps.some((step) => step.sandbox === "full") && !await requestConfirm({ title: t("app.fullWorkflowTitle"), description: t("app.fullWorkflowDescription"), detail: t("app.workflowDetail", { name: editor.name || editor.id }), confirmLabel: t("app.confirmSave"), dangerous: true })) return;
    setBusy("workflow");
    try {
      const savedID = editor.id;
      if (mode !== "demo") {
        if (editorSourceID) await WorkflowBinding.UpdateDefinition(editorSourceID, editor);
        else await WorkflowBinding.CreateDefinition(editor);
        setWorkflows(await WorkflowBinding.ListDefinitions());
      } else setWorkflows((items) => [...items.filter((item) => item.id !== editor.id && item.id !== editorSourceID), editor]);
      visitPage({ selectedWorkflowID: savedID, editor: null, editorSourceID: "", validation: [] });
      announceChange();
      notify("success", t("app.workflowSaved"));
    } catch (error) {
      notify("error", errorMessage(error));
    } finally {
      setBusy("");
    }
  };

  const deleteWorkflow = async (workflow) => {
    if (!await requestConfirm({ title: t("app.deleteWorkflowTitle", { name: workflow.name }), description: t("app.deleteWorkflowDescription"), detail: t("app.workflowDetail", { name: workflow.name || workflow.id }), confirmLabel: t("app.deleteWorkflow"), dangerous: true })) return;
    setBusy("delete-workflow");
    try {
      if (mode === "demo") setWorkflows((items) => items.filter((item) => item.id !== workflow.id));
      else {
        await WorkflowBinding.DeleteDefinition(workflow.id);
        setWorkflows(await WorkflowBinding.ListDefinitions());
      }
      if (selectedWorkflowID === workflow.id) setSelectedWorkflowID("");
      if (editorSourceID === workflow.id) {
        setEditor(null);
        setEditorSourceID("");
      }
      navigationRef.current.back = navigationRef.current.back.filter((page) => page.selectedWorkflowID !== workflow.id && page.editorSourceID !== workflow.id);
      navigationRef.current.forward = navigationRef.current.forward.filter((page) => page.selectedWorkflowID !== workflow.id && page.editorSourceID !== workflow.id);
      setNavigationVersion((version) => version + 1);
      announceChange();
      notify("success", t("app.workflowDeleted"));
    } catch (error) {
      notify("error", errorMessage(error));
    } finally {
      setBusy("");
    }
  };

  useEffect(() => {
    setSelectedWorkflowID((current) => workflows.some((item) => item.id === current) ? current : workflows[0]?.id || "");
  }, [workflows]);

  const selectedWorkflow = workflows.find((workflow) => workflow.id === selectedWorkflowID) || workflows[0] || null;
  const selectWorkflow = (workflow) => {
    visitPage({ selectedWorkflowID: workflow.id, editor: null, editorSourceID: "", validation: [] });
  };

  if (mode === "loading") return <LoadingWindow />;
  const editorContent = editor ? <Suspense fallback={<LoadingWindow />}><WorkflowEditor editor={editor} setEditor={setEditor} validation={validation} validateEditor={validateEditor} saveWorkflow={saveWorkflow} busy={busy} updateStep={updateStep} updateTransition={updateTransition} removeTransition={removeTransition} runtimes={runtimes} workers={settings.experimental?.remoteWorkersEnabled ? workers : []} defaultSandbox={settings.execution.defaultSandbox} allowFullSandbox={settings.security.allowFullSandbox} onClose={() => visitPage({ selectedWorkflowID: editorSourceID || selectedWorkflowID, editor: null, editorSourceID: "", validation: [] })} /></Suspense> : null;
  return <div className="flex h-full min-h-0 flex-col overflow-hidden bg-transparent text-foreground">
    <Suspense fallback={<LoadingWindow />}><WorkflowLibrary workflows={workflows} selectedWorkflow={selectedWorkflow} runtimes={runtimes} editorContent={editorContent} openEditor={openEditor} deleteWorkflow={deleteWorkflow} onSelect={selectWorkflow} busy={busy} canGoBack={navigationRef.current.back.length > 0} canGoForward={navigationRef.current.forward.length > 0} onGoBack={goBack} onGoForward={goForward} /></Suspense>
    <ConfirmDialog dialog={dialog} onCancel={() => resolveConfirm(false)} onConfirm={() => resolveConfirm(true)} />
    {notice && <div className={`toast ${notice.type}`}><span>{notice.text}</span></div>}
  </div>;
}
