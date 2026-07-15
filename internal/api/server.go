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

	"brightsparklabs.com/ironbark/internal/gitproxy"
)

// -----------------------------------------------------------------------------
// CONSTANTS
// -----------------------------------------------------------------------------

// -----------------------------------------------------------------------------
// PUBLIC FUNCTIONS
// -----------------------------------------------------------------------------

// Serve starts both the HTTP API server and Git proxy server on their
// respective ports and blocks until the context is cancelled or an error occurs.
//
// The API server exposes REST endpoints under /ironbark/api/v1/ for cluster
// initialisation, repository management, kubeconfig handling, and diagnostics.
//
// The Git proxy server accepts Git HTTP protocol requests and forwards them
// to the internal Gitea server.
//
// Timeout parameters control server operations:
//   - apiTimeoutSecs: Timeout for API server operations (read, write, idle)
//   - gitTimeoutSecs: Timeout for Git proxy operations (read, write, idle)
func Serve(ctx context.Context, apiPort int, gitPort int, apiTimeoutSecs int, gitTimeoutSecs int) error {
	mux := http.NewServeMux()

	// Register API routes.
	registerRoutes(mux)

	addr := fmt.Sprintf(":%d", apiPort)
	timeout := time.Duration(apiTimeoutSecs) * time.Second
	apiServer := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
		IdleTimeout:  timeout,
	}

	// Start both servers in goroutines.
	errChan := make(chan error, 2)

	// Start API server.
	go func() {
		slog.Info("Starting API server", "address", addr)
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("API server error: %w", err)
		}
	}()

	// Start Git proxy server.
	go func() {
		if err := gitproxy.Serve(ctx, gitPort, gitTimeoutSecs); err != nil {
			errChan <- fmt.Errorf("Git proxy error: %w", err)
		}
	}()

	// Wait for context cancellation or server error.
	select {
	case <-ctx.Done():
		slog.Info("Shutting down servers")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Shutdown API server (Git proxy will stop via context).
		if err := apiServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("error shutting down API server: %w", err)
		}

		return nil
	case err := <-errChan:
		return err
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
