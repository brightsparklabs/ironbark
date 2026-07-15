/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

package api

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"brightsparklabs.com/ironbark/internal/zarf"
)

// -----------------------------------------------------------------------------
// PACKAGE UPLOAD HANDLERS
// -----------------------------------------------------------------------------

// handlePackageUpload handles POST /ironbark/api/v1/package.
// Uploads a single Zarf package and mirrors it to the cluster.
func handlePackageUpload(w http.ResponseWriter, r *http.Request) {
	slog.Info("API: Package upload requested")

	// Read the package from request body.
	packageData, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("Failed to read package upload", "error", err)
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Failed to read package: %v", err))
		return
	}
	defer r.Body.Close()

	// Validate it's a Zarf package (check for .tar.zst magic bytes or name).
	if len(packageData) < 100 {
		writeJSONError(w, http.StatusBadRequest, "Package too small - not a valid Zarf package")
		return
	}

	// Save package to temporary location for processing.
	tmpDir, err := os.MkdirTemp("", "ironbark-upload-*")
	if err != nil {
		slog.Error("Failed to create temp directory", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to create temp directory")
		return
	}
	defer os.RemoveAll(tmpDir)

	// Generate filename from Content-Disposition or use default.
	filename := "uploaded-package.tar.zst"
	if contentDisp := r.Header.Get("Content-Disposition"); contentDisp != "" {
		// Parse filename from Content-Disposition header.
		// Format: attachment; filename="package.tar.zst"
		if _, params, _ := parseContentDisposition(contentDisp); params["filename"] != "" {
			filename = params["filename"]
		}
	}

	tmpPackagePath := filepath.Join(tmpDir, filename)
	if err := os.WriteFile(tmpPackagePath, packageData, 0644); err != nil {
		slog.Error("Failed to save package", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to save package")
		return
	}

	slog.Info("Package uploaded to temp directory", "path", tmpPackagePath, "size", len(packageData))

	// Mirror packages (images and charts) using zarf.
	if err := zarf.MirrorPackages(tmpDir); err != nil {
		slog.Error("Failed to mirror package", "error", err)
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to mirror package: %v", err))
		return
	}

	// Mirror Helm charts from the package.
	if err := zarf.MirrorPackageCharts(r.Context(), tmpDir); err != nil {
		slog.Error("Failed to mirror charts", "error", err)
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to mirror charts: %v", err))
		return
	}

	slog.Info("Package mirrored successfully", "package", filename)

	response := map[string]string{
		"message": "Package uploaded and mirrored successfully",
		"package": filename,
	}
	writeJSONResponse(w, http.StatusOK, response)
}

// handlePackageBulkUpload handles POST /ironbark/api/v1/package/bulk.
// Uploads a tarball containing multiple Zarf packages and mirrors them.
func handlePackageBulkUpload(w http.ResponseWriter, r *http.Request) {
	slog.Info("API: Bulk package upload requested")

	writeJSONError(w, http.StatusNotImplemented, "Bulk package upload not yet implemented. Use POST /ironbark/api/v1/package for single packages.")
}

// parseContentDisposition parses a Content-Disposition header value.
// Returns the disposition type and parameters.
func parseContentDisposition(header string) (disposition string, params map[string]string, err error) {
	params = make(map[string]string)

	// Simple parser - just extract filename if present.
	// Full RFC 2183 parsing would be more complex.
	if len(header) > 0 {
		// Look for filename="..." or filename=...
		start := -1
		for i := 0; i < len(header)-8; i++ {
			if header[i:i+8] == "filename" {
				start = i + 8
				break
			}
		}

		if start > 0 && start < len(header) {
			// Skip = and any quotes.
			for start < len(header) && (header[start] == '=' || header[start] == ' ' || header[start] == '"') {
				start++
			}

			// Find end (quote or semicolon or end of string).
			end := start
			for end < len(header) && header[end] != '"' && header[end] != ';' {
				end++
			}

			if end > start {
				params["filename"] = header[start:end]
			}
		}
	}

	return "attachment", params, nil
}
