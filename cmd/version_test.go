/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"brightsparklabs.com/ironbark/internal/version"
)

// -----------------------------------------------------------------------------
// HELPERS
// -----------------------------------------------------------------------------

// withVersionVars temporarily overrides the build-time version variables for
// the duration of the supplied function. Original values are restored on
// return regardless of test outcome.
func withVersionVars(t *testing.T, ver, commit, buildTime string, fn func()) {
	t.Helper()

	originalVersion := version.Version
	originalCommit := version.Commit
	originalBuildTime := version.BuildTime
	defer func() {
		version.Version = originalVersion
		version.Commit = originalCommit
		version.BuildTime = originalBuildTime
	}()

	version.Version = ver
	version.Commit = commit
	version.BuildTime = buildTime

	fn()
}

// -----------------------------------------------------------------------------
// TESTS: execVersion - single-field modes
// -----------------------------------------------------------------------------

// TestExecVersion_withShortFlag_printsOnlyVersion verifies that `--short`
// emits just the version string followed by a single newline.
func TestExecVersion_withShortFlag_printsOnlyVersion(t *testing.T) {
	withVersionVars(t, "9.9.9", "deadbeef", "2026-01-01T00:00:00Z", func() {
		var out bytes.Buffer
		if err := execVersion(&out, versionOptions{short: true}); err != nil {
			t.Fatalf("execVersion returned unexpected error: %v", err)
		}
		got := out.String()
		want := "9.9.9\n"
		if got != want {
			t.Fatalf("execVersion(--short) = %q, want %q", got, want)
		}
	})
}

// TestExecVersion_withCommitFlag_printsOnlyCommit verifies that `--commit`
// emits just the commit hash.
func TestExecVersion_withCommitFlag_printsOnlyCommit(t *testing.T) {
	withVersionVars(t, "9.9.9", "deadbeef", "2026-01-01T00:00:00Z", func() {
		var out bytes.Buffer
		if err := execVersion(&out, versionOptions{commit: true}); err != nil {
			t.Fatalf("execVersion returned unexpected error: %v", err)
		}
		got := out.String()
		want := "deadbeef\n"
		if got != want {
			t.Fatalf("execVersion(--commit) = %q, want %q", got, want)
		}
	})
}

// TestExecVersion_withBuildTimeFlag_printsOnlyBuildTime verifies that
// `--build-time` emits just the build timestamp.
func TestExecVersion_withBuildTimeFlag_printsOnlyBuildTime(t *testing.T) {
	withVersionVars(t, "9.9.9", "deadbeef", "2026-01-01T00:00:00Z", func() {
		var out bytes.Buffer
		if err := execVersion(&out, versionOptions{buildTime: true}); err != nil {
			t.Fatalf("execVersion returned unexpected error: %v", err)
		}
		got := out.String()
		want := "2026-01-01T00:00:00Z\n"
		if got != want {
			t.Fatalf("execVersion(--build-time) = %q, want %q", got, want)
		}
	})
}

// -----------------------------------------------------------------------------
// TESTS: execVersion - JSON mode
// -----------------------------------------------------------------------------

// TestExecVersion_withJSONFlag_printsValidJSONWithAllFields verifies that
// `--json` emits a JSON document containing all three fields with the
// expected values.
func TestExecVersion_withJSONFlag_printsValidJSONWithAllFields(t *testing.T) {
	withVersionVars(t, "1.2.3", "abc1234", "2026-05-07T03:00:00Z", func() {
		var out bytes.Buffer
		if err := execVersion(&out, versionOptions{asJSON: true}); err != nil {
			t.Fatalf("execVersion returned unexpected error: %v", err)
		}

		var got versionInfo
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("execVersion(--json) produced invalid JSON %q: %v", out.String(), err)
		}

		want := versionInfo{
			Version:   "1.2.3",
			Commit:    "abc1234",
			BuildTime: "2026-05-07T03:00:00Z",
		}
		if got != want {
			t.Fatalf("execVersion(--json) decoded to %+v, want %+v", got, want)
		}
	})
}

// -----------------------------------------------------------------------------
// TESTS: execVersion - default (multi-line) mode
// -----------------------------------------------------------------------------

// TestExecVersion_withNoFlags_printsAllFieldsHumanReadable verifies that
// the default mode prints all three fields in a multi-line human-readable
// format.
func TestExecVersion_withNoFlags_printsAllFieldsHumanReadable(t *testing.T) {
	withVersionVars(t, "1.2.3", "abc1234", "2026-05-07T03:00:00Z", func() {
		var out bytes.Buffer
		if err := execVersion(&out, versionOptions{}); err != nil {
			t.Fatalf("execVersion returned unexpected error: %v", err)
		}

		got := out.String()
		expectedSubstrings := []string{
			"ironbark",
			"version:    1.2.3",
			"commit:     abc1234",
			"build time: 2026-05-07T03:00:00Z",
		}
		for _, want := range expectedSubstrings {
			if !strings.Contains(got, want) {
				t.Errorf("execVersion default output missing %q\nfull output:\n%s", want, got)
			}
		}
	})
}

// -----------------------------------------------------------------------------
// TESTS: execVersion - fallback values
// -----------------------------------------------------------------------------

// TestExecVersion_withEmptyVersionVars_usesFallbackValues verifies that the
// fallback values defined in the `internal/version` package are surfaced
// when the linker variables are blank.
func TestExecVersion_withEmptyVersionVars_usesFallbackValues(t *testing.T) {
	withVersionVars(t, "", "", "", func() {
		var out bytes.Buffer
		if err := execVersion(&out, versionOptions{asJSON: true}); err != nil {
			t.Fatalf("execVersion returned unexpected error: %v", err)
		}

		var got versionInfo
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("execVersion(--json) produced invalid JSON: %v", err)
		}

		want := versionInfo{
			Version:   "dev",
			Commit:    "unknown",
			BuildTime: "unknown",
		}
		if got != want {
			t.Fatalf("execVersion(--json) with empty vars = %+v, want %+v", got, want)
		}
	})
}

// -----------------------------------------------------------------------------
// TESTS: newVersionCmd - mutually exclusive flag enforcement
// -----------------------------------------------------------------------------

// TestNewVersionCmd_withConflictingFlags_returnsError verifies that Cobra's
// mutual exclusion guard rejects combining single-field selection flags.
func TestNewVersionCmd_withConflictingFlags_returnsError(t *testing.T) {
	cmd := newVersionCmd()
	cmd.SetArgs([]string{"--short", "--commit"})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error when combining --short and --commit, got nil")
	}
}
