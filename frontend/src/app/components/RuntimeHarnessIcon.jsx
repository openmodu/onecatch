import { Bot } from "lucide-react";

const runtimeHarnessIcons = {
  codex: "/assets/runtime/codex.svg",
  claude: "/assets/runtime/claude-code.svg",
  modu: "/assets/runtime/modu-code.png",
};

export default function RuntimeHarnessIcon({ harness, size = 14, className = "", ...props }) {
  const src = runtimeHarnessIcons[harness];
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
