import { useEffect, useMemo, useRef, useState } from "react";
import { Folder, LoaderCircle } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { WorkspaceBinding } from "../../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js";
import { commonDirectoryPrefix, createDirectoryCompletionClient } from "../remoteDirectoryCompletion.js";
import { errorMessage } from "../format.js";

export default function RemoteDirectoryInput({ form, workspaceID, onChange }) {
  const { t } = useTranslation();
  const [state, setState] = useState({ loading: false, paths: [], message: "", active: -1, value: "" });
  const [open, setOpen] = useState(false);
  const input = useRef(null);
  const list = useRef(null);
  const request = useRef(0);
  const current = useRef(null);
  const timer = useRef(null);
  const composing = useRef(false);
  const host = form.remoteHost.trim();
  const username = form.remoteUsername.trim();
  const password = form.remotePassword;
  const value = form.remoteRoot;
  const client = useMemo(() => createDirectoryCompletionClient((path) => WorkspaceBinding.CompleteRemoteDirectories({
    host, username, password, path, workspaceId: workspaceID || "",
  })), [host, username, password, workspaceID]);
  current.current = { client, value, open };

  const dismiss = () => {
    clearTimeout(timer.current);
    request.current++;
    setOpen(false);
  };
  const choose = (path) => {
    request.current++;
    onChange(path);
    setOpen(true);
    input.current?.focus();
  };
  const refresh = async (complete = false) => {
    clearTimeout(timer.current);
    const id = ++request.current;
    // Keep the panel and its rows stable while fetching another directory.
    setState((previous) => ({ ...previous, loading: true, message: "" }));
    try {
      const paths = await client.complete(value);
      if (id !== request.current || client !== current.current.client || value !== current.current.value) return;
      if (complete) {
        const prefix = commonDirectoryPrefix(paths);
        if (paths.length === 1 || (prefix.length > value.length && prefix.startsWith(value))) {
          choose(prefix);
          return;
        }
      }
      setState((previous) => ({ loading: false, paths, message: "", value,
        active: paths.indexOf(previous.paths[previous.active]),
      }));
    } catch (error) {
      if (id === request.current && client === current.current.client && value === current.current.value) {
        setState({ loading: false, paths: [], message: errorMessage(error), active: -1, value });
      }
    }
  };

  useEffect(() => {
    request.current++;
    setState({ loading: false, paths: [], message: "", active: -1, value: "" });
  }, [client]);
  useEffect(() => {
    if (!open || !host || composing.current) return;
    if (client.hasDirectory(value)) void refresh();
    else timer.current = setTimeout(() => void refresh(), 120);
    return () => { clearTimeout(timer.current); request.current++; };
  }, [client, value, open]);
  useEffect(() => () => { clearTimeout(timer.current); request.current++; }, []);
  useEffect(() => {
    list.current?.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: "nearest" });
  }, [state.active]);

  const ready = state.value === value && !state.loading;
  const onKeyDown = (event) => {
    if (event.nativeEvent.isComposing || composing.current) return;
    if (event.key === "Escape" && open) {
      event.preventDefault();
      event.stopPropagation();
      dismiss();
    } else if ((event.key === "ArrowDown" || event.key === "ArrowUp") && host) {
      event.preventDefault();
      setOpen(true);
      if (ready && state.paths.length) {
        const down = event.key === "ArrowDown";
        setState((previous) => ({ ...previous, active: previous.active < 0
          ? (down ? 0 : previous.paths.length - 1)
          : (previous.active + (down ? 1 : -1) + previous.paths.length) % previous.paths.length }));
      }
    } else if (event.key === "Enter" && open && ready && state.active >= 0) {
      event.preventDefault();
      choose(state.paths[state.active]);
    } else if (event.key === "Tab" && !event.shiftKey && !event.ctrlKey && !event.metaKey && host) {
      event.preventDefault();
      setOpen(true);
      if (ready && state.paths.length) {
        const prefix = commonDirectoryPrefix(state.paths);
        choose(state.active >= 0 ? state.paths[state.active]
          : prefix.length > value.length && prefix.startsWith(value) ? prefix : state.paths[0]);
      } else {
        void refresh(true);
      }
    }
  };
  const shown = open && Boolean(host);
  return <>
    <div className="relative min-w-0">
      <Input ref={input} id="workspace-create-remote-root" className="pr-8 font-mono text-[13px]" autoCapitalize="none" autoCorrect="off" spellCheck="false" autoComplete="off"
        role="combobox" aria-autocomplete="list" aria-expanded={shown} aria-controls={shown ? "remote-directory-options" : undefined}
        aria-activedescendant={shown && ready && state.active >= 0 ? `remote-directory-${state.active}` : undefined} aria-describedby="remote-directory-help"
        value={value} onChange={(event) => { request.current++; onChange(event.target.value); setOpen(true); }} onKeyDown={onKeyDown}
        onCompositionStart={() => { composing.current = true; clearTimeout(timer.current); request.current++; }}
        onCompositionEnd={() => { composing.current = false; void refresh(); }}
        onFocus={() => setOpen(true)} onBlur={dismiss} placeholder="/srv/project" />
      {shown && state.loading && <LoaderCircle aria-hidden="true" className="pointer-events-none absolute right-2.5 top-3 size-3.5 animate-spin text-muted-foreground" />}
      {shown && <div className="absolute inset-x-0 top-full z-50 mt-1 overflow-hidden rounded-md border bg-popover text-popover-foreground shadow-lg">
        <ul ref={list} id="remote-directory-options" role="listbox" aria-label={t("workspace.remoteRoot")} aria-busy={state.loading}
          className="m-0 h-40 overflow-auto overscroll-contain p-1">
          {state.paths.map((path, index) => <li key={path} id={`remote-directory-${index}`} role="option" aria-selected={ready && index === state.active}
            aria-disabled={!ready} title={path}
            className={`flex items-center gap-2 rounded px-2 py-1.5 font-mono text-xs ${ready ? "cursor-pointer hover:bg-accent" : "text-muted-foreground"} ${ready && index === state.active ? "bg-accent text-accent-foreground" : ""}`}
            onMouseDown={(event) => event.preventDefault()} onClick={() => { if (ready) choose(path); }}>
            <Folder className="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" /><span className="truncate">{path.slice(0, -1).split("/").at(-1)}/</span>
          </li>)}
        </ul>
        {!state.paths.length && <div className="pointer-events-none absolute inset-x-0 top-0 flex h-40 items-center justify-center px-3 text-xs text-muted-foreground" role="status">
          <span className="line-clamp-3">{state.loading || state.value !== value ? t("common.processing") : state.message || t("workspace.remoteNoDirectories")}</span>
        </div>}
      </div>}
    </div>
    <p id="remote-directory-help" className="m-0 text-[11px] leading-relaxed text-muted-foreground">{t("workspace.remoteRootHint")}</p>
  </>;
}
