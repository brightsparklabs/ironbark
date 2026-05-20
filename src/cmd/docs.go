// Package cmd hosts the command definitions for the Ironbark CLI.
//
// This file implements the `ironbark docs` command, which prints the
// product README embedded into the binary at build time.
package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"brightsparklabs.com/ironbark/internal/docs"
)

// newDocsCmd creates the `ironbark docs` command which streams the
// embedded README to standard output.
func newDocsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "docs",
		Short: "Print the embedded Ironbark README",
		Long: `Print the embedded Ironbark README to standard output.

Pipe the output through a pager of your choice if you want interactive paging,
e.g.

  ironbark docs | bat -l asciidoc
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return execDocs(os.Stdout)
		},
	}
}

// execDocs writes the embedded README to the supplied writer.
func execDocs(out io.Writer) error {
	if _, err := io.WriteString(out, docs.ReadMe()); err != nil {
		return fmt.Errorf("could not write embedded README: %w", err)
	}
	return nil
}
