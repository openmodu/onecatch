import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const readJSON = (filename) => JSON.parse(readFileSync(filename, "utf8"));
const wails = JSON.parse(execFileSync(
  "go",
  ["list", "-m", "-json", "github.com/wailsapp/wails/v3"],
  { cwd: root, encoding: "utf8" },
));
const bundledRuntime = readJSON(join(
  wails.Dir,
  "internal/runtime/desktop/@wailsio/runtime/package.json",
)).version;
const frontend = readJSON(join(root, "frontend/package.json"));
const lock = readJSON(join(root, "frontend/package-lock.json"));
const declaredRuntime = frontend.dependencies?.["@wailsio/runtime"];
const lockedRuntime = lock.packages?.["node_modules/@wailsio/runtime"]?.version;
// beta.10 did not publish a matching npm package. Its release only changes the
// Linux host and release tooling, so the beta.9 JavaScript runtime remains the
// upstream-published runtime for that Go release.
const publishedRuntimeOverrides = new Map([
  ["v3.0.0-beta.10", "3.0.0-beta.9"],
]);
const expectedRuntime = publishedRuntimeOverrides.get(wails.Version) ?? bundledRuntime;

if (declaredRuntime !== expectedRuntime || lockedRuntime !== expectedRuntime) {
  console.error([
    `Wails runtime mismatch for Go ${wails.Version}:`,
    `  bundled:     ${bundledRuntime}`,
    `  expected:    ${expectedRuntime}`,
    `  package.json: ${declaredRuntime || "missing"}`,
    `  lockfile:     ${lockedRuntime || "missing"}`,
    "Pin @wailsio/runtime to the expected version and regenerate package-lock.json.",
  ].join("\n"));
  process.exit(1);
}

console.log(`Wails dependencies aligned: Go ${wails.Version}, JS ${expectedRuntime}`);
