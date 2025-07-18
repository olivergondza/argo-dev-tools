package main

import "regexp"

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

	cluster, err := NewK3dCluster("argo-dev-tools")
	if err != nil {
		return nil, err
	}
	err = cluster.UseNs("argocd")
	if err != nil {
		return nil, err
	}
	return cluster, nil
}
