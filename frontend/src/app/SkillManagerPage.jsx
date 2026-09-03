import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Events } from "@wailsio/runtime";
import { ChevronDown, Ellipsis, FolderSync, Play, Plus, RefreshCw, Search, SquarePen, Trash2, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Textarea } from "@/components/ui/textarea";
import { SkillBinding } from "../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js";
import { StatusBadge } from "../ui/primitives.jsx";
import MarkdownContent from "./components/MarkdownContent.jsx";
import SkillDebugPanel from "./components/SkillDebugPanel.jsx";
import { errorMessage, formatDateTime } from "./format.js";
import { demoSyncTargets, formatSkillBytes, newSkillTemplate, parseSkillDocument, syncStatusTone } from "./skills.js";
import { createFrameBatcher } from "./frameBatcher.js";
import { applyDebugFrames, publishSkillSelection, requestSkillFile, skillDocumentPath, SKILL_DEBUG_EVENT, SKILL_FILE_DRAFT_EVENT, subscribeSkillWorkspace } from "./skillWorkspace.js";

// A search field is only worth its chrome once the rail stops fitting on one
// screen, so the demo library has to be big enough to exercise that branch.
const SEARCH_THRESHOLD = 4;

const demoSkills = [
  ["release-notes", "Write concise, user-facing release notes from a real diff."],
  ["code-review", "Review a diff for correctness and report only what is verified."],
  ["commit-message", "Draft a Conventional Commits message from the staged changes."],
  ["api-docs", "Document an HTTP endpoint from its handler and its tests."],
  ["incident-report", "Turn a run log into a timeline and a root-cause summary."],
].map(([name, description], index) => ({
  name,
  description,
  path: `~/.onecatch/skills/${name}/SKILL.md`,
  updatedAt: new Date(Date.now() - index * 3_600_000).toISOString(),
  sizeBytes: 284 + index * 96,
  digest: "demo",
  content: newSkillTemplate(name, description),
}));

// Demo mode has no Modu behind it, so the streaming path is reproduced here
// rather than stubbed out: without it the browser preview would exercise a
// code path the desktop app never takes.
const DEMO_DEBUG_SCRIPT = [
  { kind: "reasoning", text: "The prompt names the skill directly, so its body is already in context." },
  { kind: "tool_use", text: "read_file ~/.onecatch/skills/{{name}}/SKILL.md" },
  { kind: "tool_result", text: "---\nname: {{name}}\n---\n\n# {{name}}\n\n(284 bytes)" },
  { kind: "message", text: "Loaded the skill explicitly and produced this preview response.\n\n- Checked the frontmatter\n- Followed the workflow in `SKILL.md`" },
];

function streamDemoDebug(name, onFrames) {
  const events = DEMO_DEBUG_SCRIPT.map((step) => ({ ...step, text: step.text.replaceAll("{{name}}", name), at: new Date().toISOString() }));
  return new Promise((resolve) => {
    let index = 0;
    const step = () => {
      if (index >= events.length) {
        resolve({ succeeded: true, output: events[events.length - 1].text, durationMs: 1280, usage: { inputTokens: 942, outputTokens: 86 }, events });
        return;
      }
      const event = events[index];
      // Prose arrives a few words at a time so the caret has something to do.
      const words = event.text.split(" ");
      let cursor = 0;
      const grow = () => {
        cursor = Math.min(words.length, cursor + 4);
        const partial = words.slice(0, cursor).join(" ");
        onFrames([{ index, event: { ...event, text: partial, streaming: cursor < words.length } }]);
        if (cursor < words.length) { window.setTimeout(grow, 90); return; }
        index += 1;
        window.setTimeout(step, 160);
      };
      grow();
    };
    window.setTimeout(step, 200);
  });
}

function demoDebugHistory(name) {
  return [
    { runId: "demo-1", skill: name, prompt: "Draft notes for v0.1.11", startedAt: new Date(Date.now() - 3_600_000).toISOString(), result: { succeeded: true, output: "Wrote five bullets from the diff.", durationMs: 2140, usage: { inputTokens: 880, outputTokens: 120 }, events: [{ kind: "message", text: "Wrote five bullets from the diff." }] } },
    { runId: "demo-2", skill: name, prompt: "Summarise the revert", startedAt: new Date(Date.now() - 86_400_000).toISOString(), result: { succeeded: false, output: "", durationMs: 640, usage: {}, events: [{ kind: "error", text: "modu: no model configured", failed: true }] } },
  ];
}

// The rail rows carry the whole library, so they stay text-first: a name and
// one line of purpose. Anything heavier (icon tiles, borders, elevation) turns
// a scannable list into a wall of cards.
function SkillRow({ skill, selected, disabled, onSelect }) {
  return <button
    type="button"
    className={`group flex w-full min-w-0 flex-col gap-0.5 rounded-lg px-2.5 py-2 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 disabled:cursor-wait disabled:opacity-60 ${selected ? "bg-accent" : "hover:bg-accent/45"}`}
    aria-pressed={selected}
    disabled={disabled}
    onClick={onSelect}
  >
    <strong className={`min-w-0 truncate text-[13px] font-medium ${selected ? "text-foreground" : "text-foreground/85"}`}>{skill.name}</strong>
    <span className="line-clamp-1 text-[11px] leading-relaxed text-muted-foreground">{skill.description}</span>
  </button>;
}

function RailTab({ active, icon: Icon, label, onSelect }) {
  return <button
    type="button"
    className={`flex h-8 w-full items-center gap-2 rounded-lg px-2.5 text-left text-[12px] font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 ${active ? "bg-accent text-foreground" : "text-muted-foreground hover:bg-accent/45 hover:text-foreground"}`}
    aria-current={active ? "page" : undefined}
    onClick={onSelect}
  >
    <Icon size={14} strokeWidth={2} aria-hidden="true" />
    <span className="min-w-0 flex-1 truncate">{label}</span>
  </button>;
}

function TargetRow({ target, skills, busy, onSync, onSelectSkills }) {
  const { t } = useTranslation();
  const syncing = busy === `sync-${target.id}`;
  // An empty selection means "everything", so the menu shows every skill
  // checked rather than none — that is what the target actually receives.
  const selection = target.skills || [];
  const everything = selection.length === 0;
  const chosen = new Set(everything ? skills.map((skill) => skill.name) : selection);
  const toggle = (name) => {
    const next = new Set(chosen);
    if (next.has(name)) next.delete(name); else next.add(name);
    onSelectSkills(target, [...next]);
  };
  return <div className="flex items-start gap-4 border-b border-border/50 py-3.5 last:border-b-0">
    <div className="min-w-0 flex-1">
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <strong className="text-[13px] font-medium text-foreground">{target.name}</strong>
        <StatusBadge status={syncStatusTone(target.status)}>{t(`skill.syncStatus.${target.status}`, { defaultValue: target.status })}</StatusBadge>
      </div>
      <code className="mt-1 block truncate font-mono text-[11px] text-muted-foreground" title={target.path}>{target.path}</code>
      <p className="mt-1 text-[11px] leading-relaxed text-muted-foreground">
        {t("skill.syncedCount", { synced: target.syncedSkills, total: target.totalSkills })} · {target.lastSyncedAt ? t("skill.lastSynced", { time: formatDateTime(target.lastSyncedAt) }) : t("skill.neverSynced")}
      </p>
      {target.lastError && <p className="mt-1 text-[11px] leading-relaxed text-destructive">{target.lastError}</p>}
    </div>
    <div className="flex shrink-0 items-center gap-0.5">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="sm" className="text-muted-foreground hover:text-foreground" disabled={Boolean(busy) || skills.length === 0}>
            {everything ? t("skill.allSkills", { count: skills.length }) : t("skill.someSkills", { count: selection.length, total: skills.length })}
            <ChevronDown aria-hidden="true" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="max-h-80 overflow-y-auto">
          <DropdownMenuLabel className="text-[11px] font-normal text-muted-foreground">{t("skill.chooseSkills", { name: target.name })}</DropdownMenuLabel>
          {skills.map((skill) => <DropdownMenuCheckboxItem key={skill.name} checked={chosen.has(skill.name)} onSelect={(event) => { event.preventDefault(); toggle(skill.name); }}>
            <span className="min-w-0 truncate text-[12px]">{skill.name}</span>
          </DropdownMenuCheckboxItem>)}
          <DropdownMenuSeparator />
          <DropdownMenuItem disabled={everything} onSelect={() => onSelectSkills(target, [])}>{t("skill.selectAllSkills")}</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <Button variant="ghost" size="sm" className="text-muted-foreground hover:text-foreground" disabled={Boolean(busy) || !target.rsyncAvailable || target.totalSkills === 0} onClick={onSync}>
        <RefreshCw className={syncing ? "animate-spin" : ""} aria-hidden="true" />
        {syncing ? t("skill.syncing") : t("skill.syncNow")}
      </Button>
    </div>
  </div>;
}

export default function SkillManagerPage({ mode, notify, onOpenInspector }) {
  const { t } = useTranslation();
  const [skills, setSkills] = useState([]);
  const [selectedName, setSelectedName] = useState("");
  const [document, setDocument] = useState(null);
  // The rendered copy. It tracks the inspector's editor keystroke by keystroke
  // so the card is a live preview of the file being edited, and falls back to
  // whatever the last save or load put on disk.
  const [preview, setPreview] = useState("");
  const [targets, setTargets] = useState([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState("");
  const [query, setQuery] = useState("");
  const [pane, setPane] = useState("skill");
  const [debugOpen, setDebugOpen] = useState(false);
  const [createDialog, setCreateDialog] = useState(false);
  const [createForm, setCreateForm] = useState({ name: "", description: "" });
  const [deleteDialog, setDeleteDialog] = useState(null);
  const [debugPrompt, setDebugPrompt] = useState("");
  const [debugResult, setDebugResult] = useState(null);
  const [debugEvents, setDebugEvents] = useState([]);
  const [debugHistory, setDebugHistory] = useState([]);
  const [viewingRunID, setViewingRunID] = useState("");
  // Frames arrive on a Wails channel that has no idea which run the panel is
  // showing, so the live run id is read from a ref inside the listener.
  const debugRunRef = useRef("");

  const loadDocument = useCallback(async (name) => {
    if (!name) { setDocument(null); setPreview(""); setSelectedName(""); publishSkillSelection({ name: "" }); return; }
    try {
      const value = mode === "demo" ? demoSkills.find((item) => item.name === name) || { ...demoSkills[0], name } : await SkillBinding.GetSkill(name);
      setDocument(value);
      setPreview(value.content || "");
      setSelectedName(name);
      setDebugResult(null);
      setDebugEvents([]);
      publishSkillSelection({ name, path: value.path || "" });
    } catch (error) {
      notify("error", errorMessage(error));
    }
  }, [mode, notify]);

  const refresh = useCallback(async ({ keepSelection = true } = {}) => {
    setLoading(true);
    try {
      const [skillItems, targetItems] = mode === "demo"
        ? [demoSkills, demoSyncTargets]
        : await Promise.all([SkillBinding.ListSkills(), SkillBinding.ScanSyncTargets()]);
      setSkills(skillItems || []);
      setTargets(targetItems || []);
      const nextName = keepSelection && (skillItems || []).some((item) => item.name === selectedName)
        ? selectedName
        : skillItems?.[0]?.name || "";
      if (!document || document.name !== nextName) await loadDocument(nextName);
    } catch (error) {
      notify("error", errorMessage(error));
    } finally {
      setLoading(false);
    }
  }, [document, loadDocument, mode, notify, selectedName]);

  useEffect(() => { void refresh({ keepSelection: false }); }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Every keystroke in the inspector's editor arrives here. Only edits to the
  // open skill's own SKILL.md change what the card renders; a save also
  // refreshes the summary, because the description and size it prints came
  // from that file.
  useEffect(() => subscribeSkillWorkspace(SKILL_FILE_DRAFT_EVENT, (event) => {
    const { path, content, saved } = event.detail || {};
    if (!selectedName || path !== skillDocumentPath(selectedName)) return;
    setPreview(content);
    if (!saved) return;
    setDocument((current) => current && current.name === selectedName ? { ...current, content, sizeBytes: content.length, updatedAt: new Date().toISOString(), description: parseSkillDocument(content).frontmatter.description || current.description } : current);
    setSkills((items) => items.map((item) => item.name === selectedName ? { ...item, sizeBytes: content.length, updatedAt: new Date().toISOString(), description: parseSkillDocument(content).frontmatter.description || item.description } : item));
  }), [selectedName]);

  const selectSkill = (name) => {
    if (pane === "sync") setPane("skill");
    if (name === selectedName) return;
    setDebugOpen(false);
    setDebugEvents([]);
    setViewingRunID("");
    void loadDocument(name);
  };

  const editDocument = () => {
    onOpenInspector?.();
    requestSkillFile(skillDocumentPath(selectedName));
  };

  const createSkill = async () => {
    const name = createForm.name.trim();
    const description = createForm.description.trim();
    if (!name || !description) { notify("error", t("skill.createFieldsRequired")); return; }
    setBusy("create");
    try {
      const content = newSkillTemplate(name, description);
      const created = mode === "demo" ? { ...demoSkills[0], name, description, content } : await SkillBinding.CreateSkill({ name, content });
      setSkills((items) => [...items.filter((item) => item.name !== created.name), created].sort((a, b) => a.name.localeCompare(b.name)));
      setCreateDialog(false);
      setCreateForm({ name: "", description: "" });
      setDocument(created);
      setPreview(created.content);
      setSelectedName(created.name);
      setPane("skill");
      publishSkillSelection({ name: created.name, path: created.path || "" });
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

  const loadDebugHistory = useCallback(async (name) => {
    if (!name) { setDebugHistory([]); return; }
    try {
      setDebugHistory(mode === "demo" ? demoDebugHistory(name) : await SkillBinding.DebugHistory(name) || []);
    } catch {
      // History is a convenience beside the live run; a panel that opens
      // without it is still usable.
      setDebugHistory([]);
    }
  }, [mode]);

  useEffect(() => { if (debugOpen) void loadDebugHistory(selectedName); }, [debugOpen, loadDebugHistory, selectedName]);

  // Streamed frames arrive on a Wails channel while Debug is still awaiting.
  // They are coalesced per display frame, the same way runtime frames are.
  useEffect(() => {
    if (mode !== "wails") return undefined;
    let pending = [];
    const scheduleFrame = typeof window.requestAnimationFrame === "function" ? window.requestAnimationFrame.bind(window) : (callback) => window.setTimeout(callback, 16);
    const cancelFrame = typeof window.cancelAnimationFrame === "function" ? window.cancelAnimationFrame.bind(window) : window.clearTimeout.bind(window);
    const batcher = createFrameBatcher(() => {
      const frames = pending;
      pending = [];
      if (frames.length) setDebugEvents((current) => applyDebugFrames(current, frames));
    }, scheduleFrame, cancelFrame);
    const off = Events.On(SKILL_DEBUG_EVENT, (event) => {
      const frame = event?.data;
      if (!frame || frame.runId !== debugRunRef.current) return;
      pending.push(frame);
      batcher.schedule();
    });
    return () => { batcher.cancel(); off(); };
  }, [mode]);

  const runDebug = async () => {
    if (!document || !debugPrompt.trim()) { notify("error", t("skill.debugPromptRequired")); return; }
    const prompt = debugPrompt.trim();
    const runID = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
    debugRunRef.current = runID;
    setBusy("debug");
    setDebugResult(null);
    setDebugEvents([]);
    setViewingRunID("");
    try {
      const result = mode === "demo"
        ? await streamDemoDebug(document.name, (frames) => setDebugEvents((current) => applyDebugFrames(current, frames)))
        : await SkillBinding.Debug({ name: document.name, prompt, runId: runID });
      if (debugRunRef.current !== runID) return;
      setDebugResult(result);
      setDebugEvents(result.events || []);
      notify(result.stopped ? "info" : result.succeeded ? "success" : "error", t(result.stopped ? "skill.debugStopped" : result.succeeded ? "skill.debugComplete" : "skill.debugFailed"));
      void loadDebugHistory(document.name);
    } catch (error) {
      // The streamed transcript is the only record of a failed run, so it
      // stays on screen; only the outcome is reported.
      notify("error", errorMessage(error));
      void loadDebugHistory(document.name);
    } finally {
      if (debugRunRef.current === runID) debugRunRef.current = "";
      setBusy("");
    }
  };

  const stopDebug = () => {
    const runID = debugRunRef.current;
    if (!runID) return;
    if (mode === "demo") { debugRunRef.current = ""; setBusy(""); return; }
    void SkillBinding.StopDebug(runID).catch((error) => notify("error", errorMessage(error)));
  };

  const viewDebugRun = (record) => {
    if (busy === "debug") return;
    setViewingRunID(record.runId);
    setDebugResult(record.result);
    setDebugEvents(record.result?.events || []);
    setDebugPrompt(record.prompt || "");
  };

  const clearDebugHistory = async () => {
    if (!selectedName) return;
    try {
      if (mode !== "demo") await SkillBinding.ClearDebugHistory(selectedName);
      setDebugHistory([]);
      setViewingRunID("");
    } catch (error) {
      notify("error", errorMessage(error));
    }
  };

  const selectTargetSkills = async (target, names) => {
    // Choosing every skill is the same thing as choosing none: both mean the
    // target follows the library, so the stored selection is cleared.
    const selection = names.length === skills.length ? [] : names;
    setBusy(`select-${target.id}`);
    try {
      const updated = mode === "demo"
        ? { ...target, skills: selection, totalSkills: selection.length || skills.length }
        : await SkillBinding.SetSyncTargetSkills({ id: target.id, skills: selection });
      setTargets((items) => items.map((item) => item.id === target.id ? updated : item));
    } catch (error) {
      notify("error", errorMessage(error));
    } finally {
      setBusy("");
    }
  };

  const syncTarget = async (target) => {
    setBusy(`sync-${target.id}`);
    try {
      if (mode !== "demo") await SkillBinding.Sync(target.id);
      const count = target.skills?.length || skills.length;
      setTargets(mode === "demo" ? targets.map((item) => item.id === target.id ? { ...item, exists: true, status: "synced", syncedSkills: count, totalSkills: count, lastSyncedAt: new Date().toISOString() } : item) : await SkillBinding.ScanSyncTargets());
      notify("success", t("skill.syncComplete", { name: target.name, count }));
    } catch (error) {
      notify("error", errorMessage(error));
    } finally {
      setBusy("");
    }
  };

  const visibleSkills = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return skills;
    return skills.filter((item) => `${item.name} ${item.description || ""}`.toLowerCase().includes(needle));
  }, [query, skills]);
  const parsed = useMemo(() => parseSkillDocument(preview), [preview]);
  const extraFrontmatter = useMemo(() => Object.entries(parsed.frontmatter).filter(([key]) => key !== "name" && key !== "description"), [parsed]);

  return <div className="grid min-h-0 min-w-0 flex-1 grid-cols-[248px_minmax(0,1fr)] overflow-hidden bg-background max-[900px]:grid-cols-[196px_minmax(0,1fr)]">
    <aside className="flex min-h-0 min-w-0 flex-col border-r border-border/60" aria-label={t("skill.library")}>
      <div className="flex h-12 shrink-0 items-center gap-0.5 pr-2 pl-4">
        <span className="min-w-0 flex-1 truncate text-[13px] font-semibold tracking-[-0.01em] text-foreground">{t("skill.library")}</span>
        <Button variant="ghost" size="icon-sm" className="text-muted-foreground hover:text-foreground" disabled={loading} aria-label={t("common.refresh")} title={t("common.refresh")} onClick={() => void refresh()}><RefreshCw className={loading ? "animate-spin" : ""} aria-hidden="true" /></Button>
        <Button variant="ghost" size="icon-sm" className="text-muted-foreground hover:text-foreground" disabled={Boolean(busy)} aria-label={t("skill.new")} title={t("skill.new")} onClick={() => setCreateDialog(true)}><Plus aria-hidden="true" /></Button>
      </div>

      {skills.length > SEARCH_THRESHOLD && <div className="relative shrink-0 px-3 pb-2">
        <Search size={13} className="pointer-events-none absolute top-1/2 left-5.5 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
        <Input className="h-7 rounded-lg border-0 bg-muted/50 pl-6 text-[12px] shadow-none focus-visible:ring-1" aria-label={t("skill.search")} placeholder={t("skill.search")} value={query} onChange={(event) => setQuery(event.target.value)} />
      </div>}

      <div className="min-h-0 flex-1 overflow-y-auto px-2 pb-2">
        <div className="flex flex-col gap-px">
          {visibleSkills.map((skill) => <SkillRow
            key={skill.name}
            skill={skill}
            selected={pane === "skill" && skill.name === selectedName}
            disabled={Boolean(busy)}
            onSelect={() => selectSkill(skill.name)}
          />)}
        </div>
        {skills.length > 0 && visibleSkills.length === 0 && <p className="px-2.5 py-3 text-[11px] text-muted-foreground">{t("skill.noMatches")}</p>}
        {!loading && skills.length === 0 && <p className="px-2.5 py-3 text-[11px] leading-relaxed text-muted-foreground">{t("skill.empty")}</p>}
      </div>

      <div className="shrink-0 border-t border-border/60 p-2">
        <RailTab active={pane === "sync"} icon={FolderSync} label={t("skill.sync")} onSelect={() => setPane("sync")} />
      </div>
    </aside>

    {pane === "sync" ? <ScrollArea className="min-h-0 min-w-0">
      <section className="mx-auto max-w-3xl px-8 pt-8 pb-10">
        <header className="min-w-0">
          <h1 className="text-[19px] font-semibold tracking-[-0.01em] text-foreground">{t("skill.syncTitle")}</h1>
          <p className="mt-1.5 text-[13px] leading-relaxed text-muted-foreground">{t("skill.syncDescription")}</p>
        </header>
        {!targets.every((target) => target.rsyncAvailable) && <p className="mt-5 rounded-lg bg-destructive/8 px-4 py-3 text-[12px] leading-relaxed text-destructive">{t("skill.rsyncRequired")}</p>}
        <div className="mt-5 border-t border-border/50">{targets.map((target) => <TargetRow key={target.id} target={target} skills={skills} busy={busy} onSync={() => void syncTarget(target)} onSelectSkills={(item, names) => void selectTargetSkills(item, names)} />)}</div>
        <p className="mt-6 text-[11px] leading-relaxed text-muted-foreground">{t("skill.targetsLiveInSettings")} {t("skill.metadataHint")}</p>
      </section>
    </ScrollArea> : document ? <div className="flex min-h-0 min-w-0 flex-col">
      <ScrollArea className="min-h-0 flex-1">
        <div className="mx-auto w-full max-w-3xl px-8 pt-7 pb-9">
          {/* One card is the whole skill: what it claims to be in its
              frontmatter, and the prose a runtime actually loads. */}
          <article className="overflow-hidden rounded-2xl border border-border/60 bg-card">
            <header className="border-b border-border/50 px-6 pt-5 pb-4">
              <div className="flex items-start gap-3">
                <div className="min-w-0 flex-1">
                  <h1 className="min-w-0 truncate text-[19px] font-semibold tracking-[-0.01em] text-foreground">{document.name}</h1>
                  <p className="mt-1 text-[13px] leading-relaxed text-muted-foreground">{parsed.frontmatter.description || document.description}</p>
                </div>
                <div className="flex shrink-0 items-center gap-1">
                  <Button variant="ghost" size="icon-sm" className="text-muted-foreground hover:text-foreground" aria-label={t("skill.editDocument")} title={t("skill.editDocument")} onClick={editDocument}><SquarePen aria-hidden="true" /></Button>
                  <Button variant="ghost" size="icon-sm" className={debugOpen ? "bg-accent text-foreground" : "text-muted-foreground hover:text-foreground"} aria-pressed={debugOpen} aria-label={t("skill.debug")} title={t("skill.debug")} onClick={() => setDebugOpen((current) => !current)}><Play aria-hidden="true" /></Button>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild><Button variant="ghost" size="icon-sm" className="text-muted-foreground hover:text-foreground" disabled={Boolean(busy)} aria-label={t("skill.actions")}><Ellipsis aria-hidden="true" /></Button></DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onSelect={editDocument}><SquarePen aria-hidden="true" />{t("skill.editDocument")}</DropdownMenuItem>
                      <DropdownMenuItem variant="destructive" onSelect={() => setDeleteDialog({ name: document.name })}><Trash2 aria-hidden="true" />{t("common.delete")}</DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              </div>
              <p className="mt-2.5 truncate font-mono text-[11px] text-muted-foreground/80" title={document.path}>{document.path} · {formatSkillBytes(document.sizeBytes)} · {formatDateTime(document.updatedAt)}</p>
              {extraFrontmatter.length > 0 && <dl className="mt-3 flex flex-wrap gap-x-4 gap-y-1">{extraFrontmatter.map(([key, value]) => <div className="flex min-w-0 items-baseline gap-1.5" key={key}>
                <dt className="shrink-0 font-mono text-[10px] text-muted-foreground">{key}</dt>
                <dd className="m-0 min-w-0 truncate text-[11px] text-foreground/80">{value}</dd>
              </div>)}</dl>}
            </header>
            <div className="px-6 py-5">
              {parsed.body.trim()
                ? <MarkdownContent className="markdown-content text-[13.5px] leading-[1.75]" content={parsed.body} />
                : <p className="text-[12px] text-muted-foreground">{t("skill.emptyBody")}</p>}
            </div>
          </article>
          <p className="mt-3 px-1 text-[11px] leading-relaxed text-muted-foreground">{t("skill.editorHint")}</p>
        </div>
      </ScrollArea>
      {debugOpen && <SkillDebugPanel
        skillName={document.name}
        prompt={debugPrompt}
        result={debugResult}
        events={debugEvents}
        history={debugHistory}
        viewingRunID={viewingRunID}
        running={busy === "debug"}
        blocked={Boolean(busy) && busy !== "debug"}
        onPromptChange={setDebugPrompt}
        onRun={() => void runDebug()}
        onStop={stopDebug}
        onViewRun={viewDebugRun}
        onClearHistory={() => void clearDebugHistory()}
        onClose={() => setDebugOpen(false)}
      />}
    </div> : <div className="flex min-h-0 min-w-0 items-center justify-center px-8">
      <div className="max-w-sm text-center">
        <h1 className="text-[17px] font-semibold tracking-[-0.01em] text-foreground">{t("skill.emptyTitle")}</h1>
        <p className="mt-2 text-[13px] leading-relaxed text-muted-foreground">{t("skill.emptyDescription")}</p>
        <Button className="mt-5" size="sm" onClick={() => setCreateDialog(true)}><Plus aria-hidden="true" />{t("skill.new")}</Button>
      </div>
    </div>}

    <Dialog open={createDialog} onOpenChange={(open) => !busy && setCreateDialog(open)}><DialogContent className="sm:max-w-md"><DialogHeader><DialogTitle>{t("skill.new")}</DialogTitle><DialogDescription>{t("skill.newDescription")}</DialogDescription></DialogHeader><div className="grid gap-4"><div className="grid gap-2"><Label htmlFor="new-skill-name">{t("skill.name")}</Label><Input id="new-skill-name" autoFocus value={createForm.name} onChange={(event) => setCreateForm((current) => ({ ...current, name: event.target.value.toLowerCase().replace(/[^a-z0-9-]/g, "-").replace(/-+/g, "-") }))} placeholder="release-notes" /></div><div className="grid gap-2"><Label htmlFor="new-skill-description">{t("skill.description")}</Label><Textarea id="new-skill-description" className="min-h-20" value={createForm.description} onChange={(event) => setCreateForm((current) => ({ ...current, description: event.target.value }))} placeholder={t("skill.descriptionPlaceholder")} /></div></div><DialogFooter><Button variant="outline" disabled={Boolean(busy)} onClick={() => setCreateDialog(false)}>{t("common.cancel")}</Button><Button disabled={Boolean(busy)} onClick={createSkill}>{busy === "create" ? t("common.processing") : t("common.add")}</Button></DialogFooter></DialogContent></Dialog>

    <Dialog open={Boolean(deleteDialog)} onOpenChange={(open) => !open && !busy && setDeleteDialog(null)}><DialogContent className="sm:max-w-md"><DialogHeader><DialogTitle>{t("skill.deleteTitle", { name: deleteDialog?.name })}</DialogTitle><DialogDescription>{t("skill.deleteDescription")}</DialogDescription></DialogHeader><DialogFooter><Button variant="outline" disabled={Boolean(busy)} onClick={() => setDeleteDialog(null)}>{t("common.cancel")}</Button><Button variant="destructive" disabled={Boolean(busy)} onClick={deleteSkill}>{busy ? t("common.processing") : t("common.delete")}</Button></DialogFooter></DialogContent></Dialog>
  </div>;
}
