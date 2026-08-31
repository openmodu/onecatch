import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Bug, FileCode2, FolderSync, Plus, RefreshCw, Save, Trash2, Play, X } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { SkillBinding } from "../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js";
import { StatusBadge } from "../ui/primitives.jsx";
import { errorMessage, formatDateTime, formatDuration, formatTokens } from "./format.js";
import { formatSkillBytes, newSkillTemplate, syncStatusTone } from "./skills.js";

const demoSkill = {
  name: "release-notes",
  description: "Write concise, user-facing release notes from a real diff.",
  path: "~/.onecatch/skills/release-notes/SKILL.md",
  updatedAt: new Date().toISOString(),
  sizeBytes: 284,
  digest: "demo",
  content: newSkillTemplate("release-notes", "Write concise, user-facing release notes from a real diff."),
};

const demoTargets = [
  { id: "codex", name: "Codex", path: "~/.codex/skills", builtin: true, exists: true, status: "synced", syncedSkills: 1, totalSkills: 1, lastSyncedAt: new Date().toISOString(), rsyncAvailable: true },
  { id: "claude", name: "Claude Code", path: "~/.claude/skills", builtin: true, exists: false, status: "missing", syncedSkills: 0, totalSkills: 1, rsyncAvailable: true },
  { id: "modu", name: "Modu", path: "~/.modu/skills", builtin: true, exists: true, status: "ready", syncedSkills: 0, totalSkills: 1, rsyncAvailable: true },
];

function SkillListItem({ skill, selected, onSelect }) {
  return <button type="button" className={`group flex w-full min-w-0 items-start gap-2.5 rounded-lg px-3 py-2.5 text-left transition-colors hover:bg-accent/70 ${selected ? "bg-accent text-accent-foreground" : "text-muted-foreground"}`} aria-current={selected ? "page" : undefined} onClick={onSelect}>
    <FileCode2 className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
    <span className="min-w-0 flex-1">
      <strong className="block truncate text-[13px] font-semibold text-foreground">{skill.name}</strong>
      <small className="mt-0.5 block truncate text-[11px] leading-relaxed text-muted-foreground">{skill.description}</small>
    </span>
  </button>;
}

function TargetCard({ target, busy, onSync, onRemove }) {
  const { t } = useTranslation();
  const status = t(`skill.syncStatus.${target.status}`, { defaultValue: target.status });
  return <article className="rounded-xl border bg-card p-4">
    <div className="flex items-start gap-3">
      <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-muted text-muted-foreground"><FolderSync className="size-4" aria-hidden="true" /></span>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <strong className="text-sm font-semibold text-foreground">{target.name}</strong>
          <StatusBadge status={syncStatusTone(target.status)}>{status}</StatusBadge>
        </div>
        <code className="mt-1.5 block truncate text-[11px] text-muted-foreground" title={target.path}>{target.path}</code>
      </div>
      {!target.builtin && <Button variant="ghost" size="icon-xs" disabled={Boolean(busy)} aria-label={t("skill.removeTarget", { name: target.name })} onClick={onRemove}><X aria-hidden="true" /></Button>}
    </div>
    <div className="mt-4 flex items-end justify-between gap-4 rounded-lg bg-muted/35 px-3 py-2.5">
      <div className="min-w-0 text-[11px] leading-relaxed text-muted-foreground">
        <span className="block">{t("skill.syncedCount", { synced: target.syncedSkills, total: target.totalSkills })}</span>
        <span className="block truncate">{target.lastSyncedAt ? t("skill.lastSynced", { time: formatDateTime(target.lastSyncedAt) }) : t("skill.neverSynced")}</span>
        {target.lastError && <span className="mt-1 block text-destructive">{target.lastError}</span>}
      </div>
      <Button variant="outline" size="sm" className="shrink-0" disabled={Boolean(busy) || !target.rsyncAvailable} onClick={onSync}>
        <RefreshCw className={busy === `sync-${target.id}` ? "animate-spin" : ""} aria-hidden="true" />
        {busy === `sync-${target.id}` ? t("skill.syncing") : t("skill.syncNow")}
      </Button>
    </div>
  </article>;
}

export default function SkillManagerPage({ mode, notify, requestConfirm }) {
  const { t } = useTranslation();
  const [skills, setSkills] = useState([]);
  const [selectedName, setSelectedName] = useState("");
  const [document, setDocument] = useState(null);
  const [draft, setDraft] = useState("");
  const [targets, setTargets] = useState([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState("");
  const [tab, setTab] = useState("editor");
  const [createDialog, setCreateDialog] = useState(false);
  const [createForm, setCreateForm] = useState({ name: "", description: "" });
  const [targetDialog, setTargetDialog] = useState(false);
  const [targetForm, setTargetForm] = useState({ name: "", path: "" });
  const [deleteDialog, setDeleteDialog] = useState(null);
  const [debugPrompt, setDebugPrompt] = useState("");
  const [debugResult, setDebugResult] = useState(null);
  const dirty = Boolean(document && draft !== document.content);

  const loadDocument = useCallback(async (name) => {
    if (!name) { setDocument(null); setDraft(""); return; }
    try {
      const value = mode === "demo" ? { ...demoSkill, name } : await SkillBinding.GetSkill(name);
      setDocument(value);
      setDraft(value.content || "");
      setSelectedName(name);
      setDebugResult(null);
    } catch (error) {
      notify("error", errorMessage(error));
    }
  }, [mode, notify]);

  const refresh = useCallback(async ({ keepSelection = true } = {}) => {
    setLoading(true);
    try {
      const [skillItems, targetItems] = mode === "demo"
        ? [[demoSkill], demoTargets]
        : await Promise.all([SkillBinding.ListSkills(), SkillBinding.ScanSyncTargets()]);
      setSkills(skillItems || []);
      setTargets(targetItems || []);
      const nextName = keepSelection && (skillItems || []).some((item) => item.name === selectedName)
        ? selectedName
        : skillItems?.[0]?.name || "";
      if (!document || document.name !== nextName || !dirty) await loadDocument(nextName);
    } catch (error) {
      notify("error", errorMessage(error));
    } finally {
      setLoading(false);
    }
  }, [dirty, document, loadDocument, mode, notify, selectedName]);

  useEffect(() => { void refresh({ keepSelection: false }); }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const selectSkill = async (name) => {
    if (name === selectedName) return;
    if (dirty && !await requestConfirm({ title: t("skill.discardTitle"), description: t("skill.discardChanges"), confirmLabel: t("skill.discard") })) return;
    void loadDocument(name);
  };

  const saveSkill = async () => {
    if (!document || !dirty) return;
    setBusy("save");
    try {
      const saved = mode === "demo" ? { ...document, content: draft, updatedAt: new Date().toISOString() } : await SkillBinding.UpdateSkill({ name: document.name, content: draft });
      setDocument(saved);
      setDraft(saved.content);
      setSkills((items) => items.map((item) => item.name === saved.name ? saved : item));
      notify("success", t("skill.saved"));
      if (mode !== "demo") setTargets(await SkillBinding.ScanSyncTargets());
    } catch (error) {
      notify("error", errorMessage(error));
    } finally {
      setBusy("");
    }
  };

  const createSkill = async () => {
    const name = createForm.name.trim();
    const description = createForm.description.trim();
    if (!name || !description) { notify("error", t("skill.createFieldsRequired")); return; }
    setBusy("create");
    try {
      const content = newSkillTemplate(name, description);
      const created = mode === "demo" ? { ...demoSkill, name, description, content } : await SkillBinding.CreateSkill({ name, content });
      setSkills((items) => [...items.filter((item) => item.name !== created.name), created].sort((a, b) => a.name.localeCompare(b.name)));
      setCreateDialog(false);
      setCreateForm({ name: "", description: "" });
      setDocument(created);
      setDraft(created.content);
      setSelectedName(created.name);
      setTab("editor");
      notify("success", t("skill.created"));
      if (mode !== "demo") setTargets(await SkillBinding.ScanSyncTargets());
    } catch (error) {
      notify("error", errorMessage(error));
    } finally {
      setBusy("");
    }
  };

  const deleteSkill = async () => {
    const name = deleteDialog?.name;
    if (!name) return;
    setBusy("delete");
    try {
      if (mode !== "demo") await SkillBinding.DeleteSkill(name);
      const next = skills.filter((item) => item.name !== name);
      setSkills(next);
      setDeleteDialog(null);
      notify("success", t("skill.deleted"));
      await loadDocument(next[0]?.name || "");
      if (mode !== "demo") setTargets(await SkillBinding.ScanSyncTargets());
    } catch (error) {
      notify("error", errorMessage(error));
    } finally {
      setBusy("");
    }
  };

  const runDebug = async () => {
    if (dirty) { notify("error", t("skill.saveBeforeAction")); return; }
    if (!document || !debugPrompt.trim()) { notify("error", t("skill.debugPromptRequired")); return; }
    setBusy("debug");
    setDebugResult(null);
    try {
      const result = mode === "demo" ? {
        succeeded: true,
        output: "The skill was loaded explicitly and produced this preview response.",
        durationMs: 1280,
        usage: { inputTokens: 942, outputTokens: 86 },
        events: [{ kind: "message", text: "The skill was loaded explicitly and produced this preview response.", at: new Date().toISOString() }],
      } : await SkillBinding.Debug({ name: document.name, prompt: debugPrompt.trim() });
      setDebugResult(result);
      notify(result.succeeded ? "success" : "error", t(result.succeeded ? "skill.debugComplete" : "skill.debugFailed"));
    } catch (error) {
      notify("error", errorMessage(error));
    } finally {
      setBusy("");
    }
  };

  const syncTarget = async (target) => {
    if (dirty) { notify("error", t("skill.saveBeforeAction")); return; }
    setBusy(`sync-${target.id}`);
    try {
      if (mode !== "demo") await SkillBinding.Sync(target.id);
      setTargets(mode === "demo" ? targets.map((item) => item.id === target.id ? { ...item, exists: true, status: "synced", syncedSkills: skills.length, totalSkills: skills.length, lastSyncedAt: new Date().toISOString() } : item) : await SkillBinding.ScanSyncTargets());
      notify("success", t("skill.syncComplete", { name: target.name, count: skills.length }));
    } catch (error) {
      notify("error", errorMessage(error));
    } finally {
      setBusy("");
    }
  };

  const addTarget = async () => {
    if (!targetForm.name.trim() || !targetForm.path.trim()) { notify("error", t("skill.targetFieldsRequired")); return; }
    setBusy("add-target");
    try {
      const target = mode === "demo" ? { ...targetForm, id: `custom-${Date.now()}`, builtin: false, exists: false, status: "missing", syncedSkills: 0, totalSkills: skills.length, rsyncAvailable: true } : await SkillBinding.AddSyncTarget(targetForm);
      setTargets((items) => [...items, target]);
      setTargetDialog(false);
      setTargetForm({ name: "", path: "" });
      notify("success", t("skill.targetAdded"));
    } catch (error) {
      notify("error", errorMessage(error));
    } finally {
      setBusy("");
    }
  };

  const removeTarget = async () => {
    const target = deleteDialog?.target;
    if (!target) return;
    setBusy("remove-target");
    try {
      if (mode !== "demo") await SkillBinding.RemoveSyncTarget(target.id);
      setTargets((items) => items.filter((item) => item.id !== target.id));
      setDeleteDialog(null);
      notify("success", t("skill.targetRemoved"));
    } catch (error) {
      notify("error", errorMessage(error));
    } finally {
      setBusy("");
    }
  };

  const debugTokens = useMemo(() => (debugResult?.usage?.inputTokens || 0) + (debugResult?.usage?.outputTokens || 0), [debugResult]);

  return <div className="grid min-h-0 flex-1 grid-cols-[240px_minmax(0,1fr)] overflow-hidden bg-background">
    <aside className="flex min-h-0 flex-col border-r bg-sidebar/55">
      <div className="flex h-[52px] shrink-0 items-center justify-between gap-3 bg-muted/25 px-4">
        <div className="min-w-0"><strong className="block text-sm font-semibold text-foreground">{t("skill.library")}</strong><small className="block text-[11px] text-muted-foreground">{t("skill.count", { count: skills.length })}</small></div>
        <Button variant="outline" size="icon-sm" aria-label={t("skill.new")} onClick={() => setCreateDialog(true)}><Plus aria-hidden="true" /></Button>
      </div>
      <ScrollArea className="min-h-0 flex-1 p-2">
        <div className="grid gap-0.5">{skills.map((skill) => <SkillListItem key={skill.name} skill={skill} selected={skill.name === selectedName} onSelect={() => void selectSkill(skill.name)} />)}</div>
        {!skills.length && !loading && <div className="px-4 py-10 text-center text-xs leading-relaxed text-muted-foreground">{t("skill.empty")}</div>}
      </ScrollArea>
      <div className="bg-muted/25 px-4 py-3"><code className="block truncate text-[10px] text-muted-foreground" title="~/.onecatch/skills">~/.onecatch/skills</code></div>
    </aside>

    <main className="flex min-h-0 min-w-0 flex-col">
      <div className="flex h-[52px] shrink-0 items-center gap-3 bg-muted/20 px-6">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2"><strong className="truncate text-sm font-semibold text-foreground">{document?.name || t("skill.title")}</strong>{dirty && <Badge variant="secondary">{t("skill.unsaved")}</Badge>}</div>
          {document && <span className="mt-0.5 block truncate font-mono text-[10px] text-muted-foreground" title={document.path}>{document.path}</span>}
        </div>
        <Button variant="ghost" size="icon-sm" disabled={loading} aria-label={t("common.refresh")} title={t("common.refresh")} onClick={() => void refresh()}><RefreshCw className={loading ? "animate-spin" : ""} aria-hidden="true" /></Button>
        {document && <Button variant="outline" size="sm" disabled={Boolean(busy) || !dirty} onClick={saveSkill}><Save aria-hidden="true" />{busy === "save" ? t("common.saving") : t("common.save")}</Button>}
      </div>

      {document ? <Tabs className="flex min-h-0 flex-1 flex-col" value={tab} onValueChange={setTab}>
        <div className="flex shrink-0 items-center bg-muted/10 px-6 py-1.5">
          <TabsList className="h-9 bg-muted/40 p-1">
            <TabsTrigger className="h-7 px-3" value="editor"><FileCode2 aria-hidden="true" />{t("skill.editor")}</TabsTrigger>
            <TabsTrigger className="h-7 px-3" value="debug"><Bug aria-hidden="true" />{t("skill.debug")}</TabsTrigger>
            <TabsTrigger className="h-7 px-3" value="sync"><FolderSync aria-hidden="true" />{t("skill.sync")}</TabsTrigger>
          </TabsList>
        </div>

        <TabsContent className="m-0 min-h-0 flex-1 data-[state=active]:flex data-[state=active]:flex-col" value="editor">
          <div className="flex shrink-0 items-start justify-between gap-6 px-6 py-4">
            <div className="min-w-0"><h2 className="text-base font-semibold text-foreground">SKILL.md</h2><p className="mt-1 text-xs leading-relaxed text-muted-foreground">{t("skill.editorHint")}</p></div>
            <div className="shrink-0 text-right text-[10px] leading-relaxed text-muted-foreground"><span className="block">{formatSkillBytes(document.sizeBytes)}</span><span className="block">{formatDateTime(document.updatedAt)}</span></div>
          </div>
          <div className="min-h-0 flex-1 px-6 pb-4"><Textarea className="h-full min-h-0 resize-none rounded-lg bg-card p-4 font-mono text-[12px] leading-6" spellCheck="false" value={draft} onChange={(event) => setDraft(event.target.value)} /></div>
          <div className="flex shrink-0 items-center justify-between gap-4 bg-muted/20 px-6 py-3"><span className="text-[11px] text-muted-foreground">{t("skill.resourcesPreserved")}</span><Button variant="ghost" size="sm" className="text-destructive hover:bg-destructive/10 hover:text-destructive" disabled={Boolean(busy)} onClick={() => setDeleteDialog({ name: document.name })}><Trash2 aria-hidden="true" />{t("common.delete")}</Button></div>
        </TabsContent>

        <TabsContent className="m-0 min-h-0 flex-1 data-[state=active]:flex data-[state=active]:flex-col" value="debug">
          <ScrollArea className="min-h-0 flex-1"><section className="mx-auto grid max-w-3xl gap-5 px-7 py-6">
            <header><h2 className="text-lg font-semibold text-foreground">{t("skill.debugTitle")}</h2><p className="mt-1.5 text-xs leading-relaxed text-muted-foreground">{t("skill.debugDescription")}</p></header>
            <div className="grid gap-2"><Label htmlFor="skill-debug-prompt">{t("skill.testPrompt")}</Label><Textarea id="skill-debug-prompt" className="min-h-28 bg-card" value={debugPrompt} onChange={(event) => setDebugPrompt(event.target.value)} placeholder={t("skill.debugPlaceholder")} /><div className="flex justify-end"><Button disabled={Boolean(busy) || dirty || !debugPrompt.trim()} onClick={runDebug}><Play aria-hidden="true" />{busy === "debug" ? t("skill.debugging") : t("skill.runDebug")}</Button></div></div>
            {debugResult && <section className="overflow-hidden rounded-xl border bg-card">
              <div className="flex items-center justify-between gap-4 bg-muted/30 px-4 py-3"><div className="flex items-center gap-2"><StatusBadge status={debugResult.succeeded ? "good" : "danger"}>{t(debugResult.succeeded ? "skill.debugPassed" : "skill.debugFailed")}</StatusBadge><span className="text-[11px] text-muted-foreground">{formatDuration(debugResult.durationMs)} · {t("skill.tokens", { count: formatTokens(debugTokens) })}</span></div>{debugResult.sessionId && <code className="text-[10px] text-muted-foreground">{debugResult.sessionId}</code>}</div>
              <div className="whitespace-pre-wrap px-4 py-4 text-sm leading-6 text-foreground">{debugResult.output || t("skill.noDebugOutput")}</div>
              {debugResult.events?.length > 0 && <details className="bg-muted/10"><summary className="cursor-pointer px-4 py-3 text-xs font-medium text-muted-foreground">{t("skill.debugEvents", { count: debugResult.events.length })}</summary><div className="grid gap-2 bg-muted/25 px-4 py-3">{debugResult.events.map((event, index) => <div className="grid grid-cols-[86px_minmax(0,1fr)] gap-3 font-mono text-[10px] leading-relaxed" key={`${event.kind}-${index}`}><span className={event.failed ? "text-destructive" : "text-muted-foreground"}>{event.kind}</span><span className="whitespace-pre-wrap text-foreground/80">{event.text}</span></div>)}</div></details>}
            </section>}
          </section></ScrollArea>
        </TabsContent>

        <TabsContent className="m-0 min-h-0 flex-1" value="sync">
          <ScrollArea className="h-full"><section className="mx-auto max-w-4xl px-7 py-6">
            <header className="mb-5 flex items-start justify-between gap-5"><div><h2 className="text-lg font-semibold text-foreground">{t("skill.syncTitle")}</h2><p className="mt-1.5 max-w-2xl text-xs leading-relaxed text-muted-foreground">{t("skill.syncDescription")}</p></div><Button variant="outline" size="sm" onClick={() => setTargetDialog(true)}><Plus aria-hidden="true" />{t("skill.addTarget")}</Button></header>
            {!targets.every((target) => target.rsyncAvailable) && <div className="mb-4 rounded-lg bg-destructive/8 px-4 py-3 text-xs leading-relaxed text-destructive">{t("skill.rsyncRequired")}</div>}
            <div className="grid grid-cols-2 gap-3 max-[850px]:grid-cols-1">{targets.map((target) => <TargetCard key={target.id} target={target} busy={busy} onSync={() => void syncTarget(target)} onRemove={() => setDeleteDialog({ target })} />)}</div>
            <p className="mt-5 text-[11px] leading-relaxed text-muted-foreground">{t("skill.metadataHint")}</p>
          </section></ScrollArea>
        </TabsContent>
      </Tabs> : <div className="flex min-h-0 flex-1 items-center justify-center px-8 text-center"><div className="max-w-sm"><FileCode2 className="mx-auto mb-3 size-8 text-muted-foreground" aria-hidden="true" /><h2 className="text-base font-semibold text-foreground">{t("skill.emptyTitle")}</h2><p className="mt-1.5 text-xs leading-relaxed text-muted-foreground">{t("skill.emptyDescription")}</p><Button className="mt-4" size="sm" onClick={() => setCreateDialog(true)}><Plus aria-hidden="true" />{t("skill.new")}</Button></div></div>}
    </main>

    <Dialog open={createDialog} onOpenChange={(open) => !busy && setCreateDialog(open)}><DialogContent className="sm:max-w-md"><DialogHeader><DialogTitle>{t("skill.new")}</DialogTitle><DialogDescription>{t("skill.newDescription")}</DialogDescription></DialogHeader><div className="grid gap-4"><div className="grid gap-2"><Label htmlFor="new-skill-name">{t("skill.name")}</Label><Input id="new-skill-name" autoFocus value={createForm.name} onChange={(event) => setCreateForm((current) => ({ ...current, name: event.target.value.toLowerCase().replace(/[^a-z0-9-]/g, "-").replace(/-+/g, "-") }))} placeholder="release-notes" /></div><div className="grid gap-2"><Label htmlFor="new-skill-description">{t("skill.description")}</Label><Textarea id="new-skill-description" className="min-h-20" value={createForm.description} onChange={(event) => setCreateForm((current) => ({ ...current, description: event.target.value }))} placeholder={t("skill.descriptionPlaceholder")} /></div></div><DialogFooter><Button variant="outline" disabled={Boolean(busy)} onClick={() => setCreateDialog(false)}>{t("common.cancel")}</Button><Button disabled={Boolean(busy)} onClick={createSkill}>{busy === "create" ? t("common.processing") : t("common.add")}</Button></DialogFooter></DialogContent></Dialog>

    <Dialog open={targetDialog} onOpenChange={(open) => !busy && setTargetDialog(open)}><DialogContent className="sm:max-w-md"><DialogHeader><DialogTitle>{t("skill.addTarget")}</DialogTitle><DialogDescription>{t("skill.addTargetDescription")}</DialogDescription></DialogHeader><div className="grid gap-4"><div className="grid gap-2"><Label htmlFor="target-name">{t("skill.targetName")}</Label><Input id="target-name" autoFocus value={targetForm.name} onChange={(event) => setTargetForm((current) => ({ ...current, name: event.target.value }))} placeholder="Cursor" /></div><div className="grid gap-2"><Label htmlFor="target-path">{t("skill.targetPath")}</Label><Input id="target-path" className="font-mono text-xs" value={targetForm.path} onChange={(event) => setTargetForm((current) => ({ ...current, path: event.target.value }))} placeholder="~/.cursor/skills" /></div></div><DialogFooter><Button variant="outline" disabled={Boolean(busy)} onClick={() => setTargetDialog(false)}>{t("common.cancel")}</Button><Button disabled={Boolean(busy)} onClick={addTarget}>{busy === "add-target" ? t("common.processing") : t("common.add")}</Button></DialogFooter></DialogContent></Dialog>

    <Dialog open={Boolean(deleteDialog)} onOpenChange={(open) => !open && !busy && setDeleteDialog(null)}><DialogContent className="sm:max-w-md"><DialogHeader><DialogTitle>{deleteDialog?.target ? t("skill.removeTargetTitle", { name: deleteDialog.target.name }) : t("skill.deleteTitle", { name: deleteDialog?.name })}</DialogTitle><DialogDescription>{deleteDialog?.target ? t("skill.removeTargetDescription") : t("skill.deleteDescription")}</DialogDescription></DialogHeader><DialogFooter><Button variant="outline" disabled={Boolean(busy)} onClick={() => setDeleteDialog(null)}>{t("common.cancel")}</Button><Button variant="destructive" disabled={Boolean(busy)} onClick={deleteDialog?.target ? removeTarget : deleteSkill}>{busy ? t("common.processing") : deleteDialog?.target ? t("common.remove") : t("common.delete")}</Button></DialogFooter></DialogContent></Dialog>
  </div>;
}
