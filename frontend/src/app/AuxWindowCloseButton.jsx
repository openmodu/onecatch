import { Window } from "@wailsio/runtime";
import { Minus, Square, X } from "lucide-react";

// Windows and Linux auxiliary windows share the app-drawn title strip so the
// sidebar divider can continue through it. macOS keeps its native controls.
export default function AuxWindowCloseButton() {
  return <div className="aux-window-controls no-drag absolute top-0 right-0 z-50 hidden h-10" onDoubleClick={(event) => event.stopPropagation()}>
    <button type="button" className="aux-window-caption-button windows-only" aria-label="最小化" onClick={() => void Window.Minimise()}><Minus size={14} aria-hidden="true" /></button>
    <button type="button" className="aux-window-caption-button windows-only" aria-label="最大化或还原" onClick={() => void Window.ToggleMaximise()}><Square size={11} aria-hidden="true" /></button>
    <button type="button" className="aux-window-caption-button close" aria-label="关闭" onClick={() => void Window.Close()}><X size={15} aria-hidden="true" /></button>
  </div>;
}
