/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package cmd

import (
	"log/slog"

	ironbarkGit "brightsparklabs.com/ironbark/internal/git"
	"brightsparklabs.com/ironbark/internal/settings"

	"github.com/spf13/cobra"
)

// newRepoCmd creates the repo command for managing repositories in the
// internal Git server (Gitea).
func newRepoCmd() *cobra.Command {
	repoCmd := &cobra.Command{
		Use:   "repo",
		Short: "Manage repositories in the internal Git server",
		Long: `Manage repositories in the internal Git server (Gitea).

The repo command provides operations for listing, cloning, pushing, and pulling
repositories from the Ironbark-managed internal Git server.`,
	}

	repoCmd.AddCommand(newRepoListCmd())
	repoCmd.AddCommand(newRepoCloneCmd())
	repoCmd.AddCommand(newRepoPushCmd())
	repoCmd.AddCommand(newRepoPullCmd())

	return repoCmd
}

// newRepoListCmd creates the list subcommand for listing repositories.
func newRepoListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all repositories in the internal Git server",
		Long: `List all repositories in the internal Git server (Gitea).

Displays repository names, URLs, and basic metadata.`,
		RunE: repoListExec,
	}
}

// newRepoCloneCmd creates the clone subcommand for cloning repositories.
func newRepoCloneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clone",
		Short: "Clone the ArgoCD App of Apps repository from the internal Git server",
		Long: `Clone the ArgoCD App of Apps repository from the internal Git server.

The repository is cloned to the configured ArgoCD repository directory.`,
		RunE: repoCloneExec,
	}
}

// newRepoPushCmd creates the push subcommand for pushing repositories.
func newRepoPushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "push",
		Short: "Push the ArgoCD App of Apps repository to the internal Git server",
		Long: `Push the ArgoCD App of Apps repository to the internal Git server.

Pushes all local commits to the remote repository in Gitea.`,
		RunE: repoPushExec,
	}
}

// newRepoPullCmd creates the pull subcommand for pulling repositories.
func newRepoPullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pull",
		Short: "Pull latest changes from the ArgoCD App of Apps repository",
		Long: `Pull latest changes from the ArgoCD App of Apps repository.

Updates the local repository with changes from the remote repository in Gitea.`,
		RunE: repoPullExec,
	}
}

// repoListExec lists all repositories in the internal Git server.
func repoListExec(cmd *cobra.Command, args []string) error {
	slog.Info("Listing repositories from internal Git server...")

	repos, err := ironbarkGit.ListRepos(cmd.Context())
	if err != nil {
		return WrapUserError(err, "Could not list repositories")
	}

	if len(repos) == 0 {
		slog.Info("No repositories found")
		return nil
	}

	slog.Info("Found repositories", "count", len(repos))
	for _, repo := range repos {
		slog.Info("Repository",
			"name", repo.Name,
			"cloneURL", repo.CloneURL,
			"owner", repo.Owner,
		)
	}

	return nil
}

// repoCloneExec clones the ArgoCD app of apps repository from the internal
// Git server.
func repoCloneExec(cmd *cobra.Command, args []string) error {
	argoCDRepoDir := settings.ArgoCDRepoDir()
	displayDir := settings.DisplayPath(argoCDRepoDir)
	slog.Info("Cloning ArgoCD App of Apps repository...", "localDir", displayDir)

	if err := ironbarkGit.CloneRepoArgoCDAppOfApps(cmd.Context(), argoCDRepoDir); err != nil {
		return WrapUserError(err, "Could not clone repository to `%s`", displayDir)
	}

	slog.Info("Successfully cloned ArgoCD App of Apps repository", "localDir", displayDir)
	return nil
}

// repoPushExec pushes the local ArgoCD app of apps repository to the
// internal Git server.
func repoPushExec(cmd *cobra.Command, args []string) error {
	argoCDRepoDir := settings.ArgoCDRepoDir()
	displayDir := settings.DisplayPath(argoCDRepoDir)
	slog.Info("Pushing ArgoCD App of Apps repository...", "localDir", displayDir)

	if err := ironbarkGit.PushRepoArgoCDAppOfApps(cmd.Context(), argoCDRepoDir); err != nil {
		return WrapUserError(err, "Could not push repository `%s`", displayDir)
	}

	slog.Info("Successfully pushed ArgoCD App of Apps repository")
	return nil
}

// repoPullExec pulls the latest changes from the ArgoCD app of apps
// repository.
func repoPullExec(cmd *cobra.Command, args []string) error {
	argoCDRepoDir := settings.ArgoCDRepoDir()
	displayDir := settings.DisplayPath(argoCDRepoDir)
	slog.Info("Pulling latest changes from ArgoCD App of Apps repository...", "localDir", displayDir)

	if err := ironbarkGit.PullRepoArgoCDAppOfApps(cmd.Context(), argoCDRepoDir); err != nil {
		return WrapUserError(err, "Could not pull repository `%s`", displayDir)
	}

	slog.Info("Successfully pulled latest changes")
	return nil
}
