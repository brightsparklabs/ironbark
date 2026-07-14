/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

// Package api provides HTTP server functionality for Ironbark, enabling
// remote interaction via REST API endpoints. This allows Ironbark to run
// as a service that accepts dynamic configuration without requiring host
// filesystem mounts.
package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// -----------------------------------------------------------------------------
// CONSTANTS
// -----------------------------------------------------------------------------

// -----------------------------------------------------------------------------
// PUBLIC FUNCTIONS
// -----------------------------------------------------------------------------

// Serve starts the HTTP API server on the specified port and blocks until
// the context is cancelled or an error occurs.
//
// The server exposes REST endpoints under /ironbark/api/v1/ for cluster
// initialisation, repository management, kubeconfig handling, and diagnostics.
//
// Timeout parameters control request handling:
//   - readTimeoutSecs: Maximum seconds for reading request (0 = no timeout)
//   - writeTimeoutSecs: Maximum seconds for writing response (0 = no timeout)
//   - idleTimeoutSecs: Maximum seconds to wait for next request (0 = no timeout)
func Serve(ctx context.Context, apiPort int, gitPort int, readTimeoutSecs int, writeTimeoutSecs int, idleTimeoutSecs int) error {
	mux := http.NewServeMux()

	// Register API routes.
	registerRoutes(mux)

	addr := fmt.Sprintf(":%d", apiPort)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  time.Duration(readTimeoutSecs) * time.Second,
		WriteTimeout: time.Duration(writeTimeoutSecs) * time.Second,
		IdleTimeout:  time.Duration(idleTimeoutSecs) * time.Second,
	}

	// Start server in goroutine so we can handle graceful shutdown.
	errChan := make(chan error, 1)
	go func() {
		slog.Info("Starting API server", "address", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for context cancellation or server error.
	select {
	case <-ctx.Done():
		slog.Info("Shutting down API server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errChan:
		return fmt.Errorf("server error: %w", err)
	}
}

// -----------------------------------------------------------------------------
// PRIVATE FUNCTIONS
// -----------------------------------------------------------------------------

// registerRoutes wires all HTTP handlers onto the provided mux.
func registerRoutes(mux *http.ServeMux) {
	// Health and diagnostics.
	mux.HandleFunc("GET /ironbark/api/v1/health", handleHealth)
	mux.HandleFunc("GET /ironbark/api/v1/version", handleVersion)
	mux.HandleFunc("GET /ironbark/api/v1/debug/settings", handleDebugSettings)

	// Kubeconfig management.
	mux.HandleFunc("POST /ironbark/api/v1/k8s/kubeconfig", handleKubeconfigUpload)
	mux.HandleFunc("GET /ironbark/api/v1/k8s/kubeconfig", handleKubeconfigGet)
	mux.HandleFunc("DELETE /ironbark/api/v1/k8s/kubeconfig", handleKubeconfigDelete)

	// Repository operations (placeholder for Phase 2/3).
	mux.HandleFunc("GET /ironbark/api/v1/repo", handleRepoList)

	// Cluster initialisation (placeholder for Phase 3).
	mux.HandleFunc("POST /ironbark/api/v1/init", handleInitAll)
	mux.HandleFunc("POST /ironbark/api/v1/init/packages", handleInitPackages)
	mux.HandleFunc("POST /ironbark/api/v1/init/argocd-repo-secrets", handleInitArgoCDRepoSecrets)
	mux.HandleFunc("POST /ironbark/api/v1/init/argocd-app-of-apps-repo", handleInitArgoCDAppOfAppsRepo)
	mux.HandleFunc("POST /ironbark/api/v1/init/argocd-app", handleInitArgoCDApp)
}
