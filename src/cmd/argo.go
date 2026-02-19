/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package cmd

import (
	"fmt"
	"os"
	"time"

	"brightsparklabs.com/ironbark/internal/constants"
	ironbarkGit "brightsparklabs.com/ironbark/internal/git"
	"brightsparklabs.com/ironbark/resources"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/cobra"
)

func newArgoCmd() *cobra.Command {
	gitCmd := &cobra.Command{
		Use:   "argo",
		Short: "Interacts with ironbark managed repositories",
	}

	gitCmd.AddCommand(newArgoInitCmd())
	gitCmd.AddCommand(newPushCmd())

	return gitCmd
}

func newArgoInitCmd() *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialises the ArgoCD repo in the internal Git server",
		Run:   initArgoExec,
	}
	return initCmd
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

// initArgoExec initialises the local ArgoCD app of apps repo and pushes it to the internal Git server.
func initArgoExec(cmd *cobra.Command, args []string) {
	repoName := constants.ArgoCDRepoName
	logger.Info("Creating ArgoCD repo ...", "repo", repoName)

	ctx := cmd.Context()
	repo, err := ironbarkGit.CreateRepoArgoCDAppOfApps(ctx)
	exitOnError(err, "Could not create remote ArgoCD repo")
	logger.Info("Successfully created repo", "url", repo.HTMLURL)

	argoCDRepoDir := constants.GetArgoCDRepoDir()
	_, err = createLocalArgoCDRepo(argoCDRepoDir)
	exitOnError(err, "Could not create local ArgoCD repo")

	err = ironbarkGit.PushRepoArgoCDAppOfApps(ctx, argoCDRepoDir)
	exitOnError(err, "Could not push argo repo")
}

// createLocalArgoCDRepo creates the local ArgoCD app of apps repo in the specified directory.
func createLocalArgoCDRepo(dir string) (*git.Repository, error) {
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, fmt.Errorf("could not create repo dir: %w", err)
	}
	logger.Info("Created argo repo dir", "dir", dir)

	localRepo, err := git.PlainInit(dir, false)
	if err != nil {
		return nil, fmt.Errorf("could not init repo: %w", err)
	}
	logger.Info("Initialised argo local repo")

	copyAppOfAppResources(dir)

	w, _ := localRepo.Worktree()
	_, _ = w.Add(".")
	_, err = w.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Ironbark",
			Email: "ironbark@brightsparklabs.dev",
			When:  time.Now(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("could not create initial commit: %w", err)
	}

	return localRepo, nil
}

// copyAppOfAppResources populates the local ArgoCD app of apps repo directory using the embedded resources template.
func copyAppOfAppResources(dir string) error {
	err := resources.Copy("repos/ironbark-argocd-app-of-apps", dir)
	return err
}
