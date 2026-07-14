/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

package api

import (
	"net/http"
)

// -----------------------------------------------------------------------------
// INITIALISATION HANDLERS
// -----------------------------------------------------------------------------

// handleInitAll handles POST /ironbark/api/v1/init.
// Initialises all components (equivalent to `ironbark init all`).
// Placeholder implementation for Phase 3.
func handleInitAll(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement in Phase 3.
	writeJSONError(w, http.StatusNotImplemented, "Initialisation not yet implemented")
}

// handleInitPackages handles POST /ironbark/api/v1/init/packages.
// Deploys and mirrors packages.
// Placeholder implementation for Phase 3.
func handleInitPackages(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement in Phase 3.
	writeJSONError(w, http.StatusNotImplemented, "Package initialisation not yet implemented")
}

// handleInitArgoCDRepoSecrets handles POST /ironbark/api/v1/init/argocd-repo-secrets.
// Initialises ArgoCD repository secrets.
// Placeholder implementation for Phase 3.
func handleInitArgoCDRepoSecrets(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement in Phase 3.
	writeJSONError(w, http.StatusNotImplemented, "ArgoCD repo secrets initialisation not yet implemented")
}

// handleInitArgoCDAppOfAppsRepo handles POST /ironbark/api/v1/init/argocd-app-of-apps-repo.
// Initialises the ArgoCD App of Apps repository.
// Placeholder implementation for Phase 3.
func handleInitArgoCDAppOfAppsRepo(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement in Phase 3.
	writeJSONError(w, http.StatusNotImplemented, "ArgoCD App of Apps repo initialisation not yet implemented")
}

// handleInitArgoCDApp handles POST /ironbark/api/v1/init/argocd-app.
// Deploys the ArgoCD app of apps.
// Placeholder implementation for Phase 3.
func handleInitArgoCDApp(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement in Phase 3.
	writeJSONError(w, http.StatusNotImplemented, "ArgoCD app deployment not yet implemented")
}
