/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"brightsparklabs.com/ironbark/internal/zarf"
	"brightsparklabs.com/ironbark/resources"

	"code.gitea.io/sdk/gitea"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/spf13/cobra"
	zarfcluster "github.com/zarf-dev/zarf/src/pkg/cluster"
	zarfstate "github.com/zarf-dev/zarf/src/pkg/state"
)

func newArgoCmd() *cobra.Command {
	gitCmd := &cobra.Command{
		Use:   "argo",
		Short: "Interacts with ironbark managed repositories",
	}

	gitCmd.AddCommand(newArgoInitCmd())

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

func initArgoExec(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()
	zarfCluster, err := zarf.GetCluster(ctx)
	exitOnError(err, "Could not retrieve zarf cluster")

	gitServer, err := zarf.GetGitServerInfo(ctx)
	logger.Info("Retrieved git server", "gitServer", gitServer)

	tunnelGit, err := zarfCluster.NewTunnel(zarfstate.ZarfNamespaceName, zarfcluster.SvcResource, zarfcluster.ZarfGitServerName, "", 0, zarfcluster.ZarfGitServerPort)
	exitOnError(err, "Could not create zarf git tunnel")
	_, err = tunnelGit.Connect(ctx)
	exitOnError(err, "Could not connect to zarf git tunnel")
	defer tunnelGit.Close()
	tunnelURLs := tunnelGit.HTTPEndpoints()
	if len(tunnelURLs) == 0 {
		logger.Error("No zarf git tunnel HTTP endpoints available")
		return
	}

	giteaOptions := gitea.SetBasicAuth(gitServer.PushUsername, gitServer.PushPassword)
	giteaClient, err := gitea.NewClient(tunnelURLs[0], giteaOptions)
	exitOnError(err, "Could create client connection to git server")

	repoName := "ironbark-argocd-app-of-apps"
	repo, _, err := giteaClient.GetRepo(gitServer.PushUsername, repoName)
	if repo.Owner != nil {
		logger.Error("Ironbark has already initialised the ArgoCD repository as it exists on git server. Delete it if it needs to be re-initialised.", "repository", repoName)
		return
	}

	repoOptions := gitea.CreateRepoOption{
		Name:        repoName,
		Description: "Ironbark created ArgoCD app of apps repo",
		// These do not seem to be picked up.
		Private: false,
		Readme:  "Created by Ironbark",
	}
	repo, _, err = giteaClient.CreateRepo(repoOptions)
	logger.Info("Created repo", "url", repo.HTMLURL)

	reposDir := filepath.Join(getDataDir(), "repos")
	dir := filepath.Join(reposDir, repoName)
	localRepo, err := createLocalArgoCDRepo(dir)
	exitOnError(err, "Could not create local ArgoCD repo")

	_, err = localRepo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{tunnelURLs[0] + "/" + repo.FullName},
	})
	exitOnError(err, "Could not add remote repo")

	auth := &http.BasicAuth{
		Username: gitServer.PushUsername,
		Password: gitServer.PushPassword,
	}

	err = localRepo.Push(&git.PushOptions{
		RemoteName: "origin",
		Auth:       auth,
	})
	exitOnError(err, "Could not push argo repo")
}

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

func copyAppOfAppResources(dir string) error {
	err := resources.Copy("resources/repos/ironbark-argocd-app-of-apps", dir)
	return err
}
