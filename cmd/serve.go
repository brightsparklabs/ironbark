/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

package cmd

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"brightsparklabs.com/ironbark/internal/api"
	"github.com/spf13/cobra"
)

// -----------------------------------------------------------------------------
// CONSTANTS
// -----------------------------------------------------------------------------

const (
	// defaultAPIPort is the default port for the REST API server.
	defaultAPIPort = 8080

	// defaultGitPort is the default port for the Git proxy server.
	defaultGitPort = 3000

	// defaultAPITimeout is the default timeout in seconds for API server
	// operations (read, write, idle).
	defaultAPITimeout = 300

	// defaultGitTimeout is the default timeout in seconds for Git proxy
	// operations (read, write, idle). Git operations can be slow.
	defaultGitTimeout = 300
)

// -----------------------------------------------------------------------------
// PUBLIC FUNCTIONS
// -----------------------------------------------------------------------------

// newServeCmd creates the `ironbark serve` command, which starts Ironbark
// in HTTP API server mode.
//
// In this mode, Ironbark runs as a long-lived service accepting REST API
// requests instead of executing a single CLI operation and exiting. This
// enables remote interaction without requiring host filesystem mounts.
func newServeCmd() *cobra.Command {
	var apiPort int
	var gitPort int
	var apiTimeout int
	var gitTimeout int

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start Ironbark in HTTP API server mode",
		Long: `Start Ironbark as a long-lived HTTP API server.

In serve mode, Ironbark exposes REST endpoints for cluster initialisation,
repository management, and kubeconfig handling. This allows remote interaction
without requiring host filesystem mounts.

The API server runs on the specified --api-port (default 8080).
The Git proxy runs on the specified --git-port (default 3000).

Timeout flags control server operations:
  --api-timeout: Timeout in seconds for API operations (default 30s)
  --git-timeout: Timeout in seconds for Git operations (default 300s)

The timeout applies to read, write, and idle operations for each server.

Example:
  ironbark serve
  ironbark serve --api-port 9000 --git-port 4000
  ironbark serve --api-timeout 60 --git-timeout 600`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return serveExec(cmd, args, apiPort, gitPort, apiTimeout, gitTimeout)
		},
	}

	cmd.Flags().IntVar(&apiPort, "api-port", defaultAPIPort, "Port for the REST API server")
	cmd.Flags().IntVar(&gitPort, "git-port", defaultGitPort, "Port for the Git proxy server")
	cmd.Flags().IntVar(&apiTimeout, "api-timeout", defaultAPITimeout, "Timeout in seconds for API operations (0 = no timeout)")
	cmd.Flags().IntVar(&gitTimeout, "git-timeout", defaultGitTimeout, "Timeout in seconds for Git operations (0 = no timeout)")

	return cmd
}

// -----------------------------------------------------------------------------
// PRIVATE FUNCTIONS
// -----------------------------------------------------------------------------

// serveExec is the execution function for the `serve` command.
// It starts the HTTP API server and blocks until interrupted.
func serveExec(cmd *cobra.Command, args []string, apiPort int, gitPort int, apiTimeout int, gitTimeout int) error {
	slog.Info("Starting Ironbark in API server mode",
		"apiPort", apiPort,
		"gitPort", gitPort,
		"apiTimeout", apiTimeout,
		"gitTimeout", gitTimeout)

	// Initialize KUBECONFIG environment variable if a kubeconfig exists.
	// This ensures cluster operations can work immediately if a kubeconfig
	// is mounted or was previously uploaded.
	if err := api.InitializeKubeconfigEnv(); err != nil {
		slog.Warn("No kubeconfig available at startup", "details", err)
		slog.Info("Cluster operations will be available after kubeconfig is uploaded via API")
	}

	// Create a context that cancels on SIGINT or SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start the API server (blocks until context is cancelled).
	if err := api.Serve(ctx, apiPort, gitPort, apiTimeout, gitTimeout); err != nil {
		return NewUserError("failed to start API server: %v", err)
	}

	slog.Info("API server stopped gracefully")
	return nil
}
