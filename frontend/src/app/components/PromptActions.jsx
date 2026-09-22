import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ArrowUpRight, Braces, Check, ChevronDown, Copy, Ellipsis, FileText, FlaskConical, Plus, ScanSearch, Search, SquarePen, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import "./promptTemplates.css";
import { builtinPromptActions, readPromptLibrary, renderPromptTemplate, templateArguments, writePromptLibrary } from "../promptActions.js";

export function PromptTemplateLibrary({ context, onInsert, management = false, insertLabel, insertDisabled = false, targetLabel = "" }) {
  const { t } = useTranslation();
  const [initial] = useState(() => {
    try { return { templates: readPromptLibrary(window.localStorage), error: false }; }
    catch { return { templates: [], error: true }; }
  });
  const [custom, setCustom] = useState(initial.templates);
  const [selectedID, setSelectedID] = useState("builtin-explain");
  const [query, setQuery] = useState("");
  const [selection, setSelection] = useState(context.selection);
  const [args, setArgs] = useState({});
  const [editor, setEditor] = useState(null);
  const [pane, setPane] = useState("use");
  const [saveError, setSaveError] = useState("");
  const library = [...builtinPromptActions(t), ...custom];
  const selected = library.find((item) => item.id === selectedID) || library[0];
  let argumentNames = [], preview = "", templateError = "";
  try {
    argumentNames = templateArguments(selected, library);
    preview = renderPromptTemplate(selected, library, { ...context, selection }, args);
    if (!preview.trim()) templateError = t("promptActions.empty");
  } catch (error) {
    templateError = t(`promptActions.error.${error.code || "invalidLibrary"}`, { detail: error.detail });
  }
  const persist = (next) => {
    try {
      if (JSON.stringify(readPromptLibrary(window.localStorage)) !== JSON.stringify(custom)) { setSaveError(t("promptActions.conflict")); return false; }
      writePromptLibrary(window.localStorage, next); setCustom(next); setSaveError(""); return true; }
    catch { setSaveError(t("promptActions.saveError")); return false; }
  };
  const save = () => {
    const next = [...custom.filter((item) => item.id !== editor.id), editor];
    try { templateArguments(editor, [...builtinPromptActions(t), ...next]); }
    catch (error) { setSaveError(t(`promptActions.error.${error.code}`, { detail: error.detail })); return; }
    if (persist(next)) { setSelectedID(editor.id); setEditor(null); setArgs({}); }
  };
  const editCopy = () => {
    setSaveError("");
    setEditor({ id: selected.builtin ? `custom-${crypto.randomUUID()}` : selected.id, name: selected.name, body: selected.body });
  };
  const visible = library.filter((item) => item.name.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase()));
  const description = (item) => item.builtin ? t(`promptActions.${item.id.replace("builtin-", "")}Description`) : t("promptActions.customDescription");
  const iconFor = (item) => ({ "builtin-explain": ScanSearch, "builtin-tests": FlaskConical, "builtin-review": Check })[item.id] || FileText;
  const Icon = iconFor(selected);
  const newTemplate = () => { setEditor({ id: `custom-${crypto.randomUUID()}`, name: "", body: "{{selection}}" }); setSaveError(""); };
  const select = (item) => {
    if (editor && !window.confirm(t("promptActions.discardEdit"))) return;
    setSelectedID(item.id); setArgs({}); setEditor(null); setSaveError("");
  };
  const remove = () => {
    if (!window.confirm(t("promptActions.deleteConfirm", { name: selected.name }))) return;
    if (persist(custom.filter((item) => item.id !== selected.id))) { setSelectedID("builtin-explain"); setEditor(null); }
  };
  const rail = <aside className="pt-library" aria-label={t("promptActions.templates")}>
    <div className="pt-library-heading"><span>{t("promptActions.library")}</span><span className="pt-count">{library.length}</span>
      {management && <Button type="button" variant="ghost" size="icon-sm" disabled={initial.error || Boolean(editor)} title={t("promptActions.new")} aria-label={t("promptActions.new")} onClick={newTemplate}><Plus size={15} /></Button>}
    </div>
    <div className="pt-search"><Search size={13} aria-hidden="true" /><Input aria-label={t("promptActions.search")} placeholder={t("promptActions.search")} value={query} onChange={(event) => setQuery(event.target.value)} /></div>
    <div className="pt-library-scroll">
      {[true, false].map((builtin) => <section key={String(builtin)} className="pt-group">
        <h2>{t(builtin ? "promptActions.builtins" : "promptActions.custom")}<span>{visible.filter((item) => Boolean(item.builtin) === builtin).length}</span></h2>
        {visible.filter((item) => Boolean(item.builtin) === builtin).map((item) => { const RowIcon = iconFor(item); return <button type="button" className={`pt-template-row ${selected.id === item.id && !editor ? "is-selected" : ""}`} key={item.id} aria-pressed={selected.id === item.id && !editor} onClick={() => select(item)}><RowIcon size={16} aria-hidden="true" /><span><strong>{item.name}</strong><small>{description(item)}</small></span></button>; })}
        {!builtin && !custom.length && !query && <p className="pt-library-empty">{t("promptActions.customEmpty")}</p>}
      </section>)}
      {!visible.length && <p role="status" className="pt-library-empty">{t("promptActions.noMatches")}</p>}
    </div>
    {management && <div className="pt-library-note"><FileText size={13} aria-hidden="true" />{t("promptActions.localLibrary")}</div>}
  </aside>;
  return <div className={`pt-workspace ${management ? "pt-management" : "pt-picker"}`} onKeyDown={(event) => event.stopPropagation()}>
    {rail}
    <Tabs value={pane} onValueChange={setPane} className="pt-detail">
      <header className="pt-detail-header">
        <div className="pt-detail-heading"><span className="pt-detail-icon"><Icon size={21} strokeWidth={1.6} /></span><div><div className="pt-title-line"><h1>{editor ? t("promptActions.editing") : selected.name}</h1><span className="pt-badge">{t(editor || !selected.builtin ? "promptActions.custom" : "promptActions.builtins")}</span></div><p>{editor ? t("promptActions.editorHint") : description(selected)}</p></div></div>
        {management && !editor && <div className="pt-header-actions"><Button type="button" variant="outline" size="sm" disabled={initial.error} onClick={editCopy}>{selected.builtin ? <Copy size={13} /> : <SquarePen size={13} />}{t(selected.builtin ? "promptActions.duplicate" : "common.edit")}</Button>{!selected.builtin && <DropdownMenu><DropdownMenuTrigger asChild><Button type="button" variant="ghost" size="icon-sm" aria-label={t("promptActions.more")}><Ellipsis size={16} /></Button></DropdownMenuTrigger><DropdownMenuContent align="end"><DropdownMenuItem disabled={initial.error} onSelect={remove}><Trash2 size={14} />{t("common.delete")}</DropdownMenuItem></DropdownMenuContent></DropdownMenu>}</div>}
      </header>
      {!editor && management && <div className="pt-tabs"><TabsList variant="line"><TabsTrigger value="use">{t("promptActions.use")}</TabsTrigger><TabsTrigger value="source">{t("promptActions.source")}</TabsTrigger></TabsList></div>}
      <TabsContent value={pane} className="pt-detail-scroll" aria-label={editor ? t("promptActions.editing") : !management ? t("promptActions.use") : undefined}>
        {initial.error && <p role="alert" className="pt-error">{t("promptActions.loadError")}</p>}
        {editor ? <div className="pt-editor">
          <label className="pt-field">{t("promptActions.name")}<Input autoFocus maxLength={120} value={editor.name} onChange={(event) => setEditor({ ...editor, name: event.target.value })} /></label>
          <label className="pt-field">{t("promptActions.body")}<Textarea className="pt-source-editor" rows={14} maxLength={32000} value={editor.body} onChange={(event) => setEditor({ ...editor, body: event.target.value })} /></label>
          <details className="pt-variable-help"><summary><Braces size={14} />{t("promptActions.variables")}<ChevronDown size={13} /></summary><p>{t("promptActions.syntax", { skipInterpolation: true })}</p><p>{t("promptActions.reference")} <code>{`{{template:${editor.id}}}`}</code></p></details>
        </div> : pane === "source" && management ? <section className="pt-source"><div className="pt-section-label"><Braces size={14} />{t("promptActions.source")}</div><pre>{selected.body.split(/(\{\{[^{}]+\}\})/g).map((part, index) => part.startsWith("{{") ? <mark key={index}>{part}</mark> : part)}</pre><p>{t("promptActions.sourceHint")}</p></section> : <div className="pt-use-grid">
          <section className="pt-input-section"><div className="pt-section-label"><span className="pt-step">1</span>{t("promptActions.inputTitle")}</div>
            <label className="pt-field">{t("promptActions.material")}<Textarea rows={7} maxLength={64000} value={selection} onChange={(event) => setSelection(event.target.value)} placeholder={t("promptActions.contextPlaceholder")} /></label>
            <p className="pt-hint">{t("promptActions.contextHint")}</p>
            {argumentNames.map((name) => <label key={name} className="pt-field pt-argument">{name}<span className="pt-required">{t("promptActions.required")}</span><Input placeholder={t("promptActions.parameterPlaceholder", { name })} value={Object.hasOwn(args, name) ? args[name] : ""} onChange={(event) => setArgs({ ...args, [name]: event.target.value })} /></label>)}
            {context.project && <div className="pt-context-meta"><span>{t("promptActions.projectContext")}</span><strong>{context.project}</strong><small title={context.path}>{context.path}</small></div>}
          </section>
          <section className="pt-preview-section" aria-label={t("promptActions.preview")}><div className="pt-section-label"><span className="pt-step">2</span>{t("promptActions.preview")}<span className="pt-readonly">{t("promptActions.readonly")}</span></div>
            {preview ? <pre className="pt-preview">{preview}</pre> : <div className="pt-preview-empty" role="status"><FileText size={27} strokeWidth={1.3} aria-hidden="true" /><strong>{t("promptActions.previewEmpty")}</strong><p>{templateError || t("promptActions.previewHint")}</p></div>}
            {preview && templateError && <p role="status" className="pt-hint">{templateError}</p>}
          </section>
        </div>}
        {saveError && <p role="alert" className="pt-error">{saveError}</p>}
      </TabsContent>
      <footer className="pt-footer">
        {editor ? <><span className="pt-footer-note">{t("promptActions.saveHint")}</span><Button type="button" variant="ghost" onClick={() => setEditor(null)}>{t("common.cancel")}</Button><Button type="button" disabled={!editor.name.trim() || !editor.body.trim()} onClick={save}>{t("common.save")}</Button></> : <><div className="pt-destination"><span>{t("promptActions.destination")}</span><strong>{insertDisabled ? t("promptActions.chooseProject") : targetLabel || context.task || t("task.createTitle")}</strong></div><Button type="button" disabled={insertDisabled || Boolean(templateError) || !preview.trim()} onClick={() => onInsert(preview)}>{insertLabel || t("promptActions.insert")}<ArrowUpRight size={14} /></Button></>}
      </footer>
    </Tabs>
  </div>;
}
