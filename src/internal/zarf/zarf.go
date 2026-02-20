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
)

var jsonHandler = slog.NewJSONHandler(os.Stderr, nil)
var logger = slog.New(jsonHandler)

var zarfCluster *zarfcluster.Cluster
var zarfState *zarfstate.State

func GetCluster(ctx context.Context) (*zarfcluster.Cluster, error) {
	if ctx == nil {
		ctx = context.TODO()
	}

	logger.Debug("Getting zarf cluster ...")
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

	logger.Debug("Getting zarf state ...")
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
	logger.Debug("Getting zarf registry info...")
	zarfState, err := GetState(ctx)
	if err != nil {
		return nil, err
	}
	return &zarfState.RegistryInfo, nil
}

func GetGitServerInfo(ctx context.Context) (*zarfstate.GitServerInfo, error) {
	logger.Debug("Getting zarf git server info...")
	zarfState, err := GetState(ctx)
	if err != nil {
		return nil, err
	}
	return &zarfState.GitServer, nil
}

func DeployPackages(dir string) error {
	return runPackagesCommand(dir, []string{"deploy", "--confirm"})
}

func MirrorPackages(dir string) error {
	return runPackagesCommand(dir, []string{"mirror-resources"})
}

func runPackagesCommand(dir string, actions []string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		logger.Warn("Not searching for packages as directory does not exist", "dir", dir)
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
