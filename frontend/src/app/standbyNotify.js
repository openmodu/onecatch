// Fires a macOS system notification for a standby edge (work done / needs you).
// Best-effort by design: the Wails notification service only works from a
// signed .app bundle, so in a raw dev binary or the browser preview the call is
// absent or errors — we swallow that quietly rather than surface a failure the
// user can do nothing about. The on-screen standby state is the reliable
// channel; the notification is the bonus that reaches you behind another app.

let bindingPromise;

// The generated binding is only present in a real Wails build. Import it lazily
// and tolerate its absence (browser preview, older backend without the service).
async function loadBinding() {
  if (!bindingPromise) {
    bindingPromise = import("../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js")
      .then((module) => module.NotifyBinding || null)
      .catch(() => null);
  }
  return bindingPromise;
}

export async function notifyStandby(title, body) {
  try {
    const binding = await loadBinding();
    if (binding?.Send) await binding.Send(title, body);
  } catch {
    // A missing bundle identifier, denied permission, or a preview build all
    // land here; the standby screen already reflects the change.
  }
}
