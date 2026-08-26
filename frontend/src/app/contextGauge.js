/* Geometry and thresholds for the context-window ring, kept out of the JSX so
   they can be tested as plain functions rather than asserted against rendered
   markup. */

/* One size, because there is one place to read this: the composer action row
   beside the harness and permission chips, where the ring is glanceable while
   you type and must not out-weigh the send button. */
const RADIUS = 7;
const STROKE = 2.5;

export const gaugeGeometry = {
  radius: RADIUS,
  stroke: STROKE,
  // The stroke straddles the radius, so the box has to clear half of it on
  // each side or the ring is shaved flat at the four compass points.
  size: (RADIUS + STROKE) * 2,
  circumference: 2 * Math.PI * RADIUS,
};

/* Fill colour is a state, not an identity, so it uses the reserved status
   tokens rather than a series colour, and it changes only at thresholds a user
   would act on: nothing to do below 75%, consider a fresh session at 75%,
   expect the harness to compact at 90%. The percentage is always rendered as
   text beside the ring, so colour reinforces the reading and never carries it
   alone. */
export const GAUGE_WARN_AT = 0.75;
export const GAUGE_CRITICAL_AT = 0.9;

export function gaugeTone(ratio = 0) {
  if (ratio >= GAUGE_CRITICAL_AT) return { arc: "text-destructive", label: "text-destructive" };
  if (ratio >= GAUGE_WARN_AT) return { arc: "text-warning", label: "text-warning" };
  return { arc: "text-primary", label: "text-foreground" };
}

/* An unknown window draws an empty track rather than a full or a zeroed arc:
   a runtime that never reported one has not told us the context is empty, and
   a ring drawn at 0% asserts something we do not know. */
export function gaugeDash(ratio, known, circumference) {
  const swept = known ? circumference * Math.min(Math.max(ratio || 0, 0), 1) : 0;
  return `${swept} ${circumference}`;
}
