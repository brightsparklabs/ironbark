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

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	zarfcluster "github.com/zarf-dev/zarf/src/pkg/cluster"
	zarfstate "github.com/zarf-dev/zarf/src/pkg/state"
)

var gitAuth *http.BasicAuth

func getAuth(ctx context.Context) (*http.BasicAuth, error) {
	if gitAuth != nil {
		return gitAuth, nil
	}

	gitServer, err := zarf.GetGitServerInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get zarf git server info: %w", err)
	}

	gitAuth = &http.BasicAuth{
		Username: gitServer.PushUsername,
		Password: gitServer.PushPassword,
	}

	return gitAuth, nil
}

func CloneRepo(ctx context.Context, localRepoDir string, remoteRepoName string) error {
	return nil
}

func PushRepo(ctx context.Context, localRepoDir string, remoteRepoName string) error {
	localRepo, err := git.PlainOpen(localRepoDir)
	if err != nil {
		return fmt.Errorf("could not open local repo `%v`: %w", localRepoDir, err)
	}

	auth, err := getAuth(ctx)
	if err != nil {
		return fmt.Errorf("could not get git credentials: %w", err)
	}

	f := func(gitTunnel *zarfcluster.Tunnel) error {
		err = localRepo.DeleteRemote("origin")
		if err != nil {
			return err
		}

		_, err = localRepo.CreateRemote(&config.RemoteConfig{
			Name: "origin",
			URLs: []string{gitTunnel.HTTPEndpoints()[0] + "/" + auth.Username + "/" + remoteRepoName},
		})

		err = localRepo.Push(&git.PushOptions{
			RemoteName: "origin",
			Auth:       auth,
		})
		return err
	}

	err = executeInGitTunnel(ctx, f)
	if err != nil {
		return fmt.Errorf("failed to execute repo push: %w", err)
	}

	return nil
}

func PushRepoArgoCDAppOfApps(ctx context.Context, localRepoDir string) error {
	return PushRepo(ctx, localRepoDir, constants.ArgoCDRepoName)
}

func PullRepo(dir string) {}

func executeInGitTunnel(ctx context.Context, f func(t *zarfcluster.Tunnel) error) error {
	zarfCluster, err := zarf.GetCluster(ctx)
	if err != nil {
		return fmt.Errorf("could get zarf cluster: %w", err)
	}

	tunnelGit, err := zarfCluster.NewTunnel(zarfstate.ZarfNamespaceName, zarfcluster.SvcResource, zarfcluster.ZarfGitServerName, "", 0, zarfcluster.ZarfGitServerPort)
	if err != nil {
		return fmt.Errorf("could not create zarf git tunnel: %w", err)
	}

	_, err = tunnelGit.Connect(ctx)
	if err != nil {
		return fmt.Errorf("could not connect to zarf git tunnel: %w", err)
	}
	defer tunnelGit.Close()

	tunnelURLs := tunnelGit.HTTPEndpoints()
	if len(tunnelURLs) == 0 {
		return errors.New("no zarf git tunnel HTTP endpoints available")
	}

	err = f(tunnelGit)
	return err
}
