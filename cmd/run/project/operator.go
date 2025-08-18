package project

import (
	"github.com/argoproj/dev-tools/cmd/run/run"
	"regexp"
)

var (
	upstreamOperator   = regexp.MustCompile("^IMAGE_TAG_BASE \\?= quay.io/argoprojlabs/argocd-operator$")
	downstreamOperator = regexp.MustCompile("^IMAGE \\?= quay.io/redhat-developer/gitops-operator$")
)

func init() {
	run.ProjectRegistry["operator"] = projectOperator{}
}

type projectOperator struct {
}

func (p projectOperator) Name() string {
	return "operator"
}

func (p projectOperator) Commands() []run.ProjectCommand {
	return []run.ProjectCommand{
		operatorRun{},
	}
}

func (p projectOperator) CheckRepo() error {
	if err := run.CheckMarker("Makefile", upstreamOperator); err == nil {
		return nil
	}

	return run.CheckMarker("Makefile", downstreamOperator)
}

type operatorRun struct{}

func (c operatorRun) Run() error {
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

	// This is the meat - here we wait for ^C
	mp := run.NewManagedProc("make", "install", "run")
	return mp.Run()
}

func (c operatorRun) Name() string {
	return "run"
}
