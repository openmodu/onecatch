import { useEffect, useRef, useState } from "react";

// Past this the release refreshes; below it the list springs back.
export const PULL_THRESHOLD = 64;
const PULL_LIMIT = 96;

// The finger travels further than the list does, so the pull has a floor that
// says "keep going" rather than snapping open on a stray drag.
export function pullOffset(delta) {
  if (!Number.isFinite(delta) || delta <= 0) return 0;
  return Math.min(PULL_LIMIT, Math.round(delta * 0.5));
}

// The shell disables WebKit's rubber band — a stray drag used to lift the whole
// app off the screen — which also takes away the platform's pull to refresh.
// This puts the gesture back on one scrolling list: only from the very top,
// only while the drag is more vertical than horizontal, so it never fights the
// left-edge back swipe.
export function useMobilePullToRefresh(elementRef, onRefresh, enabled = true) {
  const [refreshing, setRefreshing] = useState(false);
  const busy = useRef(false);
  useEffect(() => {
    const element = elementRef.current;
    if (!element || !enabled) return undefined;
    let startX = 0;
    let startY = 0;
    let pulling = false;
    let offset = 0;
    const setOffset = (value) => {
      offset = value;
      element.style.setProperty("--mobile-pull", `${value}px`);
      element.classList.toggle("pulling", value > 0);
    };
    const start = (event) => {
      if (busy.current || event.touches.length !== 1 || element.scrollTop > 0) return;
      startX = event.touches[0].clientX;
      startY = event.touches[0].clientY;
      pulling = true;
    };
    const move = (event) => {
      if (!pulling) return;
      const deltaY = event.touches[0].clientY - startY;
      const deltaX = event.touches[0].clientX - startX;
      if (deltaY <= 0 || Math.abs(deltaX) > Math.abs(deltaY)) {
        pulling = false;
        if (offset) setOffset(0);
        return;
      }
      // Claiming the gesture keeps the list still while the header is revealed.
      event.preventDefault();
      setOffset(pullOffset(deltaY));
    };
    const end = () => {
      if (!pulling) return;
      pulling = false;
      if (offset < PULL_THRESHOLD) {
        element.classList.remove("pulling");
        setOffset(0);
        return;
      }
      element.classList.remove("pulling");
      element.style.setProperty("--mobile-pull", `${PULL_THRESHOLD}px`);
      offset = PULL_THRESHOLD;
      busy.current = true;
      setRefreshing(true);
      void Promise.resolve(onRefresh?.()).finally(() => {
        busy.current = false;
        setRefreshing(false);
        setOffset(0);
      });
    };
    element.addEventListener("touchstart", start, { passive: true });
    element.addEventListener("touchmove", move, { passive: false });
    element.addEventListener("touchend", end);
    element.addEventListener("touchcancel", end);
    return () => {
      element.removeEventListener("touchstart", start);
      element.removeEventListener("touchmove", move);
      element.removeEventListener("touchend", end);
      element.removeEventListener("touchcancel", end);
      element.classList.remove("pulling");
      element.style.removeProperty("--mobile-pull");
    };
  }, [elementRef, enabled, onRefresh]);
  return refreshing;
}
