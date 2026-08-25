import test from "node:test";
import assert from "node:assert/strict";
import { gaugeDash, gaugeGeometry, gaugeTone } from "./contextGauge.js";

const FULL = gaugeGeometry("full");
const COMPACT = gaugeGeometry("compact");

test("the swept arc is proportional to occupancy", () => {
  const [quarter] = gaugeDash(0.25, true, FULL.circumference).split(" ").map(Number);
  assert.ok(Math.abs(quarter - FULL.circumference / 4) < 1e-9);
  const [full] = gaugeDash(1, true, FULL.circumference).split(" ").map(Number);
  assert.ok(Math.abs(full - FULL.circumference) < 1e-9);
});

test("an unreported window draws an empty track, not a zeroed reading", () => {
  // Both render no arc, but only one of them is a claim about the context.
  // The distinction matters to the label beside the ring, which reads "—"
  // rather than "0%" when the runtime never told us the size.
  assert.equal(gaugeDash(0.8, false, FULL.circumference).split(" ")[0], "0");
});

test("a ratio outside 0..1 cannot overdraw or reverse the arc", () => {
  const [over] = gaugeDash(1.4, true, FULL.circumference).split(" ").map(Number);
  assert.ok(Math.abs(over - FULL.circumference) < 1e-9);
  const [under] = gaugeDash(-0.2, true, FULL.circumference).split(" ").map(Number);
  assert.equal(under, 0);
});

test("tone escalates only at the thresholds a user would act on", () => {
  assert.equal(gaugeTone(0).arc, "text-primary");
  assert.equal(gaugeTone(0.74).arc, "text-primary");
  assert.equal(gaugeTone(0.75).arc, "text-warning");
  assert.equal(gaugeTone(0.89).arc, "text-warning");
  assert.equal(gaugeTone(0.9).arc, "text-destructive");
  assert.equal(gaugeTone(1).arc, "text-destructive");
});

test("the resting tone leaves the number in ordinary ink", () => {
  // Colour reinforces the reading; it never becomes the reading. A context at
  // 20% is not "good news" worth a status colour, it is simply normal.
  assert.equal(gaugeTone(0.2).label, "text-foreground");
  assert.equal(gaugeTone(0.95).label, "text-destructive");
});

test("both variants clear their own stroke so the ring is never shaved flat", () => {
  for (const geometry of [FULL, COMPACT]) {
    const { radius, stroke, size } = geometry;
    const outer = radius + stroke / 2;
    assert.ok(size / 2 - outer >= 0, `variant overflows its box: ${JSON.stringify(geometry)}`);
    assert.ok(size / 2 + outer <= size, `variant overflows its box: ${JSON.stringify(geometry)}`);
  }
});

test("the compact ring stays small enough for a toolbar row", () => {
  // The composer action row is sized by its buttons; a ring taller than them
  // would push the row and out-weigh the send button beside it.
  assert.ok(COMPACT.size <= 20, `compact gauge is ${COMPACT.size}px`);
  assert.ok(COMPACT.size < FULL.size);
});

test("occupancy is proportional in the compact ring too", () => {
  const [half] = gaugeDash(0.5, true, COMPACT.circumference).split(" ").map(Number);
  assert.ok(Math.abs(half - COMPACT.circumference / 2) < 1e-9);
});
