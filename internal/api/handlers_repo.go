/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

package api

import (
	"net/http"
)

// -----------------------------------------------------------------------------
// REPOSITORY HANDLERS
// -----------------------------------------------------------------------------

// handleRepoList handles GET /ironbark/api/v1/repo.
// Lists repositories in the internal Gitea instance.
// Placeholder implementation for Phase 2/3.
func handleRepoList(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement in Phase 2/3.
	writeJSONError(w, http.StatusNotImplemented, "Repository listing not yet implemented")
}
