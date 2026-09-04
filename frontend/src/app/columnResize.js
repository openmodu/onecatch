import { useCallback, useRef, useState } from "react";

// Drag-to-resize for one fixed-width column, with the same manners the
// workbench inspector's edge already has: pointer capture so the drag survives
// leaving the handle, arrow keys for people who cannot drag precisely, and a
// double-click back to the default.
//
// `fromRight` says which side of the handle the column is on. A column to the
// left of its handle grows as the pointer moves right; one to the right of it
// grows as the pointer moves left.
export function useColumnWidth({ defaultWidth, min, max, fromRight = false, step = 20 }) {
  const [width, setWidth] = useState(defaultWidth);
  const [resizing, setResizing] = useState(false);
  const dragRef = useRef(null);

  const clamp = useCallback((value) => Math.min(max, Math.max(min, Math.round(value))), [max, min]);

  const begin = (event) => {
    if (event.button !== 0) return;
    event.preventDefault();
    event.currentTarget.setPointerCapture?.(event.pointerId);
    dragRef.current = { pointerID: event.pointerId, startX: event.clientX, startWidth: width };
    setResizing(true);
  };

  const move = (event) => {
    const drag = dragRef.current;
    if (!drag || drag.pointerID !== event.pointerId) return;
    const delta = fromRight ? drag.startX - event.clientX : event.clientX - drag.startX;
    setWidth(clamp(drag.startWidth + delta));
  };

  const end = (event) => {
    if (dragRef.current?.pointerID !== event.pointerId) return;
    dragRef.current = null;
    setResizing(false);
    if (event.currentTarget.hasPointerCapture?.(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
  };

  const nudge = (event) => {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    event.preventDefault();
    const toward = event.key === "ArrowRight" ? step : -step;
    setWidth((current) => clamp(current + (fromRight ? -toward : toward)));
  };

  const reset = () => setWidth(clamp(defaultWidth));

  return {
    width,
    resizing,
    reset,
    separatorProps: {
      role: "separator",
      "aria-orientation": "vertical",
      "aria-valuemin": min,
      "aria-valuemax": max,
      "aria-valuenow": width,
      tabIndex: 0,
      onPointerDown: begin,
      onPointerMove: move,
      onPointerUp: end,
      onPointerCancel: end,
      onKeyDown: nudge,
      onDoubleClick: reset,
    },
  };
}

// One hairline that widens its hit area beyond the pixel it draws, so the grab
// zone is reachable without a visible gutter between the columns.
export const COLUMN_SEPARATOR_CLASS = "relative z-10 w-px shrink-0 cursor-col-resize bg-border/60 transition-colors after:absolute after:inset-y-0 after:-left-[3px] after:w-[7px] after:content-[''] hover:bg-primary/50 focus-visible:bg-primary focus-visible:outline-none";
