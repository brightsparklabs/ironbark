/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

// Package settings is the single source of truth for every IRONBARK_*
// environment variable understood (or merely forwarded) by Ironbark.
//
// The package follows a data-oriented design: a single immutable
// catalogue (`All`) describes every variable the system knows about, and
// thin typed accessors (e.g. `DataDir`, `LogLevel`) read the variables
// that are actually consumed by Go code. Anything that needs to inspect
// or render the catalogue (notably the `ironbark debug settings`
// command) iterates over `All` rather than hard-coding variable names.
//
// IMPORTANT: any new IRONBARK_* environment variable introduced anywhere
// in the codebase (Go, Dockerfile, shell templates) MUST be added to
// `All` so the catalogue stays accurate. Tests enforce that names in
// `All` are unique and well formed.
package settings

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"brightsparklabs.com/ironbark/internal/constants"

	"log/slog"
)

// -----------------------------------------------------------------------------
// TYPES
// -----------------------------------------------------------------------------

// Scope classifies how an IRONBARK_* environment variable participates
// in the Ironbark system. Used by `ironbark debug settings` to group
// variables sensibly when rendering them.
type Scope string

const (
	// ScopeGoConsumed marks variables read directly by the Ironbark Go
	// binary (e.g. `IRONBARK_DATA_DIR`). These are the variables for
	// which this package exposes typed accessors.
	ScopeGoConsumed Scope = "go-consumed"

	// ScopeContainerSentinel marks variables that the Ironbark container
	// image bakes in to advertise its own context (currently just
	// `IRONBARK_IN_CONTAINER`).
	ScopeContainerSentinel Scope = "container-sentinel"

	// ScopeLauncherSentinel marks variables that the generated launcher
	// script sets to advertise its own invocation (currently just
	// `IRONBARK_LAUNCHER_INVOKED`). Distinct from
	// `ScopeLauncherForwarded` so the validator can treat the
	// sentinel itself differently from the data it gates.
	ScopeLauncherSentinel Scope = "launcher-sentinel"

	// ScopeLauncherForwarded marks variables that the generated
	// launcher script forwards from the host into the container (the
	// `IRONBARK_HOST_*` family). When the launcher sentinel is set,
	// every variable in this scope is expected to be present and
	// non-empty - `ValidateLauncherEnv` enforces this.
	ScopeLauncherForwarded Scope = "launcher-forwarded"

	// ScopeScript marks variables read only by the generated launcher
	// and zarf-bootstrap scripts. They never reach Go but users still
	// need to know about them. By convention every variable in this
	// scope uses the `IRONBARK_SCRIPT_*` prefix.
	ScopeScript Scope = "script"
)

// Var is the immutable description of a single IRONBARK_* environment
// variable. The catalogue (`All`) is a slice of `Var` values and is the
// single source of truth for the system.
type Var struct {
	// Name is the literal environment variable name (e.g.
	// `IRONBARK_DATA_DIR`).
	Name string
	// Description is a short human-readable explanation of what the
	// variable does. Should be a complete sentence ending in a full
	// stop.
	Description string
	// Default is the value Ironbark falls back to when the variable is
	// unset or empty. An empty string here means "no default - the
	// variable is optional and unset by default".
	Default string
	// Scope describes how the variable participates in the system. See
	// the `Scope*` constants.
	Scope Scope
	// Sensitive marks variables whose values should be redacted when
	// printed (e.g. by `ironbark debug settings`). Currently no
	// variables are sensitive, but the field is reserved for future
	// credential-bearing variables (e.g. a Gitea token).
	Sensitive bool
}

// Resolved captures the runtime state of an IRONBARK_* variable: its
// current effective value plus where that value came from (env or the
// declared default). Produced by `Resolve` and rendered by
// `ironbark debug settings`.
type Resolved struct {
	// Var is the catalogue entry that produced this resolution.
	Var Var
	// Value is the effective value: the value of the environment
	// variable if it is set and non-empty, otherwise `Var.Default`.
	Value string
	// FromEnv is true when the value came from a non-empty environment
	// variable read at resolution time, and false when the value came
	// from `Var.Default` (including the case where the default itself
	// is empty).
	FromEnv bool
}

// -----------------------------------------------------------------------------
// CATALOGUE
// -----------------------------------------------------------------------------

// All returns the canonical catalogue of every IRONBARK_* environment
// variable understood by the system. The slice is freshly allocated on
// each call so callers may sort or filter it without affecting other
// callers.
//
// IMPORTANT: this is the single source of truth. When introducing a new
// IRONBARK_* variable anywhere in the codebase (Go, Dockerfile, shell
// template), add it here.
func All() []Var {
	out := make([]Var, len(catalogue))
	copy(out, catalogue)
	return out
}

// catalogue is the package-private master list. Exposed via `All`.
var catalogue = []Var{
	// ----- Go-consumed -----
	{
		Name:        envDataDir,
		Description: "Directory used by Ironbark to store application data (cloned repos, etc.).",
		Default:     defaultDataDir,
		Scope:       ScopeGoConsumed,
	},
	{
		Name:        envInternalPackagesDir,
		Description: "Directory containing the internal Zarf packages bundled with Ironbark.",
		Default:     defaultInternalPackagesDir,
		Scope:       ScopeGoConsumed,
	},
	{
		Name:        envLogLevel,
		Description: "Minimum log level for the Ironbark binary (debug, info, warn, error). Defaults to info when unset or unrecognised.",
		Default:     defaultLogLevel,
		Scope:       ScopeGoConsumed,
	},

	// ----- Container sentinel -----
	{
		Name:        envInContainer,
		Description: "Sentinel baked into the Ironbark container image so any process can detect it is running inside the container.",
		Default:     defaultInContainer,
		Scope:       ScopeContainerSentinel,
	},

	// ----- Launcher sentinel -----
	{
		Name:        envLauncherInvoked,
		Description: "Sentinel set to `true` by the generated launcher script so any process can detect that Ironbark was invoked via the launcher (rather than directly).",
		Default:     defaultLauncherInvoked,
		Scope:       ScopeLauncherSentinel,
	},

	// ----- Launcher-script (launcher + zarf-bootstrap script overrides) -----
	{
		Name:        "IRONBARK_SCRIPT_CONTAINER_ENGINE",
		Description: "Container engine (`podman` or `docker`) used by the generated launcher and zarf-bootstrap scripts. Overrides the value baked into the script at generation time.",
		Default:     "",
		Scope:       ScopeScript,
	},
	{
		Name:        "IRONBARK_SCRIPT_IMAGE",
		Description: "Container image reference used by the generated launcher and zarf-bootstrap scripts. Overrides the value baked into the script at generation time.",
		Default:     "",
		Scope:       ScopeScript,
	},
	{
		Name:        "IRONBARK_SCRIPT_NO_INTERACTIVE",
		Description: "Set to any value to make the generated launcher script run the container without `--interactive`. Useful in CI.",
		Default:     "",
		Scope:       ScopeScript,
	},
	{
		Name:        "IRONBARK_SCRIPT_NO_TTY",
		Description: "Set to any value to make the generated launcher script run the container without `--tty`. Useful when piping output or running in CI.",
		Default:     "",
		Scope:       ScopeScript,
	},
	{
		Name:        "IRONBARK_SCRIPT_VERBOSE",
		Description: "Set to any value to make the generated launcher and zarf-bootstrap scripts print verbose diagnostic output to stderr before running their main work.",
		Default:     "",
		Scope:       ScopeScript,
	},
	{
		Name:        "IRONBARK_SCRIPT_ZARF_SOURCE_PATH",
		Description: "Override for the in-container source path used by the generated zarf-bootstrap script.",
		Default:     "",
		Scope:       ScopeScript,
	},
	{
		Name:        "IRONBARK_SCRIPT_ZARF_OUTPUT_DIR",
		Description: "Override for the host output directory used by the generated zarf-bootstrap script.",
		Default:     "",
		Scope:       ScopeScript,
	},

	// ----- Launcher-forwarded (host context exposed inside the container) -----
	{
		Name:        "IRONBARK_HOST_DATA_DIR",
		Description: "Host directory bind-mounted into the container as `/mnt/data`. Set by the generated launcher script.",
		Default:     "",
		Scope:       ScopeLauncherForwarded,
	},
	{
		Name:        "IRONBARK_HOST_KUBECONFIG",
		Description: "Host kubeconfig file bind-mounted into the container at `/mnt/conf/kubeconfig` (exported as `KUBECONFIG`). Set by the generated launcher script.",
		Default:     "",
		Scope:       ScopeLauncherForwarded,
	},
	{
		Name:        "IRONBARK_HOST_CONTAINER_ENGINE",
		Description: "Container engine the launcher used to start the current container (e.g. `podman`).",
		Default:     "",
		Scope:       ScopeLauncherForwarded,
	},
	{
		Name:        "IRONBARK_HOST_USER",
		Description: "Username of the host user that invoked the launcher.",
		Default:     "",
		Scope:       ScopeLauncherForwarded,
	},
	{
		Name:        "IRONBARK_HOST_UID",
		Description: "Numeric UID of the host user that invoked the launcher.",
		Default:     "",
		Scope:       ScopeLauncherForwarded,
	},
	{
		Name:        "IRONBARK_HOST_GID",
		Description: "Numeric GID of the host user that invoked the launcher.",
		Default:     "",
		Scope:       ScopeLauncherForwarded,
	},
	{
		Name:        "IRONBARK_HOST_HOSTNAME",
		Description: "Hostname of the host machine that invoked the launcher.",
		Default:     "",
		Scope:       ScopeLauncherForwarded,
	},
	{
		Name:        "IRONBARK_HOST_PWD",
		Description: "Working directory the launcher was invoked from.",
		Default:     "",
		Scope:       ScopeLauncherForwarded,
	},
	{
		Name:        "IRONBARK_HOST_OS",
		Description: "Operating system of the host (output of `uname -s`).",
		Default:     "",
		Scope:       ScopeLauncherForwarded,
	},
	{
		Name:        "IRONBARK_HOST_ARCH",
		Description: "CPU architecture of the host (output of `uname -m`).",
		Default:     "",
		Scope:       ScopeLauncherForwarded,
	},
	{
		Name:        "IRONBARK_HOST_LAUNCHER_VERSION",
		Description: "Version of the Ironbark binary that generated the launcher script the operator is currently running.",
		Default:     "",
		Scope:       ScopeLauncherForwarded,
	},
	{
		Name:        "IRONBARK_HOST_LAUNCHER_COMMIT",
		Description: "Git commit of the Ironbark binary that generated the launcher script.",
		Default:     "",
		Scope:       ScopeLauncherForwarded,
	},
	{
		Name:        "IRONBARK_HOST_LAUNCHER_BUILD_TIME",
		Description: "Build timestamp of the Ironbark binary that generated the launcher script.",
		Default:     "",
		Scope:       ScopeLauncherForwarded,
	},
	{
		Name:        "IRONBARK_HOST_LAUNCHER_GENERATED_AT",
		Description: "Timestamp recording when the launcher script the operator is currently running was generated.",
		Default:     "",
		Scope:       ScopeLauncherForwarded,
	},
}

// -----------------------------------------------------------------------------
// PRIVATE CONSTANTS
// -----------------------------------------------------------------------------

const (
	// envDataDir is the environment variable defining the application
	// data directory.
	envDataDir = "IRONBARK_DATA_DIR"
	// envInternalPackagesDir is the environment variable defining the
	// directory containing the internal Zarf packages.
	envInternalPackagesDir = "IRONBARK_INTERNAL_PACKAGES_DIR"
	// envLogLevel is the environment variable defining the minimum log
	// level for the Ironbark binary.
	envLogLevel = "IRONBARK_LOG_LEVEL"
	// envInContainer is the environment variable that the Ironbark
	// container image bakes in to advertise its context.
	envInContainer = "IRONBARK_IN_CONTAINER"
	// envHostDataDir is the environment variable that the generated
	// launcher script forwards into the container so any process can
	// recover the host-side path of the bind-mounted data directory.
	envHostDataDir = "IRONBARK_HOST_DATA_DIR"
	// envLauncherInvoked is the sentinel environment variable the
	// generated launcher script sets so any process can detect that
	// Ironbark was invoked via the launcher.
	envLauncherInvoked = "IRONBARK_LAUNCHER_INVOKED"
	// launcherInvokedExpectedValue is the value the launcher sets
	// `IRONBARK_LAUNCHER_INVOKED` to. Anything else (including unset)
	// is treated as "not invoked via the launcher".
	launcherInvokedExpectedValue = "true"

	// defaultDataDir is the fallback for `IRONBARK_DATA_DIR` when it is
	// unset or empty. Mirrors what the Ironbark `Dockerfile` sets the
	// var to inside the container; the default here is appropriate for
	// running the binary directly on a developer workstation.
	defaultDataDir = "/tmp/ironbark/data"
	// defaultInternalPackagesDir is the fallback for
	// `IRONBARK_INTERNAL_PACKAGES_DIR` when it is unset or empty.
	defaultInternalPackagesDir = "/tmp/ironbark/packages"
	// defaultLogLevel is the fallback for `IRONBARK_LOG_LEVEL` when it
	// is unset or unrecognised. Mirrors the behaviour of
	// `resolveLogLevel`, which maps unknown values onto `slog.LevelInfo`.
	defaultLogLevel = "info"
	// defaultInContainer is the fallback for `IRONBARK_IN_CONTAINER`
	// when the sentinel is not set. Anything other than `true` is
	// treated as "not running inside the Ironbark container".
	defaultInContainer = "false"
	// defaultLauncherInvoked is the fallback for
	// `IRONBARK_LAUNCHER_INVOKED` when the sentinel is not set.
	// Anything other than `true` is treated as "not invoked via the
	// generated launcher script".
	defaultLauncherInvoked = "false"
)

// -----------------------------------------------------------------------------
// SNAPSHOT
// -----------------------------------------------------------------------------

// snapshot is the immutable, frozen-at-startup view of every setting
// the Ironbark binary needs to consult during a run. Built by
// `resolve` and stored in the package-private `current` variable.
//
// Treat instances of this struct as immutable: never mutate one after
// it has been published via `current`.
type snapshot struct {
	// dataDir is the resolved value of `IRONBARK_DATA_DIR`.
	dataDir string
	// internalPackagesDir is the resolved value of
	// `IRONBARK_INTERNAL_PACKAGES_DIR`.
	internalPackagesDir string
	// logLevel is the resolved value of `IRONBARK_LOG_LEVEL`.
	logLevel slog.Level
	// inContainer is the resolved value of `IRONBARK_IN_CONTAINER`.
	inContainer bool
	// hostDataDir is the resolved value of `IRONBARK_HOST_DATA_DIR`.
	hostDataDir string
	// launcherInvoked is the resolved value of `IRONBARK_LAUNCHER_INVOKED`.
	launcherInvoked bool
}

// current holds the frozen snapshot. Read by every accessor; written
// only by `Init` (and, lazily, by `ensureInitialised`).
var current snapshot

// initOnce guards the lazy `ensureInitialised` path so callers that
// somehow read a setting before `Init` runs still see a fully
// resolved snapshot rather than the zero value.
var initOnce sync.Once

// Init resolves every setting from the current process environment,
// stores the result as the package-wide frozen snapshot, and runs the
// launcher-environment validator. Returns any validation error
// without short-circuiting the snapshot publication: even when
// validation fails, every accessor will return a sensible, fully
// resolved value (so error messages can safely reference settings).
//
// Call from `main` exactly once, BEFORE any subcommand runs. Tests
// can re-invoke `Init` after mutating the environment via `t.Setenv`
// to refresh the snapshot.
func Init() error {
	current = resolve()
	// Mark the lazy guard as satisfied so subsequent
	// `ensureInitialised` calls become no-ops.
	initOnce.Do(func() {})
	return validateLauncherEnv(current)
}

// resolve reads every Go-consumed environment variable once and
// returns the resulting snapshot. Pure function: no side effects, no
// validation.
func resolve() snapshot {
	return snapshot{
		dataDir:             getEnvWithDefault(envDataDir, defaultDataDir),
		internalPackagesDir: getEnvWithDefault(envInternalPackagesDir, defaultInternalPackagesDir),
		logLevel:            resolveLogLevel(),
		inContainer:         strings.TrimSpace(os.Getenv(envInContainer)) == "true",
		hostDataDir:         os.Getenv(envHostDataDir),
		launcherInvoked:     strings.TrimSpace(os.Getenv(envLauncherInvoked)) == launcherInvokedExpectedValue,
	}
}

// ensureInitialised resolves the snapshot once if `Init` has not yet
// been called. This is a defensive guard so any accessor used before
// `Init` (e.g. during package-init or in a test that forgot to call
// `Init`) still returns a sensible value rather than the zero value.
//
// Production code should always call `Init` from `main` so this
// fallback is never exercised in real runs.
func ensureInitialised() {
	initOnce.Do(func() { current = resolve() })
}

// resolveLogLevel maps the `IRONBARK_LOG_LEVEL` env var (case
// insensitive, trimmed) onto a `slog.Level`. Unrecognised values fall
// back to `slog.LevelInfo`.
func resolveLogLevel() slog.Level {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envLogLevel))) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// -----------------------------------------------------------------------------
// PUBLIC ACCESSORS
// -----------------------------------------------------------------------------

// DataDir returns the frozen value of `IRONBARK_DATA_DIR`.
func DataDir() string {
	ensureInitialised()
	return current.dataDir
}

// InternalPackagesDir returns the frozen value of
// `IRONBARK_INTERNAL_PACKAGES_DIR`.
func InternalPackagesDir() string {
	ensureInitialised()
	return current.internalPackagesDir
}

// ArgoCDRepoDir returns the absolute path of the local clone of the
// ArgoCD App of Apps repository: `<DataDir>/repos/<ArgoCDRepoName>`.
// Composes the frozen `DataDir` with the compile-time constant
// `constants.ArgoCDRepoName`.
func ArgoCDRepoDir() string {
	return filepath.Join(DataDir(), "repos", constants.ArgoCDRepoName)
}

// LogLevel returns the frozen value of `IRONBARK_LOG_LEVEL` as a
// `slog.Level`.
func LogLevel() slog.Level {
	ensureInitialised()
	return current.logLevel
}

// InContainer reports whether Ironbark is running inside the Ironbark
// container image. Reads the frozen `IRONBARK_IN_CONTAINER` value.
func InContainer() bool {
	ensureInitialised()
	return current.inContainer
}

// HostDataDir returns the frozen value of `IRONBARK_HOST_DATA_DIR`.
func HostDataDir() string {
	ensureInitialised()
	return current.hostDataDir
}

// LauncherInvoked reports whether Ironbark was invoked via the
// generated launcher script. Reads the frozen
// `IRONBARK_LAUNCHER_INVOKED` value.
func LauncherInvoked() bool {
	ensureInitialised()
	return current.launcherInvoked
}

// ValidateLauncherEnv re-runs the launcher-environment validation
// against the current process environment. Most callers should rely on
// the validation `Init` runs at startup; this function is exposed so
// tests and `debug settings` can re-check after the fact.
func ValidateLauncherEnv() error {
	return validateLauncherEnv(resolve())
}

// validateLauncherEnv is the snapshot-aware implementation of the
// launcher-environment check. When the launcher sentinel is not set
// (per the supplied snapshot) the function returns nil immediately.
// When the sentinel IS set, every catalogue entry with
// `Scope == ScopeLauncherForwarded` must have a non-empty value in
// the live process environment (the launcher-forwarded variables are
// not part of the snapshot). Any missing variables are reported via
// `LauncherEnvError`.
func validateLauncherEnv(s snapshot) error {
	if !s.launcherInvoked {
		return nil
	}

	var missing []string
	for _, v := range catalogue {
		if v.Scope != ScopeLauncherForwarded {
			continue
		}
		if value, ok := os.LookupEnv(v.Name); !ok || value == "" {
			missing = append(missing, v.Name)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	return &LauncherEnvError{Missing: missing}
}

// LauncherEnvError is the error type returned by `ValidateLauncherEnv`
// when the launcher sentinel is set but one or more expected
// `IRONBARK_HOST_*` variables are missing or empty. Exposes the
// missing names so callers can render them however they like.
type LauncherEnvError struct {
	// Missing is the list of `IRONBARK_HOST_*` variable names that
	// were expected but not set (or set to the empty string). Sorted
	// in catalogue order.
	Missing []string
}

// Error renders the launcher-env error as a human-readable message
// listing every missing variable name.
func (e *LauncherEnvError) Error() string {
	return fmt.Sprintf(
		"%s=%s is set, but the following expected launcher-forwarded variables are missing or empty: %s",
		envLauncherInvoked, launcherInvokedExpectedValue,
		strings.Join(e.Missing, ", "),
	)
}

// DisplayPath returns a human-friendly version of the supplied path
// for use in log lines and other user-facing output.
//
// When the binary is running inside the Ironbark container AND the
// launcher forwarded `IRONBARK_HOST_DATA_DIR`, any path that lives
// under the in-container data directory is rewritten to its host-side
// equivalent. This makes log output meaningful to the operator (who
// only knows the host paths they bind-mounted, not the in-container
// `/mnt/data` paths).
//
// In every other case (outside the container, host data dir unknown,
// or path not under the data directory) the input is returned
// unchanged.
func DisplayPath(path string) string {
	if !InContainer() {
		return path
	}

	hostDataDir := HostDataDir()
	if hostDataDir == "" {
		return path
	}

	containerDataDir := DataDir()
	if containerDataDir == "" || containerDataDir == hostDataDir {
		return path
	}

	// Use a `/`-suffixed prefix so we do not match siblings of the data
	// dir (e.g. `/mnt/data-backup` should not be rewritten as if it
	// were under `/mnt/data`). Treat the bare data dir as a special
	// case so it round-trips cleanly to the bare host data dir.
	if path == containerDataDir {
		return hostDataDir
	}

	prefix := strings.TrimSuffix(containerDataDir, "/") + "/"
	if !strings.HasPrefix(path, prefix) {
		return path
	}

	suffix := strings.TrimPrefix(path, prefix)
	hostPrefix := strings.TrimSuffix(hostDataDir, "/") + "/"
	return hostPrefix + suffix
}

// Resolve looks up the supplied catalogue entry and returns its current
// effective value plus the source the value came from. Used by
// `ironbark debug settings` to render the catalogue.
func Resolve(v Var) Resolved {
	if raw, ok := os.LookupEnv(v.Name); ok && raw != "" {
		return Resolved{Var: v, Value: raw, FromEnv: true}
	}
	return Resolved{Var: v, Value: v.Default, FromEnv: false}
}

// ResolveAll resolves every variable in the catalogue and returns the
// results in the same order as `All`.
func ResolveAll() []Resolved {
	vars := All()
	out := make([]Resolved, len(vars))
	for i, v := range vars {
		out[i] = Resolve(v)
	}
	return out
}

// -----------------------------------------------------------------------------
// PRIVATE FUNCTIONS
// -----------------------------------------------------------------------------

// getEnvWithDefault returns the value of the named environment variable
// or the supplied default if the variable is unset or empty.
func getEnvWithDefault(name, defaultValue string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return defaultValue
}
