// A pairing code is only worth showing while it still works, so the panel
// counts it down instead of leaving a dead code on screen.
export function pairingCountdown(expiresAt, now = Date.now()) {
  const remaining = new Date(expiresAt || 0).getTime() - now;
  if (!Number.isFinite(remaining) || remaining <= 0) return "";
  const seconds = Math.floor(remaining / 1000);
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`;
}

export const demoHostedWorker = {
  running: false,
  port: 9232,
  workerId: "desktop-demo",
  name: "Demo Mac",
  addresses: ["https://192.168.1.20:9232"],
  fingerprint: "",
};
