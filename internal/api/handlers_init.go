/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

package api

import (
	"log/slog"
	"net/http"

	ironbarkInit "brightsparklabs.com/ironbark/internal/init"
)

// -----------------------------------------------------------------------------
// INITIALISATION HANDLERS
// -----------------------------------------------------------------------------

// handleInitAll handles POST /ironbark/api/v1/init.
// Initialises all components - smart and idempotent.
// Automatically detects and skips already-completed steps.
func handleInitAll(w http.ResponseWriter, r *http.Request) {
	slog.Info("API: Full initialisation requested")

	if err := ironbarkInit.InitAll(r.Context()); err != nil {
		slog.Error("Full initialization failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := map[string]string{
		"message": "All components initialized successfully",
	}
	writeJSONResponse(w, http.StatusOK, response)
}

// handleInitZarf handles POST /ironbark/api/v1/init/zarf.
// Initializes Zarf with git-server component.
// Requires zarf binary and zarf-init package to be available.
func handleInitZarf(w http.ResponseWriter, r *http.Request) {
	slog.Info("API: Zarf initialization requested")

	if err := ironbarkInit.InitZarf(r.Context()); err != nil {
		slog.Error("Zarf initialization failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := map[string]string{
		"message": "Zarf initialized successfully",
	}
	writeJSONResponse(w, http.StatusOK, response)
}

// handleInitPackages handles POST /ironbark/api/v1/init/packages.
// Deploys and mirrors packages.
// Requires Zarf to be initialized first.
func handleInitPackages(w http.ResponseWriter, r *http.Request) {
	slog.Info("API: Package initialization requested")

	if err := ironbarkInit.InitPackages(r.Context()); err != nil {
		slog.Error("Package initialization failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := map[string]string{
		"message": "Packages initialized successfully",
	}
	writeJSONResponse(w, http.StatusOK, response)
}

// handleInitArgoCDRepoSecrets handles POST /ironbark/api/v1/init/argocd-repo-secrets.
// Initialises ArgoCD repository secrets.
// Requires Zarf to be initialized first.
func handleInitArgoCDRepoSecrets(w http.ResponseWriter, r *http.Request) {
	slog.Info("API: ArgoCD repo secrets initialization requested")

	if err := ironbarkInit.InitArgoCDRepoSecrets(r.Context()); err != nil {
		slog.Error("ArgoCD repo secrets initialization failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := map[string]string{
		"message": "ArgoCD repository secrets initialized successfully",
	}
	writeJSONResponse(w, http.StatusOK, response)
}

// handleInitArgoCDAppOfAppsRepo handles POST /ironbark/api/v1/init/argocd-app-of-apps-repo.
// Initialises the ArgoCD App of Apps repository.
// Requires Zarf to be initialized first.
func handleInitArgoCDAppOfAppsRepo(w http.ResponseWriter, r *http.Request) {
	slog.Info("API: ArgoCD App of Apps repo initialization requested")

	if err := ironbarkInit.InitArgoCDAppOfAppsRepo(r.Context()); err != nil {
		slog.Error("ArgoCD App of Apps repo initialization failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := map[string]string{
		"message": "ArgoCD App of Apps repository initialized successfully",
	}
	writeJSONResponse(w, http.StatusOK, response)
}

// handleInitArgoCDApp handles POST /ironbark/api/v1/init/argocd-app.
// Deploys the ArgoCD app of apps.
// Requires Zarf to be initialized first.
func handleInitArgoCDApp(w http.ResponseWriter, r *http.Request) {
	slog.Info("API: ArgoCD app deployment requested")

	if err := ironbarkInit.InitArgoCDApp(); err != nil {
		slog.Error("ArgoCD app deployment failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := map[string]string{
		"message": "ArgoCD App of Apps deployed successfully",
	}
	writeJSONResponse(w, http.StatusOK, response)
}
