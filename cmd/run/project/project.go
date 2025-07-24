package project

import (
	"github.com/argoproj/dev-tools/cmd/run/cluster"
	"github.com/argoproj/dev-tools/cmd/run/run"
)

func startCluster(ns string) (*cluster.KubeCluster, error) {
	err := run.CheckDocker()
	if err != nil {
		return nil, err
	}
	if run.WasInterrupted() {
		return nil, nil
	}

	cluster, err := cluster.NewK3dCluster("argo-dev-tools")
	if err != nil {
		return nil, err
	}
	err = cluster.UseNs(ns)
	if err != nil {
		return nil, err
	}
	return cluster, nil
}
