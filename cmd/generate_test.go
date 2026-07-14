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
	"time"
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
		Image:          "docker.io/brightsparklabs/ironbark:latest",
		HostDataDir:    "/opt/brightsparklabs/ironbark/production/data",
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
		Image:      "docker.io/brightsparklabs/ironbark:latest",
		SourcePath: "/app/resources/zarf/init",
		OutputDir:  "/opt/brightsparklabs/ironbark/production/data/zarf/init",
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
		ZarfPath: "/opt/brightsparklabs/ironbark/production/data/zarf/init/zarf",
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
		":/mnt/conf/kubeconfig:z",
		":/mnt/data:z",

		// Runtime overrides for the optional flags.
		"IRONBARK_SCRIPT_NO_INTERACTIVE",
		"IRONBARK_SCRIPT_NO_TTY",
		"IRONBARK_SCRIPT_VERBOSE",

		// Image and engine show up in the rendered defaults.
		"docker.io/brightsparklabs/ironbark:latest",
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

	if !strings.Contains(got, `IRONBARK_SCRIPT_NO_INTERACTIVE="${IRONBARK_SCRIPT_NO_INTERACTIVE:-true}"`) {
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

	if !strings.Contains(got, `IRONBARK_SCRIPT_NO_TTY="${IRONBARK_SCRIPT_NO_TTY:-true}"`) {
		t.Errorf("expected NO_TTY to default to `true`, got:\n%s", got)
	}
}

// TestExecLauncher_withDefaults_doesNotBakeDisableDefaults verifies that
// when `NoInteractive` and `NoTTY` are both false the rendered script
// does NOT contain the bake-default assignment that would otherwise
// disable the corresponding container engine flag. The variables are
// still pre-declared (defined-but-empty) at the top of the script via
// the central `: "${VAR:=}"` block, so the omission is what keeps
// `--interactive` and `--tty` enabled by default.
func TestExecLauncher_withDefaults_doesNotBakeDisableDefaults(t *testing.T) {
	got := renderLauncher(t, validLauncherData())

	bakeDefaults := []string{
		`IRONBARK_SCRIPT_NO_INTERACTIVE="${IRONBARK_SCRIPT_NO_INTERACTIVE:-true}"`,
		`IRONBARK_SCRIPT_NO_TTY="${IRONBARK_SCRIPT_NO_TTY:-true}"`,
	}
	for _, unwanted := range bakeDefaults {
		if strings.Contains(got, unwanted) {
			t.Errorf("expected rendered launcher to NOT contain bake-default %q, got:\n%s", unwanted, got)
		}
	}

	// And the variables must still be pre-declared so unset access is
	// safe under `set -o nounset`.
	preDeclared := []string{
		`: "${IRONBARK_SCRIPT_NO_INTERACTIVE:=}"`,
		`: "${IRONBARK_SCRIPT_NO_TTY:=}"`,
	}
	for _, want := range preDeclared {
		if !strings.Contains(got, want) {
			t.Errorf("expected rendered launcher to contain pre-declaration %q, got:\n%s", want, got)
		}
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

	if !strings.Contains(got, `IRONBARK_SCRIPT_CONTAINER_ENGINE="${IRONBARK_SCRIPT_CONTAINER_ENGINE:-podman}"`) {
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

	if !strings.Contains(got, `IRONBARK_SCRIPT_CONTAINER_ENGINE="${IRONBARK_SCRIPT_CONTAINER_ENGINE:-docker}"`) {
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

// TestExecLauncher_rendersCliFlagParser verifies that the rendered
// launcher includes the CLI flag-parsing function and the per-flag
// case branches that map command-line flags to the corresponding
// `IRONBARK_SCRIPT_*` environment variables.
func TestExecLauncher_rendersCliFlagParser(t *testing.T) {
	got := renderLauncher(t, validLauncherData())

	expected := []string{
		// Flag-parser entry point and the global args array it
		// populates.
		"function parse_launcher_args()",
		"CONTAINER_ARGS=()",

		// Each supported launcher flag must be wired to the
		// corresponding IRONBARK_SCRIPT_* env var.
		"--no-interactive)",
		"export IRONBARK_SCRIPT_NO_INTERACTIVE=\"true\"",
		"--no-tty)",
		"export IRONBARK_SCRIPT_NO_TTY=\"true\"",
		"--verbose)",
		"export IRONBARK_SCRIPT_VERBOSE=\"true\"",
		"--container-engine=*)",
		"export IRONBARK_SCRIPT_CONTAINER_ENGINE=\"${1#*=}\"",
		"--image=*)",
		"export IRONBARK_SCRIPT_IMAGE=\"${1#*=}\"",

		// Help and unknown-flag handling.
		"--help)",
		"print_launcher_usage",
		"Unknown launcher option:",

		// Runtime initialisation must happen AFTER flag parsing.
		"function init_runtime_settings()",
	}
	for _, want := range expected {
		if !strings.Contains(got, want) {
			t.Errorf("rendered launcher missing CLI-parser substring %q", want)
		}
	}
}

// TestExecLauncher_cliFlags_endToEnd_executesRenderedScript renders
// the launcher, replaces the final `podman run` invocation with a
// stub, and exercises the rendered script with a representative set
// of CLI flag combinations. The test confirms that flag parsing
// actually mutates the runtime settings (rather than just appearing
// in the rendered text) and that the `--` separator semantics behave
// as advertised.
//
// Skipped when bash is not available on PATH.
func TestExecLauncher_cliFlags_endToEnd_executesRenderedScript(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash not available on PATH; skipping: %v", err)
	}

	// Render the launcher and stub out the actual container engine
	// invocation so we can assert on what would have been executed
	// without needing podman or docker to be installed.
	rendered := renderLauncher(t, validLauncherData())
	stubbed := strings.Replace(
		rendered,
		`"${IRONBARK_SCRIPT_CONTAINER_ENGINE}" run`,
		`echo CMD: "${IRONBARK_SCRIPT_CONTAINER_ENGINE}" run`,
		1,
	)

	// Use a writable host data dir so the script's `mkdir -p` does
	// not fail before the parsing logic is exercised.
	dataDir := t.TempDir()
	stubbed = strings.Replace(
		stubbed,
		"/opt/brightsparklabs/ironbark/production/data",
		dataDir,
		-1,
	)

	tmpFile, err := os.CreateTemp("", "tmp_rovodev_launcher_cli_*.sh")
	if err != nil {
		t.Fatalf("could not create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.WriteString(stubbed); err != nil {
		t.Fatalf("could not write stubbed launcher: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("could not close temp file: %v", err)
	}

	cases := []struct {
		name string
		// envOverrides are appended to `os.Environ()` for the
		// invocation. Use them to verify env-var-only override
		// scenarios where no CLI flags are supplied.
		envOverrides []string
		args         []string
		mustContain  []string
		mustOmit     []string
	}{
		{
			name:        "no separator - all args forwarded verbatim",
			args:        []string{"init", "all"},
			mustContain: []string{"docker.io/brightsparklabs/ironbark:latest init all"},
		},
		{
			name: "separator - flags before are parsed, args after are forwarded",
			args: []string{"--no-tty", "--verbose", "--", "init", "all"},
			mustContain: []string{
				"docker.io/brightsparklabs/ironbark:latest init all",
				"DEBUG:",
				// `--no-tty` should disable the engine `--tty`
				// flag so the verbose log reports it disabled.
				// We anchor on the prefix and suffix
				// independently so the assertion does not
				// brittlely depend on the exact column
				// alignment used by `printf` in the template.
				"--tty flag",
				"= disabled",
				// Verbose mode must report the resolved
				// settings table so operators can confirm
				// what was actually applied vs. baked-in.
				"Resolved launcher settings",
				"IRONBARK_SCRIPT_NO_TTY",
				// And the resolved container args.
				"Container args (forwarded verbatim to image):",
				"argv[0] = init",
				"argv[1] = all",
			},
			mustOmit: []string{
				// The actual `podman run` invocation must not
				// contain the `--tty` flag once `--no-tty` is in
				// effect. The verbose-mode debug log uses
				// "--tty flag" so we anchor on the plain
				// `--tty ` token only as it appears in the
				// final CMD line.
				"CMD: podman run --rm --interactive --tty ",
			},
		},
		{
			name: "image flag overrides the baked-in default",
			args: []string{"--image=registry.example/foo:1.0", "--", "argv"},
			mustContain: []string{
				"registry.example/foo:1.0 argv",
			},
			mustOmit: []string{
				"docker.io/brightsparklabs/ironbark:latest argv",
			},
		},
		{
			name: "container-engine flag is honoured",
			args: []string{"--container-engine=docker", "--", "noop"},
			mustContain: []string{
				"CMD: docker run",
			},
		},
		{
			// CLI flags are convenience wrappers - they must
			// not be required. Operators can still drive the
			// launcher entirely via IRONBARK_SCRIPT_* exports
			// even when no `--` separator is supplied.
			name: "env IRONBARK_SCRIPT_NO_TTY without flags or -- still disables --tty",
			envOverrides: []string{
				"IRONBARK_SCRIPT_NO_TTY=true",
				"IRONBARK_SCRIPT_VERBOSE=true",
			},
			args: []string{"init", "all"},
			mustContain: []string{
				"docker.io/brightsparklabs/ironbark:latest init all",
				"--tty flag",
				"= disabled",
			},
		},
		{
			name: "env IRONBARK_SCRIPT_IMAGE without flags or -- still overrides image",
			envOverrides: []string{
				"IRONBARK_SCRIPT_IMAGE=registry.example/from-env:1.0",
			},
			args: []string{"foo"},
			mustContain: []string{
				"registry.example/from-env:1.0 foo",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("bash", append([]string{tmpFile.Name()}, tc.args...)...)
			if len(tc.envOverrides) > 0 {
				cmd.Env = append(os.Environ(), tc.envOverrides...)
			}
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("bash execution failed: %v\noutput:\n%s", err, out)
			}
			got := string(out)
			for _, want := range tc.mustContain {
				if !strings.Contains(got, want) {
					t.Errorf("expected output to contain %q, got:\n%s", want, got)
				}
			}
			for _, unwanted := range tc.mustOmit {
				if strings.Contains(got, unwanted) {
					t.Errorf("expected output to NOT contain %q, got:\n%s", unwanted, got)
				}
			}
		})
	}
}

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
		"docker.io/brightsparklabs/ironbark:latest",
		"/app/resources/zarf/init",
		"/opt/brightsparklabs/ironbark/production/data/zarf/init",
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

	if !strings.Contains(got, `IRONBARK_SCRIPT_CONTAINER_ENGINE="${IRONBARK_SCRIPT_CONTAINER_ENGINE:-podman}"`) {
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
		"allow perm=execute exe=/opt/brightsparklabs/ironbark/production/data/zarf/init/zarf : all",
		"allow perm=open all : path=/opt/brightsparklabs/ironbark/production/data/zarf/init/zarf",

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

// -----------------------------------------------------------------------------
// HELPERS: installer
// -----------------------------------------------------------------------------

// renderInstaller renders the installer template with the supplied data
// (and a default launcher data) and returns the rendered output.
func renderInstaller(t *testing.T, data installerTemplateData) string {
	t.Helper()

	var out bytes.Buffer
	if err := execInstaller(&out, data, validLauncherData()); err != nil {
		t.Fatalf("execInstaller returned unexpected error: %v", err)
	}

	return out.String()
}

// validInstallerData returns an `installerTemplateData` populated with
// sensible defaults that pass validation. Tests can override individual
// fields via struct literal modification.
func validInstallerData() installerTemplateData {
	return installerTemplateData{
		InstallBaseDir:     "/opt/brightsparklabs/ironbark",
		ReleaseEnvironment: "production",
	}
}

// -----------------------------------------------------------------------------
// TESTS: execInstaller - successful render
// -----------------------------------------------------------------------------

// TestExecInstaller_withDefaults_rendersExpectedInvariants verifies that a
// default render produces output containing the structural invariants every
// installer script must have.
func TestExecInstaller_withDefaults_rendersExpectedInvariants(t *testing.T) {
	got := renderInstaller(t, validInstallerData())

	invariants := []string{
		// Shebang and bash strict mode.
		"#!/usr/bin/env bash",
		"set -o errexit",
		"set -o nounset",
		"set -o pipefail",

		// Baked-in constants reflecting the install location.
		`readonly IRONBARK_INSTALL_BASE_DIR="/opt/brightsparklabs/ironbark"`,
		`readonly IRONBARK_RELEASE_ENVIRONMENT="production"`,
		`readonly IRONBARK_INSTALL_DIR="/opt/brightsparklabs/ironbark/production"`,

		// Default subdirectories created by the installer.
		`"bin"`,
		`"data/zarf/init"`,
		`"data/repos"`,

		// User-resolution logic prefers `SUDO_USER`, falls back to `USER`.
		`"${SUDO_USER:-}"`,
		`"${USER:-}"`,

		// Heredoc that inlines the launcher.
		"IRONBARK_INSTALLER_LAUNCHER_EOF",

		// Symlink + versioned launcher pattern matching the appcli style.
		`_launcher_versioned=".$(date -Iseconds --utc)_ironbark_`,
		`_launcher_generic="ironbark"`,
		`ln -s "${_launcher_versioned}" "${_launcher_generic}"`,

		// Ownership + ACL handling - both scoped to the
		// per-release-environment install directory, NOT the install
		// base directory, so sibling environments are not touched.
		`chown -R "${_owner}" "${IRONBARK_INSTALL_DIR}"`,
		`command -v setfacl`,
		`setfacl -R -d -m "u:${_user}:rwX,g:${_group}:rwX" "${IRONBARK_INSTALL_DIR}"`,

		// Final "can be launched via" banner echoed on success.
		"can be launched via:",

		// The inlined launcher must show up (i.e. the heredoc is
		// populated, not empty).
		"IRONBARK_LAUNCHER_INVOKED=true",
	}

	for _, want := range invariants {
		if !strings.Contains(got, want) {
			t.Errorf("rendered installer missing expected substring %q", want)
		}
	}
}

// TestExecInstaller_withCustomFlags_rendersCustomValues verifies that
// non-default flag values flow through to the rendered installer.
func TestExecInstaller_withCustomFlags_rendersCustomValues(t *testing.T) {
	data := installerTemplateData{
		InstallBaseDir:     "/srv/ironbark",
		ReleaseEnvironment: "staging",
	}

	got := renderInstaller(t, data)

	expected := []string{
		`readonly IRONBARK_INSTALL_BASE_DIR="/srv/ironbark"`,
		`readonly IRONBARK_RELEASE_ENVIRONMENT="staging"`,
		`readonly IRONBARK_INSTALL_DIR="/srv/ironbark/staging"`,
	}
	for _, want := range expected {
		if !strings.Contains(got, want) {
			t.Errorf("rendered installer missing expected custom value %q", want)
		}
	}
}

// -----------------------------------------------------------------------------
// TESTS: execInstaller - validation failures
// -----------------------------------------------------------------------------

// TestExecInstaller_withEmptyInstallBaseDir_returnsError verifies the
// installer rejects an empty install-base-dir.
func TestExecInstaller_withEmptyInstallBaseDir_returnsError(t *testing.T) {
	data := validInstallerData()
	data.InstallBaseDir = ""

	if err := execInstaller(new(bytes.Buffer), data, validLauncherData()); err == nil {
		t.Fatal("expected error for empty install-base-dir, got nil")
	}
}

// TestExecInstaller_withRelativeInstallBaseDir_returnsError verifies the
// installer rejects a relative install-base-dir.
func TestExecInstaller_withRelativeInstallBaseDir_returnsError(t *testing.T) {
	data := validInstallerData()
	data.InstallBaseDir = "ironbark"

	err := execInstaller(new(bytes.Buffer), data, validLauncherData())
	if err == nil {
		t.Fatal("expected error for relative install-base-dir, got nil")
	}
	if !strings.Contains(err.Error(), "absolute path") {
		t.Errorf("error message %q does not mention absolute path", err.Error())
	}
}

// TestExecInstaller_withEmptyReleaseEnvironment_returnsError verifies
// the installer rejects an empty release-environment.
func TestExecInstaller_withEmptyReleaseEnvironment_returnsError(t *testing.T) {
	data := validInstallerData()
	data.ReleaseEnvironment = ""

	if err := execInstaller(new(bytes.Buffer), data, validLauncherData()); err == nil {
		t.Fatal("expected error for empty release-environment, got nil")
	}
}

// TestExecInstaller_withInvalidLauncherEngine_returnsError verifies that
// a launcher validation failure (e.g. unsupported engine) surfaces from
// the installer code path too.
func TestExecInstaller_withInvalidLauncherEngine_returnsError(t *testing.T) {
	launcher := validLauncherData()
	launcher.Engine = "kubernetes"

	err := execInstaller(new(bytes.Buffer), validInstallerData(), launcher)
	if err == nil {
		t.Fatal("expected error for unsupported launcher engine, got nil")
	}
	if !strings.Contains(err.Error(), "inlined launcher") {
		t.Errorf("error message %q does not mention the inlined launcher", err.Error())
	}
}

// TestExecInstaller_withSurroundingWhitespace_trimsFields verifies the
// installer trims surrounding whitespace on string inputs before
// validation.
func TestExecInstaller_withSurroundingWhitespace_trimsFields(t *testing.T) {
	data := installerTemplateData{
		InstallBaseDir:     "  /srv/ironbark  ",
		ReleaseEnvironment: "  prod  ",
	}

	got := renderInstaller(t, data)

	if !strings.Contains(got, `readonly IRONBARK_INSTALL_BASE_DIR="/srv/ironbark"`) {
		t.Errorf("expected install-base-dir to be trimmed, got:\n%s", got)
	}
	if !strings.Contains(got, `readonly IRONBARK_RELEASE_ENVIRONMENT="prod"`) {
		t.Errorf("expected release-environment to be trimmed, got:\n%s", got)
	}
}

// -----------------------------------------------------------------------------
// TESTS: execInstaller - bash syntax + end-to-end execution
// -----------------------------------------------------------------------------

// TestExecInstaller_renderedOutput_isValidBashSyntax verifies that the
// rendered installer script is syntactically valid bash.
func TestExecInstaller_renderedOutput_isValidBashSyntax(t *testing.T) {
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available on PATH; skipping syntax check")
	}

	got := renderInstaller(t, validInstallerData())

	tmpFile, err := os.CreateTemp("", "tmp_rovodev_installer_*.sh")
	if err != nil {
		t.Fatalf("could not create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(got); err != nil {
		t.Fatalf("could not write rendered installer to temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("could not close temp file: %v", err)
	}

	cmd := exec.Command(bashPath, "-n", tmpFile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rendered installer failed `bash -n` syntax check: %v\nbash output:\n%s\nrendered script (saved at %s):\n%s",
			err, string(output), filepath.Clean(tmpFile.Name()), got)
	}
}

// TestExecInstaller_endToEnd_createsInstallTreeAndLauncher pipes the
// rendered installer through bash against a temp install-base-dir and
// asserts the resulting directory tree, versioned launcher, and
// `ironbark` symlink are all created as expected. Also asserts the
// inlined launcher itself is syntactically valid bash.
func TestExecInstaller_endToEnd_createsInstallTreeAndLauncher(t *testing.T) {
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skipf("bash not available on PATH; skipping: %v", err)
	}

	tmpDir := t.TempDir()
	data := installerTemplateData{
		InstallBaseDir:     tmpDir,
		ReleaseEnvironment: "staging",
	}

	got := renderInstaller(t, data)

	cmd := exec.Command(bashPath, "-c", got)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("installer execution failed: %v\noutput:\n%s", err, string(output))
	}

	installDir := filepath.Join(tmpDir, "staging")
	binDir := filepath.Join(installDir, "bin")

	// Directory tree assertions.
	for _, sub := range []string{
		"",
		"bin",
		"data",
		"data/zarf",
		"data/zarf/init",
		"data/repos",
	} {
		info, err := os.Stat(filepath.Join(installDir, sub))
		if err != nil {
			t.Errorf("expected directory %q to exist: %v", filepath.Join(installDir, sub), err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("expected %q to be a directory", filepath.Join(installDir, sub))
		}
	}

	// Symlink + target assertions. The launcher and its symlink live
	// in `<install-dir>/bin/`, NOT directly in `<install-dir>/`.
	if _, err := os.Stat(filepath.Join(installDir, "ironbark")); !os.IsNotExist(err) {
		t.Errorf("expected NO launcher symlink directly under install-dir, but one exists (err=%v)", err)
	}
	linkPath := filepath.Join(binDir, "ironbark")
	linkInfo, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("expected launcher symlink at %q: %v", linkPath, err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected %q to be a symlink", linkPath)
	}
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("could not read symlink target: %v", err)
	}
	if !strings.HasPrefix(target, ".") || !strings.Contains(target, "_ironbark_") {
		t.Errorf("unexpected symlink target %q (want hidden, timestamped launcher)", target)
	}

	// The launcher itself must exist (alongside the symlink in `bin/`)
	// and be executable.
	launcherPath := filepath.Join(binDir, target)
	launcherInfo, err := os.Stat(launcherPath)
	if err != nil {
		t.Fatalf("expected versioned launcher at %q: %v", launcherPath, err)
	}
	if launcherInfo.Mode().Perm()&0o100 == 0 {
		t.Errorf("expected versioned launcher %q to be executable, mode=%o", launcherPath, launcherInfo.Mode().Perm())
	}

	// The inlined launcher script itself must be syntactically valid bash.
	syntaxCheck := exec.Command(bashPath, "-n", launcherPath)
	if syntaxOut, err := syntaxCheck.CombinedOutput(); err != nil {
		t.Fatalf("inlined launcher failed `bash -n` syntax check: %v\nbash output:\n%s",
			err, string(syntaxOut))
	}
}

// TestExecInstaller_inlinedLauncher_usesInstallDirAsDataDir verifies
// that the inlined launcher's host data directory is forced to
// `<install-dir>/data` regardless of any caller-supplied
// `HostDataDir`, so the directories the installer creates and the
// directories the launcher mounts cannot drift out of sync.
func TestExecInstaller_inlinedLauncher_usesInstallDirAsDataDir(t *testing.T) {
	data := installerTemplateData{
		InstallBaseDir:     "/srv/ironbark",
		ReleaseEnvironment: "qa",
	}
	launcher := validLauncherData()
	// Deliberately set a bogus HostDataDir to prove it is overridden.
	launcher.HostDataDir = "/this/should/be/ignored"

	var out bytes.Buffer
	if err := execInstaller(&out, data, launcher); err != nil {
		t.Fatalf("execInstaller returned unexpected error: %v", err)
	}
	got := out.String()

	if !strings.Contains(got, `/srv/ironbark/qa/data`) {
		t.Errorf("expected inlined launcher to mount install-dir/data, got:\n%s", got)
	}
	if strings.Contains(got, "/this/should/be/ignored") {
		t.Errorf("expected caller-supplied HostDataDir to be ignored, but found it in output")
	}
}

// TestNewGenerateInstallerCmd_doesNotExposeDataDirFlag locks in the
// design decision that `generate installer` deliberately does NOT
// expose a `--data-dir` flag (because the installer hard-codes the
// data directory as `<install-dir>/data`).
func TestNewGenerateInstallerCmd_doesNotExposeDataDirFlag(t *testing.T) {
	cmd := newGenerateInstallerCmd()

	if flag := cmd.Flags().Lookup("data-dir"); flag != nil {
		t.Errorf("expected `generate installer` to not expose --data-dir, but it does: %+v", flag)
	}
	// And the shorthand `-d` should also be unbound.
	if flag := cmd.Flags().ShorthandLookup("d"); flag != nil {
		t.Errorf("expected `generate installer` shorthand -d to be unbound, but found: %+v", flag)
	}
}

// TestExecInstaller_endToEnd_isIdempotentOnRerun verifies that running
// the installer a second time keeps the previous versioned launcher
// (for rollback) and retargets the generic `ironbark` symlink at the
// newly-written versioned launcher.
func TestExecInstaller_endToEnd_isIdempotentOnRerun(t *testing.T) {
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skipf("bash not available on PATH; skipping: %v", err)
	}

	tmpDir := t.TempDir()
	data := installerTemplateData{
		InstallBaseDir:     tmpDir,
		ReleaseEnvironment: "staging",
	}

	// First run.
	first := renderInstaller(t, data)
	if out, err := exec.Command(bashPath, "-c", first).CombinedOutput(); err != nil {
		t.Fatalf("first installer run failed: %v\n%s", err, string(out))
	}

	// Sleep just long enough for the second-resolution ISO timestamp
	// baked into the versioned launcher filename to differ.
	time.Sleep(1100 * time.Millisecond)

	// Second run.
	second := renderInstaller(t, data)
	if out, err := exec.Command(bashPath, "-c", second).CombinedOutput(); err != nil {
		t.Fatalf("second installer run failed: %v\n%s", err, string(out))
	}

	// Expect at least two versioned launchers (one per run) and the
	// generic symlink, all in `<install-dir>/bin/`.
	binDir := filepath.Join(tmpDir, "staging", "bin")
	entries, err := os.ReadDir(binDir)
	if err != nil {
		t.Fatalf("could not read launcher bin dir: %v", err)
	}

	var versionedCount int
	var symlinkSeen bool
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") && strings.Contains(name, "_ironbark_") {
			versionedCount++
		}
		if name == "ironbark" {
			symlinkSeen = true
		}
	}

	if versionedCount < 2 {
		t.Errorf("expected >= 2 versioned launchers after rerun, got %d", versionedCount)
	}
	if !symlinkSeen {
		t.Error("expected generic `ironbark` symlink to exist after rerun")
	}
}
