import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const appSource = new URL("./App.jsx", import.meta.url);
const sidebarSource = new URL("./components/Sidebar.jsx", import.meta.url);
const workbenchSource = new URL("./components/TaskWorkbench.jsx", import.meta.url);
const cssSource = new URL("../index.css", import.meta.url);

test("Windows integrates the conversation and workbench controls into its caption", async () => {
  const [app, sidebar, workbench, css] = await Promise.all([
    readFile(appSource, "utf8"),
    readFile(sidebarSource, "utf8"),
    readFile(workbenchSource, "utf8"),
    readFile(cssSource, "utf8"),
  ]);

  assert.match(app, /const isWindows = platform === "windows"/);
  assert.match(app, /\{!isWindows && <span>OneCatch<\/span>\}/, "the product name remains on Linux but is absent from the Windows caption");
  assert.match(app, /\{isWindows && title && <span className="windows-titlebar-task/);
  assert.match(app, /className=\{`windows-titlebar-control \$\{terminalVisible/);
  assert.match(app, /aria-label=\{inspectorCollapsed \? t\("inspector\.expand"\) : t\("inspector\.collapse"\)\}/);
  assert.match(app, /windowsChrome && view === "tasks" \? <span className="min-w-0 flex-1"/);
  assert.match(app, /!windowsChrome && view === "tasks" && !editor && inspectorCollapsed/, "the old terminal location remains available only outside Windows");
  assert.match(app, /integratedWindowsTitlebar=\{windowsChrome\}/);

  assert.match(workbench, /!integratedWindowsTitlebar && !inspectorCollapsed && !inspectorMaximized/, "Windows must not duplicate the terminal and status controls above the inspector");
  assert.match(workbench, /if \(inspectorCollapsed\) onToggleInspector\(\); else closeInspector\(\);/, "caption requests must retain the workbench's dirty-buffer close guard");
  assert.match(sidebar, /onWidthChange\?\.\(width\)/, "the caption title should stay aligned when the resizable sidebar changes width");
  assert.match(css, /:root\[data-platform="windows"\] \.windows-titlebar-brand\s*\{[^}]*--windows-titlebar-sidebar-width/s);
});
