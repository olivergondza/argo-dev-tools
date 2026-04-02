package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/argoproj/dev-tools/cmd/run/project"
	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		_, _ = fmt.Fprintf(os.Stderr, "%T / %T\n", err, errors.Unwrap(err))
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run dev-tools workflows",
	}

	cmd.AddCommand(project.NewCDCommand())
	cmd.AddCommand(project.NewRolloutsCommand())

	return cmd
}
