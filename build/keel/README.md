# Native desktop builds

Keel v0.1.11 and its Gio/typesetting forks are pinned to remote modules in `go.mod`. No local checkout or backport patches are used.

- Run: `go run ./cmd/keel`
- Compact executable: `go build -trimpath -ldflags='-s -w' -o bin/onecatch ./cmd/keel`
- Release package: `node build/scripts/package-keel.mjs`
- Verify: `go test ./...`

`appicon.png` is the source icon used by the native packager. `keel.json` defines the application identity. The packaging script temporarily injects release metadata from CHANGELOG and restores the source config after building.
