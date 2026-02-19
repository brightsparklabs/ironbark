/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

// Package git provides functions for interacting with the Zarf Git Server.
package git

import (
	"context"
	"errors"
	"fmt"

	"brightsparklabs.com/ironbark/internal/constants"
	"brightsparklabs.com/ironbark/internal/zarf"

	"code.gitea.io/sdk/gitea"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	zarfcluster "github.com/zarf-dev/zarf/src/pkg/cluster"
	zarfstate "github.com/zarf-dev/zarf/src/pkg/state"
)

func CreateRepo(ctx context.Context, remoteRepoName string) (*gitea.Repository, error) {
	f := func(gitTunnel *zarfcluster.Tunnel, gitServerInfo *zarfstate.GitServerInfo) (any, error) {

		giteaOptions := gitea.SetBasicAuth(gitServerInfo.PushUsername, gitServerInfo.PushPassword)
		giteaClient, err := gitea.NewClient(gitTunnel.HTTPEndpoints()[0], giteaOptions)
		if err != nil {
			return nil, fmt.Errorf("could not create client connection to git server: %w", err)

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

func CreateRepoArgoCDAppOfApps(ctx context.Context) (*gitea.Repository, error) {
	return CreateRepo(ctx, constants.ArgoCDRepoName)
}

func CloneRepo(ctx context.Context, localRepoDir string, remoteRepoName string) error {
	return nil
}

func CloneRepoArgoCDAppOfApps(ctx context.Context, localRepoDir string) error {
	return CloneRepo(ctx, localRepoDir, constants.ArgoCDRepoName)
}

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

func PushRepoArgoCDAppOfApps(ctx context.Context, localRepoDir string) error {
	return PushRepo(ctx, localRepoDir, constants.ArgoCDRepoName)
}

func PullRepo(dir string) {}

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

	result, err := f(tunnelGit, gitServerInfo)
	return result, err
}
