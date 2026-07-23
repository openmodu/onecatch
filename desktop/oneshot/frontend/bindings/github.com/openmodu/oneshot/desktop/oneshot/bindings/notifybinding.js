// @ts-check
// Hand-written to match the generated bindings. The project's wails3 CLI
// (alpha2.117) is a different version from the pinned module (alpha.97), so
// regenerating would churn every file and could change the numeric $Call.ByID
// hashes. Call.ByName is version-stable — it dispatches on the fully-qualified
// "package.Struct.Method" name the Go binding registers — so this one method is
// wired by hand instead.

// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore: Unused imports
import { Call as $Call } from "@wailsio/runtime";

const $$fqn = "github.com/openmodu/oneshot/desktop/oneshot/bindings.NotifyBinding.Send";

/**
 * Send a macOS system notification. No-ops on the Go side unless the app runs
 * from a signed .app bundle.
 * @param {string} title
 * @param {string} body
 * @returns {Promise<void>}
 */
export function Send(title, body) {
    return $Call.ByName($$fqn, title, body);
}
