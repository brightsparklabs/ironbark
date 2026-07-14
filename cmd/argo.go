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

// newArgoCmd creates the argo command for managing ArgoCD repositories.
func newArgoCmd() *cobra.Command {
	gitCmd := &cobra.Command{
		Use:   "argo",
		Short: "Interacts with ironbark managed repositories",
	}

	gitCmd.AddCommand(newPushCmd())
	gitCmd.AddCommand(newCloneCmd())

	return gitCmd
}

// newPushCmd creates the push subcommand for pushing the ArgoCD repository.
func newPushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "push",
		Short: "Push the ArgoCD repo to the internal Git server",
		RunE:  pushExec,
	}
}

// newCloneCmd creates the clone subcommand for cloning the ArgoCD repository.
func newCloneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clone",
		Short: "Clone the ArgoCD App of Apps repo from the internal Git server",
		RunE:  cloneExec,
	}
}

// pushExec pushes the local ArgoCD app of apps repo to the internal
// Git server. Failures are surfaced as user errors via the top-level
// handler in `Execute`.
func pushExec(cmd *cobra.Command, args []string) error {
	argoCDRepoDir := settings.ArgoCDRepoDir()
	displayDir := settings.DisplayPath(argoCDRepoDir)
	slog.Info("Pushing ArgoCD repo ...", "localDir", displayDir)
	if err := ironbarkGit.PushRepoArgoCDAppOfApps(cmd.Context(), argoCDRepoDir); err != nil {
		return WrapUserError(err, "Could not push repo `%s`", displayDir)
	}
	return nil
}

// cloneExec clones the ArgoCD app of apps repo from the internal Git
// server. Failures are surfaced as user errors via the top-level
// handler in `Execute`.
func cloneExec(cmd *cobra.Command, args []string) error {
	argoCDRepoDir := settings.ArgoCDRepoDir()
	displayDir := settings.DisplayPath(argoCDRepoDir)
	slog.Info("Cloning ArgoCD repo ...", "localDir", displayDir)
	if err := ironbarkGit.CloneRepoArgoCDAppOfApps(cmd.Context(), argoCDRepoDir); err != nil {
		return WrapUserError(err, "Could not clone repo to `%s`", displayDir)
	}
	slog.Info("Successfully cloned ArgoCD repo", "localDir", displayDir)
	return nil
}
