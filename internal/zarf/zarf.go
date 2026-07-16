/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package zarf

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	zarfcluster "github.com/zarf-dev/zarf/src/pkg/cluster"
	zarfstate "github.com/zarf-dev/zarf/src/pkg/state"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var zarfCluster *zarfcluster.Cluster
var zarfState *zarfstate.State

func GetCluster(ctx context.Context) (*zarfcluster.Cluster, error) {
	if ctx == nil {
		ctx = context.TODO()
	}

	slog.Debug("Getting zarf cluster ...")
	if zarfCluster != nil {
		return zarfCluster, nil
	}

	zarfCluster, err := zarfcluster.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve zarf cluster: %w", err)
	}

	return zarfCluster, nil
}

func GetState(ctx context.Context) (*zarfstate.State, error) {
	if ctx == nil {
		ctx = context.TODO()
	}

	slog.Debug("Getting zarf state ...")
	if zarfState != nil {
		return zarfState, nil
	}

	zarfCluster, err := GetCluster(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve zarf cluster: %w", err)
	}

	zarfState, err := zarfCluster.LoadState(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve zarf state: %w", err)
	}

	return zarfState, nil
}

func GetRegistryInfo(ctx context.Context) (*zarfstate.RegistryInfo, error) {
	slog.Debug("Getting zarf registry info...")
	zarfState, err := GetState(ctx)
	if err != nil {
		return nil, err
	}
	return &zarfState.RegistryInfo, nil
}

func GetGitServerInfo(ctx context.Context) (*zarfstate.GitServerInfo, error) {
	slog.Debug("Getting zarf git server info...")
	zarfState, err := GetState(ctx)
	if err != nil {
		return nil, err
	}
	return &zarfState.GitServer, nil
}

// IsInitialized checks if Zarf is initialized in the cluster by looking for
// the zarf namespace and the gitea deployment.
func IsInitialized(ctx context.Context) bool {
	cluster, err := GetCluster(ctx)
	if err != nil {
		slog.Debug("Zarf not initialized: cannot get cluster", "error", err)
		return false
	}

	// Check if zarf namespace exists and gitea deployment is present.
	// The gitea deployment is created by zarf init with git-server component.
	k8s := cluster.Clientset
	_, err = k8s.AppsV1().Deployments("zarf").Get(ctx, "zarf-gitea", metav1.GetOptions{})
	if err != nil {
		slog.Debug("Zarf not initialized: gitea deployment not found", "error", err)
		return false
	}

	slog.Debug("Zarf is initialized (gitea deployment found)")
	return true
}

func DeployPackages(dir string) error {
	return runPackagesCommand(dir, []string{"deploy", "--confirm"})
}

func MirrorPackages(dir string) error {
	return runPackagesCommand(dir, []string{"mirror-resources"})
}

func runPackagesCommand(dir string, actions []string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		slog.Warn("Not searching for packages as directory does not exist", "dir", dir)
		return nil
	}

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		commands := append([]string{"package"}, actions...)
		commandsWithPath := append(commands, path)

		command := exec.Command("zarf", commandsWithPath...)
		command.Stdin = os.Stdin
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr

		err = command.Run()
		if err != nil {
			return fmt.Errorf("could not run `%v %v`: %w", actions[0], path, err)
		}

		return nil
	})

	return err
}
