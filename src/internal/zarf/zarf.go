/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package zarf

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	zarfcluster "github.com/zarf-dev/zarf/src/pkg/cluster"
	zarfstate "github.com/zarf-dev/zarf/src/pkg/state"
)

var jsonHandler = slog.NewJSONHandler(os.Stderr, nil)
var logger = slog.New(jsonHandler)

var zarfCluster *zarfcluster.Cluster
var zarfState *zarfstate.State

func GetCluster() (*zarfcluster.Cluster, error) {
	logger.Debug("Getting zarf cluster ...")
	if zarfCluster != nil {
		return zarfCluster, nil
	}

	zarfCluster, err := zarfcluster.New(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("could not retrieve zarf cluster: %w", err)
	}

	return zarfCluster, nil
}

func GetState() (*zarfstate.State, error) {
	logger.Debug("Getting zarf state ...")
	if zarfState != nil {
		return zarfState, nil
	}

	zarfCluster, err := GetCluster()
	if err != nil {
		return nil, fmt.Errorf("could not retrieve zarf cluster: %w", err)
	}

	zarfState, err := zarfCluster.LoadState(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("could not retrieve zarf state: %w", err)
	}

	return zarfState, nil
}

func GetRegistryInfo() (*zarfstate.RegistryInfo, error) {
	logger.Debug("Getting zarf registry info...")
	zarfState, err := GetState()
	if err != nil {
		return nil, err
	}
	return &zarfState.RegistryInfo, nil
}

func GetGitServerInfo() (*zarfstate.GitServerInfo, error) {
	logger.Debug("Getting zarf git server info...")
	zarfState, err := GetState()
	if err != nil {
		return nil, err
	}
	return &zarfState.GitServer, nil
}
