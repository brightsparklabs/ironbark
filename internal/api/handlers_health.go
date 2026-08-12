/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

package api

import (
	"net/http"

	"brightsparklabs.com/ironbark/internal/settings"
	"brightsparklabs.com/ironbark/internal/version"
)

// -----------------------------------------------------------------------------
// HEALTH & DIAGNOSTIC HANDLERS
// -----------------------------------------------------------------------------

// handleHealth handles GET /ironbark/api/v1/health.
// Returns a simple health check response indicating the service is running.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	response := map[string]string{
		"status": "healthy",
	}
	writeJSONResponse(w, http.StatusOK, response)
}

// handleVersion handles GET /ironbark/api/v1/version.
// Returns version information about the Ironbark binary.
func handleVersion(w http.ResponseWriter, r *http.Request) {
	response := map[string]string{
		"version":   version.GetVersion(),
		"commit":    version.GetCommit(),
		"buildTime": version.GetBuildTime(),
	}
	writeJSONResponse(w, http.StatusOK, response)
}

// handleDebugSettings handles GET /ironbark/api/v1/debug/settings.
// Returns the current configuration settings for debugging purposes.
func handleDebugSettings(w http.ResponseWriter, r *http.Request) {
	allSettings := settings.ResolveAll()

	// Convert to a map for JSON serialisation.
	settingsMap := make([]map[string]interface{}, 0, len(allSettings))
	for _, s := range allSettings {
		source := "default"
		if s.FromEnv {
			source = "environment"
		}
		settingsMap = append(settingsMap, map[string]interface{}{
			"name":   s.Var.Name,
			"value":  s.Value,
			"source": source,
		})
	}

	writeJSONResponse(w, http.StatusOK, settingsMap)
}
