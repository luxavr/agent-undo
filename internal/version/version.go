// Package version is the CLI identity. Release builds override Version via:
//
//	-ldflags "-X github.com/luxavr/agent-undo/internal/version.Version=<tag>"
package version

// Version is the release identity. Untagged builds stay 0.0.0-dev.
var Version = "0.0.0-dev"
