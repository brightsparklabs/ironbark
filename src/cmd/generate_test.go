/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// -----------------------------------------------------------------------------
// HELPERS
// -----------------------------------------------------------------------------

// renderLauncher renders the launcher template with the supplied data and
// returns the rendered output. Test failures are reported via `t.Fatalf` so
// individual tests can focus on assertions about the output.
func renderLauncher(t *testing.T, data launcherTemplateData) string {
	t.Helper()

	var out bytes.Buffer
	if err := execLauncher(&out, data); err != nil {
		t.Fatalf("execLauncher returned unexpected error: %v", err)
	}

	return out.String()
}

// validLauncherData returns a `launcherTemplateData` populated with sensible
// defaults that pass engine validation. Tests can override individual fields
// via struct literal modification.
func validLauncherData() launcherTemplateData {
	return launcherTemplateData{
		Engine:         "podman",
		Image:          "brightsparklabs/ironbark:latest",
		HostDataDir:    "/opt/brightsparklabs/ironbark/data",
		HostKubeconfig: "${HOME}/.kube/config",
	}
}

// renderZarfBootstrap renders the Zarf installer template with the supplied
// data and returns the rendered output. It transparently satisfies the
// in-container and asset-existence preconditions so individual tests can
// focus on assertions about the rendered output.
func renderZarfBootstrap(t *testing.T, data zarfBootstrapTemplateData) string {
	t.Helper()

	restore := withZarfBootstrapPreconditionsMet(t)
	defer restore()

	var out bytes.Buffer
	if err := execZarfBootstrap(&out, data); err != nil {
		t.Fatalf("execZarfBootstrap returned unexpected error: %v", err)
	}

	return out.String()
}

// withZarfBootstrapPreconditionsMet swaps the package-level seam used by
// `execZarfBootstrap` so the asset-existence check passes. Returns a
// restore function which the caller must invoke (e.g. via `defer`) to
// put the original back.
func withZarfBootstrapPreconditionsMet(t *testing.T) func() {
	t.Helper()

	origAssets := zarfBootstrapAssetsPresent
	zarfBootstrapAssetsPresent = func(_ string) error { return nil }

	return func() {
		zarfBootstrapAssetsPresent = origAssets
	}
}

// validZarfBootstrapData returns a `zarfBootstrapTemplateData` populated
// with sensible defaults that pass validation. Tests can override
// individual fields via struct literal modification.
func validZarfBootstrapData() zarfBootstrapTemplateData {
	return zarfBootstrapTemplateData{
		Engine:     "podman",
		Image:      "brightsparklabs/ironbark:latest",
		SourcePath: "/app/resources/zarf/init",
		OutputDir:  "/opt/brightsparklabs/ironbark/data/zarf/init",
	}
}

// renderFapolicyRules renders the fapolicyd rules template with the
// supplied data and returns the rendered output.
func renderFapolicyRules(t *testing.T, data fapolicyRulesTemplateData) string {
	t.Helper()

	var out bytes.Buffer
	if err := execFapolicyRules(&out, data); err != nil {
		t.Fatalf("execFapolicyRules returned unexpected error: %v", err)
	}

	return out.String()
}

// validFapolicyRulesData returns a `fapolicyRulesTemplateData` populated
// with sensible defaults that pass validation. Tests can override
// individual fields via struct literal modification.
func validFapolicyRulesData() fapolicyRulesTemplateData {
	return fapolicyRulesTemplateData{
		ZarfPath: "/opt/brightsparklabs/ironbark/data/zarf/init/zarf",
		K3sPath:  "/usr/local/bin/k3s",
	}
}

// -----------------------------------------------------------------------------
// TESTS: execLauncher - successful render
// -----------------------------------------------------------------------------

// TestExecLauncher_withDefaults_rendersExpectedInvariants verifies that a
// default render produces output containing the structural invariants every
// launcher script must have.
func TestExecLauncher_withDefaults_rendersExpectedInvariants(t *testing.T) {
	got := renderLauncher(t, validLauncherData())

	invariants := []string{
		// Shebang and bash strict mode.
		"#!/usr/bin/env bash",
		"set -o errexit",
		"set -o nounset",
		"set -o pipefail",

		// Launcher sentinel emitted alongside the host env vars (so
		// any process spawned from the resulting container can detect
		// it was invoked via the launcher).
		"IRONBARK_LAUNCHER_INVOKED=true",

		// All forwarded host env vars.
		"IRONBARK_HOST_USER=",
		"IRONBARK_HOST_UID=",
		"IRONBARK_HOST_GID=",
		"IRONBARK_HOST_HOSTNAME=",
		"IRONBARK_HOST_PWD=",
		"IRONBARK_HOST_OS=",
		"IRONBARK_HOST_ARCH=",
		"IRONBARK_HOST_DATA_DIR=",
		"IRONBARK_HOST_KUBECONFIG=",
		"IRONBARK_HOST_CONTAINER_ENGINE=",
		"IRONBARK_HOST_LAUNCHER_VERSION=",
		"IRONBARK_HOST_LAUNCHER_COMMIT=",
		"IRONBARK_HOST_LAUNCHER_BUILD_TIME=",
		"IRONBARK_HOST_LAUNCHER_GENERATED_AT=",

		// Both volume mounts.
		":/root/.kube/config:z",
		":/mnt/data:z",

		// Runtime overrides for the optional flags.
		"IRONBARK_SCRIPT_NO_INTERACTIVE",
		"IRONBARK_SCRIPT_NO_TTY",
		"IRONBARK_SCRIPT_VERBOSE",

		// Image and engine show up in the rendered defaults.
		"brightsparklabs/ironbark:latest",
		"podman",
	}

	for _, want := range invariants {
		if !strings.Contains(got, want) {
			t.Errorf("rendered launcher missing expected substring %q", want)
		}
	}
}

// TestExecLauncher_withCustomFlags_rendersCustomValues verifies that
// user-supplied custom values for the user-configurable fields appear in the
// rendered output.
func TestExecLauncher_withCustomFlags_rendersCustomValues(t *testing.T) {
	data := launcherTemplateData{
		Engine:         "docker",
		Image:          "registry.example.com/myorg/ironbark:1.2.3",
		HostDataDir:    "/var/lib/ironbark",
		HostKubeconfig: "/etc/rancher/k3s/k3s.yaml",
	}

	got := renderLauncher(t, data)

	expected := []string{
		"docker",
		"registry.example.com/myorg/ironbark:1.2.3",
		"/var/lib/ironbark",
		"/etc/rancher/k3s/k3s.yaml",
	}
	for _, want := range expected {
		if !strings.Contains(got, want) {
			t.Errorf("rendered launcher missing expected custom value %q", want)
		}
	}
}

// TestExecLauncher_withNoInteractiveFlag_bakesDisableDefault verifies that
// setting `NoInteractive=true` results in the rendered script defaulting
// `NO_INTERACTIVE` to `true` (so `--interactive` is omitted unless the user
// explicitly unsets the env var at runtime).
func TestExecLauncher_withNoInteractiveFlag_bakesDisableDefault(t *testing.T) {
	data := validLauncherData()
	data.NoInteractive = true

	got := renderLauncher(t, data)

	if !strings.Contains(got, `NO_INTERACTIVE="${IRONBARK_SCRIPT_NO_INTERACTIVE-true}"`) {
		t.Errorf("expected NO_INTERACTIVE to default to `true`, got:\n%s", got)
	}
}

// TestExecLauncher_withNoTTYFlag_bakesDisableDefault verifies that setting
// `NoTTY=true` results in the rendered script defaulting `NO_TTY` to
// `true`.
func TestExecLauncher_withNoTTYFlag_bakesDisableDefault(t *testing.T) {
	data := validLauncherData()
	data.NoTTY = true

	got := renderLauncher(t, data)

	if !strings.Contains(got, `NO_TTY="${IRONBARK_SCRIPT_NO_TTY-true}"`) {
		t.Errorf("expected NO_TTY to default to `true`, got:\n%s", got)
	}
}

// TestExecLauncher_withDefaults_doesNotBakeDisableDefaults verifies that
// when `NoInteractive` and `NoTTY` are both false the rendered script
// defaults the corresponding env vars to empty (so `--interactive` and
// `--tty` are kept enabled).
func TestExecLauncher_withDefaults_doesNotBakeDisableDefaults(t *testing.T) {
	got := renderLauncher(t, validLauncherData())

	if !strings.Contains(got, `NO_INTERACTIVE="${IRONBARK_SCRIPT_NO_INTERACTIVE-}"`) {
		t.Errorf("expected NO_INTERACTIVE to default to empty, got:\n%s", got)
	}
	if !strings.Contains(got, `NO_TTY="${IRONBARK_SCRIPT_NO_TTY-}"`) {
		t.Errorf("expected NO_TTY to default to empty, got:\n%s", got)
	}
}

// -----------------------------------------------------------------------------
// TESTS: execLauncher - engine validation and normalisation
// -----------------------------------------------------------------------------

// TestExecLauncher_withUnsupportedEngine_returnsError verifies that an
// engine outside the supported set is rejected with a descriptive error.
func TestExecLauncher_withUnsupportedEngine_returnsError(t *testing.T) {
	data := validLauncherData()
	data.Engine = "kubernetes"

	var out bytes.Buffer
	err := execLauncher(&out, data)
	if err == nil {
		t.Fatal("expected error for unsupported engine, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported container engine") {
		t.Errorf("error message %q does not mention unsupported engine", err.Error())
	}
	if out.Len() != 0 {
		t.Errorf("expected no output on validation failure, got %q", out.String())
	}
}

// TestExecLauncher_withMixedCaseEngine_normalisesToLower verifies that a
// mixed-case engine name is normalised to lower case in the rendered
// output (matching the case-insensitive validation).
func TestExecLauncher_withMixedCaseEngine_normalisesToLower(t *testing.T) {
	data := validLauncherData()
	data.Engine = "PODMAN"

	got := renderLauncher(t, data)

	if !strings.Contains(got, `CONTAINER_ENGINE="${IRONBARK_SCRIPT_CONTAINER_ENGINE:-podman}"`) {
		t.Errorf("expected engine to be normalised to lower case, got:\n%s", got)
	}
}

// TestExecLauncher_withSurroundingWhitespace_trimsEngine verifies that
// surrounding whitespace on the engine name is stripped before validation
// (so callers can be sloppy without being rejected).
func TestExecLauncher_withSurroundingWhitespace_trimsEngine(t *testing.T) {
	data := validLauncherData()
	data.Engine = "  docker  "

	got := renderLauncher(t, data)

	if !strings.Contains(got, `CONTAINER_ENGINE="${IRONBARK_SCRIPT_CONTAINER_ENGINE:-docker}"`) {
		t.Errorf("expected engine to be trimmed, got:\n%s", got)
	}
}

// TestExecLauncher_withEmptyEngine_returnsError verifies that an empty
// engine is rejected (it is not in the supported set).
func TestExecLauncher_withEmptyEngine_returnsError(t *testing.T) {
	data := validLauncherData()
	data.Engine = ""

	err := execLauncher(new(bytes.Buffer), data)
	if err == nil {
		t.Fatal("expected error for empty engine, got nil")
	}
}

// -----------------------------------------------------------------------------
// TESTS: execLauncher - bash syntax validity
// -----------------------------------------------------------------------------

// TestExecLauncher_renderedOutput_isValidBashSyntax verifies that the
// rendered launcher script passes a `bash -n` parse without errors. The
// test is skipped if `bash` is not available on the system PATH.
func TestExecLauncher_renderedOutput_isValidBashSyntax(t *testing.T) {
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available on PATH; skipping syntax check")
	}

	got := renderLauncher(t, validLauncherData())

	tmpFile, err := os.CreateTemp("", "tmp_rovodev_launcher_*.sh")
	if err != nil {
		t.Fatalf("could not create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(got); err != nil {
		t.Fatalf("could not write rendered launcher to temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("could not close temp file: %v", err)
	}

	cmd := exec.Command(bashPath, "-n", tmpFile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rendered launcher failed `bash -n` syntax check: %v\nbash output:\n%s\nrendered script (saved at %s):\n%s",
			err, string(output), filepath.Clean(tmpFile.Name()), got)
	}
}

// -----------------------------------------------------------------------------
// TESTS: execZarfBootstrap - successful render
// -----------------------------------------------------------------------------

// TestExecZarfBootstrap_withDefaults_rendersExpectedInvariants verifies
// that a default render produces output containing the structural
// invariants every Zarf installer script must have.
func TestExecZarfBootstrap_withDefaults_rendersExpectedInvariants(t *testing.T) {
	got := renderZarfBootstrap(t, validZarfBootstrapData())

	invariants := []string{
		// Shebang and bash strict mode.
		"#!/usr/bin/env bash",
		"set -o errexit",
		"set -o nounset",
		"set -o pipefail",

		// Runtime overrides for all configurable values.
		"IRONBARK_SCRIPT_CONTAINER_ENGINE",
		"IRONBARK_SCRIPT_IMAGE",
		"IRONBARK_SCRIPT_ZARF_SOURCE_PATH",
		"IRONBARK_SCRIPT_ZARF_OUTPUT_DIR",
		"IRONBARK_SCRIPT_VERBOSE",

		// Core extraction logic.
		"create",
		"cp",
		"trap",

		// Default values appear in the rendered output.
		"podman",
		"brightsparklabs/ironbark:latest",
		"/app/resources/zarf/init",
		"/opt/brightsparklabs/ironbark/data/zarf/init",
	}

	for _, want := range invariants {
		if !strings.Contains(got, want) {
			t.Errorf("rendered zarf-bootstrap missing expected substring %q", want)
		}
	}
}

// TestExecZarfBootstrap_withCustomFlags_rendersCustomValues verifies that
// user-supplied custom values for the user-configurable fields appear in
// the rendered output.
func TestExecZarfBootstrap_withCustomFlags_rendersCustomValues(t *testing.T) {
	data := zarfBootstrapTemplateData{
		Engine:     "docker",
		Image:      "registry.example.com/myorg/ironbark:1.2.3",
		SourcePath: "/custom/zarf/init",
		OutputDir:  "/var/lib/ironbark/zarf",
	}

	got := renderZarfBootstrap(t, data)

	expected := []string{
		"docker",
		"registry.example.com/myorg/ironbark:1.2.3",
		"/custom/zarf/init",
		"/var/lib/ironbark/zarf",
	}
	for _, want := range expected {
		if !strings.Contains(got, want) {
			t.Errorf("rendered zarf-bootstrap missing expected custom value %q", want)
		}
	}
}

// -----------------------------------------------------------------------------
// TESTS: execZarfBootstrap - validation
// -----------------------------------------------------------------------------

// TestExecZarfBootstrap_withUnsupportedEngine_returnsError verifies that
// an engine outside the supported set is rejected with a descriptive
// error.
func TestExecZarfBootstrap_withUnsupportedEngine_returnsError(t *testing.T) {
	data := validZarfBootstrapData()
	data.Engine = "kubernetes"

	var out bytes.Buffer
	err := execZarfBootstrap(&out, data)
	if err == nil {
		t.Fatal("expected error for unsupported engine, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported container engine") {
		t.Errorf("error message %q does not mention unsupported engine", err.Error())
	}
	if out.Len() != 0 {
		t.Errorf("expected no output on validation failure, got %q", out.String())
	}
}

// TestExecZarfBootstrap_withMixedCaseEngine_normalisesToLower verifies
// that a mixed-case engine name is normalised to lower case in the
// rendered output (matching the case-insensitive validation).
func TestExecZarfBootstrap_withMixedCaseEngine_normalisesToLower(t *testing.T) {
	data := validZarfBootstrapData()
	data.Engine = "PODMAN"

	got := renderZarfBootstrap(t, data)

	if !strings.Contains(got, `CONTAINER_ENGINE="${IRONBARK_SCRIPT_CONTAINER_ENGINE:-podman}"`) {
		t.Errorf("expected engine to be normalised to lower case, got:\n%s", got)
	}
}

// TestExecZarfBootstrap_withEmptyImage_returnsError verifies that an
// empty image is rejected.
func TestExecZarfBootstrap_withEmptyImage_returnsError(t *testing.T) {
	data := validZarfBootstrapData()
	data.Image = "   "

	if err := execZarfBootstrap(new(bytes.Buffer), data); err == nil {
		t.Fatal("expected error for empty image, got nil")
	}
}

// TestExecZarfBootstrap_withEmptySourcePath_returnsError verifies that
// an empty source path is rejected.
func TestExecZarfBootstrap_withEmptySourcePath_returnsError(t *testing.T) {
	data := validZarfBootstrapData()
	data.SourcePath = ""

	if err := execZarfBootstrap(new(bytes.Buffer), data); err == nil {
		t.Fatal("expected error for empty source path, got nil")
	}
}

// TestExecZarfBootstrap_withEmptyOutputDir_returnsError verifies that an
// empty output directory is rejected.
func TestExecZarfBootstrap_withEmptyOutputDir_returnsError(t *testing.T) {
	data := validZarfBootstrapData()
	data.OutputDir = ""

	if err := execZarfBootstrap(new(bytes.Buffer), data); err == nil {
		t.Fatal("expected error for empty output dir, got nil")
	}
}

// -----------------------------------------------------------------------------
// TESTS: execZarfBootstrap - bash syntax validity
// -----------------------------------------------------------------------------

// TestExecZarfBootstrap_renderedOutput_isValidBashSyntax verifies that
// the rendered Zarf installer script passes a `bash -n` parse without
// errors. The test is skipped if `bash` is not available on the system
// PATH.
func TestExecZarfBootstrap_renderedOutput_isValidBashSyntax(t *testing.T) {
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available on PATH; skipping syntax check")
	}

	got := renderZarfBootstrap(t, validZarfBootstrapData())

	tmpFile, err := os.CreateTemp("", "tmp_rovodev_zarf_bootstrap_*.sh")
	if err != nil {
		t.Fatalf("could not create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(got); err != nil {
		t.Fatalf("could not write rendered script to temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("could not close temp file: %v", err)
	}

	cmd := exec.Command(bashPath, "-n", tmpFile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rendered zarf-bootstrap failed `bash -n` syntax check: %v\nbash output:\n%s\nrendered script (saved at %s):\n%s",
			err, string(output), filepath.Clean(tmpFile.Name()), got)
	}
}

// -----------------------------------------------------------------------------
// TESTS: execFapolicyRules - successful render
// -----------------------------------------------------------------------------

// TestExecFapolicyRules_withDefaults_rendersExpectedInvariants verifies
// that a default render contains the expected fapolicyd rule directives
// covering the Zarf and K3s binaries.
func TestExecFapolicyRules_withDefaults_rendersExpectedInvariants(t *testing.T) {
	got := renderFapolicyRules(t, validFapolicyRulesData())

	invariants := []string{
		// Header banner identifying the source.
		"Generated by `ironbark generate fapolicy-rules`",

		// Zarf rule.
		"allow perm=execute exe=/opt/brightsparklabs/ironbark/data/zarf/init/zarf : all",
		"allow perm=open all : path=/opt/brightsparklabs/ironbark/data/zarf/init/zarf",

		// K3s rule.
		"allow perm=execute exe=/usr/local/bin/k3s : all",
		"allow perm=open all : path=/usr/local/bin/k3s",
	}

	for _, want := range invariants {
		if !strings.Contains(got, want) {
			t.Errorf("rendered fapolicy-rules missing expected substring %q", want)
		}
	}
}

// TestExecFapolicyRules_outputDoesNotContainLauncherRules verifies that
// the rendered rules file never contains an Ironbark launcher rule -
// fapolicyd rules for the launcher script are intentionally out of
// scope for this command.
func TestExecFapolicyRules_outputDoesNotContainLauncherRules(t *testing.T) {
	got := renderFapolicyRules(t, validFapolicyRulesData())

	if strings.Contains(got, "launcher") || strings.Contains(got, "Launcher") {
		t.Errorf("expected rendered rules to contain no launcher references, got:\n%s", got)
	}
}

// TestExecFapolicyRules_withCustomPaths_rendersCustomValues verifies that
// user-supplied custom paths appear in the rendered output.
func TestExecFapolicyRules_withCustomPaths_rendersCustomValues(t *testing.T) {
	data := fapolicyRulesTemplateData{
		ZarfPath: "/srv/ironbark/bin/zarf",
		K3sPath:  "/opt/k3s/bin/k3s",
	}

	got := renderFapolicyRules(t, data)

	expected := []string{
		"/srv/ironbark/bin/zarf",
		"/opt/k3s/bin/k3s",
	}
	for _, want := range expected {
		if !strings.Contains(got, want) {
			t.Errorf("rendered fapolicy-rules missing expected custom value %q", want)
		}
	}
}

// -----------------------------------------------------------------------------
// TESTS: execFapolicyRules - validation
// -----------------------------------------------------------------------------

// TestExecFapolicyRules_withEmptyZarfPath_returnsError verifies that an
// empty Zarf path is rejected.
func TestExecFapolicyRules_withEmptyZarfPath_returnsError(t *testing.T) {
	data := validFapolicyRulesData()
	data.ZarfPath = ""

	if err := execFapolicyRules(new(bytes.Buffer), data); err == nil {
		t.Fatal("expected error for empty zarf path, got nil")
	}
}

// TestExecFapolicyRules_withEmptyK3sPath_returnsError verifies that an
// empty K3s path is rejected.
func TestExecFapolicyRules_withEmptyK3sPath_returnsError(t *testing.T) {
	data := validFapolicyRulesData()
	data.K3sPath = ""

	if err := execFapolicyRules(new(bytes.Buffer), data); err == nil {
		t.Fatal("expected error for empty k3s path, got nil")
	}
}

// TestExecFapolicyRules_withRelativeZarfPath_returnsError verifies that
// a relative Zarf path is rejected (fapolicyd rules require absolute
// paths).
func TestExecFapolicyRules_withRelativeZarfPath_returnsError(t *testing.T) {
	data := validFapolicyRulesData()
	data.ZarfPath = "bin/zarf"

	if err := execFapolicyRules(new(bytes.Buffer), data); err == nil {
		t.Fatal("expected error for relative zarf path, got nil")
	}
}

// TestExecFapolicyRules_withRelativeK3sPath_returnsError verifies that a
// relative K3s path is rejected.
func TestExecFapolicyRules_withRelativeK3sPath_returnsError(t *testing.T) {
	data := validFapolicyRulesData()
	data.K3sPath = "bin/k3s"

	if err := execFapolicyRules(new(bytes.Buffer), data); err == nil {
		t.Fatal("expected error for relative k3s path, got nil")
	}
}

// -----------------------------------------------------------------------------
// TESTS: execZarfBootstrap - asset existence check
// -----------------------------------------------------------------------------

// TestExecZarfBootstrap_withMissingAssets_returnsError verifies that the
// command refuses to render when the configured source path does not
// contain the Zarf assets, and that the rendered output is suppressed.
func TestExecZarfBootstrap_withMissingAssets_returnsError(t *testing.T) {
	origAssets := zarfBootstrapAssetsPresent
	defer func() { zarfBootstrapAssetsPresent = origAssets }()
	zarfBootstrapAssetsPresent = func(string) error {
		return os.ErrNotExist
	}

	var out bytes.Buffer
	err := execZarfBootstrap(&out, validZarfBootstrapData())
	if err == nil {
		t.Fatal("expected error when Zarf assets are missing, got nil")
	}
	if !strings.Contains(err.Error(), "could not verify Zarf assets") {
		t.Errorf("error message %q does not mention the asset precondition", err.Error())
	}
	if out.Len() != 0 {
		t.Errorf("expected no output on precondition failure, got %q", out.String())
	}
}

// -----------------------------------------------------------------------------
// TESTS: defaultZarfAssetsPresent
// -----------------------------------------------------------------------------

// TestDefaultZarfAssetsPresent_withCompleteAssets_returnsNoError verifies
// the happy path: a directory containing both the `zarf` binary and a
// `zarf-init-*.tar.zst` package passes the check.
func TestDefaultZarfAssetsPresent_withCompleteAssets_returnsNoError(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "zarf"), "fake binary")
	mustWriteFile(t, filepath.Join(dir, "zarf-init-amd64-v0.64.0.tar.zst"), "fake package")

	if err := defaultZarfAssetsPresent(dir); err != nil {
		t.Errorf("expected no error for complete asset directory, got: %v", err)
	}
}

// TestDefaultZarfAssetsPresent_withMissingDirectory_returnsError verifies
// that a non-existent source path is rejected with a descriptive error.
func TestDefaultZarfAssetsPresent_withMissingDirectory_returnsError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	err := defaultZarfAssetsPresent(missing)
	if err == nil {
		t.Fatal("expected error for missing directory, got nil")
	}
	if !strings.Contains(err.Error(), "not accessible") {
		t.Errorf("error message %q does not mention accessibility", err.Error())
	}
}

// TestDefaultZarfAssetsPresent_withFileInsteadOfDirectory_returnsError
// verifies that a file at the source path (rather than a directory) is
// rejected.
func TestDefaultZarfAssetsPresent_withFileInsteadOfDirectory_returnsError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	mustWriteFile(t, file, "")

	err := defaultZarfAssetsPresent(file)
	if err == nil {
		t.Fatal("expected error when source path is a file, got nil")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("error message %q does not mention the directory requirement", err.Error())
	}
}

// TestDefaultZarfAssetsPresent_withMissingZarfBinary_returnsError
// verifies that a directory missing the `zarf` binary is rejected even
// when an init package is present.
func TestDefaultZarfAssetsPresent_withMissingZarfBinary_returnsError(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "zarf-init-amd64-v0.64.0.tar.zst"), "fake package")

	err := defaultZarfAssetsPresent(dir)
	if err == nil {
		t.Fatal("expected error when zarf binary is missing, got nil")
	}
	if !strings.Contains(err.Error(), "zarf binary not found") {
		t.Errorf("error message %q does not mention the zarf binary", err.Error())
	}
}

// TestDefaultZarfAssetsPresent_withMissingInitPackage_returnsError
// verifies that a directory missing the init package is rejected even
// when the `zarf` binary is present.
func TestDefaultZarfAssetsPresent_withMissingInitPackage_returnsError(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "zarf"), "fake binary")

	err := defaultZarfAssetsPresent(dir)
	if err == nil {
		t.Fatal("expected error when init package is missing, got nil")
	}
	if !strings.Contains(err.Error(), "no zarf init package") {
		t.Errorf("error message %q does not mention the init package", err.Error())
	}
}

// TestDefaultZarfAssetsPresent_withWrongInitPackageName_returnsError
// verifies that files which do not match the
// `zarf-init-*.tar.zst` naming convention are not counted as the init
// package.
func TestDefaultZarfAssetsPresent_withWrongInitPackageName_returnsError(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "zarf"), "fake binary")
	mustWriteFile(t, filepath.Join(dir, "zarf-init.tar.gz"), "wrong format")
	mustWriteFile(t, filepath.Join(dir, "init-amd64.tar.zst"), "wrong prefix")

	err := defaultZarfAssetsPresent(dir)
	if err == nil {
		t.Fatal("expected error when init package name does not match, got nil")
	}
}

// -----------------------------------------------------------------------------
// TEST HELPERS
// -----------------------------------------------------------------------------

// mustWriteFile writes the supplied contents to `path`, failing the test
// (via `t.Fatalf`) if the write fails. Used to set up filesystem
// fixtures concisely in tests.
func mustWriteFile(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("could not write test fixture file %q: %v", path, err)
	}
}
