import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
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
import { errorMessage, formatDateTime } from "./format.js";
import { demoSyncTargets, formatSkillBytes, newSkillTemplate, parseSkillDocument, skillNameFromInput, syncStatusTone } from "./skills.js";
import { COLUMN_SEPARATOR_CLASS, useColumnWidth } from "./columnResize.js";
import { publishSkillSelection, requestSkillFile, requestSkillInspectorTab, skillDocumentPath, SKILL_FILE_DRAFT_EVENT, subscribeSkillWorkspace } from "./skillWorkspace.js";

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

// Where a skill can go. Paths and membership of the list are settings; this is
// only the status of each destination.
function TargetStatus({ target }) {
  const { t } = useTranslation();
  return <div className="flex min-w-0 items-center gap-2 py-1.5">
    <strong className="shrink-0 text-[12px] font-medium text-foreground">{target.name}</strong>
    <StatusBadge status={syncStatusTone(target.status)}>{t(`skill.syncStatus.${target.status}`, { defaultValue: target.status })}</StatusBadge>
    <code className="min-w-0 flex-1 truncate font-mono text-[11px] text-muted-foreground" title={target.path}>{target.path}</code>
    {target.lastError && <span className="shrink-0 text-[11px] text-destructive" title={target.lastError}>{t("skill.syncStatus.error")}</span>}
  </div>;
}

// One skill, the targets it goes to, and the action that pushes it. A skill is
// the unit a person iterates on, so it is the unit that gets a sync button:
// proving out one edit should not push four unfinished skills with it.
function SkillSyncRow({ skill, targets, busy, onToggleTarget, onSync }) {
  const { t } = useTranslation();
  const syncing = busy === `sync-skill-${skill.name}`;
  const receiving = targets.filter((target) => !target.skills?.length || target.skills.includes(skill.name));
  const current = receiving.filter((target) => target.syncedNames?.includes(skill.name));
  const blocked = receiving.filter((target) => !target.rsyncAvailable);
  return <div className="flex items-center gap-3 border-b border-border/50 py-3 last:border-b-0">
    <div className="min-w-0 flex-1">
      <strong className="block truncate text-[13px] font-medium text-foreground">{skill.name}</strong>
      <span className="mt-0.5 block truncate text-[11px] text-muted-foreground">
        {receiving.length === 0
          ? t("skill.noTargetsForSkill")
          : current.length === receiving.length
            ? t("skill.skillCurrentAt", { targets: receiving.map((target) => target.name).join("、") })
            : t("skill.skillBehindAt", { count: receiving.length - current.length, total: receiving.length })}
      </span>
    </div>
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="sm" className="shrink-0 text-muted-foreground hover:text-foreground" disabled={Boolean(busy) || targets.length === 0}>
          {t("skill.targetCount", { count: receiving.length, total: targets.length })}
          <ChevronDown aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuLabel className="text-[11px] font-normal text-muted-foreground">{t("skill.chooseTargets", { name: skill.name })}</DropdownMenuLabel>
        {targets.map((target) => <DropdownMenuCheckboxItem
          key={target.id}
          checked={receiving.some((item) => item.id === target.id)}
          onSelect={(event) => { event.preventDefault(); onToggleTarget(skill, target); }}
        ><span className="min-w-0 truncate text-[12px]">{target.name}</span></DropdownMenuCheckboxItem>)}
      </DropdownMenuContent>
    </DropdownMenu>
    <Button variant="ghost" size="sm" className="shrink-0 text-muted-foreground hover:text-foreground" disabled={Boolean(busy) || receiving.length === 0 || blocked.length > 0} onClick={() => onSync(skill)}>
      <RefreshCw className={syncing ? "animate-spin" : ""} aria-hidden="true" />
      {syncing ? t("skill.syncing") : t("skill.syncNow")}
    </Button>
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
  const [createDialog, setCreateDialog] = useState(false);
  const [createForm, setCreateForm] = useState({ name: "", description: "" });
  const [deleteDialog, setDeleteDialog] = useState(null);
  const rail = useColumnWidth({ defaultWidth: 248, min: 176, max: 420 });
  // A ref, not state: the guard has to be correct inside the very next input
  // event, before a re-render could have delivered it.
  const composingNameRef = useRef(false);

  const loadDocument = useCallback(async (name) => {
    if (!name) { setDocument(null); setPreview(""); setSelectedName(""); publishSkillSelection({ name: "" }); return; }
    try {
      const value = mode === "demo" ? demoSkills.find((item) => item.name === name) || { ...demoSkills[0], name } : await SkillBinding.GetSkill(name);
      setDocument(value);
      setPreview(value.content || "");
      setSelectedName(name);
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
    void loadDocument(name);
  };

  const editDocument = () => {
    onOpenInspector?.();
    requestSkillInspectorTab("files");
    requestSkillFile(skillDocumentPath(selectedName));
  };

  const openDebug = () => {
    onOpenInspector?.();
    requestSkillInspectorTab("debug");
  };

  const createSkill = async () => {
    // A composition can still be open when Add is pressed, so the raw buffer
    // is normalized here too rather than trusted from the field.
    const name = skillNameFromInput(createForm.name).trim();
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

  const applySelection = async (target, names) => {
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

  // The picker asks "does this skill go to this target", but the selection is
  // stored the other way round, so a toggle rewrites one target's list.
  const toggleSkillTarget = (skill, target) => {
    const receiving = !target.skills?.length || target.skills.includes(skill.name);
    const effective = target.skills?.length ? target.skills : skills.map((item) => item.name);
    const next = receiving ? effective.filter((name) => name !== skill.name) : [...effective, skill.name];
    void applySelection(target, next);
  };

  const syncSkill = async (skill) => {
    setBusy(`sync-skill-${skill.name}`);
    try {
      if (mode === "demo") {
        setTargets((items) => items.map((item) => (!item.skills?.length || item.skills.includes(skill.name))
          ? { ...item, exists: true, status: "synced", syncedNames: [...new Set([...(item.syncedNames || []), skill.name])], syncedSkills: (item.syncedNames || []).includes(skill.name) ? item.syncedSkills : item.syncedSkills + 1, lastSyncedAt: new Date().toISOString() }
          : item));
      } else {
        const result = await SkillBinding.SyncSkill(skill.name);
        setTargets(result.targets || []);
      }
      notify("success", t("skill.skillSynced", { name: skill.name }));
    } catch (error) {
      notify("error", errorMessage(error));
    } finally {
      setBusy("");
    }
  };

  const syncEverything = async () => {
    setBusy("sync-all");
    try {
      if (mode === "demo") {
        setTargets((items) => items.map((item) => ({ ...item, exists: true, status: "synced", syncedNames: (item.skills?.length ? item.skills : skills.map((skill) => skill.name)), syncedSkills: item.totalSkills, lastSyncedAt: new Date().toISOString() })));
      } else {
        for (const target of targets) {
          if (!target.rsyncAvailable || target.totalSkills === 0) continue;
          await SkillBinding.Sync(target.id);
        }
        setTargets(await SkillBinding.ScanSyncTargets());
      }
      notify("success", t("skill.syncedEverything", { count: skills.length }));
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

  return <div className={`flex min-h-0 min-w-0 flex-1 overflow-hidden bg-background ${rail.resizing ? "cursor-col-resize select-none" : ""}`}>
    <aside className="flex min-h-0 min-w-0 flex-col" style={{ width: `${rail.width}px` }} aria-label={t("skill.library")}>
      <div className="flex h-12 shrink-0 items-center gap-0.5 pr-2 pl-4">
        <span className="min-w-0 flex-1 truncate text-[13px] font-semibold tracking-[-0.01em] text-foreground">{t("skill.library")}</span>
        <Button variant="ghost" size="icon-sm" className="text-muted-foreground hover:text-foreground" disabled={loading} aria-label={t("common.refresh")} title={t("common.refresh")} onClick={() => void refresh()}><RefreshCw className={loading ? "animate-spin" : ""} aria-hidden="true" /></Button>
        <Button variant="ghost" size="icon-sm" className="text-muted-foreground hover:text-foreground" disabled={Boolean(busy)} aria-label={t("skill.new")} title={t("skill.new")} onClick={() => setCreateDialog(true)}><Plus aria-hidden="true" /></Button>
      </div>

      <div className="shrink-0 px-2 pb-1">
        <RailTab active={pane === "sync"} icon={FolderSync} label={t("skill.sync")} onSelect={() => setPane("sync")} />
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

    </aside>
    <span className={COLUMN_SEPARATOR_CLASS} aria-label={t("skill.resizeLibrary")} title={t("skill.resizeHint")} {...rail.separatorProps} />

    {pane === "sync" ? <ScrollArea className="min-h-0 min-w-0 flex-1">
      <section className="mx-auto max-w-3xl px-8 pt-8 pb-10">
        <header className="min-w-0">
          <h1 className="text-[19px] font-semibold tracking-[-0.01em] text-foreground">{t("skill.syncTitle")}</h1>
          <p className="mt-1.5 text-[13px] leading-relaxed text-muted-foreground">{t("skill.syncDescription")}</p>
        </header>
        {!targets.every((target) => target.rsyncAvailable) && <p className="mt-5 rounded-lg bg-destructive/8 px-4 py-3 text-[12px] leading-relaxed text-destructive">{t("skill.rsyncRequired")}</p>}

        <section className="mt-6" aria-labelledby="skill-sync-targets">
          <div className="flex items-center gap-3">
            <h2 className="min-w-0 flex-1 text-[13px] font-semibold text-foreground" id="skill-sync-targets">{t("settings.skillTargets")}</h2>
            <span className="shrink-0 text-[11px] text-muted-foreground">{t("skill.targetsLiveInSettings")}</span>
          </div>
          <div className="mt-2 rounded-xl bg-muted/30 px-3.5 py-1.5">{targets.map((target) => <TargetStatus key={target.id} target={target} />)}</div>
        </section>

        <section className="mt-7" aria-labelledby="skill-sync-skills">
          <div className="flex items-center gap-3">
            <h2 className="min-w-0 flex-1 text-[13px] font-semibold text-foreground" id="skill-sync-skills">{t("skill.library")}</h2>
            <Button variant="ghost" size="sm" className="shrink-0 text-muted-foreground hover:text-foreground" disabled={Boolean(busy) || skills.length === 0} onClick={() => void syncEverything()}>
              <RefreshCw className={busy === "sync-all" ? "animate-spin" : ""} aria-hidden="true" />
              {busy === "sync-all" ? t("skill.syncing") : t("skill.syncEverything")}
            </Button>
          </div>
          <div className="mt-1 border-t border-border/50">{skills.map((skill) => <SkillSyncRow
            key={skill.name}
            skill={skill}
            targets={targets}
            busy={busy}
            onToggleTarget={toggleSkillTarget}
            onSync={(item) => void syncSkill(item)}
          />)}</div>
        </section>

        <p className="mt-6 text-[11px] leading-relaxed text-muted-foreground">{t("skill.metadataHint")}</p>
      </section>
    </ScrollArea> : document ? <div className="flex min-h-0 min-w-0 flex-1 flex-col">
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
                  <Button variant="ghost" size="icon-sm" className="text-muted-foreground hover:text-foreground" aria-label={t("skill.debug")} title={t("skill.debug")} onClick={openDebug}><Play aria-hidden="true" /></Button>
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
    </div> : <div className="flex min-h-0 min-w-0 flex-1 items-center justify-center px-8">
      <div className="max-w-sm text-center">
        <h1 className="text-[17px] font-semibold tracking-[-0.01em] text-foreground">{t("skill.emptyTitle")}</h1>
        <p className="mt-2 text-[13px] leading-relaxed text-muted-foreground">{t("skill.emptyDescription")}</p>
        <Button className="mt-5" size="sm" onClick={() => setCreateDialog(true)}><Plus aria-hidden="true" />{t("skill.new")}</Button>
      </div>
    </div>}

    <Dialog open={createDialog} onOpenChange={(open) => !busy && setCreateDialog(open)}><DialogContent className="sm:max-w-md"><DialogHeader><DialogTitle>{t("skill.new")}</DialogTitle><DialogDescription>{t("skill.newDescription")}</DialogDescription></DialogHeader><div className="grid gap-4"><div className="grid gap-2"><Label htmlFor="new-skill-name">{t("skill.name")}</Label><Input
      id="new-skill-name"
      autoFocus
      value={createForm.name}
      placeholder="release-notes"
      aria-describedby="new-skill-name-hint"
      onCompositionStart={() => { composingNameRef.current = true; }}
      onCompositionEnd={(event) => {
        composingNameRef.current = false;
        const committed = skillNameFromInput(event.target.value);
        setCreateForm((current) => ({ ...current, name: committed }));
      }}
      onChange={(event) => {
        // While the IME holds a composition its buffer lives in this field.
        // Rewriting the value there cancels the composition and leaves the
        // half-converted latin behind, so it is kept verbatim until commit.
        const composing = composingNameRef.current || event.nativeEvent?.isComposing;
        const value = event.target.value;
        setCreateForm((current) => ({ ...current, name: composing ? value : skillNameFromInput(value) }));
      }}
    /><p className="m-0 text-[11px] leading-relaxed text-muted-foreground" id="new-skill-name-hint">{t("skill.namePattern")}</p></div><div className="grid gap-2"><Label htmlFor="new-skill-description">{t("skill.description")}</Label><Textarea id="new-skill-description" className="min-h-20" value={createForm.description} onChange={(event) => setCreateForm((current) => ({ ...current, description: event.target.value }))} placeholder={t("skill.descriptionPlaceholder")} /></div></div><DialogFooter><Button variant="outline" disabled={Boolean(busy)} onClick={() => setCreateDialog(false)}>{t("common.cancel")}</Button><Button disabled={Boolean(busy)} onClick={createSkill}>{busy === "create" ? t("common.processing") : t("common.add")}</Button></DialogFooter></DialogContent></Dialog>

    <Dialog open={Boolean(deleteDialog)} onOpenChange={(open) => !open && !busy && setDeleteDialog(null)}><DialogContent className="sm:max-w-md"><DialogHeader><DialogTitle>{t("skill.deleteTitle", { name: deleteDialog?.name })}</DialogTitle><DialogDescription>{t("skill.deleteDescription")}</DialogDescription></DialogHeader><DialogFooter><Button variant="outline" disabled={Boolean(busy)} onClick={() => setDeleteDialog(null)}>{t("common.cancel")}</Button><Button variant="destructive" disabled={Boolean(busy)} onClick={deleteSkill}>{busy ? t("common.processing") : t("common.delete")}</Button></DialogFooter></DialogContent></Dialog>
  </div>;
}
