import { Window } from "@wailsio/runtime";
import { X } from "lucide-react";

// Settings/Workflows keep the macOS-style inset drag strip on every
// platform. On Linux, unlike macOS/Windows, there is no guarantee the
// compositor draws its own titlebar (many Wayland WMs never do), so this is
// the only way to close these windows there. Hidden by default; shown only
// for data-platform="linux" in index.css.
export default function AuxWindowCloseButton() {
  return <button
    type="button"
    className="aux-window-close no-drag absolute right-3 top-3 z-50 hidden size-7 place-items-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"
    aria-label="关闭"
    onClick={() => void Window.Close()}
  >
    <X size={15} aria-hidden="true" />
  </button>;
}
