/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// zarfBootstrapTemplateFile is the path (relative to the embedded
// resources directory) of the Zarf bootstrap script template.
const zarfBootstrapTemplateFile = "zarf-bootstrap.sh.tmpl"

// fapolicyRulesTemplateFile is the path (relative to the embedded resources
// directory) of the fapolicyd rules template.
const fapolicyRulesTemplateFile = "fapolicy-rules.tmpl"

// installerTemplateFile is the path (relative to the embedded resources
// directory) of the installer script template.
const installerTemplateFile = "installer.sh.tmpl"

// defaultInstallerBaseDir is the default base directory under which the
// installer will create the per-release-environment subdirectory.
const defaultInstallerBaseDir = "/opt/brightsparklabs/ironbark"

// defaultInstallerReleaseEnvironment is the default release environment
// the installer targets when `--release-environment` is not supplied.
const defaultInstallerReleaseEnvironment = "production"

// defaultLauncherImage is the default container image used by the generated
// launcher script.
const defaultLauncherImage = "brightsparklabs/ironbark:latest"

// defaultLauncherEngine is the default container engine used by the generated
// launcher script.
const defaultLauncherEngine = "podman"

// defaultLauncherKubeconfig is the default host kubeconfig path mounted into
// the container by the generated launcher script.
const defaultLauncherKubeconfig = "${HOME}/.kube/config"

// defaultLauncherDataDir is the default host data directory mounted into
// the container by the generated launcher script. Aligns with the
// canonical Ironbark install layout under `/opt/brightsparklabs/ironbark`
// so a default-flag launcher install works out of the box.
const defaultLauncherDataDir = "/opt/brightsparklabs/ironbark/production/data"

// defaultZarfBootstrapSourcePath is the default path inside the
// Ironbark container which contains the `zarf` CLI and the
// `zarf-init-*.tar.zst` package.
//
// IMPORTANT: this constant must stay in lockstep with the path that the
// Ironbark `Dockerfile` writes the Zarf assets to (see the
// `WORKDIR /build/resources/zarf/init` block in the tooling builder
// stage and the final-stage `COPY --from=builder-tooling /build/ .` that
// transplants it under `/app/`). If the Dockerfile path changes, this
// constant MUST be updated to match (and vice versa) - the
// `generate zarf-bootstrap` asset-existence check will catch
// mismatches at runtime, but the default should be kept correct so
// operators do not have to override `--source-path`.
const defaultZarfBootstrapSourcePath = "/app/resources/zarf/init"

// defaultZarfBootstrapOutputDir is the default host directory which
// will receive the extracted Zarf assets when the bootstrap script is
// run. Lives under the launcher's default data directory
// (`defaultLauncherDataDir`) so the operator does not have to mount or
// specify a separate location: a default-flag `ironbark generate
// zarf-bootstrap` simply drops the assets into the same directory the
// container already has access to.
//
// IMPORTANT: must stay aligned with `defaultLauncherDataDir` (the parent
// directory) and with `defaultFapolicyZarfPath` (the resulting `zarf`
// binary path used by the default fapolicyd rules).
const defaultZarfBootstrapOutputDir = "/opt/brightsparklabs/ironbark/production/data/zarf/init"

// defaultFapolicyZarfPath is the default host path of the extracted
// `zarf` CLI used in the generated fapolicyd rules. Aligns with
// `defaultZarfBootstrapOutputDir` (`<output-dir>/zarf`).
const defaultFapolicyZarfPath = "/opt/brightsparklabs/ironbark/production/data/zarf/init/zarf"

// defaultFapolicyK3sPath is the default host path of the installed `k3s`
// CLI used in the generated fapolicyd rules. Matches the standard K3s
// installation location.
const defaultFapolicyK3sPath = "/usr/local/bin/k3s"

// zarfBinaryName is the name of the Zarf CLI expected to be present at
// `defaultZarfBootstrapSourcePath`. Used by the asset-existence
// precondition check.
//
// IMPORTANT: this name is dictated by what the Ironbark `Dockerfile`
// places in the source directory (see the `RUN ln /build/bin/zarf`
// line). Keep it in lockstep with the Dockerfile.
const zarfBinaryName = "zarf"

// zarfInitPackagePrefix is the filename prefix of the Zarf init package
// baked into the container at `defaultZarfBootstrapSourcePath` (e.g.
// `zarf-init-amd64-v0.64.0.tar.zst`). Used by the asset-existence
// precondition check.
//
// IMPORTANT: this prefix is dictated by what the Ironbark `Dockerfile`
// downloads (see the `ADD ... zarf-init-${ARCH}-${ZARF_VERSION}.tar.zst`
// line). Keep it in lockstep with the Dockerfile.
const zarfInitPackagePrefix = "zarf-init-"

// zarfInitPackageSuffix is the filename suffix of the Zarf init package.
//
// IMPORTANT: this suffix is dictated by what the Ironbark `Dockerfile`
// downloads (see the `ADD ... zarf-init-${ARCH}-${ZARF_VERSION}.tar.zst`
// line). Keep it in lockstep with the Dockerfile.
const zarfInitPackageSuffix = ".tar.zst"

// supportedLauncherEngines is the set of container engines supported by the
// `launcher` and `zarf-bootstrap` commands.
var supportedLauncherEngines = map[string]struct{}{
	"podman": {},
	"docker": {},
}

// zarfBootstrapAssetsPresent is the indirection used by
// `execZarfBootstrap` to verify the Zarf assets exist at the configured
// source path. Defaults to `defaultZarfAssetsPresent` which inspects the
// real filesystem. Tests can override this to simulate the assets being
// present (or absent) without writing files to disk.
var zarfBootstrapAssetsPresent = defaultZarfAssetsPresent

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
	// `IRONBARK_SCRIPT_NO_INTERACTIVE=false`.
	NoInteractive bool
	// NoTTY bakes `--tty` off as the default in the generated script.
	// Can still be re-enabled at runtime via `IRONBARK_SCRIPT_NO_TTY=false`.
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

// zarfBootstrapTemplateData holds the values rendered into the Zarf
// bootstrap script template.
type zarfBootstrapTemplateData struct {
	// Engine is the container engine used to extract Zarf from the
	// Ironbark image (e.g. `podman`).
	Engine string
	// Image is the fully qualified container image reference for Ironbark.
	Image string
	// SourcePath is the path inside the container which contains the
	// `zarf` CLI and the `zarf-init-*.tar.zst` package.
	SourcePath string
	// OutputDir is the host directory which will receive the extracted
	// Zarf assets.
	OutputDir string
	// GeneratedAt is an ISO 8601 timestamp recording when the script was
	// generated.
	GeneratedAt string
	// IronbarkVersion is the version of the Ironbark binary that
	// generated the bootstrap script.
	IronbarkVersion string
	// IronbarkCommit is the short Git commit hash of the Ironbark binary
	// that generated the bootstrap script.
	IronbarkCommit string
	// IronbarkBuildTime is the UTC ISO 8601 timestamp recording when the
	// Ironbark binary that generated the bootstrap script was built.
	IronbarkBuildTime string
}

// installerTemplateData holds the values rendered into the installer
// script template.
type installerTemplateData struct {
	// InstallBaseDir is the base directory under which the
	// per-release-environment install directory is created (e.g.
	// `/opt/brightsparklabs/ironbark`).
	InstallBaseDir string
	// ReleaseEnvironment is the release environment the installer
	// targets (e.g. `production`).
	ReleaseEnvironment string
	// InstallDir is the final per-release-environment install directory
	// (`<InstallBaseDir>/<ReleaseEnvironment>`).
	InstallDir string
	// LauncherScript is the fully-rendered launcher script (output of
	// `execLauncher`) inlined into the installer's heredoc.
	LauncherScript string
	// GeneratedAt is an ISO 8601 timestamp recording when the script was
	// generated.
	GeneratedAt string
	// IronbarkVersion is the version of the Ironbark binary that
	// generated the installer script.
	IronbarkVersion string
	// IronbarkCommit is the short Git commit hash of the Ironbark binary
	// that generated the installer script.
	IronbarkCommit string
	// IronbarkBuildTime is the UTC ISO 8601 timestamp recording when the
	// Ironbark binary that generated the installer script was built.
	IronbarkBuildTime string
}

// fapolicyRulesTemplateData holds the values rendered into the fapolicyd
// rules template.
type fapolicyRulesTemplateData struct {
	// ZarfPath is the absolute host path of the extracted `zarf` CLI.
	ZarfPath string
	// K3sPath is the absolute host path of the installed `k3s` CLI.
	K3sPath string
	// GeneratedAt is an ISO 8601 timestamp recording when the rules file
	// was generated.
	GeneratedAt string
	// IronbarkVersion is the version of the Ironbark binary that
	// generated the rules file.
	IronbarkVersion string
	// IronbarkCommit is the short Git commit hash of the Ironbark binary
	// that generated the rules file.
	IronbarkCommit string
	// IronbarkBuildTime is the UTC ISO 8601 timestamp recording when the
	// Ironbark binary that generated the rules file was built.
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
	cmd.AddCommand(newGenerateZarfBootstrapCmd())
	cmd.AddCommand(newGenerateFapolicyRulesCmd())
	cmd.AddCommand(newGenerateInstallerCmd())

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
  - Honours ` + "`IRONBARK_SCRIPT_NO_INTERACTIVE`" + ` / ` + "`IRONBARK_SCRIPT_NO_TTY`" + ` at
    runtime: define either variable to ANY value to disable
    ` + "`--interactive`" + ` / ` + "`--tty`" + `.
  - Honours ` + "`IRONBARK_SCRIPT_VERBOSE`" + `: define it to ANY value to print the
    resolved launcher settings to stderr before invoking the container engine.
  - Accepts launcher-specific CLI flags as a more convenient alternative to
    setting the equivalent ` + "`IRONBARK_SCRIPT_*`" + ` environment variables.
    When a literal ` + "`--`" + ` separator appears in the arguments, every
    argument BEFORE the ` + "`--`" + ` is parsed by the launcher itself and every
    argument AFTER is forwarded to the Ironbark container verbatim. When no
    ` + "`--`" + ` is supplied, ALL arguments are forwarded to the container
    unchanged. Run ` + "`./ironbark --help --`" + ` for the full list of
    launcher flags.

Example:

  ironbark generate launcher > ironbark
  chmod +x ironbark
  ./ironbark init all                 # all args forwarded to the container
  ./ironbark --no-tty -- init all     # disable --tty, then forward 'init all'
`,
		// Suppress cobra's automatic usage/error reprint as `Execute` already
		// surfaces errors via panic.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return execLauncher(os.Stdout, launcherTemplateData{
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
// the supplied writer.
//
// Build-time metadata (current time, Ironbark version, commit, build time) is
// populated automatically; callers only need to supply the user-configurable
// fields (engine, image, mounts, no-interactive/no-tty toggles).
func execLauncher(out io.Writer, data launcherTemplateData) error {
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

	if err := tmpl.Execute(out, data); err != nil {
		return fmt.Errorf("could not render launcher template: %w", err)
	}

	return nil
}

// -----------------------------------------------------------------------------
// COMMAND: zarf-bootstrap
// -----------------------------------------------------------------------------

// newGenerateZarfBootstrapCmd creates the `generate zarf-bootstrap`
// command which generates a host-side script that extracts the Zarf CLI
// and the `zarf-init-*.tar.zst` package out of the Ironbark container
// image. The extracted assets are intended to be used to bootstrap a
// host (e.g. by running `zarf init` to install K3s).
//
// This command is only required when the host does not already have a
// Kubernetes cluster available; if a cluster already exists, Ironbark
// can talk to it directly via its kubeconfig and no Zarf bootstrap is
// needed.
func newGenerateZarfBootstrapCmd() *cobra.Command {
	var (
		engine     string
		image      string
		sourcePath string
		outputDir  string
	)

	cmd := &cobra.Command{
		Use:   "zarf-bootstrap",
		Short: "Generate a script that extracts Zarf from the Ironbark container to bootstrap a new K8s cluster",
		Long: `Generate a host-side script that extracts the Zarf CLI and the
` + "`zarf-init-*.tar.zst`" + ` package out of the Ironbark container image.

This command is ONLY required when the target host does not already have a
Kubernetes cluster available. If a cluster already exists, Ironbark can talk
to it directly via its kubeconfig and you can skip this command entirely.
Run this command (and the resulting script) when you need to bootstrap a
fresh host - typically to install K3s on it via ` + "`zarf init`" + `.

The generated script is written to standard output and can be redirected to a
file (typically ` + "`extract-zarf.sh`" + `) and executed on the host. When
run, the script:
  - Validates that the requested container engine is available on PATH.
  - Creates the output directory (if it does not already exist).
  - Creates a throwaway Ironbark container, copies the Zarf assets out, then
    removes the throwaway container.
  - Prints next-step instructions for running ` + "`zarf init`" + `.

The generated script does NOT run ` + "`zarf init`" + `; the operator
retains explicit control over the bootstrap step. To allow execution of the
extracted Zarf binary (and the resulting K3s binary) under fapolicyd, also
generate a matching rules file via ` + "`ironbark generate fapolicy-rules`" + `.

Example:

  ironbark generate zarf-bootstrap > extract-zarf.sh
  chmod +x extract-zarf.sh
  ./extract-zarf.sh
  cd /opt/brightsparklabs/ironbark/production/data/zarf/init
  sudo ./zarf init
`,
		// Suppress cobra's automatic usage/error reprint as `Execute` already
		// surfaces errors via panic.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return execZarfBootstrap(os.Stdout, zarfBootstrapTemplateData{
				Engine:     engine,
				Image:      image,
				SourcePath: sourcePath,
				OutputDir:  outputDir,
			})
		},
	}

	cmd.Flags().StringVarP(&engine, "container-engine", "e", defaultLauncherEngine,
		"Container engine to use (`podman` or `docker`)")
	cmd.Flags().StringVarP(&image, "image", "i", defaultLauncherImage,
		"Container image reference for Ironbark")
	cmd.Flags().StringVarP(&sourcePath, "source-path", "s", defaultZarfBootstrapSourcePath,
		"Path inside the container containing the Zarf CLI and init package")
	cmd.Flags().StringVarP(&outputDir, "output-dir", "o", defaultZarfBootstrapOutputDir,
		"Host directory which will receive the extracted Zarf assets")

	return cmd
}

// execZarfBootstrap renders the Zarf bootstrap script template and
// writes the result to the supplied writer.
//
// Build-time metadata (current time, Ironbark version, commit, build time)
// is populated automatically; callers only need to supply the
// user-configurable fields (engine, image, source path, output directory).
//
// The function refuses to render when the Zarf assets are not physically
// present at the configured source path (the `zarf` CLI plus a
// `zarf-init-*.tar.zst` package). This catches misconfigurations such as
// a stale image or a wrong `--source-path` before the operator runs the
// resulting script on the host. The check works equally inside the
// Ironbark container (where the assets are baked into the image) and
// outside it (e.g. when the assets have been pre-extracted to a known
// location).
func execZarfBootstrap(out io.Writer, data zarfBootstrapTemplateData) error {
	normalisedEngine := strings.ToLower(strings.TrimSpace(data.Engine))
	if _, ok := supportedLauncherEngines[normalisedEngine]; !ok {
		return fmt.Errorf("unsupported container engine %q (must be one of: podman, docker)", data.Engine)
	}
	data.Engine = normalisedEngine

	if strings.TrimSpace(data.Image) == "" {
		return fmt.Errorf("image must not be empty")
	}
	if strings.TrimSpace(data.SourcePath) == "" {
		return fmt.Errorf("source-path must not be empty")
	}
	if strings.TrimSpace(data.OutputDir) == "" {
		return fmt.Errorf("output-dir must not be empty")
	}

	// Verify the Zarf assets are physically present at the configured
	// source path before emitting a script that promises to extract them.
	if err := zarfBootstrapAssetsPresent(data.SourcePath); err != nil {
		return fmt.Errorf("could not verify Zarf assets at source path: %w", err)
	}

	data.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	data.IronbarkVersion = version.GetVersion()
	data.IronbarkCommit = version.GetCommit()
	data.IronbarkBuildTime = version.GetBuildTime()

	tmpl, err := resources.LoadTemplate(zarfBootstrapTemplateFile)
	if err != nil {
		return fmt.Errorf("could not load zarf-bootstrap template: %w", err)
	}

	if err := tmpl.Execute(out, data); err != nil {
		return fmt.Errorf("could not render zarf-bootstrap template: %w", err)
	}

	return nil
}

// defaultZarfAssetsPresent verifies that the Zarf assets are present in
// the supplied `sourcePath` directory. It checks for both the `zarf` CLI
// and at least one `zarf-init-*.tar.zst` package (matching the layout
// produced by the Ironbark `Dockerfile`). Returns a descriptive error
// when any expected asset is missing.
func defaultZarfAssetsPresent(sourcePath string) error {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("source path %q is not accessible: %w", sourcePath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("source path %q is not a directory", sourcePath)
	}

	zarfBinary := filepath.Join(sourcePath, zarfBinaryName)
	if _, err := os.Stat(zarfBinary); err != nil {
		return fmt.Errorf("zarf binary not found at %q: %w", zarfBinary, err)
	}

	entries, err := os.ReadDir(sourcePath)
	if err != nil {
		return fmt.Errorf("could not read source path %q: %w", sourcePath, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, zarfInitPackagePrefix) && strings.HasSuffix(name, zarfInitPackageSuffix) {
			return nil
		}
	}

	return fmt.Errorf(
		"no zarf init package (matching %q) found in %q",
		zarfInitPackagePrefix+"*"+zarfInitPackageSuffix, sourcePath,
	)
}

// -----------------------------------------------------------------------------
// COMMAND: fapolicy-rules
// -----------------------------------------------------------------------------

// newGenerateFapolicyRulesCmd creates the `generate fapolicy-rules` command
// which generates an fapolicyd rules file allowing execution of the
// binaries Ironbark relies on (the extracted Zarf CLI and the installed
// K3s CLI).
func newGenerateFapolicyRulesCmd() *cobra.Command {
	var (
		zarfPath string
		k3sPath  string
	)

	cmd := &cobra.Command{
		Use:   "fapolicy-rules",
		Short: "Generate an fapolicyd rules file for the binaries used by Ironbark",
		Long: `Generate an fapolicyd rules file allowing execution of the binaries
Ironbark relies on when bootstrapping a host.

The generated rules cover:
  - The extracted Zarf CLI (path configurable via ` + "`--zarf-path`" + `).
  - The installed K3s CLI (path configurable via ` + "`--k3s-path`" + `).

The generated file is written to standard output and is intended to be
redirected into ` + "`/etc/fapolicyd/rules.d/`" + ` (or piped into another
tool). After installing the file, reload fapolicyd:

  sudo systemctl restart fapolicyd

Example:

  ironbark generate fapolicy-rules \
      --zarf-path /opt/brightsparklabs/ironbark/production/data/zarf/init/zarf \
      --k3s-path /usr/local/bin/k3s \
      | sudo tee /etc/fapolicyd/rules.d/30-ironbark.rules > /dev/null
  sudo systemctl restart fapolicyd
`,
		// Suppress cobra's automatic usage/error reprint as `Execute` already
		// surfaces errors via panic.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return execFapolicyRules(os.Stdout, fapolicyRulesTemplateData{
				ZarfPath: zarfPath,
				K3sPath:  k3sPath,
			})
		},
	}

	cmd.Flags().StringVar(&zarfPath, "zarf-path", defaultFapolicyZarfPath,
		"Absolute host path of the extracted Zarf CLI")
	cmd.Flags().StringVar(&k3sPath, "k3s-path", defaultFapolicyK3sPath,
		"Absolute host path of the installed K3s CLI")

	return cmd
}

// execFapolicyRules renders the fapolicyd rules template and writes the
// result to the supplied writer.
//
// Build-time metadata (current time, Ironbark version, commit, build time)
// is populated automatically; callers only need to supply the
// user-configurable paths.
func execFapolicyRules(out io.Writer, data fapolicyRulesTemplateData) error {
	data.ZarfPath = strings.TrimSpace(data.ZarfPath)
	data.K3sPath = strings.TrimSpace(data.K3sPath)

	if data.ZarfPath == "" {
		return fmt.Errorf("zarf-path must not be empty")
	}
	if data.K3sPath == "" {
		return fmt.Errorf("k3s-path must not be empty")
	}
	if !strings.HasPrefix(data.ZarfPath, "/") {
		return fmt.Errorf("zarf-path must be an absolute path, got %q", data.ZarfPath)
	}
	if !strings.HasPrefix(data.K3sPath, "/") {
		return fmt.Errorf("k3s-path must be an absolute path, got %q", data.K3sPath)
	}

	data.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	data.IronbarkVersion = version.GetVersion()
	data.IronbarkCommit = version.GetCommit()
	data.IronbarkBuildTime = version.GetBuildTime()

	tmpl, err := resources.LoadTemplate(fapolicyRulesTemplateFile)
	if err != nil {
		return fmt.Errorf("could not load fapolicy-rules template: %w", err)
	}

	if err := tmpl.Execute(out, data); err != nil {
		return fmt.Errorf("could not render fapolicy-rules template: %w", err)
	}

	return nil
}

// -----------------------------------------------------------------------------
// COMMAND: installer
// -----------------------------------------------------------------------------

// newGenerateInstallerCmd creates the `generate installer` command which
// generates a self-contained bash installer script for setting up Ironbark
// on a target host.
func newGenerateInstallerCmd() *cobra.Command {
	var (
		installBaseDir     string
		releaseEnvironment string

		// Launcher pass-through flags. These mirror `generate launcher` so
		// the installer can produce a fully-configured launcher with the
		// same level of control.
		//
		// NOTE: `--data-dir` is deliberately NOT exposed - the installer
		// creates `<install-dir>/data/{zarf/init,repos}` on the host and
		// bakes that same path into the inlined launcher's data-dir, so
		// the two stay consistent. Allowing the operator to override it
		// would silently desync the directories the installer creates
		// from those the launcher mounts.
		engine         string
		image          string
		hostKubeconfig string
		noInteractive  bool
		noTTY          bool
	)

	cmd := &cobra.Command{
		Use:   "installer",
		Short: "Generate a self-contained installer script for Ironbark",
		Long: `Generate a self-contained bash installer script for Ironbark.

The generated script is written to standard output. When piped through
` + "`bash`" + ` on a target host it will:

  - Create the per-release-environment install directory tree under
    ` + "`<install-base-dir>/<release-environment>/`" + ` (defaults to
    ` + "`/opt/brightsparklabs/ironbark/production/`" + `), including:
      - ` + "`bin/`" + `
      - ` + "`data/zarf/init/`" + `
      - ` + "`data/repos/`" + `
  - Restore ownership of the install tree to the invoking user/group
    (` + "`SUDO_USER`" + ` falling back to ` + "`USER`" + `).
  - If ` + "`setfacl`" + ` is available, apply recursive default ACLs on the
    install base directory granting the invoking user/group rwx, so new
    files created beneath it inherit the right permissions.
  - Write a versioned launcher script as
    ` + "`<install-dir>/bin/.<timestamp>_ironbark_<version>`" + ` and
    (re)point the generic ` + "`<install-dir>/bin/ironbark`" + ` symlink at
    it. The launcher inlined into the installer is equivalent to the
    output of ` + "`ironbark generate launcher`" + `, so the same launcher
    flags apply.

Example:

  # Generate the installer and write it to a file:
  ironbark generate installer > install-ironbark.sh
  chmod +x install-ironbark.sh
  sudo ./install-ironbark.sh

  # Or pipe directly into bash:
  ironbark generate installer --release-environment staging | sudo bash
`,
		// Suppress cobra's automatic usage/error reprint as `Execute` already
		// surfaces errors via panic.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return execInstaller(os.Stdout, installerTemplateData{
				InstallBaseDir:     installBaseDir,
				ReleaseEnvironment: releaseEnvironment,
			}, launcherTemplateData{
				Engine: engine,
				Image:  image,
				// HostDataDir is intentionally left unset here; it is
				// derived from the install directory inside
				// `execInstaller` to ensure the launcher and installer
				// agree on the data directory location.
				HostKubeconfig: hostKubeconfig,
				NoInteractive:  noInteractive,
				NoTTY:          noTTY,
			})
		},
	}

	// Installer-specific flags.
	cmd.Flags().StringVar(&installBaseDir, "install-base-dir", defaultInstallerBaseDir,
		"Base directory under which the per-release-environment install directory is created")
	cmd.Flags().StringVar(&releaseEnvironment, "release-environment", defaultInstallerReleaseEnvironment,
		"Release environment the installer targets (e.g. `production`, `staging`)")

	// Launcher pass-through flags (matching `generate launcher`, except
	// `--data-dir` which is deliberately not exposed - see the variable
	// declaration above).
	cmd.Flags().StringVarP(&engine, "container-engine", "e", defaultLauncherEngine,
		"Container engine to bake into the inlined launcher (`podman` or `docker`)")
	cmd.Flags().StringVarP(&image, "image", "i", defaultLauncherImage,
		"Container image reference to bake into the inlined launcher")
	cmd.Flags().StringVarP(&hostKubeconfig, "kubeconfig", "k", defaultLauncherKubeconfig,
		"Host kubeconfig file to mount into the container")
	cmd.Flags().BoolVar(&noInteractive, "no-interactive", false,
		"Bake `--interactive` off as the default in the inlined launcher")
	cmd.Flags().BoolVar(&noTTY, "no-tty", false,
		"Bake `--tty` off as the default in the inlined launcher")

	return cmd
}

// execInstaller renders the installer script template (with the launcher
// script inlined) and writes the result to the supplied writer.
//
// Build-time metadata (current time, Ironbark version, commit, build time)
// is populated automatically; callers only need to supply the
// user-configurable installer fields and the launcher pass-through data.
func execInstaller(out io.Writer, data installerTemplateData, launcherData launcherTemplateData) error {
	data.InstallBaseDir = strings.TrimSpace(data.InstallBaseDir)
	data.ReleaseEnvironment = strings.TrimSpace(data.ReleaseEnvironment)

	if data.InstallBaseDir == "" {
		return fmt.Errorf("install-base-dir must not be empty")
	}
	if !strings.HasPrefix(data.InstallBaseDir, "/") {
		return fmt.Errorf("install-base-dir must be an absolute path, got %q", data.InstallBaseDir)
	}
	if data.ReleaseEnvironment == "" {
		return fmt.Errorf("release-environment must not be empty")
	}

	data.InstallDir = filepath.Join(data.InstallBaseDir, data.ReleaseEnvironment)

	// Force the inlined launcher's host data directory to match the
	// data directory the installer creates on the host (and ignore any
	// caller-supplied value). This keeps the directories created by the
	// installer in lockstep with those mounted by the launcher.
	launcherData.HostDataDir = filepath.Join(data.InstallDir, "data")

	// Render the launcher so it can be inlined into the installer
	// template's heredoc. Use a strings.Builder as the sink so we can
	// capture the rendered output without writing to disk.
	var launcherBuf strings.Builder
	if err := execLauncher(&launcherBuf, launcherData); err != nil {
		return fmt.Errorf("could not render inlined launcher: %w", err)
	}
	data.LauncherScript = launcherBuf.String()
	data.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	data.IronbarkVersion = version.GetVersion()
	data.IronbarkCommit = version.GetCommit()
	data.IronbarkBuildTime = version.GetBuildTime()

	tmpl, err := resources.LoadTemplate(installerTemplateFile)
	if err != nil {
		return fmt.Errorf("could not load installer template: %w", err)
	}

	if err := tmpl.Execute(out, data); err != nil {
		return fmt.Errorf("could not render installer template: %w", err)
	}

	return nil
}
