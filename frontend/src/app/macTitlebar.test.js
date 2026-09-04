import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { runInNewContext } from "node:vm";
import test from "node:test";
import { build } from "esbuild";

const source = (path) => readFileSync(new URL(path, import.meta.url), "utf8");
const css = source("../index.css");
const sidebar = source("./components/Sidebar.jsx");
const native = source("../../../internal/app/desktop/window_corner_darwin.go");

// Run the installed Wails drag handler, not a copy of its implementation.
// Only the host/DOM boundary is stubbed; no native window is opened or resized.
const runtime = await build({
  entryPoints: [fileURLToPath(new URL("../../node_modules/@wailsio/runtime/dist/drag.js", import.meta.url))],
  bundle: true,
  format: "iife",
  write: false,
  plugins: [{
    name: "window-host",
    setup(builder) {
      builder.onResolve({ filter: /^\.\/(system|flags|utils|environment)\.js$/ }, () => ({ path: "host", namespace: "test" }));
      builder.onLoad({ filter: /.*/, namespace: "test" }, () => ({ contents: `
        export const hasDOM = true;
        export const IsMac = () => true;
        export const IsWindows = () => false;
        export const IsLinux = () => false;
        export const GetFlag = () => false;
        export const canTrackButtons = () => true;
        export const eventTarget = event => event.target;
        export const invoke = message => window.nativeCalls.push(message);
      ` }));
    },
  }],
});

function element(className = "", parentElement = null) {
  return { className, parentElement, clientWidth: 200, clientHeight: 52 };
}

function createWindow() {
  const listeners = new Map();
  const window = {
    nativeCalls: [],
    addEventListener(type, callback) {
      listeners.set(type, [...(listeners.get(type) || []), callback]);
    },
    getComputedStyle(target) {
      return { getPropertyValue(property) {
        // CSS custom properties inherit, including through icons in controls.
        for (let node = target; node; node = node.parentElement) {
          for (const utility of node.className.split(/\s+/)) {
            const rule = css.match(new RegExp(`@utility ${utility} \\{([^}]+)\\}`));
            const value = rule?.[1].match(new RegExp(`${property}:\\s*([^;]+);`));
            if (value) return value[1];
          }
        }
        return "";
      } };
    },
    setInterval: () => 0,
    clearInterval() {},
  };
  window.top = window;
  runInNewContext(runtime.outputFiles[0].text, {
    window,
    navigator: { userAgent: "Macintosh" },
    document: { addEventListener() {} },
  });
  return {
    calls: window.nativeCalls,
    fire(type, target, options = {}) {
      const event = {
        type, target, button: 0, buttons: 0, detail: 1, offsetX: 5, offsetY: 5,
        clientX: 200, clientY: 26, defaultPrevented: false, stopped: false,
        preventDefault() { this.defaultPrevented = true; },
        stopPropagation() {},
        stopImmediatePropagation() { this.stopped = true; },
        ...options,
      };
      for (const listener of listeners.get(type) || []) {
        listener(event);
        if (event.stopped) break;
      }
      return event;
    },
  };
}

test("macOS has one titlebar double-click owner, with no content-view height heuristic", () => {
  assert.doesNotMatch(native, /NSClickGestureRecognizer|addGestureRecognizer|titlebarHeight|\[self\.window zoom:/);
  assert.match(native, /onecatchEnableClickToFocus\(window\)/, "removing the gesture must preserve first-click activation");
  assert.doesNotMatch(sidebar, /disarmMacSidebarDoubleClick|finishMacSidebarDoubleClick/);
  assert.match(sidebar, /className="drag-region h-\[52px\] shrink-0 cursor-default" aria-hidden="true" \/>/);
});

test("a titlebar double-click is forwarded to Wails exactly once", () => {
  const host = createWindow();
  const titlebar = element("drag-region");
  for (const detail of [1, 2]) {
    host.fire("mousedown", titlebar, { detail, buttons: 1 });
    host.fire("mouseup", titlebar, { detail });
    host.fire("click", titlebar, { detail });
  }
  host.fire("dblclick", titlebar, { detail: 2 });
  assert.deepEqual(host.calls, ["wails:drag:doubleclick"]);
});

test("titlebar controls and their nested icons do not resize or drag the window", () => {
  const host = createWindow();
  const control = element("no-drag", element("drag-region"));
  for (const target of [control, element("", control)]) {
    host.fire("mousedown", target, { buttons: 1 });
    host.fire("mousemove", target, { buttons: 1 });
    host.fire("mouseup", target);
    assert.equal(host.fire("dblclick", target, { detail: 2 }).defaultPrevented, false);
  }
  assert.deepEqual(host.calls, []);
});

test("review and inspector toolbars remain content even inside the old top 80pt band", () => {
  const host = createWindow();
  for (const [path, className] of [
    ["./components/inspectors/ReviewPanel.jsx", "review-toolbar"],
    ["./components/inspectors/InspectorPanel.jsx", "workbench-inspector-toolbar"],
  ]) {
    assert.ok(source(path).includes(`className="${className}"`));
    const target = element(className);
    for (const clientY of [26, 52, 64, 79, 80, 120]) {
      assert.equal(host.fire("dblclick", target, { detail: 2, clientY }).defaultPrevented, false);
    }
  }
  assert.deepEqual(host.calls, []);
});

test("the workflow window's caption is a drag region, so double-clicking it zooms", () => {
  // It used to be pointer-events-none, which left the content column without a
  // drag region: only the rail's own strip answered a titlebar double-click.
  assert.match(source("./components/workflow/WorkflowLibrary.jsx"), /: <div className="workflow-titlebar drag-region /);
  const host = createWindow();
  host.fire("dblclick", element("workflow-titlebar drag-region"), { detail: 2 });
  assert.deepEqual(host.calls, ["wails:drag:doubleclick"]);
});

test("single clicks remain immediate and dragging titlebar space still works", () => {
  const host = createWindow();
  const titlebar = element("drag-region");
  assert.equal(host.fire("mousedown", titlebar, { buttons: 1 }).defaultPrevented, false);
  assert.deepEqual(host.calls, []);
  host.fire("mousemove", titlebar, { buttons: 1 });
  assert.deepEqual(host.calls, ["wails:drag"]);
  host.fire("mouseup", titlebar);
});
