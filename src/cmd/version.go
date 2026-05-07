/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"brightsparklabs.com/ironbark/internal/version"

	"github.com/spf13/cobra"
)

// -----------------------------------------------------------------------------
// COMMAND: version
// -----------------------------------------------------------------------------

// versionInfo is the JSON-serialisable view of the build-time metadata.
type versionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
}

// newVersionCmd creates the `version` command which prints the build-time
// metadata of the Ironbark binary.
//
// By default all three fields (version, commit, build time) are printed in a
// human-friendly multi-line format. Convenience flags are provided for
// retrieving an individual field (handy for scripting) and for emitting JSON.
func newVersionCmd() *cobra.Command {
	var (
		short     bool
		commit    bool
		buildTime bool
		asJSON    bool
	)

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the Ironbark version, commit and build time",
		Long: `Print build-time metadata of the Ironbark binary.

By default all three fields are printed in a human-friendly multi-line
format. Use the convenience flags to print just one field (handy in shell
pipelines), or ` + "`--json`" + ` to emit a machine-readable JSON object.

Examples:

  ironbark version
  ironbark version --short
  ironbark version --commit
  ironbark version --build-time
  ironbark version --json
`,
		// Suppress cobra's automatic usage/error reprint as `Execute` already
		// surfaces errors via panic.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return execVersion(os.Stdout, versionOptions{
				short:     short,
				commit:    commit,
				buildTime: buildTime,
				asJSON:    asJSON,
			})
		},
	}

	cmd.Flags().BoolVarP(&short, "short", "s", false,
		"Print only the version string")
	cmd.Flags().BoolVar(&commit, "commit", false,
		"Print only the short Git commit hash")
	cmd.Flags().BoolVar(&buildTime, "build-time", false,
		"Print only the UTC build timestamp")
	cmd.Flags().BoolVar(&asJSON, "json", false,
		"Print all fields as a JSON object")

	// Single-field selection flags are mutually exclusive with each other
	// and with `--json`.
	cmd.MarkFlagsMutuallyExclusive("short", "commit", "build-time", "json")

	return cmd
}

// versionOptions holds the user-supplied flags for the `version` command.
// Mutually-exclusive flag enforcement is handled by Cobra at the command
// definition level.
type versionOptions struct {
	short     bool
	commit    bool
	buildTime bool
	asJSON    bool
}

// execVersion writes the requested version field(s) to the supplied writer.
// Reads build metadata from the `internal/version` package.
func execVersion(out io.Writer, opts versionOptions) error {
	info := versionInfo{
		Version:   version.GetVersion(),
		Commit:    version.GetCommit(),
		BuildTime: version.GetBuildTime(),
	}

	switch {
	case opts.short:
		fmt.Fprintln(out, info.Version)
	case opts.commit:
		fmt.Fprintln(out, info.Commit)
	case opts.buildTime:
		fmt.Fprintln(out, info.BuildTime)
	case opts.asJSON:
		encoded, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return fmt.Errorf("could not marshal version info to JSON: %w", err)
		}
		fmt.Fprintln(out, string(encoded))
	default:
		fmt.Fprintln(out, "ironbark")
		fmt.Fprintf(out, "  version:    %s\n", info.Version)
		fmt.Fprintf(out, "  commit:     %s\n", info.Commit)
		fmt.Fprintf(out, "  build time: %s\n", info.BuildTime)
	}

	return nil
}
