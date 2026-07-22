// End-to-end cost of opening a run, measured on REAL payloads dumped from a
// real ~/.oneshot (see TestRealDataOpenRunCost). Covers the webview half of the
// trip that the Go benchmark could not: JSON.parse of what crosses the Wails
// bridge, building the timeline, and rendering the markdown each round contains.
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { unified } from "unified";
import remarkParse from "remark-parse";
import remarkGfm from "remark-gfm";
import remarkRehype from "remark-rehype";
import rehypeSanitize from "rehype-sanitize";
import { buildRunConversation } from "../src/app/runConversation.js";

const dir = process.argv[2] || "/tmp/oneshot-payloads";

const processor = unified().use(remarkParse).use(remarkGfm).use(remarkRehype).use(rehypeSanitize);
const parseMarkdown = (text) => processor.runSync(processor.parse(text));

const ms = (start) => Number(process.hrtime.bigint() - start) / 1e6;
const best = (runs, fn) => {
  let lowest = Infinity;
  let value;
  for (let i = 0; i < runs; i += 1) {
    const start = process.hrtime.bigint();
    value = fn();
    lowest = Math.min(lowest, ms(start));
  }
  return [lowest, value];
};

const files = readdirSync(dir).filter((name) => name.endsWith(".json"))
  .map((name) => ({ name, path: join(dir, name), size: statSync(join(dir, name)).size }))
  .sort((a, b) => b.size - a.size);

console.log("REAL payloads — webview cost of opening a run (best of 5)\n");
console.log("run            payload |  JSON.parse | buildConversation | markdown render |    TOTAL");
console.log("-----------------------|-------------|-------------------|-----------------|---------");

let totals = { parse: 0, build: 0, md: 0 };
for (const file of files) {
  const raw = readFileSync(file.path, "utf8");
  const [parseMs, detail] = best(5, () => JSON.parse(raw));
  const [buildMs, conversation] = best(5, () => buildRunConversation(detail));

  // Every markdown body the timeline would render on open.
  const bodies = [];
  for (const item of conversation) {
    if (item.type === "user") bodies.push(item.text || "");
    else if (item.type === "round") {
      for (const entry of item.items || []) {
        if (entry.type === "message") bodies.push(entry.text || "");
      }
    }
  }
  const [mdMs] = best(3, () => { for (const body of bodies) parseMarkdown(body); });

  totals.parse += parseMs;
  totals.build += buildMs;
  totals.md += mdMs;
  const total = parseMs + buildMs + mdMs;
  console.log(
    `${file.name.replace("run_", "").slice(0, 12).padEnd(13)} ${(file.size / 1024).toFixed(0).padStart(5)}KB |` +
    ` ${parseMs.toFixed(1).padStart(8)}ms | ${buildMs.toFixed(1).padStart(14)}ms |` +
    ` ${mdMs.toFixed(1).padStart(12)}ms (${String(bodies.length).padStart(3)}) | ${total.toFixed(1).padStart(6)}ms`,
  );
}
console.log("\nSum across all runs: " +
  `JSON.parse ${totals.parse.toFixed(1)}ms + build ${totals.build.toFixed(1)}ms + markdown ${totals.md.toFixed(1)}ms`);
console.log("(markdown count in parens = message bodies parsed on open)");
