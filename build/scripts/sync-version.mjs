import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(scriptDir, "../..");
const version = fs.readFileSync(path.join(repoRoot, "VERSION"), "utf8").trim();

if (!/^\d+\.\d+\.\d+$/.test(version)) {
  throw new Error(`VERSION must use X.Y.Z numeric format, got: ${version}`);
}

if (process.argv.includes("--print")) {
  process.stdout.write(version);
  process.exit(0);
}

const targets = [
  {
    file: "build/desktop/config.yml",
    update(content) {
      return content.replace(
        /(^info:\n(?:^[ \t].*\n)*?^  version:\s*)"[^"]+"/m,
        `$1"${version}"`,
      );
    },
  },
  ...["build/desktop/darwin/Info.plist", "build/desktop/darwin/Info.dev.plist"].map(
    (file) => ({
      file,
      update(content) {
        return content.replace(
          /(<key>CFBundle(?:ShortVersionString|Version)<\/key>\s*<string>)[^<]+(<\/string>)/g,
          `$1${version}$2`,
        );
      },
    }),
  ),
];

const checkOnly = process.argv.includes("--check");
const stale = [];

for (const target of targets) {
  const filename = path.join(repoRoot, target.file);
  const current = fs.readFileSync(filename, "utf8");
  const updated = target.update(current);
  if (updated === current) continue;

  stale.push(target.file);
  if (!checkOnly) fs.writeFileSync(filename, updated);
}

if (checkOnly && stale.length > 0) {
  throw new Error(
    `version metadata is stale in: ${stale.join(", ")}; run go tool wails3 task version:sync`,
  );
}

if (!checkOnly && stale.length > 0) {
  process.stdout.write(`Synced ${version} to ${stale.join(", ")}\n`);
}
