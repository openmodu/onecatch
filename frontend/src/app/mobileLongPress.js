import { useEffect, useRef } from "react";

export const LONG_PRESS_MS = 450;
// A finger is never perfectly still; past this the press was a scroll.
const MOVE_TOLERANCE = 12;

// A row that opens on tap needs somewhere to put rename and delete. On a phone
// that is a long press: no extra button crowding a list this app deliberately
// keeps quiet. The press must not also fire the tap it was made on, so the
// caller asks `consume()` before acting on the click.
export function useMobileLongPress(onLongPress, delay = LONG_PRESS_MS) {
  const timer = useRef(0);
  const origin = useRef(null);
  const fired = useRef(false);
  useEffect(() => () => window.clearTimeout(timer.current), []);
  const cancel = () => {
    window.clearTimeout(timer.current);
    timer.current = 0;
    origin.current = null;
  };
  return {
    // consume reports — and clears — a press that already opened the menu.
    consume() {
      if (!fired.current) return false;
      fired.current = false;
      return true;
    },
    handlers: {
      onPointerDown(event) {
        if (event.pointerType === "mouse" && event.button !== 0) return;
        fired.current = false;
        origin.current = { x: event.clientX, y: event.clientY };
        window.clearTimeout(timer.current);
        timer.current = window.setTimeout(() => {
          fired.current = true;
          timer.current = 0;
          onLongPress?.();
        }, delay);
      },
      onPointerMove(event) {
        if (!origin.current) return;
        const moved = Math.abs(event.clientX - origin.current.x) + Math.abs(event.clientY - origin.current.y);
        if (moved > MOVE_TOLERANCE) cancel();
      },
      onPointerUp: cancel,
      onPointerCancel: cancel,
      onPointerLeave: cancel,
      // iOS shows its own callout on a long press; this row has its own menu.
      onContextMenu(event) { event.preventDefault(); },
    },
  };
}
