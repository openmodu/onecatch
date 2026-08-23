export const REMOTE_FS_HEALTH_INTERVAL_MS = 5 * 60 * 1000;

// Failed targets stay quiet until the user explicitly retries them. Healthy
// and not-yet-checked targets participate in the five-minute heartbeat.
export function shouldAutoCheckRemoteFS(status) {
  return !status || (status.healthy === true && !status.checking);
}
