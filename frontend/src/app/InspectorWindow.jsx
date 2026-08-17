import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { PictureInPicture2 } from "lucide-react";
import { Events } from "@wailsio/runtime";
import { WindowBinding } from "../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js";
import InspectorPanel from "./components/inspectors/InspectorPanel.jsx";
import { INSPECTOR_ACTION_EVENT, INSPECTOR_CONTEXT_EVENT, INSPECTOR_REQUEST_EVENT } from "./inspectorContext.js";

const emptyContext = { mode: "wails", workspaceID: "", runWorkerID: "", draft: false, detail: null, queuedTask: null, queuePosition: 0 };

// The run inspector, floated out of the workbench so it can live on a second
// display. It holds no data of its own: the main window stays the single source
// of truth and publishes the slice this panel renders (see inspectorContext.js).
// The self-fetching tabs — files and Git — talk to their bindings directly,
// which works the same in any window.
export default function InspectorWindow() {
  const { t } = useTranslation();
  const [context, setContext] = useState(emptyContext);
  const [notice, setNotice] = useState(null);

  const notify = useCallback((type, text) => {
    setNotice({ type, text });
    window.setTimeout(() => setNotice(null), 4200);
  }, []);

  useEffect(() => {
    const off = Events.On(INSPECTOR_CONTEXT_EVENT, (event) => {
      if (event?.data) setContext({ ...emptyContext, ...event.data });
    });
    // This window opens after the main one has already published its latest
    // context, so ask for a replay instead of waiting for the next change — an
    // idle run would otherwise leave the panel blank indefinitely.
    void Events.Emit(INSPECTOR_REQUEST_EVENT, {});
    return () => off();
  }, []);

  // The terminal dock only exists in the workbench window; hand the command back.
  const openTerminal = useCallback((command) => {
    void Events.Emit(INSPECTOR_ACTION_EVENT, { type: "open-terminal", command });
  }, []);

  const dock = useCallback(() => { void WindowBinding.CloseInspector(); }, []);

  return <div className="inspector-window relative grid h-full min-h-0 grid-rows-[auto_minmax(0,1fr)] overflow-hidden bg-background text-foreground">
    <div className="pointer-events-none absolute inset-x-0 top-0 z-40 flex h-[52px] select-none items-center justify-center text-sm font-semibold tracking-[-0.01em] text-foreground/85" aria-hidden="true">{t("inspector.rail")}</div>
    <div className="drag-region flex h-[52px] shrink-0 cursor-default items-center justify-end px-3">
      <button type="button" className="workbench-inspector-close no-drag" aria-label={t("inspector.dock")} title={t("inspector.dockHint")} onClick={dock}><PictureInPicture2 size={16} strokeWidth={2} aria-hidden="true" /></button>
    </div>
    <InspectorPanel
      className="inspector-window-panel grid min-h-0 min-w-0"
      mode={context.mode}
      workspaceID={context.workspaceID}
      detail={context.detail}
      queuedTask={context.queuedTask}
      queuePosition={context.queuePosition}
      draft={context.draft}
      runWorkerID={context.runWorkerID}
      notify={notify}
      onOpenTerminal={openTerminal}
    />
    {notice && <div className={`toast ${notice.type}`}><span>{notice.text}</span></div>}
  </div>;
}
