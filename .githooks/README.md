# Git hooks

Enable the repository-managed hooks once after cloning:

```bash
go tool wails3 task hooks:install
```

The hooks run the following checks:

- `pre-commit`: staged whitespace errors and `gofmt` for staged Go files.
- `pre-push`: the complete Go test suite with `go test ./...`.

Git runs these scripts through its bundled POSIX shell, so the same hooks work
on macOS, Linux, and Git for Windows.
