import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const appSource = new URL("./App.jsx", import.meta.url);
const sidebarSource = new URL("./components/Sidebar.jsx", import.meta.url);
const workbenchSource = new URL("./components/TaskWorkbench.jsx", import.meta.url);
const cssSource = new URL("../index.css", import.meta.url);

test("Windows and Linux integrate the conversation controls into one desktop caption", async () => {
  const [app, sidebar, workbench, css] = await Promise.all([
    readFile(appSource, "utf8"),
    readFile(sidebarSource, "utf8"),
    readFile(workbenchSource, "utf8"),
    readFile(cssSource, "utf8"),
  ]);

  assert.match(app, /const desktopChrome = .*\["windows", "linux"\]\.includes/);
  assert.match(app, /windows-titlebar-brand[^>]*><img[^>]*\/><\/span>/, "the product name is absent from both desktop captions");
  assert.match(app, /\{title && <span className="windows-titlebar-task/);
  assert.match(app, /className="windows-titlebar-control relative" aria-label=\{t\("lock\.enter"\)\}/, "standby lock belongs in the desktop caption");
  assert.match(app, /className=\{`windows-titlebar-control \$\{terminalVisible/);
  assert.match(app, /aria-label=\{inspectorCollapsed \? t\("inspector\.expand"\) : t\("inspector\.collapse"\)\}/);
  assert.match(app, /\{!desktopChrome && <div className="app-titlebar/, "the legacy second toolbar remains available only on macOS");
  assert.match(app, /integratedDesktopTitlebar=\{desktopChrome\}/);

  assert.match(workbench, /!integratedDesktopTitlebar && !inspectorCollapsed && !inspectorMaximized/, "desktop captions must not duplicate terminal and status controls above the inspector");
  assert.match(workbench, /if \(inspectorCollapsed\) onToggleInspector\(\); else closeInspector\(\);/, "caption requests must retain the workbench's dirty-buffer close guard");
  assert.match(sidebar, /onWidthChange\?\.\(width\)/, "the caption title should stay aligned when the resizable sidebar changes width");
  assert.match(css, /:root:is\(\[data-platform="windows"\], \[data-platform="linux"\]\) \.sidebar-visibility-toggle\s*\{[^}]*left:\s*36px/s, "the sidebar toggle sits immediately after the app icon");
  assert.match(css, /:root:is\(\[data-platform="windows"\], \[data-platform="linux"\]\) \.windows-titlebar-brand\s*\{[^}]*--windows-titlebar-sidebar-width/s);
  assert.match(css, /:root:is\(\[data-platform="windows"\], \[data-platform="linux"\]\) \.workbench-inspector\.open\s*\{[^}]*top:\s*8px[^}]*height:\s*calc\(100% - 16px\)/s, "the inspector no longer overlaps the removed second toolbar");
});
