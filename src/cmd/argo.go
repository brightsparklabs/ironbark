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

// pushExec pushed the local ArgoCD app of apps repo to the internal Git server.
func pushExec(cmd *cobra.Command, args []string) {
	argoCDRepoDir := constants.GetArgoCDRepoDir()
	logger.Info("Pushing ArgoCD repo ...", "localDir", argoCDRepoDir)
	err := ironbarkGit.PushRepoArgoCDAppOfApps(cmd.Context(), argoCDRepoDir)
	exitOnError(err, "Could not push repo `"+argoCDRepoDir+"`")
}
