/* Geometry and thresholds for the context-window ring, kept out of the JSX so
   they can be tested as plain functions rather than asserted against rendered
   markup. */

/* Two sizes for two reading distances. "compact" rides the composer action row
   beside the harness and permission chips, where it is glanceable while you
   type and must not out-weigh the send button. "full" is the standalone
   readout for anywhere with room for the token counts spelled out. */
const VARIANTS = {
  compact: { radius: 7, stroke: 2.5 },
  full: { radius: 26, stroke: 6 },
};

export function gaugeGeometry(variant = "full") {
  const { radius, stroke } = VARIANTS[variant] || VARIANTS.full;
  return {
    radius,
    stroke,
    // The stroke straddles the radius, so the box has to clear half of it on
    // each side or the ring is shaved flat at the four compass points.
    size: (radius + stroke) * 2,
    circumference: 2 * Math.PI * radius,
  };
}

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
