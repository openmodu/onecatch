import { Bot } from "lucide-react";

// Runtime marks are looked up by harness id rather than through a map, so
// adding a harness means dropping in one asset instead of editing this file.
// The two raster and legacy-named marks keep an explicit entry.
const runtimeHarnessIconOverrides = {
  claude: "/assets/runtime/claude-code.svg",
  modu: "/assets/runtime/modu-code.png",
  dsh: "/assets/runtime/deepseek.svg",
};

// Harnesses that ship a mark. A harness absent here renders the generic icon
// rather than requesting an asset that does not exist.
const runtimeHarnessesWithIcons = new Set(["codex", "claude", "modu", "pi", "grok", "dsh"]);

export function runtimeHarnessIcon(harness) {
  if (!runtimeHarnessesWithIcons.has(harness)) return "";
  return runtimeHarnessIconOverrides[harness] || `/assets/runtime/${harness}.svg`;
}

export default function RuntimeHarnessIcon({ harness, size = 14, className = "", ...props }) {
  const src = runtimeHarnessIcon(harness);
  if (!src) return <Bot size={size} className={className} {...props} />;

  return <img
    src={src}
    alt=""
    width={size}
    height={size}
    className={`runtime-harness-icon runtime-harness-icon-${harness} ${className}`.trim()}
    {...props}
  />;
}
