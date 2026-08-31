import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { fileURLToPath } from "node:url";

const inspectorPath = fileURLToPath(new URL("./app/components/inspectors/StatusInspector.jsx", import.meta.url));

test("status metrics render as a narrow full-width ledger", async () => {
  const source = await readFile(inspectorPath, "utf8");

  assert.match(source, /function TokenLedger/);
  assert.match(source, /className="status-token-ledger-heading/);
  assert.match(source, /<TokenLedger icon=\{Download\}[^>]*details=\{inputTokenDetails\}/);
  assert.match(source, /<TokenLedger icon=\{Upload\}[^>]*details=\{outputTokenDetails\}/);
  assert.match(source, /details\.map\(\(item\) => <div className="flex min-w-0 items-baseline justify-between/);
  assert.doesNotMatch(source, /visibleDetails\.map[\s\S]{0,180}\.join\(" · "\)/, "details must not collapse back into wrapping prose");
  assert.match(source, /className="status-run-metrics/);
});
