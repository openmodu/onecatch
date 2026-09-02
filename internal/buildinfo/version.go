// Package buildinfo exposes metadata injected into OneCatch binaries at build time.
package buildinfo

// Version is replaced from the latest CHANGELOG.md release through -ldflags.
// Keeping a development fallback makes direct `go run` and `go test` builds
// identifiable without introducing a second version source.
var Version = "dev"

// UpdatePublicKey is the base64-encoded raw Ed25519 public key pinned into
// release builds. The release feed may provide signatures, but it cannot
// replace this trust root.
var UpdatePublicKey = ""

// UpdateFeedURL is stable while the artifacts behind GitHub's latest-release
// redirect change. Keeping it injectable also makes staged rollout testing
// possible without changing application code.
var UpdateFeedURL = "https://github.com/openmodu/onecatch/releases/latest/download"
