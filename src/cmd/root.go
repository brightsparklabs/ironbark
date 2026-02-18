/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"log/slog"
	zarfcmd "github.com/zarf-dev/zarf/src/cmd"
)

const envCliName = "IRONBARK_CLI_NAME"
const envDataDir = "IRONBARK_DATA_DIR"

var jsonHandler = slog.NewJSONHandler(os.Stderr, nil)
var logger = slog.New(jsonHandler)

func getEnvVar(name, defaultValue string) string {
	value := os.Getenv(name)
	if value == "" {
		value = defaultValue
	}
	return value
}

func getCliName() string {
	return getEnvVar(envCliName, "ironbark")
}

func getDataDir() string {
	return getEnvVar(envDataDir, "/tmp/data")
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use: "ironbark",
	Annotations: map[string]string{
		cobra.CommandDisplayNameAnnotation: getCliName(),
	},
	Short: "Kubernetes management using the brightSPARK Labs opinionated deployment pattern",
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

	rootCmd.AddCommand(newArgoCmd())
	rootCmd.AddCommand(newInitCmd())

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

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.ironbark.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	//rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
