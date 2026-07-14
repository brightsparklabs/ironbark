/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

// This file implements the `ironbark exec` command, which exposes
// the tools bundled into the Ironbark container image as first-class
// subcommands so operators can invoke them without entering the
// container manually.
//
// Tools are auto-discovered at startup by walking `/app/bin/`; the
// exact set therefore reflects whatever the image ships rather than
// a hard-coded list. Run `ironbark exec --help` to see what is
// available.
//
// In addition to the bundled tools, a runtime probe checks for shell
// binaries (`bash`, `sh`) in well-known locations and registers each
// present one as a subcommand by basename. The probe deliberately
// avoids any compile-time assumption that a shell is present, so the
// command remains correct on a stripped-down (e.g. SCRATCH) image
// that ships no shell.
//
// The implementation `execve`s into the requested tool, so signal
// handling, exit codes and stdio behave exactly as if the tool were
// invoked directly.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/spf13/cobra"
)

// -----------------------------------------------------------------------------
// CONSTANTS
// -----------------------------------------------------------------------------

// execToolsDir is the absolute path on the container filesystem
// where bundled tools live. Auto-discovery walks this directory and
// registers each executable entry as a child of `ironbark exec`,
// regardless of what tools the image happens to ship.
const execToolsDir = "/app/bin"

// -----------------------------------------------------------------------------
// VARIABLES
// -----------------------------------------------------------------------------

// execToolBlocklist is the set of tool names which must NOT be
// exposed via `ironbark exec`, even if present in `execToolsDir`.
// Currently this is just the Ironbark binary itself, to prevent
// confusing self-invocation patterns like
// `ironbark exec ironbark ...`.
var execToolBlocklist = map[string]struct{}{
	"ironbark": {},
}

// execShellCandidates is the ordered list of shell binaries to probe
// for when registering interactive-shell subcommands. Each candidate
// that exists and is executable is registered as a subcommand named
// after its basename (e.g. `ironbark exec bash`). The list is
// intentionally short and only covers POSIX-standard locations so a
// stripped-down (e.g. SCRATCH) image produces no shell subcommands
// rather than silently surfacing a wrong path.
var execShellCandidates = []string{
	"/bin/bash",
	"/usr/bin/bash",
	"/bin/sh",
	"/usr/bin/sh",
}

// -----------------------------------------------------------------------------
// PUBLIC FUNCTIONS
// -----------------------------------------------------------------------------

// newExecCmd creates the `ironbark exec` command and registers one
// child subcommand per tool auto-discovered in `execToolsDir`, plus
// one child per shell binary probed via `execShellCandidates`.
//
// Registration happens at command construction time so the tools
// appear in `ironbark exec --help` output.
func newExecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec <tool> [args...]",
		Short: "Invoke a tool bundled in the Ironbark container image",
		Long: `Invoke a tool bundled in the Ironbark container image.

Tools shipped under /app/bin/ are auto-discovered at startup and
surfaced as subcommands of ` + "`ironbark exec`" + `, so operators can
invoke them without having to enter the container manually. The
list below reflects what the current image ships - it is not a
fixed set; future image revisions may add or remove tools.

If the image also ships a shell binary (bash or sh), it is
registered as a subcommand under its own name, useful for one-off
troubleshooting or ad-hoc commands.

The exact set of available subcommands is listed under "Available
Commands" in the help output.
`,
		// `ironbark exec` with no tool name (or an unknown tool
		// name) must fail loudly rather than silently dispatching
		// elsewhere. `cobra.NoArgs` rejects positional args
		// (including unknown subcommand names if cobra cannot
		// resolve them), so unknown tools surface cobra's standard
		// "unknown command" error.
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Reached only when no subcommand is supplied (cobra
			// dispatches to the matching subcommand before calling
			// `RunE` of the parent).
			return NewUserError("no tool specified; run `ironbark exec --help` for available tools")
		},
	}

	// Register one subcommand per discovered bundled tool in
	// `/app/bin/`. The result is sorted so `--help` output is stable.
	for _, tool := range discoverExecTools(execToolsDir, execToolBlocklist) {
		cmd.AddCommand(newExecToolCmd(tool, filepath.Join(execToolsDir, tool)))
	}

	// Register one subcommand per discovered shell binary. Shells
	// live outside `/app/bin/` so they need a separate probe.
	for _, shell := range discoverExecShells(execShellCandidates) {
		cmd.AddCommand(newExecToolCmd(filepath.Base(shell), shell))
	}

	return cmd
}

// -----------------------------------------------------------------------------
// PRIVATE FUNCTIONS
// -----------------------------------------------------------------------------

// discoverExecTools walks `dir` and returns the names of every
// executable regular file (or symlink to one) found there, excluding
// entries listed in `blocklist`. The returned slice is sorted for
// deterministic output.
//
// Missing or unreadable directories return an empty slice (rather
// than an error), so the command remains usable in test or
// development environments where `/app/bin` does not exist.
func discoverExecTools(dir string, blocklist map[string]struct{}) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var tools []string
	for _, entry := range entries {
		name := entry.Name()
		if _, blocked := blocklist[name]; blocked {
			continue
		}

		// Resolve the entry to its underlying file info so symlinks
		// are followed when checking the executable bit.
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		// Require at least one execute bit (owner / group / other).
		if info.Mode().Perm()&0o111 == 0 {
			continue
		}

		tools = append(tools, name)
	}

	sort.Strings(tools)
	return tools
}

// discoverExecShells walks `candidates` and returns the absolute path
// of every entry that exists and is executable. The first candidate
// encountered per basename wins (e.g. if both `/bin/bash` and
// `/usr/bin/bash` exist, only `/bin/bash` is returned), so each
// basename appears at most once.
//
// Returns an empty slice if no candidate is present - which is the
// expected outcome for a SCRATCH-based image that ships no shell.
func discoverExecShells(candidates []string) []string {
	seen := make(map[string]struct{})
	var found []string
	for _, path := range candidates {
		base := filepath.Base(path)
		if _, already := seen[base]; already {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if info.Mode().Perm()&0o111 == 0 {
			continue
		}
		seen[base] = struct{}{}
		found = append(found, path)
	}
	sort.Strings(found)
	return found
}

// newExecToolCmd creates a `ironbark exec <name>` subcommand that
// exec's into `binary` when invoked. All arguments after the
// subcommand name are forwarded verbatim, so flag parsing is
// disabled (the tool owns its own flags).
//
// `name` is the user-facing subcommand name (typically the basename
// of `binary`); separating it allows shell binaries to be registered
// by basename even though their absolute path differs from
// `/app/bin/`.
func newExecToolCmd(name, binary string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: fmt.Sprintf("Invoke the bundled `%s` tool", name),
		Long: fmt.Sprintf(`Invoke the bundled %q tool with the supplied arguments.

All arguments are forwarded verbatim to the underlying binary, so
the tool's own flags (including --help) work as expected. The tool
is exec'd into the current process: signal handling, exit codes and
stdio behave exactly as if you had invoked the tool directly.

Resolved binary: %s
`, name, binary),
		// Forward every following arg verbatim; the underlying tool
		// is responsible for its own flag parsing.
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return execIntoTool(binary, args)
		},
	}
}

// execIntoTool replaces the current process with the supplied
// executable, forwarding the current environment and the supplied
// arguments. On success, this function does not return; on failure
// (e.g. file not found, permission denied) a `UserError` is returned
// so the top-level handler renders it concisely.
func execIntoTool(binary string, args []string) error {
	// Sanity-check existence before attempting the syscall so the
	// error message is clearer than the raw `no such file or
	// directory` from `execve`.
	if _, err := os.Stat(binary); err != nil {
		if os.IsNotExist(err) {
			return NewUserError("tool not found at %s; is this binary built into the Ironbark image?", binary)
		}
		return WrapUserError(err, "could not stat %s", binary)
	}

	// `syscall.Exec` requires argv[0] to be the program name (by
	// convention), followed by the actual command-line arguments.
	argv := append([]string{filepath.Base(binary)}, args...)

	if err := syscall.Exec(binary, argv, os.Environ()); err != nil {
		return WrapUserError(err, "could not exec %s", binary)
	}
	// Unreachable: `syscall.Exec` either replaces the process or
	// returns an error.
	return nil
}
