/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"brightsparklabs.com/ironbark/internal/settings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// -----------------------------------------------------------------------------
// CONSTANTS
// -----------------------------------------------------------------------------

// redactedDisplay is the placeholder rendered in place of the value of
// any `Var` whose `Sensitive` flag is set. Chosen to be visually
// obvious and unmistakable for a real value.
const redactedDisplay = "<REDACTED>"

// supportedDebugSettingsFormats is the set of output formats accepted
// by `debug settings --format`.
var supportedDebugSettingsFormats = map[string]struct{}{
	"text": {},
	"json": {},
	"yaml": {},
}

// -----------------------------------------------------------------------------
// COMMAND: debug
// -----------------------------------------------------------------------------

// newDebugCmd creates the `debug` parent command, which groups
// diagnostic subcommands intended for support and troubleshooting (as
// opposed to the production-facing `init` / `argo` commands).
func newDebugCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "debug",
		Short: "Diagnostic commands for inspecting Ironbark's runtime state",
		Long: `Diagnostic commands for inspecting Ironbark's runtime state.

These commands are intended for support and troubleshooting. They never
modify any external system; they only read and print information that
Ironbark already has access to.
`,
	}

	cmd.AddCommand(newDebugSettingsCmd())

	return cmd
}

// -----------------------------------------------------------------------------
// COMMAND: debug settings
// -----------------------------------------------------------------------------

// newDebugSettingsCmd creates the `debug settings` command, which
// renders the full catalogue of IRONBARK_* environment variables along
// with their current effective values, declared defaults, and
// descriptions.
func newDebugSettingsCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "settings",
		Short: "Print the current Ironbark settings (env vars, defaults, sources)",
		Long: `Print the current Ironbark settings.

Renders every IRONBARK_* environment variable Ironbark knows about,
including its current effective value, declared default, source (env or
default), scope (which subsystem consumes it), and a short description.

The default output is a human-readable table; pass ` + "`--format json`" + `
or ` + "`--format yaml`" + ` for machine-readable output.

Variables marked sensitive (currently none) are rendered with their
value redacted.
`,
		// Suppress cobra's automatic usage/error reprint as `Execute` already
		// surfaces errors via panic.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return execDebugSettings(os.Stdout, settings.ResolveAll(), format)
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", "text",
		"Output format (`text`, `json`, or `yaml`)")

	return cmd
}

// execDebugSettings renders the supplied resolved settings to `out` in
// the requested format. Extracted from the cobra command for direct
// unit testing.
func execDebugSettings(out io.Writer, resolved []settings.Resolved, format string) error {
	normalised := strings.ToLower(strings.TrimSpace(format))
	if _, ok := supportedDebugSettingsFormats[normalised]; !ok {
		return fmt.Errorf("unsupported format %q (must be one of: text, json, yaml)", format)
	}

	switch normalised {
	case "text":
		return renderDebugSettingsText(out, resolved)
	case "json":
		return renderDebugSettingsJSON(out, resolved)
	case "yaml":
		return renderDebugSettingsYAML(out, resolved)
	}

	// Unreachable - the validation above narrows `normalised` to one of
	// the three formats handled in the switch. Keep a defensive guard
	// to satisfy the compiler's exhaustiveness intuition and to fail
	// loudly if the validation set and the switch ever drift.
	return fmt.Errorf("unhandled format %q", normalised)
}

// -----------------------------------------------------------------------------
// RENDERERS
// -----------------------------------------------------------------------------

// renderDebugSettingsText renders the resolved settings as a
// human-readable, tab-aligned table. Columns: NAME, VALUE, DEFAULT,
// SOURCE, SCOPE, DESCRIPTION.
//
// A short summary line is printed above the table reporting whether
// Ironbark was invoked via the launcher and, if so, whether all
// expected `IRONBARK_HOST_*` variables are present. The summary lets
// operators spot launcher misconfiguration at a glance without having
// to scan every row of the catalogue.
func renderDebugSettingsText(out io.Writer, resolved []settings.Resolved) error {
	if _, err := fmt.Fprintln(out, launcherStatusSummary()); err != nil {
		return fmt.Errorf("could not write launcher status summary: %w", err)
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return fmt.Errorf("could not write blank line after launcher summary: %w", err)
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "NAME\tVALUE\tDEFAULT\tSOURCE\tSCOPE\tDESCRIPTION"); err != nil {
		return fmt.Errorf("could not write debug settings header: %w", err)
	}
	for _, r := range resolved {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Var.Name,
			displayValue(r),
			displayDefault(r.Var),
			displaySource(r),
			r.Var.Scope,
			r.Var.Description,
		); err != nil {
			return fmt.Errorf("could not write debug settings row for %s: %w", r.Var.Name, err)
		}
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("could not flush debug settings table: %w", err)
	}
	return nil
}

// renderDebugSettingsJSON renders the resolved settings as a pretty-
// printed JSON array. Each element exposes the same fields as the text
// table (the schema is stable and intended for consumption by other
// tools).
func renderDebugSettingsJSON(out io.Writer, resolved []settings.Resolved) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(toRenderable(resolved)); err != nil {
		return fmt.Errorf("could not encode debug settings as JSON: %w", err)
	}
	return nil
}

// renderDebugSettingsYAML renders the resolved settings as YAML.
// Schema matches `renderDebugSettingsJSON`.
func renderDebugSettingsYAML(out io.Writer, resolved []settings.Resolved) error {
	encoder := yaml.NewEncoder(out)
	defer encoder.Close()
	encoder.SetIndent(2)
	if err := encoder.Encode(toRenderable(resolved)); err != nil {
		return fmt.Errorf("could not encode debug settings as YAML: %w", err)
	}
	return nil
}

// renderableSetting is the wire-format struct used for JSON and YAML
// output. Kept separate from `settings.Resolved` so the on-the-wire
// schema is stable even if the in-memory types evolve.
type renderableSetting struct {
	Name        string `json:"name"        yaml:"name"`
	Value       string `json:"value"       yaml:"value"`
	Default     string `json:"default"     yaml:"default"`
	Source      string `json:"source"      yaml:"source"`
	Scope       string `json:"scope"       yaml:"scope"`
	Description string `json:"description" yaml:"description"`
	Sensitive   bool   `json:"sensitive"   yaml:"sensitive"`
}

// toRenderable converts the in-memory `Resolved` slice to the
// stable wire-format `renderableSetting` slice, applying redaction.
func toRenderable(resolved []settings.Resolved) []renderableSetting {
	out := make([]renderableSetting, len(resolved))
	for i, r := range resolved {
		out[i] = renderableSetting{
			Name:        r.Var.Name,
			Value:       displayValue(r),
			Default:     r.Var.Default,
			Source:      displaySource(r),
			Scope:       string(r.Var.Scope),
			Description: r.Var.Description,
			Sensitive:   r.Var.Sensitive,
		}
	}
	return out
}

// -----------------------------------------------------------------------------
// HELPERS
// -----------------------------------------------------------------------------

// displayValue returns the string to render in place of the resolved
// value, applying redaction when the variable is marked sensitive.
func displayValue(r settings.Resolved) string {
	if r.Var.Sensitive && r.Value != "" {
		return redactedDisplay
	}
	return r.Value
}

// displayDefault returns the string to render in the DEFAULT column.
// Unlike values, defaults are not redacted (they are part of the
// catalogue contract and never carry secrets).
func displayDefault(v Var) string {
	return v.Default
}

// displaySource returns the human-readable label for the source of a
// resolved value.
func displaySource(r settings.Resolved) string {
	if r.FromEnv {
		return "env"
	}
	return "default"
}

// Var is an alias to `settings.Var` so the helpers above can refer to
// the catalogue type without forcing every caller to import the
// `settings` package.
type Var = settings.Var

// launcherStatusSummary returns the one-line "Launcher invoked: ..."
// banner shown above the text-format settings table. Uses
// `settings.LauncherInvoked` and `settings.ValidateLauncherEnv` so the
// summary is always consistent with what the underlying validator
// would have done.
func launcherStatusSummary() string {
	if !settings.LauncherInvoked() {
		return "Launcher invoked: no"
	}

	if err := settings.ValidateLauncherEnv(); err != nil {
		return fmt.Sprintf(
			"Launcher invoked: yes (WARNING: validation failed: %s)",
			err.Error(),
		)
	}

	return "Launcher invoked: yes (all expected variables present)"
}
