/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package cmd

import (
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"code.gitea.io/sdk/gitea"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/spf13/cobra"
	zarfcmd "github.com/zarf-dev/zarf/src/cmd"
	zarfcluster "github.com/zarf-dev/zarf/src/pkg/cluster"
	zarfstate "github.com/zarf-dev/zarf/src/pkg/state"
)

const envCliName = "IRONBARK_CLI_NAME"

var jsonHandler = slog.NewJSONHandler(os.Stderr, nil)
var logger = slog.New(jsonHandler)

func getCliName() string {
	cliName := os.Getenv(envCliName)
	if cliName == "" {
		cliName = "ironbark"
	}
	return cliName
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use: "ironbark",
	Annotations: map[string]string{
		cobra.CommandDisplayNameAnnotation: getCliName(),
	},
	Short: "A brief description of your application",
	Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	zarfCmd := zarfcmd.NewZarfCommand()

	var nestedZarfCmd = &cobra.Command{
		Use: "list-packages",
		Run: func(cmd *cobra.Command, args []string) {
			zarfCmd.SetArgs([]string{"package", "list"})
			zarfCmd.Execute()
		},
	}
	rootCmd.AddCommand(nestedZarfCmd)

	var initArgoRepoCmd = &cobra.Command{
		Use: "initArgoRepo",
		Run: initArgoRepoExec,
	}
	rootCmd.AddCommand(initArgoRepoCmd)

	err := rootCmd.Execute()
	if err != nil {
		panic(err)
	}
}

func exitOnError(err error, errorMessage string) {
	if err != nil {
		logger.Error(errorMessage, "err", err)
		panic(err)
	}
}

func initArgoRepoExec(cmd *cobra.Command, args []string) {
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

	dir, err := os.MkdirTemp("", "ironbark-argo-repo-*")
	exitOnError(err, "Could not retrieve zarf state")
	// TODO:Uncomment when testing done.
	//defer os.RemoveAll(dir) // Clean up the directory after use
	logger.Info("Created temp dir", "dir", dir)

	localRepo, err := git.PlainInit(dir, false)
	exitOnError(err, "Error initialising repo")
	logger.Info("Initialised argo local repo")

	file := filepath.Join(dir, "README.md")
	os.WriteFile(file, []byte("Created by Ironbark.\n"), 0644)
	logger.Info("README created successfully")

	w, _ := localRepo.Worktree()
	_, _ = w.Add("README.md")
	_, err = w.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Ironbark",
			Email: "ironbark@brightsparklabs.dev",
			When:  time.Now(),
		},
	})
	exitOnError(err, "Could not create initial commit")

	_, err = localRepo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{tunnelURLs[0] + "/" + repo.FullName},
	})
	exitOnError(err, "Could not create initial commit")

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

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.ironbark.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
