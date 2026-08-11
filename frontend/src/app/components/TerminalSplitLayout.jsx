import { useRef } from "react";
import { clampSplitRatio, layoutGeometry } from "../terminalLayout.js";

const percent = (value) => `${value * 100}%`;
const MIN_PANE_WIDTH = 180;
const MIN_PANE_HEIGHT = 96;

export default function TerminalSplitLayout({ node, renderPane, onRatioChange, resizeLabel }) {
  const containerRef = useRef(null);
  if (!node) return null;
  const geometry = layoutGeometry(node);

  const updateFromPointer = (split, event) => {
    const bounds = containerRef.current?.getBoundingClientRect();
    if (!bounds) return;
    const localLeft = bounds.left + split.rect.x * bounds.width;
    const localTop = bounds.top + split.rect.y * bounds.height;
    const availablePixels = split.direction === "vertical" ? split.rect.width * bounds.width : split.rect.height * bounds.height;
    const requestedRatio = split.direction === "vertical"
      ? (event.clientX - localLeft) / (split.rect.width * bounds.width)
      : (event.clientY - localTop) / (split.rect.height * bounds.height);
    onRatioChange(split.id, clampSplitRatio(requestedRatio, availablePixels, split.direction === "vertical" ? MIN_PANE_WIDTH : MIN_PANE_HEIGHT));
  };

  return <div className="terminal-split-canvas" ref={containerRef}>
    {geometry.panes.map(({ paneID, rect }) => <div className="terminal-pane-slot" key={paneID} style={{ left: percent(rect.x), top: percent(rect.y), width: percent(rect.width), height: percent(rect.height) }}>{renderPane(paneID)}</div>)}
    {geometry.splits.map((split) => {
      const vertical = split.direction === "vertical";
      const style = vertical
        ? { left: percent(split.rect.x + split.rect.width * split.ratio), top: percent(split.rect.y), height: percent(split.rect.height) }
        : { top: percent(split.rect.y + split.rect.height * split.ratio), left: percent(split.rect.x), width: percent(split.rect.width) };
      const resizeWithKeyboard = (event) => {
        const backward = vertical ? event.key === "ArrowLeft" : event.key === "ArrowUp";
        const forward = vertical ? event.key === "ArrowRight" : event.key === "ArrowDown";
        if (!backward && !forward) return;
        event.preventDefault();
        const bounds = containerRef.current?.getBoundingClientRect();
        const availablePixels = bounds ? (vertical ? split.rect.width * bounds.width : split.rect.height * bounds.height) : 0;
        onRatioChange(split.id, clampSplitRatio(split.ratio + (backward ? -0.04 : 0.04), availablePixels, vertical ? MIN_PANE_WIDTH : MIN_PANE_HEIGHT));
      };
      return <span key={split.id} className={`terminal-split-handle ${vertical ? "vertical" : "horizontal"}`} style={style} role="separator" aria-label={resizeLabel} aria-orientation={vertical ? "vertical" : "horizontal"} aria-valuemin="12" aria-valuemax="88" aria-valuenow={Math.round(split.ratio * 100)} tabIndex="0" onPointerDown={(event) => { event.preventDefault(); event.currentTarget.setPointerCapture(event.pointerId); updateFromPointer(split, event); }} onPointerMove={(event) => event.currentTarget.hasPointerCapture(event.pointerId) && updateFromPointer(split, event)} onKeyDown={resizeWithKeyboard} />;
    })}
  </div>;
}
