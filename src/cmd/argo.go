/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package cmd

import (
	"brightsparklabs.com/ironbark/internal/constants"
	ironbarkGit "brightsparklabs.com/ironbark/internal/git"

	"github.com/spf13/cobra"
)

func newArgoCmd() *cobra.Command {
	gitCmd := &cobra.Command{
		Use:   "argo",
		Short: "Interacts with ironbark managed repositories",
	}

	gitCmd.AddCommand(newPushCmd())
	gitCmd.AddCommand(newCloneCmd())

	return gitCmd
}

func newPushCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push the ArgoCD repo to the internal Git server",
		Run:   pushExec,
	}

	return cmd
}

func newCloneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clone",
		Short: "Clone the ArgoCD App of Apps repo from the internal Git server",
		Run:   cloneExec,
	}

	return cmd
}

// pushExec pushed the local ArgoCD app of apps repo to the internal Git server.
func pushExec(cmd *cobra.Command, args []string) {
	argoCDRepoDir := constants.GetArgoCDRepoDir()
	logger.Info("Pushing ArgoCD repo ...", "localDir", argoCDRepoDir)
	err := ironbarkGit.PushRepoArgoCDAppOfApps(cmd.Context(), argoCDRepoDir)
	exitOnError(err, "Could not push repo `"+argoCDRepoDir+"`")
}

// cloneExec clones the ArgoCD app of apps repo from the internal Git server.
func cloneExec(cmd *cobra.Command, args []string) {
	argoCDRepoDir := constants.GetArgoCDRepoDir()
	logger.Info("Cloning ArgoCD repo ...", "localDir", argoCDRepoDir)
	err := ironbarkGit.CloneRepoArgoCDAppOfApps(cmd.Context(), argoCDRepoDir)
	exitOnError(err, "Could not clone repo to `"+argoCDRepoDir+"`")
	logger.Info("Successfully cloned ArgoCD repo", "localDir", argoCDRepoDir)
}
