//go:build windows

package desktop

// probeRuntimeVersion deliberately avoids starting the runtime on Windows.
// Many CLI installations are .cmd/.bat shims and some of their descendants
// allocate a console even when the parent was created with CREATE_NO_WINDOW.
// Availability is already determined with exec.LookPath by the runner, so the
// only omitted information is the optional version label.
func probeRuntimeVersion(string) string { return "" }
