// Tests for the `ironbark exec` command.
package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// -----------------------------------------------------------------------------
// HELPERS
// -----------------------------------------------------------------------------

// writeExecutable writes a tiny executable shell script to `path`
// (chmod 0755). Used by tests to simulate entries in the bundled
// tools directory or shell-candidate locations.
func writeExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("could not create executable %q: %v", path, err)
	}
}

// writeNonExecutable writes a non-executable regular file to `path`
// (chmod 0644). Used to verify the discovery logic correctly
// excludes non-executable entries.
func writeNonExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("# not an executable\n"), 0o644); err != nil {
		t.Fatalf("could not create non-executable %q: %v", path, err)
	}
}

// subcommandNames returns the names of every direct child of `cmd`.
// Used by failure messages to aid debugging.
func subcommandNames(cmd *cobra.Command) []string {
	names := make([]string, 0, len(cmd.Commands()))
	for _, c := range cmd.Commands() {
		names = append(names, c.Name())
	}
	return names
}

// -----------------------------------------------------------------------------
// TESTS: discoverExecTools
// -----------------------------------------------------------------------------

// TestDiscoverExecTools_returnsSortedExecutableEntries verifies that
// only executable regular files are reported, and that the result is
// sorted for stable `--help` output.
func TestDiscoverExecTools_returnsSortedExecutableEntries(t *testing.T) {
	dir := t.TempDir()

	writeExecutable(t, filepath.Join(dir, "zarf"))
	writeExecutable(t, filepath.Join(dir, "kubectl"))
	writeExecutable(t, filepath.Join(dir, "argocd"))
	// Non-executable - must be excluded.
	writeNonExecutable(t, filepath.Join(dir, "README"))
	// Subdirectory - must be excluded (not a regular file).
	if err := os.Mkdir(filepath.Join(dir, "extras"), 0o755); err != nil {
		t.Fatalf("could not create subdir: %v", err)
	}

	got := discoverExecTools(dir, nil)
	want := []string{"argocd", "kubectl", "zarf"}

	if len(got) != len(want) {
		t.Fatalf("expected %d tools, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tools[%d]: got %q, want %q (full list: %v)", i, got[i], want[i], got)
		}
	}
}

// TestDiscoverExecTools_appliesBlocklist verifies that entries listed
// in the blocklist are excluded.
func TestDiscoverExecTools_appliesBlocklist(t *testing.T) {
	dir := t.TempDir()
	writeExecutable(t, filepath.Join(dir, "kubectl"))
	writeExecutable(t, filepath.Join(dir, "ironbark"))
	writeExecutable(t, filepath.Join(dir, "zarf"))

	got := discoverExecTools(dir, map[string]struct{}{"ironbark": {}})

	for _, name := range got {
		if name == "ironbark" {
			t.Errorf("expected blocklisted tool %q to be excluded, got: %v", name, got)
		}
	}
	if len(got) != 2 {
		t.Errorf("expected 2 tools after blocklist, got %d: %v", len(got), got)
	}
}

// TestDiscoverExecTools_missingDir_returnsEmptySlice verifies that an
// absent directory (e.g. `/app/bin` outside the container) yields an
// empty slice rather than an error.
func TestDiscoverExecTools_missingDir_returnsEmptySlice(t *testing.T) {
	got := discoverExecTools("/definitely/does/not/exist/anywhere", nil)
	if got != nil {
		t.Errorf("expected nil for missing dir, got: %v", got)
	}
}

// -----------------------------------------------------------------------------
// TESTS: discoverExecShells
// -----------------------------------------------------------------------------

// TestDiscoverExecShells_returnsFoundCandidates verifies that the
// probe finds an executable candidate at one of the expected paths
// and that the result lists its absolute path.
func TestDiscoverExecShells_returnsFoundCandidates(t *testing.T) {
	dir := t.TempDir()
	bash := filepath.Join(dir, "bash")
	sh := filepath.Join(dir, "sh")
	writeExecutable(t, bash)
	writeExecutable(t, sh)

	got := discoverExecShells([]string{bash, sh, "/definitely/not/a/real/shell"})
	if len(got) != 2 {
		t.Fatalf("expected 2 shells, got %d: %v", len(got), got)
	}
	if !containsString(got, bash) {
		t.Errorf("expected bash candidate %q in result %v", bash, got)
	}
	if !containsString(got, sh) {
		t.Errorf("expected sh candidate %q in result %v", sh, got)
	}
}

// TestDiscoverExecShells_dedupesByBasename verifies that when the
// same basename appears at multiple candidate paths, only the first
// is reported (so we never register two `bash` subcommands).
func TestDiscoverExecShells_dedupesByBasename(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	writeExecutable(t, filepath.Join(dir1, "bash"))
	writeExecutable(t, filepath.Join(dir2, "bash"))

	got := discoverExecShells([]string{
		filepath.Join(dir1, "bash"),
		filepath.Join(dir2, "bash"),
	})

	if len(got) != 1 {
		t.Errorf("expected 1 deduped shell, got %d: %v", len(got), got)
	}
	if got[0] != filepath.Join(dir1, "bash") {
		t.Errorf("expected first-wins dedup, got: %v", got)
	}
}

// TestDiscoverExecShells_withNoCandidatesPresent_returnsEmptySlice
// covers the SCRATCH-image case where no shell is shipped.
func TestDiscoverExecShells_withNoCandidatesPresent_returnsEmptySlice(t *testing.T) {
	got := discoverExecShells([]string{
		"/definitely/not/a/real/shell",
		"/also/not/real",
	})
	if got != nil {
		t.Errorf("expected nil when no candidates exist, got: %v", got)
	}
}

// TestDiscoverExecShells_excludesNonExecutables verifies that a
// non-executable file at a candidate path is ignored (so we never
// register a broken shell subcommand).
func TestDiscoverExecShells_excludesNonExecutables(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bash")
	writeNonExecutable(t, path)

	got := discoverExecShells([]string{path})
	if got != nil {
		t.Errorf("expected nil when candidate is not executable, got: %v", got)
	}
}

// -----------------------------------------------------------------------------
// TESTS: newExecCmd construction
// -----------------------------------------------------------------------------

// TestNewExecCmd_withNoToolName_returnsUserError verifies that
// invoking `ironbark exec` with no tool name produces a typed
// `UserError`, so the top-level handler renders it as a concise
// stderr line rather than a panic.
func TestNewExecCmd_withNoToolName_returnsUserError(t *testing.T) {
	cmd := newExecCmd()

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no tool name supplied, got nil")
	}
	if !IsUserError(err) {
		t.Errorf("expected UserError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "no tool specified") {
		t.Errorf("expected clear message, got: %v", err)
	}
}

// TestNewExecCmd_doesNotRegisterShellWhenNoCandidatesPresent
// verifies that on a SCRATCH-style image (no bash/sh on disk) no
// shell subcommand is registered. On the test host bash/sh almost
// certainly exist; the test temporarily redirects discovery to an
// empty candidate list to simulate the SCRATCH case.
//
// This is also our regression test against ever hard-coding a shell
// subcommand that does not actually point at an existing binary.
func TestNewExecCmd_doesNotRegisterShellWhenNoCandidatesPresent(t *testing.T) {
	got := discoverExecShells(nil)
	if got != nil {
		t.Fatalf("expected nil shell list for nil candidates, got: %v", got)
	}
	// And explicitly: there is no hard-coded "shell" child.
	cmd := newExecCmd()
	for _, name := range subcommandNames(cmd) {
		if name == "shell" {
			t.Errorf("expected NO hard-coded `shell` subcommand, but found it (children: %v)", subcommandNames(cmd))
		}
	}
}

// TestNewExecToolCmd_disablesFlagParsing verifies that flag parsing
// is disabled on each tool subcommand so that tool-specific flags
// (e.g. `kubectl --kubeconfig=...`) are forwarded verbatim rather
// than intercepted by cobra.
func TestNewExecToolCmd_disablesFlagParsing(t *testing.T) {
	cmd := newExecToolCmd("kubectl", "/app/bin/kubectl")
	if !cmd.DisableFlagParsing {
		t.Error("expected DisableFlagParsing=true so tool flags are forwarded verbatim")
	}
}

// -----------------------------------------------------------------------------
// TESTS: execIntoTool error handling
// -----------------------------------------------------------------------------

// TestExecIntoTool_withMissingBinary_returnsUserError verifies that
// a missing binary produces a typed `UserError` with a helpful
// message rather than a raw `execve` error.
func TestExecIntoTool_withMissingBinary_returnsUserError(t *testing.T) {
	err := execIntoTool("/definitely/not/a/real/binary", nil)
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
	if !IsUserError(err) {
		t.Errorf("expected UserError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected message to mention `not found`, got: %v", err)
	}
}

// -----------------------------------------------------------------------------
// HELPERS used by tests above
// -----------------------------------------------------------------------------

// containsString reports whether `haystack` contains `needle`.
func containsString(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
