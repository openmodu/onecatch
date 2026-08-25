import { useTranslation } from "react-i18next";
import { formatTokens } from "../format.js";
import { gaugeDash, gaugeGeometry, gaugeTone } from "../contextGauge.js";

/* A ring, not a bar, because the question is "how much room is left" rather
   than "how does this compare to the others" — there is nothing to compare
   against, only one quantity against its own ceiling.

   Two stroked circles rather than an arc path keeps the geometry exact at any
   radius: the track is the full circumference, the fill is a dash of
   ratio × circumference. The rotation starts the sweep at twelve o'clock. */

function Ring({ variant, arc, dash, geometry }) {
  const { radius, stroke, size } = geometry;
  return <svg
    className="shrink-0 -rotate-90"
    width={size}
    height={size}
    viewBox={`0 0 ${size} ${size}`}
    aria-hidden="true"
    focusable="false"
    data-variant={variant}
  >
    <circle cx={size / 2} cy={size / 2} r={radius} fill="none" strokeWidth={stroke} className="text-muted" stroke="currentColor" />
    <circle cx={size / 2} cy={size / 2} r={radius} fill="none" strokeWidth={stroke} strokeLinecap="round" strokeDasharray={dash} className={arc} stroke="currentColor" />
  </svg>;
}

export default function ContextGauge({ window: contextWindow = 0, tokens = 0, known = false, ratio = 0, variant = "full" }) {
  const { t } = useTranslation();
  const geometry = gaugeGeometry(variant);
  const percent = Math.round(ratio * 100);
  const { arc, label } = gaugeTone(ratio);
  const dash = gaugeDash(ratio, known, geometry.circumference);
  const title = known
    ? t("inspector.contextGaugeLabel", { percent, tokens: formatTokens(tokens), window: formatTokens(contextWindow) })
    : t("inspector.contextUnknown");

  /* The compact form spells the reading out in the tooltip rather than on the
     row: the token counts are the detail you go looking for, the percentage is
     the part you want without asking. The ring is aria-hidden and the whole
     control carries one label, so a screen reader reads the sentence once
     instead of announcing a decorative graphic beside it. */
  if (variant === "compact") {
    return <span className="context-gauge-compact inline-flex items-center gap-1.5 text-xs tabular-nums" title={title} role="img" aria-label={title}>
      <Ring variant={variant} arc={arc} dash={dash} geometry={geometry} />
      <span className={known ? label : "text-muted-foreground"}>{known ? `${percent}%` : "—"}</span>
    </span>;
  }

  return <div className="flex items-center gap-3 px-1.5 py-3" title={title} role="img" aria-label={title}>
    <Ring variant={variant} arc={arc} dash={dash} geometry={geometry} />
    <div className="min-w-0">
      <span className="block text-[11px] text-muted-foreground">{t("inspector.contextWindow")}</span>
      {known
        ? <>
            <strong className={`mt-1 block text-[17px] font-semibold tabular-nums ${label}`}>{percent}%</strong>
            <small className="mt-1 block text-[11px] leading-snug tabular-nums text-muted-foreground">
              {formatTokens(tokens)} / {formatTokens(contextWindow)}
            </small>
          </>
        : <strong className="mt-1 block text-[17px] font-semibold text-muted-foreground">—</strong>}
    </div>
  </div>;
}
