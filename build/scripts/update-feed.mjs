#!/usr/bin/env node

import { createHash, generateKeyPairSync, createPrivateKey, createPublicKey, sign } from "node:crypto";
import { mkdir, readFile, readdir, stat, writeFile } from "node:fs/promises";
import path from "node:path";

const args = process.argv.slice(2);
const command = args.shift();
const option = (name, fallback = "") => {
  const index = args.indexOf(`--${name}`);
  return index >= 0 ? args[index + 1] : fallback;
};

if (command === "keygen") {
  const privatePath = option("private", ".secrets/update-ed25519-private.pem");
  const publicPath = option("public", "build/update/public-key.base64");
  const exists = async (candidate) => stat(candidate).then(() => true, () => false);
  if (!args.includes("--force") && (await exists(privatePath) || await exists(publicPath))) {
    throw new Error("refusing to overwrite an update key; pass --force only for an intentional, planned rotation");
  }
  const { privateKey, publicKey } = generateKeyPairSync("ed25519");
  const privatePEM = privateKey.export({ format: "pem", type: "pkcs8" });
  const publicRaw = publicKey.export({ format: "der", type: "spki" }).subarray(-32);
  await mkdir(path.dirname(privatePath), { recursive: true });
  await mkdir(path.dirname(publicPath), { recursive: true });
  await writeFile(privatePath, privatePEM, { mode: 0o600 });
  await writeFile(publicPath, `${publicRaw.toString("base64")}\n`);
  console.log(`Private key: ${privatePath}`);
  console.log(`Public key: ${publicPath}`);
  process.exit(0);
}

if (command !== "generate") {
  throw new Error("usage: update-feed.mjs keygen|generate");
}

const dist = path.resolve(option("dist", "dist"));
const version = option("version").replace(/^v/, "");
const baseURL = option("base-url").replace(/\/$/, "");
const releaseNotesFile = option("release-notes-file");
const releaseNotes = releaseNotesFile ? (await readFile(releaseNotesFile, "utf8")).trim() : "";
const privateKeyFile = option("private-key-file");
const privateValue = process.env.ONECATCH_UPDATE_PRIVATE_KEY || option("private-key") || (privateKeyFile ? await readFile(privateKeyFile, "utf8") : "");
const expectedPublic = (await readFile(option("public", "build/update/public-key.base64"), "utf8")).trim();
if (!/^\d+\.\d+\.\d+$/.test(version)) throw new Error(`invalid version: ${version}`);
if (!baseURL) throw new Error("--base-url is required");
if (!privateValue) throw new Error("ONECATCH_UPDATE_PRIVATE_KEY is required");

const privatePEM = privateValue.includes("BEGIN PRIVATE KEY")
  ? privateValue
  : Buffer.from(privateValue, "base64").toString("utf8");
const privateKey = createPrivateKey(privatePEM);
const publicRaw = createPublicKey(privateKey).export({ format: "der", type: "spki" }).subarray(-32).toString("base64");
if (publicRaw !== expectedPublic) throw new Error("release private key does not match build/update/public-key.base64");

const files = await readdir(dist);
const targets = [];
for (const filename of files) {
  let platform = "";
  let arch = "";
  if (filename === `OneCatch-${version}-darwin-arm64.zip`) [platform, arch] = ["darwin", "arm64"];
  else if (filename === `OneCatch-${version}-darwin-amd64.zip`) [platform, arch] = ["darwin", "amd64"];
  else if (filename === `OneCatch-${version}-Windows-x64-Setup.exe`) [platform, arch] = ["windows", "amd64"];
  else if (filename === `OneCatch-${version}-Windows-arm64-Setup.exe`) [platform, arch] = ["windows", "arm64"];
  else if (filename === `OneCatch-${version}-Linux-x64.AppImage`) [platform, arch] = ["linux", "amd64"];
  else if (filename === `OneCatch-${version}-Linux-arm64.AppImage`) [platform, arch] = ["linux", "arm64"];
  if (!platform) continue;
  const artifactPath = path.join(dist, filename);
  const payload = await readFile(artifactPath);
  const digest = createHash("sha256").update(payload).digest();
  const signature = sign(null, digest, privateKey).toString("base64");
  const size = (await stat(artifactPath)).size;
  targets.push({ filename, platform, arch, signature, size });
}
if (!targets.length) throw new Error(`no updater artifacts found in ${dist}`);

const escape = (value) => String(value).replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;");
const published = new Date().toUTCString();
for (const target of targets) {
  const artifactURL = `${baseURL}/${encodeURIComponent(target.filename)}`;
  const releaseURL = baseURL.replace(/\/download\/v[^/]+$/, `/tag/v${version}`);
  const xml = `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
  <channel>
    <title>OneCatch updates</title>
    <item>
      <title>OneCatch ${escape(version)}</title>
      <pubDate>${published}</pubDate>
      <description>${escape(releaseNotes || `Release notes: ${releaseURL}`)}</description>
      <sparkle:releaseNotesLink>${escape(releaseURL)}</sparkle:releaseNotesLink>
      <sparkle:version>${escape(version)}</sparkle:version>
      <sparkle:shortVersionString>${escape(version)}</sparkle:shortVersionString>
      <sparkle:os>${target.platform}</sparkle:os>
      <enclosure url="${escape(artifactURL)}" length="${target.size}" type="application/octet-stream" sparkle:edSignature="${target.signature}" />
    </item>
  </channel>
</rss>
`;
  const output = path.join(dist, `appcast-${target.platform}-${target.arch}.xml`);
  await writeFile(output, xml);
  console.log(output);
}
