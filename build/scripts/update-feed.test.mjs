import assert from "node:assert/strict";
import { generateKeyPairSync } from "node:crypto";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import test from "node:test";
import { fileURLToPath } from "node:url";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));

test("embeds escaped release notes in generated update feeds", async () => {
  const tempRoot = await mkdtemp(path.join(os.tmpdir(), "onecatch-update-feed-test-"));
  try {
    const dist = path.join(tempRoot, "dist");
    const privateKeyFile = path.join(tempRoot, "private.pem");
    const publicKeyFile = path.join(tempRoot, "public.base64");
    const releaseNotesFile = path.join(tempRoot, "release-notes.md");
    await mkdir(dist);

    const { privateKey, publicKey } = generateKeyPairSync("ed25519");
    const privatePEM = privateKey.export({ format: "pem", type: "pkcs8" });
    const publicRaw = publicKey.export({ format: "der", type: "spki" }).subarray(-32);
    await writeFile(privateKeyFile, privatePEM);
    await writeFile(publicKeyFile, `${publicRaw.toString("base64")}\n`);
    await writeFile(releaseNotesFile, "- Added <skills> & fixed updates.\n");
    await writeFile(path.join(dist, "OneCatch-1.2.3-darwin-arm64.zip"), "artifact");

    const result = spawnSync(process.execPath, [
      path.join(scriptDir, "update-feed.mjs"),
      "generate",
      "--dist", dist,
      "--version", "v1.2.3",
      "--base-url", "https://example.com/releases/download/v1.2.3",
      "--private-key-file", privateKeyFile,
      "--public", publicKeyFile,
      "--release-notes-file", releaseNotesFile,
    ], { encoding: "utf8" });

    assert.equal(result.status, 0, result.stderr);
    const xml = await readFile(path.join(dist, "appcast-darwin-arm64.xml"), "utf8");
    assert.match(xml, /<description>- Added &lt;skills&gt; &amp; fixed updates\.<\/description>/);
  } finally {
    await rm(tempRoot, { recursive: true, force: true });
  }
});
