import { useEffect, useState } from "react";

// WKWebView never resizes the layout viewport when the software keyboard
// opens: 100dvh keeps reporting the full screen, so the bottom-anchored
// composer ends up underneath the keyboard with no way to reach the send
// button. The visual viewport is the only surface that shrinks, so the gap
// between it and the layout viewport is the inset the shell has to give back.
export function keyboardInsetFrom(viewport, innerHeight) {
  if (!viewport || !Number.isFinite(innerHeight)) return 0;
  const covered = innerHeight - (viewport.height + viewport.offsetTop);
  // Rubber-banding and sub-pixel rounding leave a pixel or two of noise behind;
  // only a real keyboard covers a meaningful slice of the screen.
  return covered > 24 ? Math.round(covered) : 0;
}

export function useKeyboardInset() {
  const [inset, setInset] = useState(0);
  useEffect(() => {
    const viewport = globalThis.visualViewport;
    if (!viewport) return undefined;
    const sync = () => setInset(keyboardInsetFrom(viewport, window.innerHeight));
    sync();
    viewport.addEventListener("resize", sync);
    viewport.addEventListener("scroll", sync);
    return () => {
      viewport.removeEventListener("resize", sync);
      viewport.removeEventListener("scroll", sync);
    };
  }, []);
  return inset;
}

// Streamed output should follow the run, but only while the reader is still at
// the bottom — yanking the view back while they scroll through an earlier tool
// call is worse than letting new text arrive off-screen.
export function isPinnedToBottom(element, threshold = 90) {
  if (!element) return true;
  return element.scrollHeight - element.scrollTop - element.clientHeight < threshold;
}
