/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package version

import "testing"

// -----------------------------------------------------------------------------
// TESTS: GetVersion
// -----------------------------------------------------------------------------

// TestGetVersion_withDefaultValue_returnsDev verifies that the default value
// of `Version` is reported as `dev` when no ldflags override has been
// applied.
func TestGetVersion_withDefaultValue_returnsDev(t *testing.T) {
	original := Version
	defer func() { Version = original }()

	Version = "dev"
	got := GetVersion()
	if got != "dev" {
		t.Fatalf("GetVersion() = %q, want %q", got, "dev")
	}
}

// TestGetVersion_withInjectedValue_returnsInjectedValue verifies that an
// ldflags-style override is reflected by `GetVersion`.
func TestGetVersion_withInjectedValue_returnsInjectedValue(t *testing.T) {
	original := Version
	defer func() { Version = original }()

	Version = "1.2.3"
	got := GetVersion()
	if got != "1.2.3" {
		t.Fatalf("GetVersion() = %q, want %q", got, "1.2.3")
	}
}

// TestGetVersion_withEmptyValue_returnsDev verifies that an empty value
// (which can occur if ldflags is set to an empty string) falls back to
// `dev` to ensure a non-empty result.
func TestGetVersion_withEmptyValue_returnsDev(t *testing.T) {
	original := Version
	defer func() { Version = original }()

	Version = ""
	got := GetVersion()
	if got != "dev" {
		t.Fatalf("GetVersion() = %q, want %q", got, "dev")
	}
}

// -----------------------------------------------------------------------------
// TESTS: GetCommit
// -----------------------------------------------------------------------------

// TestGetCommit_withInjectedValue_returnsInjectedValue verifies that an
// ldflags-style override is reflected by `GetCommit`.
func TestGetCommit_withInjectedValue_returnsInjectedValue(t *testing.T) {
	original := Commit
	defer func() { Commit = original }()

	Commit = "abc1234"
	got := GetCommit()
	if got != "abc1234" {
		t.Fatalf("GetCommit() = %q, want %q", got, "abc1234")
	}
}

// TestGetCommit_withEmptyValue_returnsUnknown verifies that an empty value
// falls back to `unknown`.
func TestGetCommit_withEmptyValue_returnsUnknown(t *testing.T) {
	original := Commit
	defer func() { Commit = original }()

	Commit = ""
	got := GetCommit()
	if got != "unknown" {
		t.Fatalf("GetCommit() = %q, want %q", got, "unknown")
	}
}

// -----------------------------------------------------------------------------
// TESTS: GetBuildTime
// -----------------------------------------------------------------------------

// TestGetBuildTime_withInjectedValue_returnsInjectedValue verifies that an
// ldflags-style override is reflected by `GetBuildTime`.
func TestGetBuildTime_withInjectedValue_returnsInjectedValue(t *testing.T) {
	original := BuildTime
	defer func() { BuildTime = original }()

	BuildTime = "2026-05-07T03:00:00Z"
	got := GetBuildTime()
	if got != "2026-05-07T03:00:00Z" {
		t.Fatalf("GetBuildTime() = %q, want %q", got, "2026-05-07T03:00:00Z")
	}
}

// TestGetBuildTime_withEmptyValue_returnsUnknown verifies that an empty value
// falls back to `unknown`.
func TestGetBuildTime_withEmptyValue_returnsUnknown(t *testing.T) {
	original := BuildTime
	defer func() { BuildTime = original }()

	BuildTime = ""
	got := GetBuildTime()
	if got != "unknown" {
		t.Fatalf("GetBuildTime() = %q, want %q", got, "unknown")
	}
}
