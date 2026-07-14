/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

package settings

import (
	"strings"
	"testing"

	"log/slog"
)

// -----------------------------------------------------------------------------
// TEST HELPERS
// -----------------------------------------------------------------------------

// setEnv mutates the process environment for the duration of the test
// (auto-restored by `t.Setenv`) AND immediately re-resolves the
// settings snapshot so subsequent accessor calls observe the new
// value. Tests that mutate env via this helper do NOT need to call
// `Init()` themselves.
func setEnv(t *testing.T, name, value string) {
	t.Helper()
	t.Setenv(name, value)
	if err := Init(); err != nil {
		// Validation errors are not relevant to most env-mutation
		// tests (they exercise individual accessors). The dedicated
		// validator tests bypass this helper.
		_ = err
	}
}

// -----------------------------------------------------------------------------
// TESTS: catalogue invariants
// -----------------------------------------------------------------------------

// TestAll_returnsFreshSlice verifies that mutating the slice returned
// by `All` does not affect subsequent callers.
func TestAll_returnsFreshSlice(t *testing.T) {
	a := All()
	b := All()
	if &a[0] == &b[0] {
		t.Fatal("expected All to return a freshly allocated slice on each call")
	}

	a[0].Name = "MUTATED"
	if All()[0].Name == "MUTATED" {
		t.Fatal("expected mutating the returned slice to leave the catalogue untouched")
	}
}

// TestAll_namesAreUniqueAndIronbarkPrefixed verifies the catalogue
// invariant that every variable has a unique, well-formed name.
func TestAll_namesAreUniqueAndIronbarkPrefixed(t *testing.T) {
	seen := map[string]struct{}{}
	for _, v := range All() {
		if !strings.HasPrefix(v.Name, "IRONBARK_") {
			t.Errorf("catalogue entry %q does not start with IRONBARK_", v.Name)
		}
		if _, dup := seen[v.Name]; dup {
			t.Errorf("catalogue contains duplicate entry for %q", v.Name)
		}
		seen[v.Name] = struct{}{}
	}
}

// TestAll_descriptionsEndWithFullStop enforces the project-wide rule
// that every catalogue description must be a complete sentence ending
// with a full stop.
func TestAll_descriptionsEndWithFullStop(t *testing.T) {
	for _, v := range All() {
		if !strings.HasSuffix(v.Description, ".") {
			t.Errorf("catalogue entry %q has a description that does not end with a full stop: %q",
				v.Name, v.Description)
		}
	}
}

// TestAll_scopesAreKnown verifies that every catalogue entry has a
// recognised scope.
func TestAll_scopesAreKnown(t *testing.T) {
	known := map[Scope]struct{}{
		ScopeGoConsumed:        {},
		ScopeContainerSentinel: {},
		ScopeLauncherSentinel:  {},
		ScopeLauncherForwarded: {},
		ScopeScript:            {},
	}
	for _, v := range All() {
		if _, ok := known[v.Scope]; !ok {
			t.Errorf("catalogue entry %q has unknown scope %q", v.Name, v.Scope)
		}
	}
}

// TestAll_includesEveryGoConsumedAccessor verifies the catalogue stays
// in sync with the typed accessors exposed by this package: every
// accessor must have a corresponding entry of scope `ScopeGoConsumed`.
// New accessors added without a catalogue entry will fail this test.
func TestAll_includesEveryGoConsumedAccessor(t *testing.T) {
	expected := map[string]struct{}{
		"IRONBARK_DATA_DIR":              {},
		"IRONBARK_INTERNAL_PACKAGES_DIR": {},
		"IRONBARK_LOG_LEVEL":             {},
	}
	for _, v := range All() {
		if v.Scope != ScopeGoConsumed {
			continue
		}
		delete(expected, v.Name)
	}
	if len(expected) != 0 {
		t.Errorf("catalogue is missing go-consumed entries: %v", expected)
	}
}

// -----------------------------------------------------------------------------
// TESTS: typed accessors
// -----------------------------------------------------------------------------

// TestDataDir_withEnvSet_returnsEnvValue verifies the env var takes
// precedence over the declared default.
func TestDataDir_withEnvSet_returnsEnvValue(t *testing.T) {
	setEnv(t, "IRONBARK_DATA_DIR", "/custom/data")
	if got := DataDir(); got != "/custom/data" {
		t.Errorf("expected /custom/data, got %q", got)
	}
}

// TestDataDir_withEnvUnset_returnsDefault verifies the declared default
// is used when the env var is unset (or empty).
func TestDataDir_withEnvUnset_returnsDefault(t *testing.T) {
	setEnv(t, "IRONBARK_DATA_DIR", "")
	if got := DataDir(); got != "/tmp/ironbark/data" {
		t.Errorf("expected /tmp/ironbark/data, got %q", got)
	}
}

// TestArgoCDRepoDir_composesDataDirAndRepoName verifies that
// `ArgoCDRepoDir` joins the current `DataDir` with the
// `repos/<ArgoCDRepoName>` segments. Uses an env override to make the
// composition observable.
func TestArgoCDRepoDir_composesDataDirAndRepoName(t *testing.T) {
	setEnv(t, "IRONBARK_DATA_DIR", "/tmp/test-data")

	got := ArgoCDRepoDir()
	want := "/tmp/test-data/repos/ironbark-argocd-app-of-apps"
	if got != want {
		t.Errorf("ArgoCDRepoDir = %q, want %q", got, want)
	}
}

// TestInternalPackagesDir_honoursEnvAndDefault verifies the env var
// override and the declared default for `InternalPackagesDir`.
func TestInternalPackagesDir_honoursEnvAndDefault(t *testing.T) {
	setEnv(t, "IRONBARK_INTERNAL_PACKAGES_DIR", "/custom/pkgs")
	if got := InternalPackagesDir(); got != "/custom/pkgs" {
		t.Errorf("expected /custom/pkgs, got %q", got)
	}

	setEnv(t, "IRONBARK_INTERNAL_PACKAGES_DIR", "")
	if got := InternalPackagesDir(); got != "/tmp/ironbark/packages" {
		t.Errorf("expected default, got %q", got)
	}
}

// TestLogLevel_recognisedValues verifies that each recognised log-level
// string maps to the right `slog.Level`.
func TestLogLevel_recognisedValues(t *testing.T) {
	cases := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"DEBUG": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}
	for input, want := range cases {
		setEnv(t, "IRONBARK_LOG_LEVEL", input)
		if got := LogLevel(); got != want {
			t.Errorf("LogLevel(%q) = %v, want %v", input, got, want)
		}
	}
}

// TestLogLevel_unrecognisedDefaultsToInfo verifies that any
// unrecognised value (and the empty string) falls back to
// `slog.LevelInfo`.
func TestLogLevel_unrecognisedDefaultsToInfo(t *testing.T) {
	for _, input := range []string{"", "trace", "garbage"} {
		setEnv(t, "IRONBARK_LOG_LEVEL", input)
		if got := LogLevel(); got != slog.LevelInfo {
			t.Errorf("LogLevel(%q) = %v, want LevelInfo", input, got)
		}
	}
}

// TestInContainer_recognisesSentinel verifies that exactly the literal
// value `true` (case-sensitive, after trimming) is treated as "in
// container".
func TestInContainer_recognisesSentinel(t *testing.T) {
	setEnv(t, "IRONBARK_IN_CONTAINER", "true")
	if !InContainer() {
		t.Error("expected InContainer() to be true when IRONBARK_IN_CONTAINER=true")
	}

	for _, input := range []string{"", "1", "TRUE", "yes", "false"} {
		setEnv(t, "IRONBARK_IN_CONTAINER", input)
		if InContainer() {
			t.Errorf("expected InContainer() to be false for %q", input)
		}
	}
}

// -----------------------------------------------------------------------------
// TESTS: Resolve / ResolveAll
// -----------------------------------------------------------------------------

// TestResolve_withEnvSet_marksFromEnvTrue verifies that a non-empty env
// value is returned with `FromEnv=true`.
func TestResolve_withEnvSet_marksFromEnvTrue(t *testing.T) {
	setEnv(t, "IRONBARK_DATA_DIR", "/from/env")

	v := Var{Name: "IRONBARK_DATA_DIR", Default: "/from/default"}
	r := Resolve(v)

	if r.Value != "/from/env" || !r.FromEnv {
		t.Errorf("expected env value with FromEnv=true, got %+v", r)
	}
}

// TestResolve_withEnvUnset_marksFromEnvFalse verifies that an unset
// (or empty) env value falls back to the declared default and is
// marked `FromEnv=false`.
func TestResolve_withEnvUnset_marksFromEnvFalse(t *testing.T) {
	setEnv(t, "IRONBARK_DATA_DIR", "")

	v := Var{Name: "IRONBARK_DATA_DIR", Default: "/from/default"}
	r := Resolve(v)

	if r.Value != "/from/default" || r.FromEnv {
		t.Errorf("expected default value with FromEnv=false, got %+v", r)
	}
}

// -----------------------------------------------------------------------------
// TESTS: HostDataDir + DisplayPath
// -----------------------------------------------------------------------------

// TestHostDataDir_returnsEnvValueOrEmpty verifies that `HostDataDir`
// returns the env value verbatim when set, and the empty string when
// unset.
func TestHostDataDir_returnsEnvValueOrEmpty(t *testing.T) {
	setEnv(t, "IRONBARK_HOST_DATA_DIR", "/host/data")
	if got := HostDataDir(); got != "/host/data" {
		t.Errorf("HostDataDir = %q, want /host/data", got)
	}

	setEnv(t, "IRONBARK_HOST_DATA_DIR", "")
	if got := HostDataDir(); got != "" {
		t.Errorf("HostDataDir = %q, want empty", got)
	}
}

// TestDisplayPath_outsideContainer_returnsInputUnchanged verifies that
// rewriting only happens when running inside the container.
func TestDisplayPath_outsideContainer_returnsInputUnchanged(t *testing.T) {
	setEnv(t, "IRONBARK_IN_CONTAINER", "")
	setEnv(t, "IRONBARK_HOST_DATA_DIR", "/host/data")
	setEnv(t, "IRONBARK_DATA_DIR", "/mnt/data")

	if got := DisplayPath("/mnt/data/repos/foo"); got != "/mnt/data/repos/foo" {
		t.Errorf("expected unchanged path outside container, got %q", got)
	}
}

// TestDisplayPath_inContainerWithoutHostDataDir_returnsInputUnchanged
// verifies that rewriting is skipped when the launcher did not forward
// the host data dir.
func TestDisplayPath_inContainerWithoutHostDataDir_returnsInputUnchanged(t *testing.T) {
	setEnv(t, "IRONBARK_IN_CONTAINER", "true")
	setEnv(t, "IRONBARK_HOST_DATA_DIR", "")
	setEnv(t, "IRONBARK_DATA_DIR", "/mnt/data")

	if got := DisplayPath("/mnt/data/repos/foo"); got != "/mnt/data/repos/foo" {
		t.Errorf("expected unchanged path without host data dir, got %q", got)
	}
}

// TestDisplayPath_inContainerWithHostDataDir_rewritesPathsUnderDataDir
// verifies the happy path: paths under the in-container data dir are
// rewritten to their host-side equivalents, paths outside it are left
// untouched.
func TestDisplayPath_inContainerWithHostDataDir_rewritesPathsUnderDataDir(t *testing.T) {
	setEnv(t, "IRONBARK_IN_CONTAINER", "true")
	setEnv(t, "IRONBARK_HOST_DATA_DIR", "/opt/brightsparklabs/ironbark/data")
	setEnv(t, "IRONBARK_DATA_DIR", "/mnt/data")

	cases := map[string]string{
		// Bare data dir round-trips to the host data dir.
		"/mnt/data": "/opt/brightsparklabs/ironbark/data",
		// Nested paths get rewritten.
		"/mnt/data/repos/foo":              "/opt/brightsparklabs/ironbark/data/repos/foo",
		"/mnt/data/repos/foo/bar/baz.yaml": "/opt/brightsparklabs/ironbark/data/repos/foo/bar/baz.yaml",
		// Sibling paths are left alone (no false-positive prefix match).
		"/mnt/data-backup/foo": "/mnt/data-backup/foo",
		// Paths outside the data dir are left alone.
		"/etc/passwd": "/etc/passwd",
	}

	for input, want := range cases {
		if got := DisplayPath(input); got != want {
			t.Errorf("DisplayPath(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestDisplayPath_whenContainerAndHostDataDirsMatch_returnsInputUnchanged
// verifies the no-op case where the launcher mounts the data directory
// at the same path inside the container as on the host.
func TestDisplayPath_whenContainerAndHostDataDirsMatch_returnsInputUnchanged(t *testing.T) {
	setEnv(t, "IRONBARK_IN_CONTAINER", "true")
	setEnv(t, "IRONBARK_HOST_DATA_DIR", "/mnt/data")
	setEnv(t, "IRONBARK_DATA_DIR", "/mnt/data")

	if got := DisplayPath("/mnt/data/repos/foo"); got != "/mnt/data/repos/foo" {
		t.Errorf("expected unchanged path when host and container data dirs match, got %q", got)
	}
}

// -----------------------------------------------------------------------------
// TESTS: LauncherInvoked + ValidateLauncherEnv
// -----------------------------------------------------------------------------

// allLauncherForwardedNames returns every catalogue variable name with
// scope `ScopeLauncherForwarded`. Used by tests to populate or
// systematically clear the launcher environment without hard-coding
// the list (so adding a new forwarded var only requires a catalogue
// update, not a test update).
func allLauncherForwardedNames() []string {
	var out []string
	for _, v := range All() {
		if v.Scope == ScopeLauncherForwarded {
			out = append(out, v.Name)
		}
	}
	return out
}

// setAllLauncherForwarded sets every catalogue-declared
// launcher-forwarded variable to a non-empty placeholder via
// `t.Setenv` (which is automatically cleaned up at test end).
func setAllLauncherForwarded(t *testing.T) {
	t.Helper()
	for _, name := range allLauncherForwardedNames() {
		setEnv(t, name, "set")
	}
}

// TestLauncherInvoked_recognisesSentinel verifies that exactly the
// literal value `true` (after trimming) is recognised, and everything
// else (including unset, empty, alternative truthy strings) is not.
func TestLauncherInvoked_recognisesSentinel(t *testing.T) {
	setEnv(t, "IRONBARK_LAUNCHER_INVOKED", "true")
	if !LauncherInvoked() {
		t.Error("expected LauncherInvoked() to be true when set to `true`")
	}

	setEnv(t, "IRONBARK_LAUNCHER_INVOKED", "  true  ")
	if !LauncherInvoked() {
		t.Error("expected LauncherInvoked() to honour surrounding whitespace")
	}

	for _, input := range []string{"", "1", "TRUE", "yes", "false"} {
		setEnv(t, "IRONBARK_LAUNCHER_INVOKED", input)
		if LauncherInvoked() {
			t.Errorf("expected LauncherInvoked() to be false for %q", input)
		}
	}
}

// TestValidateLauncherEnv_withoutSentinel_returnsNil verifies the
// validator is a no-op when the launcher sentinel is not set, even if
// every host variable is missing.
func TestValidateLauncherEnv_withoutSentinel_returnsNil(t *testing.T) {
	setEnv(t, "IRONBARK_LAUNCHER_INVOKED", "")
	for _, name := range allLauncherForwardedNames() {
		setEnv(t, name, "")
	}

	if err := ValidateLauncherEnv(); err != nil {
		t.Errorf("expected nil when launcher sentinel is unset, got: %v", err)
	}
}

// TestValidateLauncherEnv_withSentinelAndAllVars_returnsNil verifies
// the happy path: sentinel set + every forwarded var populated.
func TestValidateLauncherEnv_withSentinelAndAllVars_returnsNil(t *testing.T) {
	setEnv(t, "IRONBARK_LAUNCHER_INVOKED", "true")
	setAllLauncherForwarded(t)

	if err := ValidateLauncherEnv(); err != nil {
		t.Errorf("expected nil when sentinel and all vars are set, got: %v", err)
	}
}

// TestValidateLauncherEnv_withSentinelAndMissingVars_returnsError
// verifies that every missing forwarded variable is reported in the
// returned error, and that the error type is `*LauncherEnvError`.
func TestValidateLauncherEnv_withSentinelAndMissingVars_returnsError(t *testing.T) {
	setEnv(t, "IRONBARK_LAUNCHER_INVOKED", "true")
	for _, name := range allLauncherForwardedNames() {
		setEnv(t, name, "")
	}

	err := ValidateLauncherEnv()
	if err == nil {
		t.Fatal("expected error when sentinel is set but all forwarded vars are empty, got nil")
	}

	envErr, ok := err.(*LauncherEnvError)
	if !ok {
		t.Fatalf("expected *LauncherEnvError, got %T", err)
	}

	expected := allLauncherForwardedNames()
	if len(envErr.Missing) != len(expected) {
		t.Errorf("expected %d missing entries, got %d (%v)",
			len(expected), len(envErr.Missing), envErr.Missing)
	}

	// Spot-check: every expected name appears at least once.
	missingSet := map[string]struct{}{}
	for _, name := range envErr.Missing {
		missingSet[name] = struct{}{}
	}
	for _, want := range expected {
		if _, ok := missingSet[want]; !ok {
			t.Errorf("expected missing list to contain %q, got %v", want, envErr.Missing)
		}
	}

	if !strings.Contains(err.Error(), "IRONBARK_LAUNCHER_INVOKED=true is set") {
		t.Errorf("expected error message to mention the sentinel, got: %s", err.Error())
	}
}

// TestValidateLauncherEnv_withSentinelAndOneMissingVar_reportsOnlyMissing
// verifies that only the truly missing variable appears in the error,
// not every forwarded variable.
func TestValidateLauncherEnv_withSentinelAndOneMissingVar_reportsOnlyMissing(t *testing.T) {
	setEnv(t, "IRONBARK_LAUNCHER_INVOKED", "true")
	setAllLauncherForwarded(t)
	setEnv(t, "IRONBARK_HOST_PWD", "")

	err := ValidateLauncherEnv()
	if err == nil {
		t.Fatal("expected error when one forwarded var is missing, got nil")
	}

	envErr, ok := err.(*LauncherEnvError)
	if !ok {
		t.Fatalf("expected *LauncherEnvError, got %T", err)
	}
	if len(envErr.Missing) != 1 || envErr.Missing[0] != "IRONBARK_HOST_PWD" {
		t.Errorf("expected exactly [IRONBARK_HOST_PWD], got %v", envErr.Missing)
	}
}

// TestLauncherEnvError_Error_listsAllMissingNames verifies the
// human-readable message lists every missing variable name.
func TestLauncherEnvError_Error_listsAllMissingNames(t *testing.T) {
	err := &LauncherEnvError{Missing: []string{"IRONBARK_HOST_USER", "IRONBARK_HOST_PWD"}}
	msg := err.Error()
	for _, want := range []string{"IRONBARK_HOST_USER", "IRONBARK_HOST_PWD"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected error message to contain %q, got: %s", want, msg)
		}
	}
}

// TestResolveAll_returnsCatalogueOrder verifies that `ResolveAll`
// returns one entry per catalogue var, in the same order.
func TestResolveAll_returnsCatalogueOrder(t *testing.T) {
	all := All()
	resolved := ResolveAll()
	if len(resolved) != len(all) {
		t.Fatalf("ResolveAll length = %d, want %d", len(resolved), len(all))
	}
	for i := range all {
		if resolved[i].Var.Name != all[i].Name {
			t.Errorf("ResolveAll[%d].Var.Name = %q, want %q", i, resolved[i].Var.Name, all[i].Name)
		}
	}
}
