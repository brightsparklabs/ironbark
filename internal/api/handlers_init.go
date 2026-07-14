/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

package api

import (
	"log/slog"
	"net/http"
)

// -----------------------------------------------------------------------------
// INITIALISATION HANDLERS
// -----------------------------------------------------------------------------

// handleInitAll handles POST /ironbark/api/v1/init.
// Initialises all components (equivalent to `ironbark init all`).
//
// Note: Full implementation deferred to Phase 4 (Git Proxy).
// For now, returns not implemented to avoid premature refactoring.
func handleInitAll(w http.ResponseWriter, r *http.Request) {
	slog.Info("API: Full initialisation requested")
	writeJSONError(w, http.StatusNotImplemented, "Full initialisation via API not yet implemented. Use CLI: ironbark init all")
}

// handleInitPackages handles POST /ironbark/api/v1/init/packages.
// Deploys and mirrors packages.
//
// Note: Implementation deferred to Phase 4. Init operations are complex
// and tightly coupled to CLI for now.
func handleInitPackages(w http.ResponseWriter, r *http.Request) {
	writeJSONError(w, http.StatusNotImplemented, "Package initialisation via API not yet implemented. Use CLI: ironbark init packages")
}

// handleInitArgoCDRepoSecrets handles POST /ironbark/api/v1/init/argocd-repo-secrets.
// Initialises ArgoCD repository secrets.
//
// Note: Implementation deferred to Phase 4.
func handleInitArgoCDRepoSecrets(w http.ResponseWriter, r *http.Request) {
	writeJSONError(w, http.StatusNotImplemented, "ArgoCD repo secrets initialisation via API not yet implemented. Use CLI: ironbark init argocd-repo-secrets")
}

// handleInitArgoCDAppOfAppsRepo handles POST /ironbark/api/v1/init/argocd-app-of-apps-repo.
// Initialises the ArgoCD App of Apps repository.
//
// Note: Implementation deferred to Phase 4.
func handleInitArgoCDAppOfAppsRepo(w http.ResponseWriter, r *http.Request) {
	writeJSONError(w, http.StatusNotImplemented, "ArgoCD App of Apps repo initialisation via API not yet implemented. Use CLI: ironbark init argocd-app-of-apps-repo")
}

// handleInitArgoCDApp handles POST /ironbark/api/v1/init/argocd-app.
// Deploys the ArgoCD app of apps.
//
// Note: Implementation deferred to Phase 4.
func handleInitArgoCDApp(w http.ResponseWriter, r *http.Request) {
	writeJSONError(w, http.StatusNotImplemented, "ArgoCD app deployment via API not yet implemented. Use CLI: ironbark init argocd-app")
}
