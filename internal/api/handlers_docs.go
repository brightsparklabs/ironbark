/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

package api

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"brightsparklabs.com/ironbark/resources"
)

// -----------------------------------------------------------------------------
// DOCUMENTATION HANDLERS
// -----------------------------------------------------------------------------

// handleWelcomePage handles GET / and GET /ironbark.
// Serves the embedded welcome.html page which provides links to documentation and API endpoints.
func handleWelcomePage(w http.ResponseWriter, r *http.Request) {
	// Load the welcome page from embedded resources.
	welcomeHTML, err := resources.ReadFile("welcome.html")
	if err != nil {
		// If we can't even load the embedded welcome page, serve a minimal fallback.
		slog.Error("Failed to load embedded welcome.html", "error", err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<h1>🌲 Welcome to Ironbark</h1><p>API server is running.</p>"))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(welcomeHTML)
	slog.Info("Served welcome page")
}

// handleDocsReadme handles GET /ironbark/docs/.
// Serves the README.html documentation file.
//
// The README.html file is expected to be located at /app/resources/docs/README.html.
// This path is coupled with the Dockerfile builder-documentation stage, which generates
// the documentation and copies it to this location in the final image.
// See Dockerfile: COPY --from=builder-documentation /build/docs/README.html /app/resources/docs/
//
// This handler should only be registered if the README.html file exists.
func handleDocsReadme(w http.ResponseWriter, r *http.Request) {
	// Path where README.html is copied during Docker build.
	// This MUST match the COPY destination in the Dockerfile.
	readmePath := "/app/resources/docs/README.html"

	// Serve the file with appropriate content type.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Disposition", "inline; filename="+filepath.Base(readmePath))

	http.ServeFile(w, r, readmePath)
	slog.Info("Served README.html documentation", "path", readmePath)
}

// docsReadmeExists checks if the README.html documentation file exists.
func docsReadmeExists() bool {
	readmePath := "/app/resources/docs/README.html"
	_, err := os.Stat(readmePath)
	return err == nil
}
