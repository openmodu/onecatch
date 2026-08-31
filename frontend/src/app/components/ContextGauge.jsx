import { useTranslation } from "react-i18next";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { formatTokens } from "../format.js";
import { gaugeDash, gaugeGeometry, gaugeTone } from "../contextGauge.js";
import ProgressRing from "./ProgressRing.jsx";

/* A ring, not a bar, because the question is "how much room is left" rather
   than "how does this compare to the others" — there is nothing to compare
   against, only one quantity against its own ceiling.

   Two stroked circles rather than an arc path keeps the geometry exact at any
   radius: the track is the full circumference, the fill is a dash of
   ratio × circumference. The rotation starts the sweep at twelve o'clock. */

function Ring({ arc, dash }) {
  const { radius, stroke, size } = gaugeGeometry;
  const [swept, circumference] = dash.split(" ").map(Number);
  return <ProgressRing ratio={circumference > 0 ? swept / circumference : 0} size={size} radius={radius} stroke={stroke} className={arc} />;
}

/* The gauge spells its reading out in the tooltip rather than on the row: the
   token counts are the detail you go looking for, the percentage is the part
   you want without asking. The ring is aria-hidden and the whole control
   carries one label, so a screen reader reads the sentence once instead of
   announcing a decorative graphic beside it.

   A rendered tooltip rather than a native `title`, because this label is
   rewritten on every usage frame of a running turn: browsers cancel a native
   tooltip whose title changes under the cursor and will not show it again
   until the pointer leaves and returns, which made the reading unavailable
   exactly while the run was spending the context it reports. */
export default function ContextGauge({ window: contextWindow = 0, tokens = 0, known = false, ratio = 0 }) {
  const { t } = useTranslation();
  const percent = Math.round(ratio * 100);
  const { arc, label } = gaugeTone(ratio);
  const dash = gaugeDash(ratio, known, gaugeGeometry.circumference);
  const title = known
    ? t("inspector.contextGaugeLabel", { percent, tokens: formatTokens(tokens), window: formatTokens(contextWindow) })
    : t("inspector.contextUnknown");

  return <Tooltip>
    <TooltipTrigger asChild>
      <span className="context-gauge-compact inline-flex items-center gap-1.5 text-xs tabular-nums" role="img" aria-label={title}>
        <Ring arc={arc} dash={dash} />
        <span className={known ? label : "text-muted-foreground"}>{known ? `${percent}%` : "—"}</span>
      </span>
    </TooltipTrigger>
    <TooltipContent side="top" sideOffset={6}>{title}</TooltipContent>
  </Tooltip>;
}
