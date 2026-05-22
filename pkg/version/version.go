// Package version exposes the evva SDK's release identity. Downstream
// apps can log Version on startup or assert against it when their
// integration depends on a specific evva surface.
//
// The Version constant is the source of truth for the release.
// BuildStamp is empty by default; release builds can set it via
// `-ldflags "-X github.com/johnny1110/evva/pkg/version.BuildStamp=..."`
// to capture a commit hash, build timestamp, or CI run id without
// touching tracked source.
package version

import "fmt"

// Version is the SDK release identifier. Bumped on every tagged
// release. Pre-1.0 versions carry the `-alpha.N` / `-beta.N` suffix;
// once Phase 19f completes the surface promise, this drops to a clean
// semver string.
const Version = "0.3.0-alpha.1"

// BuildStamp is an optional build-identifying string populated at link
// time via -ldflags. Empty for `go build` / `go run` invocations off
// the source tree; tagged release binaries carry the commit short hash
// + build date.
var BuildStamp = ""

// String returns "vX.Y.Z" (Phase 19f and after) or "vX.Y.Z-<stamp>"
// when a build stamp is present. Suitable for status bars, log lines,
// and "--version" CLI output.
func String() string {
	if BuildStamp == "" {
		return "v" + Version
	}
	return fmt.Sprintf("v%s+%s", Version, BuildStamp)
}
