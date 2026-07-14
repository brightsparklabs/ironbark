/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

package api

import (
	"log/slog"
	"net/http"

	ironbarkGit "brightsparklabs.com/ironbark/internal/git"
)

// -----------------------------------------------------------------------------
// REPOSITORY HANDLERS
// -----------------------------------------------------------------------------

// handleRepoList handles GET /ironbark/api/v1/repo.
// Lists repositories in the internal Gitea instance.
func handleRepoList(w http.ResponseWriter, r *http.Request) {
	repos, err := ironbarkGit.ListRepos(r.Context())
	if err != nil {
		slog.Error("Failed to list repositories", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to list repositories")
		return
	}

	// Convert to a response format suitable for JSON.
	repoList := make([]map[string]string, 0, len(repos))
	for _, repo := range repos {
		repoList = append(repoList, map[string]string{
			"name":     repo.Name,
			"cloneURL": repo.CloneURL,
			"owner":    repo.Owner,
		})
	}

	response := map[string]interface{}{
		"repositories": repoList,
		"count":        len(repos),
	}

	writeJSONResponse(w, http.StatusOK, response)
}
