import { useEffect, useRef } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { SearchAddon } from "@xterm/addon-search";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { X } from "lucide-react";
import "@xterm/xterm/css/xterm.css";
import { resolveTerminalTheme } from "../terminalThemes.js";

export default function TerminalPane({ tabID, active, theme, onReady, onData, onResize, onCopy, onSearch, onOpenLink, onActivate, onClose, closeLabel, canClose }) {
  const hostRef = useRef(null);
  const mountRef = useRef(null);
  const terminalRef = useRef(null);
  const activeRef = useRef(active);
  activeRef.current = active;

  useEffect(() => {
    const host = hostRef.current;
    const mount = mountRef.current;
    if (!host || !mount) return undefined;
    const terminal = new Terminal({ cursorBlink: true, cursorStyle: "bar", fontFamily: 'ui-monospace, "SFMono-Regular", "SF Mono", Menlo, Consolas, monospace', fontSize: 12, lineHeight: 1.24, scrollback: 10000, theme: resolveTerminalTheme(theme) });
    const fit = new FitAddon();
    const search = new SearchAddon();
    const links = new WebLinksAddon((_event, uri) => onOpenLink(uri));
    terminal.loadAddon(fit);
    terminal.loadAddon(search);
    terminal.loadAddon(links);
    terminal.open(mount);
    terminalRef.current = terminal;
    const input = terminal.onData((data) => onData(tabID, data));
    terminal.attachCustomKeyEventHandler((event) => {
      const modifier = event.metaKey || event.ctrlKey;
      if (!modifier || event.type !== "keydown") return true;
      if (event.key.toLowerCase() === "f") { onSearch(); return false; }
      if (event.key.toLowerCase() === "c" && terminal.hasSelection()) { onCopy(); return false; }
      // XTerm handles the browser paste event itself. Intercepting Cmd/Ctrl+V
      // here and writing the clipboard to the PTY would submit it twice.
      return true;
    });
    const observer = new ResizeObserver(() => {
      if (activeRef.current && host.offsetWidth > 0 && host.offsetHeight > 0) {
        fit.fit();
        onResize(tabID, terminal.rows, terminal.cols);
      }
    });
    observer.observe(host);
    onReady(tabID, { terminal, fit, search });
    requestAnimationFrame(() => { if (activeRef.current) { fit.fit(); terminal.focus(); onResize(tabID, terminal.rows, terminal.cols); } });
    return () => { observer.disconnect(); input.dispose(); terminal.dispose(); terminalRef.current = null; onReady(tabID, null); };
  }, [onCopy, onData, onOpenLink, onReady, onResize, onSearch, tabID]);

  useEffect(() => {
    if (terminalRef.current) terminalRef.current.options.theme = resolveTerminalTheme(theme);
  }, [theme]);

  return <div className={`terminal-host ${active ? "active" : ""}`} ref={hostRef} style={{ "--terminal-pane-background": resolveTerminalTheme(theme).background }} aria-hidden={!active} onMouseDown={() => onActivate(tabID)} onFocusCapture={() => onActivate(tabID)}>
    <div className="terminal-mount" ref={mountRef} />
    {canClose && <button type="button" className="terminal-pane-close" aria-label={closeLabel} title={closeLabel} onPointerDown={(event) => event.stopPropagation()} onClick={(event) => { event.stopPropagation(); onClose(tabID); }}><X size={12} aria-hidden="true" /></button>}
  </div>;
}
