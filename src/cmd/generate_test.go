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
		HostDataDir:    "${PWD}/ironbark-data",
		HostKubeconfig: "${HOME}/.kube/config",
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

		// Container sentinel (the only env var that crosses the host →
		// container boundary without an `IRONBARK_HOST_` prefix).
		"IRONBARK_IN_CONTAINER=true",

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
		"IRONBARK_NO_INTERACTIVE",
		"IRONBARK_NO_TTY",
		"IRONBARK_VERBOSE",

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

	if !strings.Contains(got, `NO_INTERACTIVE="${IRONBARK_NO_INTERACTIVE-true}"`) {
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

	if !strings.Contains(got, `NO_TTY="${IRONBARK_NO_TTY-true}"`) {
		t.Errorf("expected NO_TTY to default to `true`, got:\n%s", got)
	}
}

// TestExecLauncher_withDefaults_doesNotBakeDisableDefaults verifies that
// when `NoInteractive` and `NoTTY` are both false the rendered script
// defaults the corresponding env vars to empty (so `--interactive` and
// `--tty` are kept enabled).
func TestExecLauncher_withDefaults_doesNotBakeDisableDefaults(t *testing.T) {
	got := renderLauncher(t, validLauncherData())

	if !strings.Contains(got, `NO_INTERACTIVE="${IRONBARK_NO_INTERACTIVE-}"`) {
		t.Errorf("expected NO_INTERACTIVE to default to empty, got:\n%s", got)
	}
	if !strings.Contains(got, `NO_TTY="${IRONBARK_NO_TTY-}"`) {
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

	if !strings.Contains(got, `CONTAINER_ENGINE="${IRONBARK_CONTAINER_ENGINE:-podman}"`) {
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

	if !strings.Contains(got, `CONTAINER_ENGINE="${IRONBARK_CONTAINER_ENGINE:-docker}"`) {
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
