import { useEffect } from "react";
import { Window } from "@wailsio/runtime";

// The iOS view controller insets its WKWebView by the safe area, so the page
// never paints the strips behind the status bar and the home indicator — the
// native window colour does. That colour is fixed at launch (see
// internal/app/mobile/mobile.go), which left a cream band above and below a
// dark app. Push the page's own resolved background down to the native chrome
// instead of guessing it in Go, so the strips follow the theme.
export function parseRGB(value) {
  const match = /^rgba?\(\s*(\d+)[,\s]+(\d+)[,\s]+(\d+)/.exec(String(value || ""));
  if (!match) return null;
  return { red: Number(match[1]), green: Number(match[2]), blue: Number(match[3]) };
}

export async function syncNativeChrome(target = Window) {
  // The shell owns the real surface colour; the body only carries the boot
  // placeholder painted before React mounts.
  const surface = document.querySelector(".mobile-app-shell") || document.body;
  const colour = parseRGB(getComputedStyle(surface).backgroundColor);
  if (!colour) return false;
  try {
    await target.SetBackgroundColour(colour.red, colour.green, colour.blue, 255);
    return true;
  } catch {
    // Running outside the native shell (or against an older runtime): the page
    // still renders correctly, only the strips keep their launch colour.
    return false;
  }
}

export function useNativeChrome() {
  useEffect(() => {
    // The theme attribute lands on <html> a frame before the new background is
    // painted, so read the colour back after the browser has applied it.
    const sync = () => requestAnimationFrame(() => { void syncNativeChrome(); });
    sync();
    // The mobile client has no theme picker of its own; it follows the phone.
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    media.addEventListener("change", sync);
    return () => media.removeEventListener("change", sync);
  }, []);
}
