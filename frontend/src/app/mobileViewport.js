import { useEffect } from "react";

// WKWebView keeps the layout viewport at full-screen size while the keyboard
// shrinks and can pan the visual viewport. A bottom inset alone only fixes the
// keyboard edge: once WebKit pans to the focused textarea, the navigation bar
// is still left above the visible screen. Position the whole app in the visual
// viewport so both edges stay attached to the pixels that are actually shown.
export function viewportFrameFrom(viewport, innerHeight) {
  const fallbackHeight = Number.isFinite(innerHeight) && innerHeight > 0 ? innerHeight : 0;
  const viewportHeight = Number(viewport?.height);
  const viewportTop = Number(viewport?.offsetTop);
  return {
    height: Math.max(0, Math.round(Number.isFinite(viewportHeight) && viewportHeight > 0 ? viewportHeight : fallbackHeight)),
    top: Math.max(0, Math.round(Number.isFinite(viewportTop) ? viewportTop : 0)),
  };
}

export function useMobileViewportFrame(elementRef) {
  useEffect(() => {
    const viewport = globalThis.visualViewport;
    const element = elementRef.current;
    if (!element) return undefined;
    let frame = 0;
    const apply = () => {
      frame = 0;
      const visible = viewportFrameFrom(viewport, window.innerHeight);
      element.style.setProperty("--mobile-viewport-top", `${visible.top}px`);
      element.style.setProperty("--mobile-viewport-height", `${visible.height}px`);
    };
    const sync = () => {
      if (frame) window.cancelAnimationFrame(frame);
      frame = window.requestAnimationFrame(apply);
    };
    sync();
    viewport?.addEventListener("resize", sync);
    viewport?.addEventListener("scroll", sync);
    window.addEventListener("resize", sync);
    return () => {
      if (frame) window.cancelAnimationFrame(frame);
      viewport?.removeEventListener("resize", sync);
      viewport?.removeEventListener("scroll", sync);
      window.removeEventListener("resize", sync);
    };
  }, [elementRef]);
}

// Streamed output should follow the run, but only while the reader is still at
// the bottom — yanking the view back while they scroll through an earlier tool
// call is worse than letting new text arrive off-screen.
export function isPinnedToBottom(element, threshold = 90) {
  if (!element) return true;
  return element.scrollHeight - element.scrollTop - element.clientHeight < threshold;
}
