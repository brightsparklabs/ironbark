/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

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
	zarfCluster, err := zarfcluster.New(ctx)
	exitOnError(err, "Could not retrieve zarf cluster")

	zarfState, err := zarfCluster.LoadState(ctx)
	exitOnError(err, "Could not retrieve zarf state")

	registryInfo := zarfState.RegistryInfo
	logger.Info("Retrieved registry info", "registryInfo", registryInfo)

	gitServer := zarfState.GitServer
	logger.Info("Retrieved git server", "gitServer", gitServer)

	tunnelGit, err := zarfCluster.NewTunnel(zarfstate.ZarfNamespaceName, zarfcluster.SvcResource, zarfcluster.ZarfGitServerName, "", 0, zarfcluster.ZarfGitServerPort)
	exitOnError(err, "Could not create zarf git tunnel")
	_, err = tunnelGit.Connect(ctx)
	exitOnError(err, "Could not connect to zarf git tunnel")
	defer tunnelGit.Close()
	tunnelURLs := tunnelGit.HTTPEndpoints()
	if len(tunnelURLs) == 0 {
		logger.Error("No zarf git tunnel HTTP endpoints available")
		panic(1)
	}

	giteaOptions := gitea.SetBasicAuth(gitServer.PushUsername, gitServer.PushPassword)
	giteaClient, err := gitea.NewClient(tunnelURLs[0], giteaOptions)
	repoOptions := gitea.CreateRepoOption{
		Name:        "ironbark-argo",
		Description: "Ironbark created Argo app of apps repo",
		// These do not seem to be picked up.
		Private: false,
		Readme:  "Created by Ironbark",
	}
	repo, response, err := giteaClient.CreateRepo(repoOptions)
	logger.Info("Created repo", "repo", repo)
	logger.Info("Got response", "response", response.Body)

	reposDir := filepath.Join(getDataDir(), "repos")
	dir := filepath.Join(reposDir, "ironbark-argo")
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
	err := resources.Copy("resources/argo-repo", dir)
	return err
}
