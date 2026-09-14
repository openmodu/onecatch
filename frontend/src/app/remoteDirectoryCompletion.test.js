import test from "node:test";
import assert from "node:assert/strict";
import { commonDirectoryPrefix } from "./remoteDirectoryCompletion.js";

test("directory completion keeps unique paths and common prefixes", () => {
  assert.equal(commonDirectoryPrefix([]), "");
  assert.equal(commonDirectoryPrefix(["/srv/project/"]), "/srv/project/");
  assert.equal(commonDirectoryPrefix(["/srv/project-a/", "/srv/project-b/"]), "/srv/project-");
  assert.equal(commonDirectoryPrefix(["/a/", "/b/"]), "/");
  assert.equal(commonDirectoryPrefix(["/项目/甲/", "/项目/乙/"]), "/项目/");
});

import { createDirectoryCompletionClient, directoryQuery } from "./remoteDirectoryCompletion.js";

test("splits absolute, home and relative input into directory and prefix", () => {
  assert.deepEqual(directoryQuery("/srv/pro"), { directory: "/srv/", prefix: "pro" });
  assert.deepEqual(directoryQuery("~/"), { directory: "~/", prefix: "" });
  assert.deepEqual(directoryQuery("~"), { directory: "~/", prefix: "" });
  assert.deepEqual(directoryQuery("项目"), { directory: "", prefix: "项目" });
});

test("typing and backspacing share one whole-directory request", async () => {
  let resolve;
  const calls = [];
  const client = createDirectoryCompletionClient((directory) => {
    calls.push(directory);
    return new Promise((done) => { resolve = done; });
  });
  const first = client.complete("/srv/pr");
  const second = client.complete("/srv/project");
  await Promise.resolve();
  assert.deepEqual(calls, ["/srv/"]);
  resolve(["/srv/project/", "/srv/preview/", "/srv/other/"]);
  assert.deepEqual(await first, ["/srv/project/", "/srv/preview/"]);
  assert.deepEqual(await second, ["/srv/project/"]);
  assert.deepEqual(await client.complete("/srv/"), ["/srv/project/", "/srv/preview/", "/srv/other/"]);
  assert.equal(calls.length, 1);
});

test("filters canonical home results and refreshes expired listings", async () => {
  let time = 0;
  let calls = 0;
  const client = createDirectoryCompletionClient(async () => {
    calls++;
    return ["/home/me/项目/", "/home/me/project a/"];
  }, () => time);
  assert.deepEqual(await client.complete("~/项"), ["/home/me/项目/"]);
  assert.deepEqual(await client.complete("~/pro"), ["/home/me/project a/"]);
  assert.equal(calls, 1);
  assert.equal(client.hasDirectory("~/new"), true);
  assert.equal(client.hasDirectory("/other/"), false);
  time = 30_001;
  assert.equal(client.hasDirectory("~/new"), false);
  await client.complete("~/");
  assert.equal(calls, 2);
});

test("failed requests can be retried and clients do not share hosts or credentials", async () => {
  let calls = 0;
  const client = createDirectoryCompletionClient(async () => {
    if (++calls === 1) throw new Error("offline");
    return ["/srv/a/"];
  });
  await assert.rejects(client.complete("/srv/"), /offline/);
  assert.deepEqual(await client.complete("/srv/"), ["/srv/a/"]);
  const other = createDirectoryCompletionClient(async () => ["/srv/b/"]);
  assert.deepEqual(await other.complete("/srv/"), ["/srv/b/"]);
});
