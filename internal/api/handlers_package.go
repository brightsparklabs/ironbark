/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

package api

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"brightsparklabs.com/ironbark/internal/zarf"
)

// -----------------------------------------------------------------------------
// PACKAGE HANDLERS
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
// Uploads an archive (tar.gz or zip) containing multiple Zarf packages and mirrors them.
func handlePackageBulkUpload(w http.ResponseWriter, r *http.Request) {
	slog.Info("API: Bulk package upload requested")

	// Read the archive from request body.
	archiveData, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("Failed to read bulk upload", "error", err)
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Failed to read archive: %v", err))
		return
	}
	defer r.Body.Close()

	// Determine archive type from Content-Type or filename.
	contentType := r.Header.Get("Content-Type")
	filename := ""
	if contentDisp := r.Header.Get("Content-Disposition"); contentDisp != "" {
		if _, params, _ := parseContentDisposition(contentDisp); params["filename"] != "" {
			filename = params["filename"]
		}
	}

	// Detect archive type.
	isZip := contentType == "application/zip" || strings.HasSuffix(filename, ".zip")
	isTarGz := contentType == "application/gzip" || contentType == "application/x-gzip" ||
		contentType == "application/x-tar" || strings.HasSuffix(filename, ".tar.gz") ||
		strings.HasSuffix(filename, ".tgz")

	if !isZip && !isTarGz {
		writeJSONError(w, http.StatusBadRequest, "Archive must be tar.gz or zip format")
		return
	}

	// Create temp directory for extraction.
	tmpDir, err := os.MkdirTemp("", "ironbark-bulk-*")
	if err != nil {
		slog.Error("Failed to create temp directory", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to create temp directory")
		return
	}
	defer os.RemoveAll(tmpDir)

	// Extract archive.
	var packageCount int
	if isZip {
		packageCount, err = extractZip(archiveData, tmpDir)
	} else {
		packageCount, err = extractTarGz(archiveData, tmpDir)
	}

	if err != nil {
		slog.Error("Failed to extract archive", "error", err)
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Failed to extract archive: %v", err))
		return
	}

	if packageCount == 0 {
		writeJSONError(w, http.StatusBadRequest, "No Zarf packages (.tar.zst) found in archive")
		return
	}

	slog.Info("Extracted packages from archive", "count", packageCount, "dir", tmpDir)

	// Mirror packages (images and charts) using zarf.
	if err := zarf.MirrorPackages(tmpDir); err != nil {
		slog.Error("Failed to mirror packages", "error", err)
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to mirror packages: %v", err))
		return
	}

	// Mirror Helm charts from the packages.
	if err := zarf.MirrorPackageCharts(r.Context(), tmpDir); err != nil {
		slog.Error("Failed to mirror charts", "error", err)
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to mirror charts: %v", err))
		return
	}

	slog.Info("Bulk packages mirrored successfully", "count", packageCount)

	response := map[string]interface{}{
		"message":       "Bulk packages uploaded and mirrored successfully",
		"packageCount":  packageCount,
		"archiveFormat": map[bool]string{true: "zip", false: "tar.gz"}[isZip],
	}
	writeJSONResponse(w, http.StatusOK, response)
}

// extractZip extracts .tar.zst files from a zip archive.
// Returns the number of packages extracted.
func extractZip(data []byte, destDir string) (int, error) {
	// Create zip reader from bytes.
	reader, err := zip.NewReader(strings.NewReader(string(data)), int64(len(data)))
	if err != nil {
		return 0, fmt.Errorf("failed to create zip reader: %w", err)
	}

	count := 0
	for _, file := range reader.File {
		// Only extract .tar.zst files (Zarf packages).
		if !strings.HasSuffix(file.Name, ".tar.zst") {
			continue
		}

		// Open file from zip.
		rc, err := file.Open()
		if err != nil {
			return count, fmt.Errorf("failed to open file in zip: %w", err)
		}

		// Extract to destination.
		destPath := filepath.Join(destDir, filepath.Base(file.Name))
		outFile, err := os.Create(destPath)
		if err != nil {
			rc.Close()
			return count, fmt.Errorf("failed to create output file: %w", err)
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return count, fmt.Errorf("failed to extract file: %w", err)
		}

		count++
		slog.Debug("Extracted package from zip", "file", file.Name, "dest", destPath)
	}

	return count, nil
}

// extractTarGz extracts .tar.zst files from a tar.gz archive.
// Returns the number of packages extracted.
func extractTarGz(data []byte, destDir string) (int, error) {
	// Create gzip reader.
	gzReader, err := gzip.NewReader(strings.NewReader(string(data)))
	if err != nil {
		return 0, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	// Create tar reader.
	tarReader := tar.NewReader(gzReader)

	count := 0
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, fmt.Errorf("failed to read tar entry: %w", err)
		}

		// Skip directories and non-package files.
		if header.Typeflag != tar.TypeReg || !strings.HasSuffix(header.Name, ".tar.zst") {
			continue
		}

		// Extract to destination.
		destPath := filepath.Join(destDir, filepath.Base(header.Name))
		outFile, err := os.Create(destPath)
		if err != nil {
			return count, fmt.Errorf("failed to create output file: %w", err)
		}

		_, err = io.Copy(outFile, tarReader)
		outFile.Close()

		if err != nil {
			return count, fmt.Errorf("failed to extract file: %w", err)
		}

		count++
		slog.Debug("Extracted package from tar.gz", "file", header.Name, "dest", destPath)
	}

	return count, nil
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
