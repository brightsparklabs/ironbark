/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

// Package git provides functions for interacting with the Zarf Git Server.
package git

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"brightsparklabs.com/ironbark/internal/constants"
	"brightsparklabs.com/ironbark/internal/zarf"

	"code.gitea.io/sdk/gitea"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	zarfcluster "github.com/zarf-dev/zarf/src/pkg/cluster"
	zarfstate "github.com/zarf-dev/zarf/src/pkg/state"
)

// getGiteaClient creates a Gitea client authenticated with the provided credentials.
func getGiteaClient(gitTunnel *zarfcluster.Tunnel, gitServerInfo *zarfstate.GitServerInfo) (*gitea.Client, error) {
	giteaOptions := gitea.SetBasicAuth(gitServerInfo.PushUsername, gitServerInfo.PushPassword)
	giteaClient, err := gitea.NewClient(gitTunnel.HTTPEndpoints()[0], giteaOptions)
	if err != nil {
		return nil, fmt.Errorf("could not create client connection to git server: %w", err)
	}
	return giteaClient, nil
}

// CreateRepo creates a new repository on the Zarf Git server.
func CreateRepo(ctx context.Context, remoteRepoName string) (*gitea.Repository, error) {
	f := func(gitTunnel *zarfcluster.Tunnel, gitServerInfo *zarfstate.GitServerInfo) (any, error) {
		giteaClient, err := getGiteaClient(gitTunnel, gitServerInfo)
		if err != nil {
			return nil, err
		}

		repo, _, err := giteaClient.GetRepo(gitServerInfo.PushUsername, remoteRepoName)
		if repo.Owner != nil {
			return nil, errors.New("Repository `" + remoteRepoName + "` already exists on git server. Delete it if it needs to be re-created.")
		}

		repoOptions := gitea.CreateRepoOption{
			Name: remoteRepoName,
		}
		repo, _, err = giteaClient.CreateRepo(repoOptions)

		return repo, err
	}

	result, err := executeInGitTunnel(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("could not create repo: %w", err)
	}

	repo, ok := result.(*gitea.Repository)
	if !ok {
		return nil, fmt.Errorf("could not convert response to repo")
	}

	return repo, nil
}

// CreateRepoArgoCDAppOfApps creates the ArgoCD App of Apps repository on the Zarf Git server.
func CreateRepoArgoCDAppOfApps(ctx context.Context) (*gitea.Repository, error) {
	return CreateRepo(ctx, constants.ArgoCDRepoName)
}

// CloneRepo clones a repository from the Zarf Git server to the specified local directory.
func CloneRepo(ctx context.Context, localRepoDir string, remoteRepoName string) error {
	f := func(gitTunnel *zarfcluster.Tunnel, gitServerInfo *zarfstate.GitServerInfo) (any, error) {
		giteaClient, err := getGiteaClient(gitTunnel, gitServerInfo)
		if err != nil {
			return nil, err
		}

		_, _, err = giteaClient.GetRepo(gitServerInfo.PushUsername, remoteRepoName)
		if err != nil {
			return nil, fmt.Errorf("could not get repo `%s`: %w", remoteRepoName, err)
		}

		auth := &http.BasicAuth{
			Username: gitServerInfo.PushUsername,
			Password: gitServerInfo.PushPassword,
		}

		cloneUrl := fmt.Sprintf("%v/%v/%v.git", gitTunnel.HTTPEndpoints()[0], gitServerInfo.PushUsername, remoteRepoName)
		slog.Debug("Cloning through tunnel", "url", cloneUrl)

		_, err = git.PlainClone(localRepoDir, false, &git.CloneOptions{
			URL:  cloneUrl,
			Auth: auth,
		})

		return nil, err
	}

	_, err := executeInGitTunnel(ctx, f)
	if err != nil {
		return fmt.Errorf("failed to clone repo: %w", err)
	}

	return nil
}

// CloneRepoArgoCDAppOfApps clones the ArgoCD App of Apps repository from the Zarf Git server.
func CloneRepoArgoCDAppOfApps(ctx context.Context, localRepoDir string) error {
	return CloneRepo(ctx, localRepoDir, constants.ArgoCDRepoName)
}

// PushRepo pushes a local repository to the Zarf Git server.
func PushRepo(ctx context.Context, localRepoDir string, remoteRepoName string) error {
	localRepo, err := git.PlainOpen(localRepoDir)
	if err != nil {
		return fmt.Errorf("could not open local repo `%v`: %w", localRepoDir, err)
	}

	f := func(gitTunnel *zarfcluster.Tunnel, gitServerInfo *zarfstate.GitServerInfo) (any, error) {
		extantOrigin, err := localRepo.Remote("origin")
		if extantOrigin != nil {
			err = localRepo.DeleteRemote("origin")
			if err != nil {
				return nil, fmt.Errorf("could not delete remote `origin`: %w", err)
			}
		}

		_, err = localRepo.CreateRemote(&config.RemoteConfig{
			Name: "origin",
			URLs: []string{gitTunnel.HTTPEndpoints()[0] + "/" + gitServerInfo.PushUsername + "/" + remoteRepoName},
		})

		auth := &http.BasicAuth{
			Username: gitServerInfo.PushUsername,
			Password: gitServerInfo.PushPassword,
		}

		err = localRepo.Push(&git.PushOptions{
			RemoteName: "origin",
			Auth:       auth,
		})
		return nil, err
	}

	_, err = executeInGitTunnel(ctx, f)
	if err != nil {
		return fmt.Errorf("failed to execute repo push: %w", err)
	}

	return nil
}

// PushRepoArgoCDAppOfApps pushes the local ArgoCD App of Apps repository to the Zarf Git server.
func PushRepoArgoCDAppOfApps(ctx context.Context, localRepoDir string) error {
	return PushRepo(ctx, localRepoDir, constants.ArgoCDRepoName)
}

// PullRepo pulls changes from the remote repository (not yet implemented).
func PullRepo(dir string) {}

// executeInGitTunnel creates a tunnel to the Zarf Git server and executes the provided function with access to the tunnel and Git server info.
func executeInGitTunnel(ctx context.Context, f func(t *zarfcluster.Tunnel, gitServerInfo *zarfstate.GitServerInfo) (any, error)) (any, error) {
	zarfCluster, err := zarf.GetCluster(ctx)
	if err != nil {
		return nil, fmt.Errorf("could get zarf cluster: %w", err)
	}

	gitServerInfo, err := zarf.GetGitServerInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get zarf git server info: %w", err)
	}

	tunnelGit, err := zarfCluster.NewTunnel(zarfstate.ZarfNamespaceName, zarfcluster.SvcResource, zarfcluster.ZarfGitServerName, "", 0, zarfcluster.ZarfGitServerPort)
	if err != nil {
		return nil, fmt.Errorf("could not create zarf git tunnel: %w", err)
	}

	_, err = tunnelGit.Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not connect to zarf git tunnel: %w", err)
	}
	defer tunnelGit.Close()

	tunnelURLs := tunnelGit.HTTPEndpoints()
	if len(tunnelURLs) == 0 {
		return nil, errors.New("no zarf git tunnel HTTP endpoints available")
	}

	slog.Debug("Created git tunnel", "tunnelURLs", tunnelURLs)

	result, err := f(tunnelGit, gitServerInfo)
	return result, err
}
