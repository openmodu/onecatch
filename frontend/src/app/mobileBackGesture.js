import { useEffect } from "react";

export function createBackGesture(onBack) {
  let start = null;
  let horizontal = false;
  return {
    start(x, y, count = 1) {
      start = count === 1 && x >= 0 && x <= 24 ? { x, y } : null;
      horizontal = false;
    },
    move(x, y, count = 1) {
      if (!start) return false;
      const dx = x - start.x, dy = Math.abs(y - start.y);
      if (count !== 1 || (!horizontal && (dy > 12 && dy >= dx || dx < -12))) {
        start = null;
        return false;
      }
      if (dx > 12 && dx > dy * 1.5) horizontal = true;
      return horizontal;
    },
    end(x, y) {
      const back = start && horizontal && x - start.x >= 80 && x - start.x > Math.abs(y - start.y) * 1.5;
      start = null;
      horizontal = false;
      if (back) onBack();
    },
    cancel() { start = null; horizontal = false; },
  };
}

export function useMobileBackGesture(elementRef, onBack, blocked) {
  useEffect(() => {
    const element = elementRef.current;
    if (!element || !onBack || blocked) return undefined;
    const gesture = createBackGesture(onBack);
    const start = (event) => {
      const touch = event.touches[0];
      gesture.start(touch.clientX, touch.clientY, event.touches.length);
    };
    const move = (event) => {
      const touch = event.touches[0];
      if (gesture.move(touch.clientX, touch.clientY, event.touches.length) && event.cancelable) event.preventDefault();
    };
    const end = (event) => {
      const touch = event.changedTouches[0];
      gesture.end(touch.clientX, touch.clientY);
    };
    element.addEventListener("touchstart", start, { passive: true });
    element.addEventListener("touchmove", move, { passive: false });
    element.addEventListener("touchend", end);
    element.addEventListener("touchcancel", gesture.cancel);
    return () => {
      element.removeEventListener("touchstart", start);
      element.removeEventListener("touchmove", move);
      element.removeEventListener("touchend", end);
      element.removeEventListener("touchcancel", gesture.cancel);
    };
  }, [elementRef, onBack, blocked]);
}
