import { useEffect, useRef } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { SearchAddon } from "@xterm/addon-search";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { X } from "lucide-react";
import "@xterm/xterm/css/xterm.css";
import { hasLostTerminalSelectionMouseUp, isAccidentalTerminalSelectionDrag, isTerminalCopyShortcut, shouldStartTerminalSelection } from "../terminalInteraction.js";
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
      if (event.type !== "keydown") return true;
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "f") { onSearch(); return false; }
      if (isTerminalCopyShortcut(event)) {
        if (terminal.hasSelection()) onCopy();
        return false;
      }
      // XTerm handles the browser paste event itself. Intercepting Cmd/Ctrl+V
      // here and writing the clipboard to the PTY would submit it twice.
      return true;
    });
    const terminalElement = terminal.element;
    const terminalDocument = terminalElement.ownerDocument;
    let replayingSelectionStart = false;
    let selectionActive = false;
    let selectionPending = null;
    let selectionOrigin = null;
    const captureSelectionPointer = (event) => {
      if (event.button !== 0) return;
      try { terminalElement.setPointerCapture(event.pointerId); } catch { /* WebKit may reject capture during teardown. */ }
    };
    const beginSelection = (event) => {
      if (replayingSelectionStart || event.button !== 0) return;
      selectionPending = null;
      selectionOrigin = null;
      const modified = event.altKey || event.ctrlKey || event.metaKey || event.shiftKey;
      const passDirectly = event.detail !== 1 || modified || terminal.modes.mouseTrackingMode !== "none" || terminalElement.classList.contains("xterm-cursor-pointer");
      if (passDirectly) {
        selectionActive = event.detail > 1 || modified;
        return;
      }
      selectionActive = false;
      selectionPending = {
        clientX: event.clientX,
        clientY: event.clientY,
        screenX: event.screenX,
        screenY: event.screenY,
        timeStamp: event.timeStamp,
      };
      terminal.focus();
      onActivate(tabID);
      event.preventDefault();
      event.stopImmediatePropagation();
    };
    const replaySelection = (event) => {
      const origin = selectionPending;
      const MouseEventConstructor = terminalDocument.defaultView?.MouseEvent;
      if (!origin || !MouseEventConstructor) return;
      selectionPending = null;
      selectionActive = true;
      selectionOrigin = origin;
      replayingSelectionStart = true;
      try {
        terminalElement.dispatchEvent(new MouseEventConstructor("mousedown", {
          bubbles: true,
          cancelable: true,
          view: terminalDocument.defaultView,
          button: 0,
          buttons: 1,
          detail: 1,
          clientX: origin.clientX,
          clientY: origin.clientY,
          screenX: origin.screenX,
          screenY: origin.screenY,
        }));
      } finally {
        replayingSelectionStart = false;
      }
      terminalElement.dispatchEvent(new MouseEventConstructor("mousemove", {
        bubbles: true,
        cancelable: true,
        view: terminalDocument.defaultView,
        button: 0,
        buttons: 1,
        clientX: event.clientX,
        clientY: event.clientY,
        screenX: event.screenX,
        screenY: event.screenY,
      }));
    };
    const finishSelection = (event) => {
      if (selectionPending) {
        selectionPending = null;
        selectionActive = false;
        terminal.clearSelection();
        return;
      }
      selectionActive = false;
      const origin = selectionOrigin;
      selectionOrigin = null;
      if (!isAccidentalTerminalSelectionDrag(origin, event)) return;
      queueMicrotask(() => { if (terminalRef.current === terminal) terminal.clearSelection(); });
    };
    const cancelStuckSelection = (event = selectionOrigin) => {
      selectionPending = null;
      if (!selectionActive) return;
      selectionActive = false;
      selectionOrigin = null;
      terminal.clearSelection();
      const MouseEventConstructor = terminalDocument.defaultView?.MouseEvent;
      if (!MouseEventConstructor) return;
      terminalElement.dispatchEvent(new MouseEventConstructor("mouseup", {
        bubbles: true,
        button: 0,
        buttons: 0,
        clientX: event?.clientX || 0,
        clientY: event?.clientY || 0,
      }));
    };
    const recoverLostMouseUp = (event) => {
      if (selectionPending) {
        if ((event.buttons & 1) === 0) {
          selectionPending = null;
          return;
        }
        if (shouldStartTerminalSelection(selectionPending, event)) replaySelection(event);
        return;
      }
      if (hasLostTerminalSelectionMouseUp(selectionActive, event)) cancelStuckSelection(event);
    };
    terminalElement.addEventListener("pointerdown", captureSelectionPointer);
    terminalElement.addEventListener("pointercancel", cancelStuckSelection);
    terminalElement.addEventListener("mousedown", beginSelection, true);
    terminalDocument.addEventListener("mousemove", recoverLostMouseUp, true);
    terminalDocument.addEventListener("mouseup", finishSelection);
    terminalDocument.defaultView?.addEventListener("blur", cancelStuckSelection);
    const observer = new ResizeObserver(() => {
      if (activeRef.current && host.offsetWidth > 0 && host.offsetHeight > 0) {
        fit.fit();
        onResize(tabID, terminal.rows, terminal.cols);
      }
    });
    observer.observe(host);
    onReady(tabID, { terminal, fit, search });
    requestAnimationFrame(() => { if (activeRef.current) { fit.fit(); terminal.focus(); onResize(tabID, terminal.rows, terminal.cols); } });
    return () => {
      observer.disconnect();
      input.dispose();
      terminalElement.removeEventListener("pointerdown", captureSelectionPointer);
      terminalElement.removeEventListener("pointercancel", cancelStuckSelection);
      terminalElement.removeEventListener("mousedown", beginSelection, true);
      terminalDocument.removeEventListener("mousemove", recoverLostMouseUp, true);
      terminalDocument.removeEventListener("mouseup", finishSelection);
      terminalDocument.defaultView?.removeEventListener("blur", cancelStuckSelection);
      terminal.dispose();
      terminalRef.current = null;
      onReady(tabID, null);
    };
  }, [onActivate, onCopy, onData, onOpenLink, onReady, onResize, onSearch, tabID]);

  useEffect(() => {
    if (terminalRef.current) terminalRef.current.options.theme = resolveTerminalTheme(theme);
  }, [theme]);

  return <div className={`terminal-host ${active ? "active" : ""}`} ref={hostRef} style={{ "--terminal-pane-background": resolveTerminalTheme(theme).background }} aria-hidden={!active} onMouseDown={() => onActivate(tabID)} onFocusCapture={() => onActivate(tabID)}>
    <div className="terminal-mount" ref={mountRef} />
    {canClose && <button type="button" className="terminal-pane-close" aria-label={closeLabel} title={closeLabel} onPointerDown={(event) => event.stopPropagation()} onClick={(event) => { event.stopPropagation(); onClose(tabID); }}><X size={12} aria-hidden="true" /></button>}
  </div>;
}
