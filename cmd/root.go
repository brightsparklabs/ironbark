/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package cmd

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// -----------------------------------------------------------------------------
// CONSTANTS
// -----------------------------------------------------------------------------

// Exit codes used by the CLI. The choice follows the POSIX convention
// where 1 signals a general error and 2 signals misuse / unexpected
// condition.
const (
	// exitCodeUserError is returned when a `UserError` reaches the top
	// of the call stack. The error message is printed to stderr
	// without a stack trace and the process exits with this code.
	exitCodeUserError = 1

	// exitCodeInternalError is returned for any error reaching the top
	// of the call stack that is NOT classified as a `UserError`. The
	// error is logged via `slog` (which includes structured context
	// useful for triage) and the process exits with this code.
	exitCodeInternalError = 2
)

// -----------------------------------------------------------------------------
// VARIABLES
// -----------------------------------------------------------------------------

// rootCmd represents the base command when called without any subcommands.
//
// Note: launcher-environment validation runs in `settings.Init` (called
// from `main`), so by the time any subcommand executes the
// `IRONBARK_HOST_*` invariants have already been enforced.
var rootCmd = &cobra.Command{
	Use:   "ironbark",
	Short: "Kubernetes management using the brightSPARK Labs opinionated deployment pattern",

	// Silence cobra's automatic usage/error reprinting at the root.
	// Subcommands inherit these defaults, so the top-level handler in
	// `Execute` is the single source of truth for how errors are
	// rendered to the user.
	SilenceUsage:  true,
	SilenceErrors: true,
}

// -----------------------------------------------------------------------------
// PUBLIC FUNCTIONS
// -----------------------------------------------------------------------------

// Execute adds all child commands to the root command and dispatches.
// On error, prints a user-friendly message to stderr (no stack trace
// for `UserError`s, structured slog output otherwise) and exits the
// process with the appropriate code.
//
// This is the function `main` calls.
func Execute() {
	exitCode := ExecuteWithDeps(os.Stderr)
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

// ExecuteWithDeps is the testable entry point used by `Execute`. It
// returns the exit code instead of calling `os.Exit` directly, so
// tests can drive the CLI in-process and assert on the resulting
// stderr stream + exit code without spawning a subprocess.
//
// `stderr` receives both the user-facing error message (for
// `UserError`s) and the structured slog output (for internal errors).
// Callers wanting to capture each separately can wrap the writer.
func ExecuteWithDeps(stderr io.Writer) int {
	registerSubcommands()

	err := rootCmd.Execute()
	if err == nil {
		return 0
	}

	return reportError(stderr, err)
}

// -----------------------------------------------------------------------------
// PRIVATE FUNCTIONS
// -----------------------------------------------------------------------------

// registerSubcommands wires every subcommand onto `rootCmd`.
// Extracted from `Execute` so the test entry point and the production
// entry point share the same registration logic.
//
// Guard against double-registration when `ExecuteWithDeps` is invoked
// more than once in the same process (e.g. across table-driven
// tests).
func registerSubcommands() {
	if hasSubcommand(rootCmd, "version") {
		return
	}
	rootCmd.AddCommand(newArgoCmd())
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newGenerateCmd())
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newDebugCmd())
	rootCmd.AddCommand(newDocsCmd())
	rootCmd.AddCommand(newExecCmd())
	rootCmd.AddCommand(newServeCmd())
}

// hasSubcommand returns true if `parent` already has a direct child
// named `name`.
func hasSubcommand(parent *cobra.Command, name string) bool {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return true
		}
	}
	return false
}

// reportError classifies `err` and writes the user-facing rendering
// to `stderr`. Returns the exit code that should be returned to the
// shell.
func reportError(stderr io.Writer, err error) int {
	// User errors and cobra's own validation errors (e.g. "unknown
	// flag", "unknown command", "required flag not set") get the
	// short stderr treatment.
	if IsUserError(err) || isCobraUsageError(err) {
		fmt.Fprintf(stderr, "Error: %s\n", err.Error())
		return exitCodeUserError
	}

	// Anything else is treated as an internal/unexpected failure.
	// Surface via slog so structured context is preserved.
	slog.Error("Unexpected error", "err", err)
	return exitCodeInternalError
}

// isCobraUsageError heuristically detects errors raised by cobra/pflag
// during command-line parsing (as opposed to errors returned by a
// `RunE`). These are always user-actionable, so should be rendered as
// concise messages rather than as internal failures.
//
// Cobra does not expose a typed error for these, so a small set of
// well-known prefixes is matched. Adjust as cobra evolves.
func isCobraUsageError(err error) bool {
	msg := err.Error()
	prefixes := []string{
		"unknown command",
		"unknown flag",
		"unknown shorthand flag",
		"flag needs an argument",
		"invalid argument",
		"required flag(s)",
		"requires at least",
		"accepts ",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(msg, p) {
			return true
		}
	}
	return false
}
