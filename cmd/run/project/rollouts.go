package project

import (
	"regexp"

	"github.com/argoproj/dev-tools/cmd/run/outcolor"
	"github.com/argoproj/dev-tools/cmd/run/run"
	"github.com/spf13/cobra"
)

func NewRolloutsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rollouts",
		Short: "Argo Rollouts workflows",
	}
	cmd.AddCommand(newRolloutsE2ECommand())
	return cmd
}

func newRolloutsE2ECommand() *cobra.Command {
	return &cobra.Command{
		Use:   "e2e",
		Short: "Run Argo Rollouts e2e workflow",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRolloutsE2E()
		},
	}
}

func runRolloutsE2E() error {
	err := run.CheckMarker("Makefile", regexp.MustCompile("^PACKAGE=github.com/argoproj/argo-rollouts$"))
	if err != nil {
		return err
	}

	cluster, err := startCluster("argo-rollouts")
	if err != nil {
		return err
	}
	if cluster == nil {
		return nil
	}
	defer cluster.Close()

	if err = cluster.KubectlProc("apply", "-k", "manifests/crds").Run(); err != nil {
		return err
	}
	if err = cluster.KubectlProc("apply", "-f", "test/e2e/crds").Run(); err != nil {
		return err
	}

	mp := run.NewManagedProc("make", "start-e2e")
	mp.StderrTransformer = outcolor.ColorizeGoLog
	mp.StdoutTransformer = outcolor.ColorizeGoLog
	return mp.Run()
}
