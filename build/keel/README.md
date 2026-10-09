# Keel component and macOS callback backports

Current development uses `go.mod`'s local replacement at
`~/Code/go/src/github.com/dyike/keel`. The launcher detects this replacement and
runs Go directly against the checkout, so edits there are immediately available.
It does not apply the patches below or copy the local checkout. Normal `go test`
uses the same local dependency. Restart an existing watch process after changing
the replacement. The current Sidebar integration also requires the local
checkout's Suffix, ContextMenu and IconView APIs. The pinned-release backports
below are retained from the initial migration; removing the local replacement
alone is insufficient to build the current client against v0.1.8.

Keel v0.1.8 leaves its default keyboard focus stroke on the inner text-height
trigger when `Select.Appearance(false)` is used. The outer field padding also
falls outside that trigger, so clicks there do not open the menu. This affects
all shared Select instances, including project access, runtime settings and
workflow fields.

`go/select.patch` fixes the component itself: the trigger owns the padding and
control height; frameless keyboard focus uses the highlight fill; framed fields
keep the single outer focus border. Text, caret and hit area use the same row.
The normal 36dp height and `Size` scaling remain unchanged. Keel's existing menu,
keyboard, disabled-option and focus-restoration behavior are reused.

`go/titlebar.patch` fixes a native macOS double-click deadlock. AppKit invokes
the titlebar callback on its main thread; synchronous `core.Update` calls Gio's
`Invalidate`, which flushes events inline and re-enters its invalidation mutex.
Post the update from a Go goroutine so the native event callback can return.
The window action still executes under Keel's frame lock, respects closed
windows and preserves the user's macOS double-click preference.

The fixes are also applied in the local Keel checkout for upstream review. This
repository retains the initial patches without modifying the module cache.
Current builds use the requested local checkout. Keel's MIT license is included
beside the patch.

## Build and verify

Run from the OneCatch repository root (Go and Git are required):

```sh
go run ./build/keel/go run ./cmd/keel
go run ./build/keel/go test ./...
go run ./build/keel/go test -race ./internal/app/keel
go run ./build/keel/go vet ./...
go run ./build/keel/go tool keel build -debug -o bin/keel
```

The Go launcher makes a private copy of the pinned dependency under the ignored
`bin/keel-patches/` directory, applies the reviewed patches and uses a temporary
`-modfile` replacement for the child build command. Tests, gopls and Agent
subprocesses retain the original `GOFLAGS`; Keel watch-mode launches restore
those flags before any desktop process role or project tool starts. It preserves the project's
`go.mod`/`go.sum`, the module cache and unrelated edits in a local Keel checkout.
An incompatible version, existing module replacement or skipped patch stops the
build instead of silently producing an unpatched client. `Taskfile.yml` uses
this entry point for Keel development, binary builds and packaging.

Use `-debug` for packaging: Keel v0.1.8's release packager overwrites `GOFLAGS`
and would discard the temporary replacement. Direct binary builds can still
use `-trimpath`. No launcher or Git dependency is included in the desktop app.

Without a local replacement, normal `go test` checks the upstream dependency.
The current Sidebar APIs require the local replacement as described above. The project
padding-hit regression explicitly requires the patched launcher; it skips in
that baseline run. The patched checks cover the production dialog through Gio
input routing, including padding clicks, permission selection, reopening and
Esc without closing the parent dialog. Keel-side tests additionally cover
framed/plain controls at 32/36/48dp.

The Keel-side macOS regression additionally stalls the invalidation callback
and verifies that the native titlebar callback returns independently, then
drains the pending frame and checks the maximize action.

After the fix is available in a Keel release, update the pinned dependency,
remove this launcher/patch and return the Taskfile commands to the standard Go
entry points. Re-run the focused-dialog captures and native input tests.
