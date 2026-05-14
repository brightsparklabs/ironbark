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
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push the ArgoCD repo to the internal Git server",
		Run:   pushExec,
	}

	return cmd
}

// newCloneCmd creates the clone subcommand for cloning the ArgoCD repository.
func newCloneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clone",
		Short: "Clone the ArgoCD App of Apps repo from the internal Git server",
		Run:   cloneExec,
	}

	return cmd
}

// pushExec pushes the local ArgoCD app of apps repo to the internal Git server.
func pushExec(cmd *cobra.Command, args []string) {
	argoCDRepoDir := settings.ArgoCDRepoDir()
	displayDir := settings.DisplayPath(argoCDRepoDir)
	slog.Info("Pushing ArgoCD repo ...", "localDir", displayDir)
	err := ironbarkGit.PushRepoArgoCDAppOfApps(cmd.Context(), argoCDRepoDir)
	exitOnError(err, "Could not push repo `"+displayDir+"`")
}

// cloneExec clones the ArgoCD app of apps repo from the internal Git server.
func cloneExec(cmd *cobra.Command, args []string) {
	argoCDRepoDir := settings.ArgoCDRepoDir()
	displayDir := settings.DisplayPath(argoCDRepoDir)
	slog.Info("Cloning ArgoCD repo ...", "localDir", displayDir)
	err := ironbarkGit.CloneRepoArgoCDAppOfApps(cmd.Context(), argoCDRepoDir)
	exitOnError(err, "Could not clone repo to `"+displayDir+"`")
	slog.Info("Successfully cloned ArgoCD repo", "localDir", displayDir)
}
