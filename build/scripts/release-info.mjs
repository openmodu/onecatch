#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const VERSION_PATTERN = "(0|[1-9]\\d*)\\.(0|[1-9]\\d*)\\.(0|[1-9]\\d*)";
const VERSION_HEADING = new RegExp(`^##[ \\t]+(${VERSION_PATTERN})[ \\t]*$`);
const ANY_SECOND_LEVEL_HEADING = /^##[ \t]+/;

export function androidVersionCode(version) {
  const parts = version.split(".").map(Number);
  if (parts.length !== 3 || parts.some((part) => !Number.isSafeInteger(part))) {
    throw new Error(`invalid release version: ${version}`);
  }
  if (parts[1] > 999 || parts[2] > 999) {
    throw new Error(
      `Android versionCode requires minor and patch values below 1000, got: ${version}`,
    );
  }

  const code = parts[0] * 1_000_000 + parts[1] * 1_000 + parts[2];
  if (code < 1 || code > 2_100_000_000) {
    throw new Error(`Android versionCode is outside the supported range: ${code}`);
  }
  return code;
}

export function parseChangelog(markdown) {
  const lines = markdown.replaceAll("\r\n", "\n").split("\n");
  const headingIndex = lines.findIndex((line) => ANY_SECOND_LEVEL_HEADING.test(line));
  if (headingIndex < 0) {
    throw new Error("CHANGELOG.md must contain a '## X.Y.Z' version section");
  }

  const match = VERSION_HEADING.exec(lines[headingIndex]);
  if (!match) {
    throw new Error("the first CHANGELOG.md section must use the exact '## X.Y.Z' format");
  }
  const version = match[1];
  const nextHeadingOffset = lines
    .slice(headingIndex + 1)
    .findIndex((line) => ANY_SECOND_LEVEL_HEADING.test(line));
  const notesEnd = nextHeadingOffset < 0
    ? lines.length
    : headingIndex + 1 + nextHeadingOffset;
  const notes = lines.slice(headingIndex + 1, notesEnd).join("\n").trim();

  if (!notes || !notes.replace(/<!--[^]*?-->/g, "").trim()) {
    throw new Error(`CHANGELOG.md release ${version} must include release notes`);
  }

  return {
    version,
    notes,
    androidVersionCode: androidVersionCode(version),
  };
}

export function validateVersionTemplates(repoRoot) {
  const config = JSON.parse(fs.readFileSync(path.join(repoRoot, "keel.json"), "utf8"));
  if (config.version !== "0.0.0") {
    throw new Error("keel.json version template must remain at 0.0.0");
  }
}

const scriptPath = fileURLToPath(import.meta.url);
const isMain = process.argv[1] && path.resolve(process.argv[1]) === scriptPath;

if (isMain) {
  const repoRoot = path.resolve(path.dirname(scriptPath), "../..");
  const changelogPath = path.join(repoRoot, "CHANGELOG.md");
  const release = parseChangelog(fs.readFileSync(changelogPath, "utf8"));
  const command = process.argv[2] ?? "--check";

  switch (command) {
    case "--version":
    case "--print":
      process.stdout.write(release.version);
      break;
    case "--notes":
      process.stdout.write(`${release.notes}\n`);
      break;
    case "--android-version-code":
      process.stdout.write(String(release.androidVersionCode));
      break;
    case "--check":
      validateVersionTemplates(repoRoot);
      process.stdout.write(
        `Release ${release.version} is valid (Keel desktop).\n`,
      );
      break;
    default:
      throw new Error(
        "usage: release-info.mjs --check|--version|--notes|--android-version-code",
      );
  }
}
