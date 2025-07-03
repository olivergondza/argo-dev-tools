package main

import (
	"regexp"
)

type projectCd struct {
}

func (p projectCd) Name() string {
	return "cd"
}

func (p projectCd) Commands() []projectCommand {
	return []projectCommand{
		cdLocal{},
		cdE2e{},
	}
}

type cdLocal struct{}

func (c cdLocal) Run() error {
	cluster, err := startCluster()
	if err != nil {
		return err
	}
	// Interrupted
	if cluster == nil {
		return nil
	}
	defer cluster.Close()

	return nil
}

func (c cdLocal) Name() string {
	return "local"
}

type cdE2e struct{}

func (c cdE2e) Run() error {
	cluster, err := startCluster()
	if err != nil {
		return err
	}
	// Interrupted
	if cluster == nil {
		return nil
	}
	defer cluster.Close()

	makeStartE2eLocal := []string{
		"make", "start-e2e-local",
		"ARGOCD_E2E_REPOSERVER_PORT=8088",
		"COVERAGE_ENABLED=true",
		"ARGOCD_FAKE_IN_CLUSTER=true",
		"ARGOCD_E2E_K3S=true",
	}
	// This is the meat - here we wait for ^C
	if err := osExecGoreman(makeStartE2eLocal...); err != nil {
		return err
	}

	return nil
}

func (c cdE2e) Name() string {
	return "e2e"
}

func init() {
	projectRegistry["cd"] = projectCd{}
}

func startCluster() (*kubeCluster, error) {
	err := checkMarker("Makefile", regexp.MustCompile("^PACKAGE=github.com/argoproj/argo-cd/"))
	if err != nil {
		return nil, err
	}
	if wasInterrupted() {
		return nil, nil
	}

	err = checkDocker()
	if err != nil {
		return nil, err
	}
	if wasInterrupted() {
		return nil, nil
	}

	cluster, err := startK3dCluster("argo-dev-tools")
	if err != nil {
		return nil, err
	}
	return cluster, nil
}
