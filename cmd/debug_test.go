/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"brightsparklabs.com/ironbark/internal/settings"

	"gopkg.in/yaml.v3"
)

// -----------------------------------------------------------------------------
// FIXTURES
// -----------------------------------------------------------------------------

// fixtureResolved returns a small synthetic resolved-settings slice
// used by the renderer tests. Includes one var per source/sensitivity
// combination to exercise every code path.
func fixtureResolved() []settings.Resolved {
	return []settings.Resolved{
		{
			Var: settings.Var{
				Name:        "IRONBARK_DATA_DIR",
				Description: "Data directory.",
				Default:     "/tmp/ironbark/data",
				Scope:       settings.ScopeGoConsumed,
			},
			Value:   "/custom/data",
			FromEnv: true,
		},
		{
			Var: settings.Var{
				Name:        "IRONBARK_LOG_LEVEL",
				Description: "Minimum log level.",
				Default:     "",
				Scope:       settings.ScopeGoConsumed,
			},
			Value:   "",
			FromEnv: false,
		},
		{
			Var: settings.Var{
				Name:        "IRONBARK_GITEA_TOKEN",
				Description: "Gitea API token.",
				Default:     "",
				Scope:       settings.ScopeGoConsumed,
				Sensitive:   true,
			},
			Value:   "super-secret-value",
			FromEnv: true,
		},
	}
}

// -----------------------------------------------------------------------------
// TESTS: format validation
// -----------------------------------------------------------------------------

// TestExecDebugSettings_unsupportedFormat_returnsError verifies that
// any format outside the supported set is rejected with a descriptive
// error and no output is produced.
func TestExecDebugSettings_unsupportedFormat_returnsError(t *testing.T) {
	var buf bytes.Buffer
	err := execDebugSettings(&buf, fixtureResolved(), "xml", false)
	if err == nil {
		t.Fatal("expected error for unsupported format, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported format") {
		t.Errorf("error message %q does not mention unsupported format", err.Error())
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output on validation failure, got %q", buf.String())
	}
}

// TestExecDebugSettings_formatIsCaseInsensitive verifies that mixed-
// case format names are normalised before being looked up.
func TestExecDebugSettings_formatIsCaseInsensitive(t *testing.T) {
	for _, input := range []string{"TEXT", "JSON", "Yaml", " text "} {
		var buf bytes.Buffer
		if err := execDebugSettings(&buf, fixtureResolved(), input, false); err != nil {
			t.Errorf("execDebugSettings(%q) returned error: %v", input, err)
		}
		if buf.Len() == 0 {
			t.Errorf("execDebugSettings(%q) produced no output", input)
		}
	}
}

// -----------------------------------------------------------------------------
// TESTS: text renderer
// -----------------------------------------------------------------------------

// TestExecDebugSettings_text_includesHeaderAndAllRows verifies that
// the text renderer produces the expected column header and emits a
// row for each input setting.
func TestExecDebugSettings_text_includesHeaderAndAllRows(t *testing.T) {
	var buf bytes.Buffer
	if err := execDebugSettings(&buf, fixtureResolved(), "text", false); err != nil {
		t.Fatalf("execDebugSettings returned error: %v", err)
	}
	got := buf.String()

	for _, want := range []string{
		"NAME", "VALUE", "DEFAULT", "SOURCE", "SCOPE", "DESCRIPTION",
		"IRONBARK_DATA_DIR", "/custom/data", "/tmp/ironbark/data", "env",
		"IRONBARK_LOG_LEVEL", "default",
		"IRONBARK_GITEA_TOKEN",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected text output to contain %q, got:\n%s", want, got)
		}
	}
}

// TestExecDebugSettings_text_redactsSensitiveValues verifies that the
// value of a sensitive variable is rendered as `<redacted>` and the
// real value never appears in the output.
func TestExecDebugSettings_text_redactsSensitiveValues(t *testing.T) {
	var buf bytes.Buffer
	if err := execDebugSettings(&buf, fixtureResolved(), "text", false); err != nil {
		t.Fatalf("execDebugSettings returned error: %v", err)
	}
	got := buf.String()

	if !strings.Contains(got, redactedDisplay) {
		t.Errorf("expected text output to contain %q for sensitive value, got:\n%s",
			redactedDisplay, got)
	}
	if strings.Contains(got, "super-secret-value") {
		t.Errorf("text output unexpectedly contains the raw sensitive value, got:\n%s", got)
	}
}

// -----------------------------------------------------------------------------
// TESTS: JSON renderer
// -----------------------------------------------------------------------------

// TestExecDebugSettings_json_producesValidJSONWithExpectedSchema
// verifies the JSON output round-trips through `encoding/json` and
// contains the expected fields and values, including redaction.
func TestExecDebugSettings_json_producesValidJSONWithExpectedSchema(t *testing.T) {
	var buf bytes.Buffer
	if err := execDebugSettings(&buf, fixtureResolved(), "json", false); err != nil {
		t.Fatalf("execDebugSettings returned error: %v", err)
	}

	var got []renderableSetting
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("could not unmarshal JSON output: %v\noutput:\n%s", err, buf.String())
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
	if got[0].Name != "IRONBARK_DATA_DIR" || got[0].Source != "env" || got[0].Value != "/custom/data" {
		t.Errorf("unexpected first entry: %+v", got[0])
	}
	if got[1].Source != "default" {
		t.Errorf("expected second entry source=default, got %+v", got[1])
	}
	if got[2].Value != redactedDisplay {
		t.Errorf("expected sensitive entry value to be redacted, got %q", got[2].Value)
	}
	if !got[2].Sensitive {
		t.Errorf("expected sensitive entry Sensitive=true, got %+v", got[2])
	}
	if strings.Contains(buf.String(), "super-secret-value") {
		t.Errorf("JSON output unexpectedly contains the raw sensitive value")
	}
}

// -----------------------------------------------------------------------------
// TESTS: YAML renderer
// -----------------------------------------------------------------------------

// TestExecDebugSettings_yaml_producesValidYAMLWithExpectedSchema
// verifies the YAML output round-trips through `gopkg.in/yaml.v3` and
// contains the expected fields and values, including redaction.
func TestExecDebugSettings_yaml_producesValidYAMLWithExpectedSchema(t *testing.T) {
	var buf bytes.Buffer
	if err := execDebugSettings(&buf, fixtureResolved(), "yaml", false); err != nil {
		t.Fatalf("execDebugSettings returned error: %v", err)
	}

	var got []renderableSetting
	if err := yaml.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("could not unmarshal YAML output: %v\noutput:\n%s", err, buf.String())
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
	if got[2].Value != redactedDisplay {
		t.Errorf("expected sensitive entry value to be redacted, got %q", got[2].Value)
	}
	if strings.Contains(buf.String(), "super-secret-value") {
		t.Errorf("YAML output unexpectedly contains the raw sensitive value")
	}
}

// -----------------------------------------------------------------------------
// TESTS: helpers
// -----------------------------------------------------------------------------

// TestDisplayValue_doesNotRedactEmptySensitiveValue verifies that an
// empty value for a sensitive variable is rendered as the empty
// string, not as `<redacted>`. Redacting empty values would be
// misleading (it would suggest a value is present when none is).
func TestDisplayValue_doesNotRedactEmptySensitiveValue(t *testing.T) {
	r := settings.Resolved{
		Var: settings.Var{
			Name:      "IRONBARK_GITEA_TOKEN",
			Sensitive: true,
		},
		Value:   "",
		FromEnv: false,
	}
	if got := displayValue(r); got != "" {
		t.Errorf("expected empty string for empty sensitive value, got %q", got)
	}
}

// -----------------------------------------------------------------------------
// TESTS: launcher status summary
// -----------------------------------------------------------------------------

// TestLauncherStatusSummary_notInvoked verifies the summary when the
// launcher sentinel is not set.
func TestLauncherStatusSummary_notInvoked(t *testing.T) {
	t.Setenv("IRONBARK_LAUNCHER_INVOKED", "")
	if err := settings.Init(); err != nil {
		_ = err
	}
	if got := launcherStatusSummary(); got != "Launcher invoked: no" {
		t.Errorf("expected `Launcher invoked: no`, got %q", got)
	}
}

// TestLauncherStatusSummary_invokedAllPresent verifies the summary
// when the launcher sentinel is set and every expected forwarded var
// is also set.
func TestLauncherStatusSummary_invokedAllPresent(t *testing.T) {
	t.Setenv("IRONBARK_LAUNCHER_INVOKED", "true")
	if err := settings.Init(); err != nil {
		_ = err
	}
	for _, v := range settings.All() {
		if v.Scope == settings.ScopeLauncherForwarded {
			t.Setenv(v.Name, "set")
		}
	}

	got := launcherStatusSummary()
	if !strings.Contains(got, "yes") || !strings.Contains(got, "all expected") {
		t.Errorf("expected affirmative summary, got %q", got)
	}
}

// TestLauncherStatusSummary_invokedMissingVars verifies the summary
// surfaces the validation error when one or more forwarded vars are
// missing.
func TestLauncherStatusSummary_invokedMissingVars(t *testing.T) {
	t.Setenv("IRONBARK_LAUNCHER_INVOKED", "true")
	if err := settings.Init(); err != nil {
		_ = err
	}
	for _, v := range settings.All() {
		if v.Scope == settings.ScopeLauncherForwarded {
			t.Setenv(v.Name, "")
		}
	}

	got := launcherStatusSummary()
	if !strings.Contains(got, "WARNING") {
		t.Errorf("expected summary to mention WARNING when validation fails, got %q", got)
	}
}

// TestRenderDebugSettingsText_includesLauncherSummary verifies that
// the text renderer emits the launcher summary line above the table
// header.
func TestRenderDebugSettingsText_includesLauncherSummary(t *testing.T) {
	t.Setenv("IRONBARK_LAUNCHER_INVOKED", "")
	if err := settings.Init(); err != nil {
		_ = err
	}

	var buf bytes.Buffer
	if err := execDebugSettings(&buf, fixtureResolved(), "text", false); err != nil {
		t.Fatalf("execDebugSettings returned error: %v", err)
	}
	got := buf.String()

	idxSummary := strings.Index(got, "Launcher invoked")
	idxHeader := strings.Index(got, "NAME")
	if idxSummary == -1 {
		t.Fatalf("expected text output to contain launcher summary, got:\n%s", got)
	}
	if idxHeader == -1 || idxSummary >= idxHeader {
		t.Errorf("expected launcher summary to appear before the table header, got:\n%s", got)
	}
}

// -----------------------------------------------------------------------------
// TESTS: helpers
// -----------------------------------------------------------------------------

// TestDisplaySource_returnsExpectedLabels verifies the source-label
// mapping for both the env and default cases.
func TestDisplaySource_returnsExpectedLabels(t *testing.T) {
	if displaySource(settings.Resolved{FromEnv: true}) != "env" {
		t.Error("expected `env` for FromEnv=true")
	}
	if displaySource(settings.Resolved{FromEnv: false}) != "default" {
		t.Error("expected `default` for FromEnv=false")
	}
}

// -----------------------------------------------------------------------------
// TESTS: launcher context filtering
// -----------------------------------------------------------------------------

// fixtureAllScopes returns one resolved entry per scope so the
// filtering tests exercise every code path.
func fixtureAllScopes() []settings.Resolved {
	return []settings.Resolved{
		{
			Var: settings.Var{
				Name:  "IRONBARK_DATA_DIR",
				Scope: settings.ScopeGoConsumed,
			},
		},
		{
			Var: settings.Var{
				Name:  "IRONBARK_IN_CONTAINER",
				Scope: settings.ScopeContainerSentinel,
			},
		},
		{
			Var: settings.Var{
				Name:  "IRONBARK_LAUNCHER_INVOKED",
				Scope: settings.ScopeLauncherSentinel,
			},
		},
		{
			Var: settings.Var{
				Name:  "IRONBARK_HOST_DATA_DIR",
				Scope: settings.ScopeLauncherForwarded,
			},
		},
		{
			Var: settings.Var{
				Name:  "IRONBARK_SCRIPT_CONTAINER_ENGINE",
				Scope: settings.ScopeScript,
			},
		},
	}
}

// containsName reports whether the supplied resolved slice contains an
// entry whose `Var.Name` matches `name`.
func containsName(rs []settings.Resolved, name string) bool {
	for _, r := range rs {
		if r.Var.Name == name {
			return true
		}
	}
	return false
}

// TestFilterForLauncherContext_invoked_returnsAllRows verifies that
// every row is preserved when the launcher sentinel is set.
func TestFilterForLauncherContext_invoked_returnsAllRows(t *testing.T) {
	in := fixtureAllScopes()
	got := filterForLauncherContext(in, true)
	if len(got) != len(in) {
		t.Fatalf("expected %d rows, got %d", len(in), len(got))
	}
}

// TestFilterForLauncherContext_notInvoked_dropsLauncherForwardedAndShellOnly
// verifies that launcher-forwarded and script rows are removed
// while go-consumed, container-sentinel, and launcher-sentinel rows
// are retained.
func TestFilterForLauncherContext_notInvoked_dropsLauncherForwardedAndShellOnly(t *testing.T) {
	got := filterForLauncherContext(fixtureAllScopes(), false)

	mustKeep := []string{
		"IRONBARK_DATA_DIR",
		"IRONBARK_IN_CONTAINER",
		"IRONBARK_LAUNCHER_INVOKED",
	}
	for _, name := range mustKeep {
		if !containsName(got, name) {
			t.Errorf("expected %s to be retained, got %+v", name, got)
		}
	}

	mustDrop := []string{
		"IRONBARK_HOST_DATA_DIR",
		"IRONBARK_SCRIPT_CONTAINER_ENGINE",
	}
	for _, name := range mustDrop {
		if containsName(got, name) {
			t.Errorf("expected %s to be filtered out, got %+v", name, got)
		}
	}
}

// TestExecDebugSettings_showAll_notInvoked_includesAllScopes verifies
// that passing `showAll=true` overrides the launcher-context filter so
// every known setting is rendered, even when the launcher sentinel is
// not set.
func TestExecDebugSettings_showAll_notInvoked_includesAllScopes(t *testing.T) {
	t.Setenv("IRONBARK_LAUNCHER_INVOKED", "")
	if err := settings.Init(); err != nil {
		_ = err
	}

	var buf bytes.Buffer
	if err := execDebugSettings(&buf, fixtureAllScopes(), "text", true); err != nil {
		t.Fatalf("execDebugSettings returned error: %v", err)
	}
	got := buf.String()

	mustContain := []string{
		"IRONBARK_DATA_DIR",
		"IRONBARK_IN_CONTAINER",
		"IRONBARK_LAUNCHER_INVOKED",
		"IRONBARK_HOST_DATA_DIR",
		"IRONBARK_SCRIPT_CONTAINER_ENGINE",
	}
	for _, want := range mustContain {
		if !strings.Contains(got, want) {
			t.Errorf("expected text output with --all to contain %q, got:\n%s", want, got)
		}
	}
}

// TestExecDebugSettings_text_filtered_includesNotice verifies that the
// "settings hidden" notice is rendered above the table when the
// launcher-context filter has actually removed at least one row.
func TestExecDebugSettings_text_filtered_includesNotice(t *testing.T) {
	t.Setenv("IRONBARK_LAUNCHER_INVOKED", "")
	if err := settings.Init(); err != nil {
		_ = err
	}

	var buf bytes.Buffer
	if err := execDebugSettings(&buf, fixtureAllScopes(), "text", false); err != nil {
		t.Fatalf("execDebugSettings returned error: %v", err)
	}
	got := buf.String()

	if !strings.Contains(got, "launcher-forwarded and script settings are hidden") {
		t.Errorf("expected filtered notice in output, got:\n%s", got)
	}
	if !strings.Contains(got, "--all") {
		t.Errorf("expected filtered notice to mention --all flag, got:\n%s", got)
	}
}

// TestExecDebugSettings_text_showAll_omitsNotice verifies that the
// "settings hidden" notice is NOT rendered when `--all` is passed,
// because nothing was actually hidden.
func TestExecDebugSettings_text_showAll_omitsNotice(t *testing.T) {
	t.Setenv("IRONBARK_LAUNCHER_INVOKED", "")
	if err := settings.Init(); err != nil {
		_ = err
	}

	var buf bytes.Buffer
	if err := execDebugSettings(&buf, fixtureAllScopes(), "text", true); err != nil {
		t.Fatalf("execDebugSettings returned error: %v", err)
	}
	got := buf.String()

	if strings.Contains(got, "launcher-forwarded and script settings are hidden") {
		t.Errorf("did NOT expect filtered notice when --all is passed, got:\n%s", got)
	}
}

// TestExecDebugSettings_text_noFilteringNeeded_omitsNotice verifies
// that the notice is suppressed when the input contains nothing
// filterable, even without `--all`.
func TestExecDebugSettings_text_noFilteringNeeded_omitsNotice(t *testing.T) {
	t.Setenv("IRONBARK_LAUNCHER_INVOKED", "")
	if err := settings.Init(); err != nil {
		_ = err
	}

	var buf bytes.Buffer
	if err := execDebugSettings(&buf, fixtureResolved(), "text", false); err != nil {
		t.Fatalf("execDebugSettings returned error: %v", err)
	}
	got := buf.String()

	if strings.Contains(got, "launcher-forwarded and script settings are hidden") {
		t.Errorf("did NOT expect filtered notice when no rows were filtered out, got:\n%s", got)
	}
}

// TestExecDebugSettings_text_notInvoked_omitsLauncherForwardedAndShellOnly
// verifies the end-to-end text rendering filters out the right scopes
// when the launcher sentinel is not set.
func TestExecDebugSettings_text_notInvoked_omitsLauncherForwardedAndShellOnly(t *testing.T) {
	t.Setenv("IRONBARK_LAUNCHER_INVOKED", "")
	if err := settings.Init(); err != nil {
		_ = err
	}

	var buf bytes.Buffer
	if err := execDebugSettings(&buf, fixtureAllScopes(), "text", false); err != nil {
		t.Fatalf("execDebugSettings returned error: %v", err)
	}
	got := buf.String()

	for _, want := range []string{"IRONBARK_DATA_DIR", "IRONBARK_LAUNCHER_INVOKED", "IRONBARK_IN_CONTAINER"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected text output to contain %q, got:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"IRONBARK_HOST_DATA_DIR", "IRONBARK_SCRIPT_CONTAINER_ENGINE"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("expected text output to NOT contain %q, got:\n%s", unwanted, got)
		}
	}
}
