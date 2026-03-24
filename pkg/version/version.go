package version

import "fmt"

var (
	// Version is the current version of the application
	// Can be injected at build time using -ldflags="-X github.com/limyedb/limyedb/pkg/version.Version=vX.Y.Z"
	Version = "0.1.0-dev"

	// Commit is the git commit hash
	Commit = "unknown"

	// BuildTime is the compilation timestamp
	BuildTime = "unknown"
)

// String returns a formatted version string
func String() string {
	return fmt.Sprintf("%s (commit: %s, build time: %s)", Version, Commit, BuildTime)
}
