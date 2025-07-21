package project

import (
	"github.com/argoproj/dev-tools/cmd/run"
	"github.com/argoproj/dev-tools/cmd/run/outcolor"
	"regexp"
)

func init() {
	run.ProjectRegistry["rollouts"] = projectRollouts{}
}

type projectRollouts struct {
}

func (p projectRollouts) Name() string {
	return "rollouts"
}

func (p projectRollouts) Commands() []run.ProjectCommand {
	return []run.ProjectCommand{
		rolloutsLocal{},
		rolloutsE2e{},
	}
}

func (p projectRollouts) CheckRepo() error {
	return run.CheckMarker("Makefile", regexp.MustCompile("^PACKAGE=github.com/argoproj/argo-rollouts$"))
}

type rolloutsLocal struct{}

func (c rolloutsLocal) Run() error {
	cluster, err := startCluster("argo-rollouts")
	if err != nil {
		return err
	}
	// Interrupted
	if cluster == nil {
		return nil
	}
	defer cluster.Close()

	// TODO

	return nil
}

func (c rolloutsLocal) Name() string {
	return "local"
}

type rolloutsE2e struct{}

func (c rolloutsE2e) Run() error {
	// https://argo-rollouts.readthedocs.io/en/stable/CONTRIBUTING/#running-e2e-tests

	cluster, err := startCluster("argo-rollouts")
	if err != nil {
		return err
	}
	// Interrupted
	if cluster == nil {
		return nil
	}
	defer cluster.Close()

	if err = cluster.Kubectl("apply", "-k", "manifests/crds").Run(); err != nil {
		return err
	}

	if err = cluster.Kubectl("apply", "-f", "test/e2e/crds").Run(); err != nil {
		return err
	}

	// This is the meat - here we wait for ^C
	mp := run.NewManagedProc("make", "start-e2e")
	mp.StderrTransformer = outcolor.ColorizeGoLog
	mp.StdoutTransformer = outcolor.ColorizeGoLog
	return mp.Run()
}

func (c rolloutsE2e) Name() string {
	return "e2e"
}
