// Application-wide channel names shared between the workbench and the detached
// settings/workflow windows.
//
// They live in their own module because a name is not a reason to load a window:
// importing them from AuxiliaryWindow.jsx pulled the whole settings and workflow
// screen into the workbench's first load for the sake of two strings.
export const settingsChangedEvent = "onecatch:settings-changed";
export const workflowsChangedEvent = "onecatch:workflows-changed";

// Retained windows are hidden on close rather than destroyed, so reopening one
// is a Show() with stale state on screen. Go announces the reveal here with
// {name} so the window can reconcile.
export const auxiliaryWindowShownEvent = "onecatch:aux-window-shown";

// A runtime status probe spawns a process per runtime, so ListRuntimes serves
// its cache and re-probes in the background. This is how the corrected list
// reaches windows that already rendered.
export const runtimesChangedEvent = "onecatch:runtimes-changed";
