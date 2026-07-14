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

	// defaultReadTimeout is the default maximum duration in seconds for
	// reading the entire request, including the body.
	defaultReadTimeout = 30

	// defaultWriteTimeout is the default maximum duration in seconds before
	// timing out writes of the response.
	defaultWriteTimeout = 30

	// defaultIdleTimeout is the default maximum duration in seconds to wait
	// for the next request when keep-alives are enabled.
	defaultIdleTimeout = 60
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
	var readTimeout int
	var writeTimeout int
	var idleTimeout int

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start Ironbark in HTTP API server mode",
		Long: `Start Ironbark as a long-lived HTTP API server.

In serve mode, Ironbark exposes REST endpoints for cluster initialisation,
repository management, and kubeconfig handling. This allows remote interaction
without requiring host filesystem mounts.

The API server runs on the specified --api-port (default 8080).
The Git proxy runs on the specified --git-port (default 3000).

Timeout flags control how long the server waits for various operations:
  --read-timeout: Maximum duration for reading entire request (default 30s)
  --write-timeout: Maximum duration for writing response (default 30s)
  --idle-timeout: Maximum duration to wait for next request (default 60s)

Example:
  ironbark serve
  ironbark serve --api-port 9000 --git-port 4000
  ironbark serve --read-timeout 60 --write-timeout 60 --idle-timeout 120`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return serveExec(cmd, args, apiPort, gitPort, readTimeout, writeTimeout, idleTimeout)
		},
	}

	cmd.Flags().IntVar(&apiPort, "api-port", defaultAPIPort, "Port for the REST API server")
	cmd.Flags().IntVar(&gitPort, "git-port", defaultGitPort, "Port for the Git proxy server")
	cmd.Flags().IntVar(&readTimeout, "read-timeout", defaultReadTimeout, "Maximum seconds for reading request (0 = no timeout)")
	cmd.Flags().IntVar(&writeTimeout, "write-timeout", defaultWriteTimeout, "Maximum seconds for writing response (0 = no timeout)")
	cmd.Flags().IntVar(&idleTimeout, "idle-timeout", defaultIdleTimeout, "Maximum seconds to wait for next request when keep-alives enabled (0 = no timeout)")

	return cmd
}

// -----------------------------------------------------------------------------
// PRIVATE FUNCTIONS
// -----------------------------------------------------------------------------

// serveExec is the execution function for the `serve` command.
// It starts the HTTP API server and blocks until interrupted.
func serveExec(cmd *cobra.Command, args []string, apiPort int, gitPort int, readTimeout int, writeTimeout int, idleTimeout int) error {
	slog.Info("Starting Ironbark in API server mode",
		"apiPort", apiPort,
		"gitPort", gitPort,
		"readTimeout", readTimeout,
		"writeTimeout", writeTimeout,
		"idleTimeout", idleTimeout)

	// Create a context that cancels on SIGINT or SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start the API server (blocks until context is cancelled).
	if err := api.Serve(ctx, apiPort, gitPort, readTimeout, writeTimeout, idleTimeout); err != nil {
		return NewUserError("failed to start API server: %v", err)
	}

	slog.Info("API server stopped gracefully")
	return nil
}
