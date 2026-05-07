/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"brightsparklabs.com/ironbark/internal/version"
	"brightsparklabs.com/ironbark/resources"

	"github.com/spf13/cobra"
)

// -----------------------------------------------------------------------------
// CONSTANTS
// -----------------------------------------------------------------------------

// launcherTemplateFile is the path (relative to the embedded resources
// directory) of the launcher script template.
const launcherTemplateFile = "launcher.sh.tmpl"

// defaultLauncherImage is the default container image used by the generated
// launcher script.
const defaultLauncherImage = "brightsparklabs/ironbark:latest"

// defaultLauncherEngine is the default container engine used by the generated
// launcher script.
const defaultLauncherEngine = "podman"

// defaultLauncherKubeconfig is the default host kubeconfig path mounted into
// the container by the generated launcher script.
const defaultLauncherKubeconfig = "${HOME}/.kube/config"

// defaultLauncherDataDir is the default host data directory mounted into the
// container by the generated launcher script.
const defaultLauncherDataDir = "${PWD}/ironbark-data"

// supportedLauncherEngines is the set of container engines supported by the
// `launcher` command.
var supportedLauncherEngines = map[string]struct{}{
	"podman": {},
	"docker": {},
}

// -----------------------------------------------------------------------------
// TYPES
// -----------------------------------------------------------------------------

// launcherTemplateData holds the values rendered into the launcher script
// template.
type launcherTemplateData struct {
	// Engine is the container engine used to run Ironbark (e.g. `podman`).
	Engine string
	// Image is the fully qualified container image reference for Ironbark.
	Image string
	// HostDataDir is the host directory which holds Ironbark application
	// data and is mounted into the container.
	HostDataDir string
	// HostKubeconfig is the host kubeconfig file mounted into the
	// container.
	HostKubeconfig string
	// NoInteractive bakes `--interactive` off as the default in the
	// generated script. Can still be re-enabled at runtime via
	// `IRONBARK_NO_INTERACTIVE=false`.
	NoInteractive bool
	// NoTTY bakes `--tty` off as the default in the generated script.
	// Can still be re-enabled at runtime via `IRONBARK_NO_TTY=false`.
	NoTTY bool
	// GeneratedAt is an ISO 8601 timestamp recording when the script was
	// generated.
	GeneratedAt string
	// IronbarkVersion is the version of the Ironbark binary that generated
	// the launcher script.
	IronbarkVersion string
	// IronbarkCommit is the short Git commit hash of the Ironbark binary
	// that generated the launcher script.
	IronbarkCommit string
	// IronbarkBuildTime is the UTC ISO 8601 timestamp recording when the
	// Ironbark binary that generated the launcher script was built.
	IronbarkBuildTime string
}

// -----------------------------------------------------------------------------
// COMMAND: ROOT
// -----------------------------------------------------------------------------

// newGenerateCmd creates the `generate` command which groups subcommands that
// emit generated/templated files (e.g. launcher scripts) to standard output.
func newGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "generate",
		Aliases: []string{"gen"},
		Short:   "Generate files (e.g. launcher scripts) and write them to stdout",
		Long: `Generate templated files and write them to standard output.

Each subcommand renders an embedded template using the supplied flags and
writes the result to standard output. The output is intended to be redirected
to a file (or piped into another tool) by the caller.
`,
	}

	cmd.AddCommand(newGenerateLauncherCmd())

	return cmd
}

// -----------------------------------------------------------------------------
// COMMAND: launcher
// -----------------------------------------------------------------------------

// newGenerateLauncherCmd creates the `generate launcher` command which
// generates a wrapper script for invoking Ironbark via a container engine.
func newGenerateLauncherCmd() *cobra.Command {
	var (
		engine         string
		image          string
		hostDataDir    string
		hostKubeconfig string
		noInteractive  bool
		noTTY          bool
	)

	cmd := &cobra.Command{
		Use:   "launcher",
		Short: "Generate a launcher script for invoking Ironbark via a container engine",
		Long: `Generate a launcher script for invoking Ironbark via a container engine.

The generated script is written to standard output and can be redirected to a
file (typically ` + "`ironbark`" + ` on your $PATH) to provide a convenient
wrapper around the underlying ` + "`podman run`" + ` (or ` + "`docker run`" + `)
invocation.

The generated script:
  - Attempts to create the host data directory if it does not exist; if the
    creation fails, it reports the error and exits with a non-zero status.
  - Sets ` + "`IRONBARK_IN_CONTAINER=true`" + ` inside the container so code can
    detect that it is running via the launcher.
  - Forwards a number of useful host details into the container as
    ` + "`IRONBARK_HOST_*`" + ` environment variables (user, uid, gid, hostname,
    pwd, os, arch, container engine, launcher generated-at).
  - Honours ` + "`IRONBARK_NO_INTERACTIVE`" + ` / ` + "`IRONBARK_NO_TTY`" + ` at
    runtime: define either variable to ANY value to disable
    ` + "`--interactive`" + ` / ` + "`--tty`" + `.
  - Honours ` + "`IRONBARK_VERBOSE`" + `: define it to ANY value to print the
    resolved launcher settings to stderr before invoking the container engine.

Example:

  ironbark generate launcher > ironbark
  chmod +x ironbark
  ./ironbark init all
`,
		// Suppress cobra's automatic usage/error reprint as `Execute` already
		// surfaces errors via panic.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return execLauncher(launcherTemplateData{
				Engine:         engine,
				Image:          image,
				HostDataDir:    hostDataDir,
				HostKubeconfig: hostKubeconfig,
				NoInteractive:  noInteractive,
				NoTTY:          noTTY,
			})
		},
	}

	cmd.Flags().StringVarP(&engine, "container-engine", "e", defaultLauncherEngine,
		"Container engine to use (`podman` or `docker`)")
	cmd.Flags().StringVarP(&image, "image", "i", defaultLauncherImage,
		"Container image reference for Ironbark")
	cmd.Flags().StringVarP(&hostDataDir, "data-dir", "d", defaultLauncherDataDir,
		"Host directory to mount as the Ironbark data directory")
	cmd.Flags().StringVarP(&hostKubeconfig, "kubeconfig", "k", defaultLauncherKubeconfig,
		"Host kubeconfig file to mount into the container")
	cmd.Flags().BoolVar(&noInteractive, "no-interactive", false,
		"Bake `--interactive` off as the default in the generated script")
	cmd.Flags().BoolVar(&noTTY, "no-tty", false,
		"Bake `--tty` off as the default in the generated script")

	return cmd
}

// execLauncher renders the launcher script template and writes the result to
// standard output.
func execLauncher(data launcherTemplateData) error {
	normalisedEngine := strings.ToLower(strings.TrimSpace(data.Engine))
	if _, ok := supportedLauncherEngines[normalisedEngine]; !ok {
		return fmt.Errorf("unsupported container engine %q (must be one of: podman, docker)", data.Engine)
	}
	data.Engine = normalisedEngine
	data.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	data.IronbarkVersion = version.GetVersion()
	data.IronbarkCommit = version.GetCommit()
	data.IronbarkBuildTime = version.GetBuildTime()

	tmpl, err := resources.LoadTemplate(launcherTemplateFile)
	if err != nil {
		return fmt.Errorf("could not load launcher template: %w", err)
	}

	if err := tmpl.Execute(os.Stdout, data); err != nil {
		return fmt.Errorf("could not render launcher template: %w", err)
	}

	return nil
}
