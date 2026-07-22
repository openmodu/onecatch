// Measures the claim behind the streaming-jank fix: while a message streams,
// the old MarkdownContent re-ran the whole remark/rehype pipeline over the
// entire (growing) text on every ~80ms flush. This runs that exact plugin chain
// the way react-markdown does, then compares it against what ships now — plain
// text during streaming, one parse when the message settles.
import { unified } from "unified";
import remarkParse from "remark-parse";
import remarkGfm from "remark-gfm";
import remarkRehype from "remark-rehype";
import rehypeSanitize from "rehype-sanitize";

const processor = unified()
  .use(remarkParse)
  .use(remarkGfm)
  .use(remarkRehype)
  .use(rehypeSanitize);

function parseMarkdown(text) {
  return processor.runSync(processor.parse(text));
}

// A realistic agent reply: prose, headings, lists, a fenced code block, a table.
const CHUNK = `
## 分析结果

这是一段中等长度的说明文字，描述了刚才那一步做了什么，以及为什么这么做。

- 第一个要点，带一点 \`inline code\`
- 第二个要点，链接 [文档](https://example.com)
- 第三个要点

\`\`\`js
function handle(event) {
  if (!event) return null;
  return { ...event, at: Date.now() };
}
\`\`\`

| 字段 | 含义 |
| --- | --- |
| seq | 序号 |
| kind | 类型 |

`;

function buildMessage(targetChars) {
  let text = "";
  while (text.length < targetChars) text += CHUNK;
  return text.slice(0, targetChars);
}

// Streaming delivers the message in flushes; each flush re-parsed everything
// from the start. Simulate that: N flushes over a message that grows to `size`.
function simulateStreamingParse(size, flushes) {
  const full = buildMessage(size);
  const start = process.hrtime.bigint();
  for (let i = 1; i <= flushes; i += 1) {
    parseMarkdown(full.slice(0, Math.floor((full.length * i) / flushes)));
  }
  return Number(process.hrtime.bigint() - start) / 1e6;
}

// What ships now: streaming frames are plain text (a string pass), and markdown
// is parsed exactly once when the message settles.
function simulateStreamingPlain(size, flushes) {
  const full = buildMessage(size);
  const start = process.hrtime.bigint();
  let sink = 0;
  for (let i = 1; i <= flushes; i += 1) {
    // Plain-text render: React sets textContent; cost is proportional to slicing.
    sink += String(full.slice(0, Math.floor((full.length * i) / flushes))).length;
  }
  parseMarkdown(full);
  const ms = Number(process.hrtime.bigint() - start) / 1e6;
  if (sink < 0) throw new Error("unreachable");
  return ms;
}

function singleParse(size) {
  const full = buildMessage(size);
  const start = process.hrtime.bigint();
  parseMarkdown(full);
  return Number(process.hrtime.bigint() - start) / 1e6;
}

const sizes = [2_000, 8_000, 20_000, 50_000];
// An 80ms flush cadence: a reply that takes ~16s to stream is ~200 flushes.
const flushes = 200;

console.log(`plugin chain: remark-parse -> remark-gfm -> remark-rehype -> rehype-sanitize`);
console.log(`simulating ${flushes} flushes (80ms cadence ~= ${(flushes * 80) / 1000}s of streaming)\n`);
console.log("msg chars |  1 parse | OLD re-parse/flush | NEW plain+1 parse |  speedup");
console.log("----------|----------|--------------------|-------------------|---------");
for (const size of sizes) {
  // Warm up so the first row is not paying for JIT.
  parseMarkdown(buildMessage(size));
  const one = singleParse(size);
  const oldMs = simulateStreamingParse(size, flushes);
  const newMs = simulateStreamingPlain(size, flushes);
  console.log(
    `${String(size).padStart(9)} | ${one.toFixed(1).padStart(7)}ms | ${oldMs.toFixed(0).padStart(17)}ms | ${newMs.toFixed(0).padStart(16)}ms | ${(oldMs / newMs).toFixed(0).padStart(7)}x`,
  );
}

// Per-flush budget check: at 80ms cadence, anything near/over 80ms means the
// main thread never catches up and every interaction queues behind it.
console.log("\nper-flush parse cost at the END of the stream (the worst frame):");
for (const size of sizes) {
  const worst = singleParse(size);
  const verdict = worst >= 80 ? "OVER BUDGET — main thread saturated" : worst >= 16 ? "drops frames (>16ms)" : "ok";
  console.log(`  ${String(size).padStart(6)} chars: ${worst.toFixed(1).padStart(6)}ms  ${verdict}`);
}
