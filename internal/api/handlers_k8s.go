/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

package api

import (
	"io"
	"log/slog"
	"net/http"
)

// -----------------------------------------------------------------------------
// KUBERNETES HANDLERS
// -----------------------------------------------------------------------------

// handleKubeconfigUpload handles POST /ironbark/api/v1/k8s/kubeconfig.
// Accepts a kubeconfig file upload and stores it for use by Ironbark operations.
func handleKubeconfigUpload(w http.ResponseWriter, r *http.Request) {
	// Read the request body (kubeconfig content).
	content, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}
	defer r.Body.Close()

	if len(content) == 0 {
		writeJSONError(w, http.StatusBadRequest, "Empty kubeconfig content")
		return
	}

	// Save the uploaded kubeconfig (includes validation).
	uploadedPath, err := SaveUploadedKubeconfig(content)
	if err != nil {
		slog.Error("Failed to save uploaded kubeconfig", "error", err)
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Set the KUBECONFIG environment variable globally so all subsequent
	// cluster operations can use it.
	if err := SetKubeconfigEnvFromPath(uploadedPath); err != nil {
		slog.Error("Failed to set KUBECONFIG environment variable", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Kubeconfig saved but failed to set globally: "+err.Error())
		return
	}

	response := map[string]string{
		"message": "Kubeconfig uploaded successfully. This will override any mounted kubeconfig.",
		"source":  "uploaded",
	}
	writeJSONResponse(w, http.StatusOK, response)
}

// handleKubeconfigGet handles GET /ironbark/api/v1/k8s/kubeconfig.
// Returns the currently active kubeconfig.
func handleKubeconfigGet(w http.ResponseWriter, r *http.Request) {
	content, source, err := GetActiveKubeconfig()
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	// Return the kubeconfig content as plain text (YAML format).
	w.Header().Set("Content-Type", "text/yaml")
	w.Header().Set("X-Kubeconfig-Source", source)
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(content); err != nil {
		slog.Error("Failed to write kubeconfig response", "error", err)
	}
}

// handleKubeconfigDelete handles DELETE /ironbark/api/v1/k8s/kubeconfig.
// Removes any uploaded kubeconfig, reverting to mounted kubeconfig if present.
func handleKubeconfigDelete(w http.ResponseWriter, r *http.Request) {
	if err := DeleteUploadedKubeconfig(); err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	// Check if a mounted kubeconfig is available after deletion.
	kubeconfigPath, source, err := GetActiveKubeconfigPath()
	message := "Uploaded kubeconfig deleted."
	if err == nil {
		message += " Reverted to " + source + "."
		// Update KUBECONFIG environment variable to point to the fallback.
		if err := SetKubeconfigEnvFromPath(kubeconfigPath); err != nil {
			slog.Error("Failed to update KUBECONFIG environment variable", "error", err)
		}
	} else {
		message += " No mounted kubeconfig available."
		// Clear the KUBECONFIG environment variable since no kubeconfig is available.
		ClearKubeconfigEnv()
	}

	response := map[string]string{
		"message": message,
	}
	writeJSONResponse(w, http.StatusOK, response)
}
