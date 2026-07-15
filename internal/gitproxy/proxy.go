/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

// Package gitproxy provides a Git HTTP protocol proxy that forwards requests
// to the internal Gitea server via a Zarf tunnel. This allows external Git
// clients to interact with repositories in the air-gapped environment.
package gitproxy

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"brightsparklabs.com/ironbark/internal/zarf"
	zarfcluster "github.com/zarf-dev/zarf/src/pkg/cluster"
	zarfstate "github.com/zarf-dev/zarf/src/pkg/state"
)

// -----------------------------------------------------------------------------
// PUBLIC FUNCTIONS
// -----------------------------------------------------------------------------

// Serve starts the Git proxy server on the specified port and blocks until
// the context is cancelled or an error occurs.
//
// The proxy creates a tunnel to the internal Gitea server and forwards Git
// HTTP protocol requests through it, handling authentication automatically.
//
// The timeoutSecs parameter controls read, write, and idle timeouts for the
// Git proxy server.
func Serve(ctx context.Context, gitPort int, timeoutSecs int) error {
	slog.Info("Starting Git proxy server", "port", gitPort)

	// Create a lazy proxy that connects on first request.
	// This allows the server to start even if no cluster is available.
	proxy := createLazyGiteaProxy(ctx)

	// Create HTTP server with the proxy handler.
	addr := fmt.Sprintf(":%d", gitPort)
	timeout := time.Duration(timeoutSecs) * time.Second
	server := &http.Server{
		Addr:         addr,
		Handler:      proxy,
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
		IdleTimeout:  timeout,
	}

	// Start server in goroutine so we can handle graceful shutdown.
	errChan := make(chan error, 1)
	go func() {
		slog.Info("Git proxy server listening", "address", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for context cancellation or server error.
	select {
	case <-ctx.Done():
		slog.Info("Shutting down Git proxy server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errChan:
		return fmt.Errorf("Git proxy server error: %w", err)
	}
}

// -----------------------------------------------------------------------------
// PRIVATE FUNCTIONS
// -----------------------------------------------------------------------------

// createLazyGiteaProxy creates a handler that lazily creates the Gitea tunnel
// and proxy on the first request. This allows the server to start even if no
// cluster is available.
func createLazyGiteaProxy(ctx context.Context) http.Handler {
	var (
		proxyMu  sync.RWMutex
		proxy    *httputil.ReverseProxy
		tunnel   *zarfcluster.Tunnel
		proxyErr error
		created  bool
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if proxy is already created.
		proxyMu.RLock()
		if created {
			proxyMu.RUnlock()
			if proxyErr != nil {
				http.Error(w, fmt.Sprintf("Git proxy unavailable: %v", proxyErr), http.StatusServiceUnavailable)
				return
			}
			proxy.ServeHTTP(w, r)
			return
		}
		proxyMu.RUnlock()

		// Create proxy (first request only).
		proxyMu.Lock()
		defer proxyMu.Unlock()

		// Check again in case another goroutine created it.
		if created {
			if proxyErr != nil {
				http.Error(w, fmt.Sprintf("Git proxy unavailable: %v", proxyErr), http.StatusServiceUnavailable)
				return
			}
			proxy.ServeHTTP(w, r)
			return
		}

		// Mark as created (even if it fails, we don't want to retry on every request).
		created = true

		// Create the tunnel and proxy.
		slog.Info("Creating Gitea tunnel (first request)")

		// Get cluster.
		zarfCluster, err := zarf.GetCluster(ctx)
		if err != nil {
			proxyErr = fmt.Errorf("could not get Zarf cluster: %w", err)
			slog.Error("Failed to create Gitea proxy", "error", proxyErr)
			http.Error(w, fmt.Sprintf("Git proxy unavailable: %v", proxyErr), http.StatusServiceUnavailable)
			return
		}

		// Get Git server info.
		gitServerInfo, err := zarf.GetGitServerInfo(ctx)
		if err != nil {
			proxyErr = fmt.Errorf("could not get Git server info: %w", err)
			slog.Error("Failed to create Gitea proxy", "error", proxyErr)
			http.Error(w, fmt.Sprintf("Git proxy unavailable: %v", proxyErr), http.StatusServiceUnavailable)
			return
		}

		// Create tunnel.
		tunnel, err = zarfCluster.NewTunnel(zarfstate.ZarfNamespaceName, zarfcluster.SvcResource, zarfcluster.ZarfGitServerName, "", 0, zarfcluster.ZarfGitServerPort)
		if err != nil {
			proxyErr = fmt.Errorf("could not create Gitea tunnel: %w", err)
			slog.Error("Failed to create Gitea proxy", "error", proxyErr)
			http.Error(w, fmt.Sprintf("Git proxy unavailable: %v", proxyErr), http.StatusServiceUnavailable)
			return
		}

		// Connect tunnel.
		_, err = tunnel.Connect(ctx)
		if err != nil {
			proxyErr = fmt.Errorf("could not connect to Gitea tunnel: %w", err)
			slog.Error("Failed to create Gitea proxy", "error", proxyErr)
			http.Error(w, fmt.Sprintf("Git proxy unavailable: %v", proxyErr), http.StatusServiceUnavailable)
			return
		}

		// Get tunnel endpoint.
		tunnelURLs := tunnel.HTTPEndpoints()
		if len(tunnelURLs) == 0 {
			proxyErr = fmt.Errorf("no tunnel HTTP endpoints available")
			slog.Error("Failed to create Gitea proxy", "error", proxyErr)
			http.Error(w, fmt.Sprintf("Git proxy unavailable: %v", proxyErr), http.StatusServiceUnavailable)
			return
		}

		tunnelURL := tunnelURLs[0]
		slog.Info("Created Gitea tunnel", "tunnelURL", tunnelURL)

		// Create the reverse proxy.
		proxy, err = createGiteaProxy(tunnelURL, gitServerInfo)
		if err != nil {
			proxyErr = fmt.Errorf("failed to create proxy: %w", err)
			slog.Error("Failed to create Gitea proxy", "error", proxyErr)
			http.Error(w, fmt.Sprintf("Git proxy unavailable: %v", proxyErr), http.StatusServiceUnavailable)
			return
		}

		// Proxy the request.
		proxy.ServeHTTP(w, r)
	})
}

// createGiteaProxy creates a reverse proxy that forwards requests to Gitea
// via the tunnel.
func createGiteaProxy(tunnelURL string, gitServerInfo *zarfstate.GitServerInfo) (*httputil.ReverseProxy, error) {
	// Parse the tunnel URL.
	targetURL, err := url.Parse(tunnelURL)
	if err != nil {
		return nil, fmt.Errorf("invalid tunnel URL: %w", err)
	}

	// Create reverse proxy with custom rewrite function.
	proxy := &httputil.ReverseProxy{}

	// Customise the request before forwarding.
	proxy.Rewrite = func(req *httputil.ProxyRequest) {
		// Set the target URL.
		req.SetURL(targetURL)
		req.SetXForwarded()

		// Add Gitea authentication using push credentials.
		// Push user owns the repositories and has full access.
		req.Out.SetBasicAuth(gitServerInfo.PushUsername, gitServerInfo.PushPassword)

		// Log the proxied request.
		slog.Debug("Proxying Git request",
			"method", req.Out.Method,
			"path", req.Out.URL.Path,
			"service", req.Out.URL.Query().Get("service"),
		)
	}

	// Handle errors from the backend.
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("Git proxy error",
			"error", err,
			"method", r.Method,
			"path", r.URL.Path,
		)
		http.Error(w, "Git proxy error: "+err.Error(), http.StatusBadGateway)
	}

	// Add response modifier to log responses.
	proxy.ModifyResponse = func(resp *http.Response) error {
		slog.Debug("Git proxy response",
			"status", resp.StatusCode,
			"path", resp.Request.URL.Path,
		)
		return nil
	}

	return proxy, nil
}
