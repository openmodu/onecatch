import { forwardRef, useEffect, useImperativeHandle, useRef } from "react";
import { basicSetup } from "codemirror";
import { Compartment, EditorState } from "@codemirror/state";
import { EditorView, keymap } from "@codemirror/view";
import { loadEditorLanguage } from "../editorLanguage.js";
import { editorOffsetAt, lspPositionAt } from "../editorPosition.js";

const editorTheme = EditorView.theme({
  "&": {
    height: "100%",
    backgroundColor: "transparent",
    color: "var(--foreground)",
    fontSize: "12px",
  },
  "&.cm-focused": { outline: "none" },
  ".cm-scroller": {
    overflow: "auto",
    fontFamily: "var(--font-ui-mono)",
    fontVariantLigatures: "none",
    lineHeight: "20px",
  },
  ".cm-content": { minHeight: "100%", padding: "12px 0", caretColor: "var(--foreground)" },
  ".cm-line": { padding: "0 12px" },
  ".cm-gutters": {
    minWidth: "42px",
    border: "0",
    backgroundColor: "color-mix(in oklab, var(--muted) 35%, transparent)",
    color: "color-mix(in oklab, var(--muted-foreground) 75%, transparent)",
  },
  ".cm-lineNumbers .cm-gutterElement": { minWidth: "34px", padding: "0 8px 0 4px" },
  ".cm-activeLine": { backgroundColor: "color-mix(in oklab, var(--accent) 34%, transparent)" },
  ".cm-activeLineGutter": { backgroundColor: "transparent", color: "var(--primary)", fontWeight: "600" },
  ".cm-selectionBackground, &.cm-focused .cm-selectionBackground": {
    backgroundColor: "color-mix(in oklab, var(--primary) 25%, transparent) !important",
  },
  ".cm-cursor": { borderLeftColor: "var(--foreground)" },
  ".cm-tooltip": { borderColor: "var(--border)", backgroundColor: "var(--popover)", color: "var(--popover-foreground)" },
  ".cm-panels": {
    backgroundColor: "color-mix(in oklab, var(--background) 96%, var(--muted))",
    color: "var(--foreground)",
    fontFamily: "var(--font-ui-sans)",
  },
  ".cm-panels.cm-panels-bottom": {
    borderTop: "1px solid var(--border)",
  },
  ".cm-panel.cm-search": {
    display: "flex",
    alignItems: "center",
    flexWrap: "wrap",
    gap: "6px",
    padding: "8px 38px 8px 10px",
  },
  ".cm-panel.cm-search br": {
    flexBasis: "100%",
    width: "0",
    height: "0",
  },
  ".cm-panel.cm-search input.cm-textfield": {
    boxSizing: "border-box",
    flex: "1 1 260px",
    minWidth: "160px",
    height: "28px",
    margin: "0",
    border: "1px solid var(--border)",
    borderRadius: "6px",
    outline: "none",
    backgroundColor: "var(--background)",
    color: "var(--foreground)",
    padding: "0 9px",
    fontFamily: "var(--font-ui-sans)",
    fontSize: "11px",
  },
  ".cm-panel.cm-search input.cm-textfield:focus": {
    borderColor: "var(--ring)",
    boxShadow: "0 0 0 2px color-mix(in oklab, var(--ring) 18%, transparent)",
  },
  ".cm-panel.cm-search button.cm-button": {
    boxSizing: "border-box",
    height: "28px",
    margin: "0",
    border: "1px solid var(--border)",
    borderRadius: "6px",
    backgroundImage: "none",
    backgroundColor: "var(--background)",
    color: "var(--foreground)",
    padding: "0 10px",
    fontFamily: "var(--font-ui-sans)",
    fontSize: "11px",
    lineHeight: "26px",
    cursor: "pointer",
  },
  ".cm-panel.cm-search button.cm-button:hover": {
    backgroundColor: "var(--accent)",
  },
  ".cm-panel.cm-search label": {
    display: "inline-flex",
    alignItems: "center",
    gap: "4px",
    margin: "0 2px 0 0",
    color: "var(--muted-foreground)",
    fontFamily: "var(--font-ui-sans)",
    fontSize: "10px",
    lineHeight: "28px",
    whiteSpace: "nowrap",
  },
  ".cm-panel.cm-search input[type=checkbox]": {
    width: "14px",
    height: "14px",
    margin: "0",
    accentColor: "var(--primary)",
  },
  ".cm-panel.cm-search button[name=close]": {
    top: "8px",
    right: "9px",
    display: "grid",
    width: "24px",
    height: "24px",
    placeItems: "center",
    borderRadius: "5px",
    color: "var(--muted-foreground)",
    cursor: "pointer",
  },
  ".cm-panel.cm-search button[name=close]:hover": {
    backgroundColor: "var(--accent)",
    color: "var(--foreground)",
  },
  ".cm-searchMatch": {
    backgroundColor: "color-mix(in oklab, var(--warning) 30%, transparent) !important",
    outline: "1px solid color-mix(in oklab, var(--warning) 55%, transparent)",
  },
  ".cm-searchMatch-selected": {
    backgroundColor: "color-mix(in oklab, var(--primary) 30%, transparent) !important",
    outlineColor: "color-mix(in oklab, var(--primary) 65%, transparent)",
  },
});

const CodeEditor = forwardRef(function CodeEditor({
  path,
  value,
  ariaLabel,
  initialViewState,
  navigation,
  onChange,
  onCursorLineChange,
  onDefinition,
  onViewStateChange,
}, forwardedRef) {
  const hostRef = useRef(null);
  const viewRef = useRef(null);
  const languageRef = useRef(null);
  const applyingValueRef = useRef(false);
  const callbacksRef = useRef({ onChange, onCursorLineChange, onDefinition, onViewStateChange });
  callbacksRef.current = { onChange, onCursorLineChange, onDefinition, onViewStateChange };

  const readViewState = () => {
    const view = viewRef.current;
    if (!view) return { anchor: 0, scrollTop: 0, scrollLeft: 0 };
    return {
      anchor: view.state.selection.main.head,
      scrollTop: view.scrollDOM.scrollTop,
      scrollLeft: view.scrollDOM.scrollLeft,
    };
  };

  const reportViewState = () => callbacksRef.current.onViewStateChange?.(path, readViewState());

  const requestDefinition = (view, offset) => {
    const bounded = Math.max(0, Math.min(offset, view.state.doc.length));
    callbacksRef.current.onDefinition?.(lspPositionAt(view.state.doc, bounded));
  };

  useImperativeHandle(forwardedRef, () => ({
    getViewState: readViewState,
    focus: () => viewRef.current?.focus(),
    revealRange(range) {
      const view = viewRef.current;
      if (!view || !range) return;
      const anchor = editorOffsetAt(view.state.doc, range.start);
      const head = editorOffsetAt(view.state.doc, range.end);
      view.dispatch({
        selection: { anchor, head },
        effects: EditorView.scrollIntoView(anchor, { y: "center" }),
      });
      view.focus();
    },
  }), []);

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return undefined;
    const language = new Compartment();
    languageRef.current = language;
    const initialAnchor = Math.max(0, Math.min(initialViewState?.anchor || 0, value.length));
    const view = new EditorView({
      parent: host,
      state: EditorState.create({
        doc: value,
        selection: { anchor: initialAnchor },
        extensions: [
          basicSetup,
          language.of([]),
          editorTheme,
          EditorView.contentAttributes.of({ spellcheck: "false", "aria-label": ariaLabel || `Edit ${path}` }),
          keymap.of([{
            key: "F12",
            run(current) {
              requestDefinition(current, current.state.selection.main.head);
              return true;
            },
          }]),
          EditorView.domEventHandlers({
            mousedown(event, current) {
              if (event.button !== 0 || (!event.metaKey && !event.ctrlKey)) return false;
              const offset = current.posAtCoords({ x: event.clientX, y: event.clientY });
              if (offset == null) return false;
              event.preventDefault();
              current.dispatch({ selection: { anchor: offset } });
              requestDefinition(current, offset);
              return true;
            },
          }),
          EditorView.updateListener.of((update) => {
            if (update.docChanged && !applyingValueRef.current) callbacksRef.current.onChange?.(update.state.doc.toString());
            if (update.selectionSet || update.docChanged) {
              const line = update.state.doc.lineAt(update.state.selection.main.head).number;
              callbacksRef.current.onCursorLineChange?.(line);
              reportViewState();
            }
          }),
        ],
      }),
    });
    viewRef.current = view;
    const onScroll = () => reportViewState();
    view.scrollDOM.addEventListener("scroll", onScroll, { passive: true });
    requestAnimationFrame(() => {
      if (viewRef.current !== view) return;
      view.scrollDOM.scrollTop = initialViewState?.scrollTop || 0;
      view.scrollDOM.scrollLeft = initialViewState?.scrollLeft || 0;
      callbacksRef.current.onCursorLineChange?.(view.state.doc.lineAt(initialAnchor).number);
    });
    return () => {
      callbacksRef.current.onViewStateChange?.(path, {
        anchor: view.state.selection.main.head,
        scrollTop: view.scrollDOM.scrollTop,
        scrollLeft: view.scrollDOM.scrollLeft,
      });
      view.scrollDOM.removeEventListener("scroll", onScroll);
      view.destroy();
      if (viewRef.current === view) viewRef.current = null;
    };
  }, [path]);

  useEffect(() => {
    const view = viewRef.current;
    if (!view || view.state.doc.toString() === value) return;
    applyingValueRef.current = true;
    view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: value } });
    applyingValueRef.current = false;
  }, [value]);

  useEffect(() => {
    const view = viewRef.current;
    const compartment = languageRef.current;
    if (!view || !compartment) return undefined;
    let cancelled = false;
    void loadEditorLanguage(path).then((support) => {
      if (support && !cancelled && viewRef.current === view) view.dispatch({ effects: compartment.reconfigure(support) });
    });
    return () => { cancelled = true; };
  }, [path]);

  useEffect(() => {
    if (navigation?.path === path) forwardedRef?.current?.revealRange?.(navigation.range);
  }, [forwardedRef, navigation, path]);

  return <div ref={hostRef} className="code-editor h-full min-h-0 min-w-0 overflow-hidden" />;
});

export default CodeEditor;
