/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

package api

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// -----------------------------------------------------------------------------
// ARTIFACT DOWNLOAD HANDLERS
// -----------------------------------------------------------------------------

// handleArtifactsList handles GET /ironbark/api/v1/artifact.
// Lists available artifacts that can be downloaded.
func handleArtifactsList(w http.ResponseWriter, r *http.Request) {
	artifacts := []map[string]string{}

	// Check for zarf-init package.
	zarfInitDir := "/app/resources/zarf/init"
	if files, err := os.ReadDir(zarfInitDir); err == nil {
		for _, file := range files {
			if strings.HasPrefix(file.Name(), "zarf-init-") && strings.HasSuffix(file.Name(), ".tar.zst") {
				artifacts = append(artifacts, map[string]string{
					"name":        "zarf-init",
					"path":        filepath.Join(zarfInitDir, file.Name()),
					"filename":    file.Name(),
					"downloadURL": "/ironbark/api/v1/artifact/zarf-init",
				})
			}
		}
	}

	// Check for RKE2 artifacts (only in ironbark-rke2 variant).
	rke2Dir := "/app/resources/rke2"
	if stat, err := os.Stat(rke2Dir); err == nil && stat.IsDir() {
		artifacts = append(artifacts, map[string]string{
			"name":        "rke2",
			"path":        rke2Dir,
			"type":        "directory",
			"downloadURL": "/ironbark/api/v1/artifact/rke2",
		})
	}

	response := map[string]interface{}{
		"artifacts": artifacts,
		"count":     len(artifacts),
	}

	writeJSONResponse(w, http.StatusOK, response)
}

// handleZarfInitDownload handles GET /ironbark/api/v1/artifact/zarf-init.
// Downloads the zarf-init package.
func handleZarfInitDownload(w http.ResponseWriter, r *http.Request) {
	zarfInitDir := "/app/resources/zarf/init"

	// Find the zarf-init package.
	files, err := os.ReadDir(zarfInitDir)
	if err != nil {
		slog.Error("Failed to read zarf init directory", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to read zarf init directory")
		return
	}

	var zarfInitFile string
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "zarf-init-") && strings.HasSuffix(file.Name(), ".tar.zst") {
			zarfInitFile = filepath.Join(zarfInitDir, file.Name())
			break
		}
	}

	if zarfInitFile == "" {
		writeJSONError(w, http.StatusNotFound, "Zarf init package not found")
		return
	}

	// Serve the file.
	w.Header().Set("Content-Type", "application/zstd")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filepath.Base(zarfInitFile)))

	http.ServeFile(w, r, zarfInitFile)
	slog.Info("Served zarf-init package", "file", zarfInitFile)
}

// handleRKE2Download handles GET /ironbark/api/v1/artifact/rke2.
// Downloads RKE2 artifacts as a tar.gz archive (only available in ironbark-rke2 variant).
func handleRKE2Download(w http.ResponseWriter, r *http.Request) {
	rke2Dir := "/app/resources/rke2"

	// Check if RKE2 directory exists.
	if stat, err := os.Stat(rke2Dir); err != nil || !stat.IsDir() {
		writeJSONError(w, http.StatusNotFound, "RKE2 artifacts not available (this is the K3s variant)")
		return
	}

	// Create tar.gz on the fly.
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", "attachment; filename=rke2-artifacts.tar.gz")

	gzWriter := gzip.NewWriter(w)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	// Walk the rke2 directory and add all files to the tar.
	err := filepath.Walk(rke2Dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Create tar header.
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		// Preserve relative path structure.
		relPath, err := filepath.Rel(rke2Dir, path)
		if err != nil {
			return err
		}
		header.Name = filepath.Join("rke2", relPath)

		// Write header.
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// Write file content if it's a regular file.
		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			if _, err := io.Copy(tarWriter, file); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		slog.Error("Failed to create RKE2 archive", "error", err)
		// Can't write JSON error here as we've already started writing the tar.
		return
	}

	slog.Info("Served RKE2 artifacts archive")
}
