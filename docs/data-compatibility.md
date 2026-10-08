# Local data compatibility

Releases share `~/.onecatch`. Adding a harness or optional field must not prevent
an earlier compatible reader from opening the application, and saving through
that reader must not discard data it does not understand.

## Settings contract

- Additive fields and new harnesses stay in the current settings schema. New
  readers fill missing defaults without changing explicit `false` values.
- The repository exposes only installed harnesses to validation, UI and runtime
  execution. Unknown harness entries remain opaque JSON on disk.
- Saving merges the fields owned by the current reader into the original file.
  Unknown top-level fields, nested options and harness entries survive, including
  exact JSON numbers. Clearing a known optional field must still clear it.
- Reading alone does not rewrite the file. Existing validation, atomic writes
  and revision conflict checks remain in force.
- Do not rename/remove fields, change their types, or introduce enum values that
  older readers reject within a compatible schema. Add a separate optional field
  or implement a tested compatibility reader first.
- A schema version above the reader's supported version is still rejected without
  modifying the file. Arbitrary incompatible formats are not safe to interpret.
  Before shipping a breaking schema change, implement a recoverable backup,
  migration tests and an explicit downgrade/export path; a version bump alone is
  not a migration strategy.

The compatibility reader does not retrofit already shipped binaries. In
particular, the original v0.3.6 rejects `runtimes.trae`; it needs a patched build
or a backed-up configuration without that entry. Do not advertise unrestricted
rollback to that binary.

## Release checks

For changes to persistent settings, run:

```sh
go test ./internal/repo/settings ./internal/domain/settings ./internal/service/desktop
```

The regression tests cover newer data read and saved through an older reader,
unknown harnesses and nested fields, cleared optional values, exact large
numbers, old-schema defaults, and refusing incompatible writes. Add a fixture
whenever the persisted vocabulary changes.

For other persistent records (tasks, workflows, runs and events), apply the same
rules and add round-trip tests in the owning repository before changing their
schema. Settings compatibility does not establish compatibility for those
formats. Derived caches may be discarded and regenerated; user records may not.
