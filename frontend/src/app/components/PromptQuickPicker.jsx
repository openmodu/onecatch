import { useEffect, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Popover } from "radix-ui";
import { ArrowLeft, ArrowUpRight, FileText, Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { builtinPromptActions, capturePromptSelection, readPromptLibrary, renderPromptTemplate, templateArguments, templateContextNames } from "../promptActions.js";
import "./promptQuickPicker.css";

function QuickPickerContent({ context, onInsert, onClose }) {
  const { t } = useTranslation();
  const [saved] = useState(() => {
    try { return { items: readPromptLibrary(window.localStorage) }; }
    catch { return { items: [], error: true }; }
  });
  const library = [...builtinPromptActions(t), ...saved.items];
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const [chosen, setChosen] = useState(null);
  const [selection, setSelection] = useState(context.selection);
  const [args, setArgs] = useState({});
  const listID = useId();
  const filtered = library.filter((item) => item.name.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase()));
  const activeID = `${listID}-${active}`;
  useEffect(() => { document.getElementById(activeID)?.scrollIntoView({ block: "nearest" }); }, [activeID]);
  let names = [], needsSelection = false, preview = "", error = "";
  if (chosen) {
    try {
      names = templateArguments(chosen, library);
      needsSelection = templateContextNames(chosen, library).includes("selection");
      preview = renderPromptTemplate(chosen, library, { ...context, selection }, args);
    } catch (failure) { error = t(`promptActions.error.${failure.code || "invalidLibrary"}`, { detail: failure.detail }); }
  }
  const choose = (item) => {
    if (!item) return;
    try {
      const text = renderPromptTemplate(item, library, { ...context, selection }, {});
      if (text.trim()) { onInsert(text); return; }
    } catch { /* Missing fields are collected in the next step. */ }
    setChosen(item); setArgs({});
  };
  const searchKeys = (event) => {
    if (event.nativeEvent.isComposing) return;
    if (["ArrowDown", "ArrowUp"].includes(event.key)) {
      event.preventDefault();
      setActive((current) => filtered.length ? (current + (event.key === "ArrowDown" ? 1 : -1) + filtered.length) % filtered.length : 0);
    } else if (event.key === "Enter") { event.preventDefault(); choose(filtered[active]); }
  };
  return <div className="quick-template" onKeyDown={(event) => { event.stopPropagation(); if (event.key === "Escape") { event.preventDefault(); onClose(); } }}>
    {!chosen ? <>
      <div className="quick-template-search"><Search size={14} aria-hidden="true" /><Input autoFocus role="combobox" aria-expanded="true" aria-controls={listID} aria-activedescendant={filtered.length ? activeID : undefined} aria-label={t("promptActions.search")} placeholder={t("promptActions.quickSearch")} value={query} onChange={(event) => { setQuery(event.target.value); setActive(0); }} onKeyDown={searchKeys} /></div>
      <div className="quick-template-list" role="listbox" id={listID} aria-label={t("promptActions.templates")}>
        {filtered.map((item, index) => <button type="button" role="option" aria-selected={active === index} id={`${listID}-${index}`} key={item.id} tabIndex={-1} className={active === index ? "is-active" : ""} onPointerMove={() => setActive(index)} onMouseDown={(event) => event.preventDefault()} onClick={() => choose(item)}><FileText size={15} aria-hidden="true" /><span><strong>{item.name}</strong><small>{item.builtin ? t(`promptActions.${item.id.replace("builtin-", "")}Description`) : t("promptActions.custom")}</small></span><ArrowUpRight size={13} className="quick-template-row-arrow" aria-hidden="true" /></button>)}
        {!filtered.length && <p className="quick-template-message">{t("promptActions.noMatches")}</p>}
      </div>
      {saved.error && <p role="alert" className="quick-template-message">{t("promptActions.loadError")}</p>}
      <div className="quick-template-footnote"><span>{t("promptActions.quickHint")}</span><kbd>↑ ↓</kbd><kbd>↵</kbd></div>
    </> : <>
      <header className="quick-template-header"><Button type="button" variant="ghost" size="icon-sm" aria-label={t("common.back")} onClick={() => setChosen(null)}><ArrowLeft size={14} /></Button><strong>{chosen.name}</strong></header>
      <div className="quick-template-fields">
        {needsSelection && <label>{t("promptActions.material")}<Textarea autoFocus rows={3} maxLength={64000} value={selection} onChange={(event) => setSelection(event.target.value)} placeholder={t("promptActions.quickContext")} /></label>}
        {names.map((name, index) => <label key={name}>{name}<Input autoFocus={!needsSelection && index === 0} value={Object.hasOwn(args, name) ? args[name] : ""} onChange={(event) => setArgs({ ...args, [name]: event.target.value })} /></label>)}
        {error && <p role="status" className="quick-template-message">{error}</p>}
        {preview && <details><summary>{t("promptActions.preview")}</summary><pre>{preview}</pre></details>}
      </div>
      <footer className="quick-template-footer"><span>{t("promptActions.quickNoSend")}</span><Button type="button" size="sm" disabled={Boolean(error) || !preview.trim()} onClick={() => onInsert(preview)}>{t("promptActions.insert")}<ArrowUpRight size={13} /></Button></footer>
    </>}
  </div>;
}

export default function PromptQuickPicker({ workspace, taskTitle = "", disabled, onInsert, getSelection }) {
  const { t } = useTranslation();
  const [context, setContext] = useState(null);
  const inserted = useRef(false);
  const open = (value) => {
    if (!value) { setContext(null); return; }
    inserted.current = false;
    setContext({ selection: capturePromptSelection(document) || getSelection?.() || "", project: workspace?.name || "", path: workspace?.remoteFs?.root || workspace?.path || "", task: taskTitle, date: new Date().toLocaleDateString() });
  };
  return <Popover.Root open={Boolean(context)} onOpenChange={open}>
    <Popover.Trigger asChild><Button type="button" variant="ghost" size="sm" className="quick-template-trigger" disabled={disabled} aria-label={t("promptActions.quickChoose")} title={t("promptActions.quickChoose")} onMouseDown={(event) => event.preventDefault()}><FileText size={14} aria-hidden="true" /><span>{t("sidebar.templates")}</span></Button></Popover.Trigger>
    <Popover.Portal><Popover.Content side="top" align="start" sideOffset={8} collisionPadding={12} className="quick-template-popover" onCloseAutoFocus={(event) => { if (inserted.current) event.preventDefault(); }}>
      {context && <QuickPickerContent context={context} onClose={() => setContext(null)} onInsert={(prompt) => { inserted.current = true; onInsert(prompt); setContext(null); }} />}
    </Popover.Content></Popover.Portal>
  </Popover.Root>;
}
