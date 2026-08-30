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
  assert.match(app, /const titlebarSidebarWidth = sidebarCollapsed \? 68 : Math\.max\(sidebarWidth, 136\)/, "a collapsed sidebar brings the project and title closer to the window controls");
  assert.match(app, /sidebarCollapsed \? "sidebar-is-collapsed" : ""/, "the desktop caption exposes its collapsed state to CSS");
  assert.match(app, /\{title && <span className="windows-titlebar-task[^"]*pr-1 pl-3/, "the caption title keeps compact breathing room before the lock action");
  assert.match(app, /\{workspaceName && <Tooltip>[\s\S]*?<Folder size=\{14\}[\s\S]*?<TooltipContent side="bottom" sideOffset=\{6\}>\{workspaceName\}<\/TooltipContent>/, "the project folder in the caption reveals its workspace name on hover");
  assert.match(app, /workspaceName=\{view === "tasks" \? selectedWorkspace\?\.name \|\| "" : ""\}/, "only task views place the current project beside the caption title");
  assert.match(app, /<\/span>\}\s*<button type="button" className="windows-titlebar-control no-drag relative shrink-0" aria-label=\{t\("lock\.enter"\)\}/, "standby lock sits immediately after the desktop caption title");
  assert.match(app, /className="windows-titlebar-workbench-actions no-drag ml-auto[^\"]*pr-0/, "status and terminal controls remain aligned flush beside the window buttons");
  assert.match(app, /<div className="no-drag flex h-full"/, "the window buttons sit directly beside the workbench controls");
  assert.doesNotMatch(app, /<div className="no-drag ml-auto flex h-full"/, "the window buttons must not introduce a second auto margin");
  assert.match(app, /className=\{`windows-titlebar-control \$\{terminalVisible/);
  assert.match(app, /aria-label=\{inspectorCollapsed \? t\("inspector\.expand"\) : t\("inspector\.collapse"\)\}/);
  assert.match(app, /\{!desktopChrome && <div className="app-titlebar/, "the legacy second toolbar remains available only on macOS");
  assert.match(app, /integratedDesktopTitlebar=\{desktopChrome\}/);

  assert.match(workbench, /!integratedDesktopTitlebar && !inspectorCollapsed && !inspectorMaximized/, "desktop captions must not duplicate terminal and status controls above the inspector");
  assert.match(workbench, /if \(inspectorCollapsed\) onToggleInspector\(\); else closeInspector\(\);/, "caption requests must retain the workbench's dirty-buffer close guard");
  assert.match(sidebar, /onWidthChange\?\.\(width\)/, "the caption title should stay aligned when the resizable sidebar changes width");
  assert.match(css, /:root:is\(\[data-platform="windows"\], \[data-platform="linux"\]\) \.sidebar-visibility-toggle\s*\{[^}]*left:\s*36px/s, "the sidebar toggle sits immediately after the app icon");
  assert.match(css, /:root:is\(\[data-platform="windows"\], \[data-platform="linux"\]\) \.sidebar\s*\{[^}]*border-right:\s*1px solid var\(--border\)/s, "the sidebar divider reaches the bottom of the content rail");
  assert.match(css, /:root:is\(\[data-platform="windows"\], \[data-platform="linux"\]\) \.windows-titlebar::after\s*\{[^}]*left:\s*var\(--windows-titlebar-sidebar-width[^}]*height:\s*1px/s, "the caption rule starts at the main content instead of crossing the sidebar");
  assert.match(css, /\.windows-titlebar\.sidebar-is-collapsed::after\s*\{[^}]*left:\s*0/s, "the caption rule reaches the left window edge when the sidebar is collapsed");
  assert.match(css, /:root:is\(\[data-platform="windows"\], \[data-platform="linux"\]\) \.windows-titlebar-brand\s*\{[^}]*--windows-titlebar-sidebar-width/s);
  assert.match(css, /\.windows-titlebar-task\s*\{[^}]*max-width:\s*min\(40vw, 420px\)/s, "long desktop caption titles are constrained before truncating with an ellipsis");
  assert.match(css, /:root:is\(\[data-platform="windows"\], \[data-platform="linux"\]\) \.windows-titlebar-brand\s*\{[^}]*border-right:\s*1px solid var\(--border\)[^}]*background:\s*var\(--sidebar\)/s, "the sidebar colour and divider continue through the caption");
  assert.match(css, /\.windows-titlebar\.sidebar-is-collapsed \.windows-titlebar-brand\s*\{[^}]*border-right:\s*0[^}]*background:\s*transparent/s, "the collapsed caption removes the divider before the project and title");
  assert.match(css, /:root:is\(\[data-platform="windows"\], \[data-platform="linux"\]\) \.workbench-inspector\.open\s*\{[^}]*top:\s*8px[^}]*height:\s*calc\(100% - 16px\)/s, "the inspector no longer overlaps the removed second toolbar");
});
