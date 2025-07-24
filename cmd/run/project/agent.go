package project

import (
	"github.com/argoproj/dev-tools/cmd/run/cluster"
	"github.com/argoproj/dev-tools/cmd/run/run"
	"os"
	"regexp"
	"time"
)

func init() {
	run.ProjectRegistry["agent"] = projectAgent{}
}

type projectAgent struct {
}

func (p projectAgent) Name() string {
	return "agent"
}

func (p projectAgent) Commands() []run.ProjectCommand {
	return []run.ProjectCommand{
		agentLocal{},
	}
}

func (p projectAgent) CheckRepo() error {
	return run.CheckMarker("Makefile", regexp.MustCompile("^BIN_NAME_ARGOCD_AGENT=argocd-agent$"))
}

type agentLocal struct{}

func (c agentLocal) Run() error {
	grid, err := startClusters()
	if err != nil {
		return err
	}
	// Interrupted
	if grid == nil {
		return nil
	}
	defer grid.Close()

	return nil
}

// waitForArgoCdAdminSecret returns the secret one it gets populated.
func (c agentLocal) waitForArgoCdAdminSecret(cluster *cluster.KubeCluster) string {
	for {
		secret, err := getInitialArgoCdAdminSecret(cluster)
		if err == nil {
			return secret
		}

		run.Out(os.Stderr, "Waiting for Argo CD initialized...")
		time.Sleep(5 * time.Second)
	}
}

func (c agentLocal) Name() string {
	return "local"
}

type agentClusterGrid struct {
	principal  *cluster.KubeCluster
	managed    *cluster.KubeCluster
	autonomous *cluster.KubeCluster
}

func (c agentClusterGrid) Close() {
	if c.managed != nil {
		c.managed.Close()
	}
	if c.autonomous != nil {
		c.autonomous.Close()
	}
	if c.principal != nil {
		c.principal.Close()
	}
}

// startCluster start all the clusters needed for the grid
// It returns either all the clusters up, so users is responsible to Close() after, or no cluster up at all
func startClusters() (*agentClusterGrid, error) {
	var err error
	acg := &agentClusterGrid{}

	err = run.CheckDocker()
	if err != nil {
		return nil, err
	}

	acg.principal, err = cluster.NewK3dCluster("argocd-agent-control-plane")
	if err != nil || acg.principal == nil {
		return nil, err
	}

	acg.managed, err = cluster.NewK3dCluster("argocd-agent-managed")
	if err != nil || acg.managed == nil {
		acg.principal.Close()
		return nil, err
	}

	acg.autonomous, err = cluster.NewK3dCluster("argocd-agent-principal")
	if err != nil || acg.autonomous == nil {
		acg.principal.Close()
		acg.managed.Close()
		return nil, err
	}

	for _, kubeCluster := range []*cluster.KubeCluster{acg.principal, acg.autonomous, acg.managed} {
		err := kubeCluster.UseNs("argocd-agent")
		if err != nil {
			acg.Close()
			return nil, err
		}
	}

	return acg, nil
}
