/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

// Package version exposes build-time metadata about the Ironbark binary.
//
// The values in this package are intended to be overridden at build time using
// `go build -ldflags`, e.g.:
//
//	go build -ldflags "-X brightsparklabs.com/ironbark/internal/version.Version=1.2.3" .
//
// When the binary is built without ldflags overrides (e.g. via `go run`) the
// default values defined here are used. This guarantees the package always
// returns a sensible, non-empty value.
package version

// -----------------------------------------------------------------------------
// PACKAGE VARIABLES (overridable via ldflags)
// -----------------------------------------------------------------------------

// Version is the Ironbark version string.
// Defaults to `dev` when not overridden via ldflags at build time.
var Version = "dev"

// Commit is the short Git commit hash the binary was built from.
// Defaults to `unknown` when not overridden via ldflags at build time.
var Commit = "unknown"

// BuildTime is the UTC ISO 8601 timestamp recording when the binary was built.
// Defaults to `unknown` when not overridden via ldflags at build time.
var BuildTime = "unknown"

// -----------------------------------------------------------------------------
// PUBLIC FUNCTIONS
// -----------------------------------------------------------------------------

// GetVersion returns the Ironbark version string, falling back to `dev` if
// the linker variable was somehow set to an empty string.
func GetVersion() string {
	if Version == "" {
		return "dev"
	}
	return Version
}

// GetCommit returns the short Git commit hash the binary was built from,
// falling back to `unknown` if the linker variable was somehow set to an
// empty string.
func GetCommit() string {
	if Commit == "" {
		return "unknown"
	}
	return Commit
}

// GetBuildTime returns the UTC ISO 8601 timestamp recording when the binary
// was built, falling back to `unknown` if the linker variable was somehow
// set to an empty string.
func GetBuildTime() string {
	if BuildTime == "" {
		return "unknown"
	}
	return BuildTime
}
