package project

import (
	"github.com/argoproj/dev-tools/cmd/run/cluster"
	"github.com/argoproj/dev-tools/cmd/run/project/agent"
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
	grid, err := agent.NewGrid()
	if err != nil {
		return err
	}
	// Interrupted
	if grid == nil {
		return nil
	}
	defer grid.Close()
	grid.PrintDetails()

	manifests, err := agent.NewManifests("./hack/dev-env/")
	if err != nil {
		return err
	}

	// TODO manifest modifications

	err = grid.DeployArgos(manifests)
	if err != nil {
		return err
	}

	// TODO
	if err := run.NewManagedProc("sleep", "inf").Run(); err != nil {
		return err
	}

	err = c.startPrincipal(grid)
	if err != nil {
		return err
	}

	c.deployTestApps(grid)

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

func (c agentLocal) deployTestApps(grid *agent.Grid) {
	grid.Managed.KubectlProc("apply", "-f", "./hack/dev-env/apps/managed-guestbook.yaml")
	grid.Autonomous.KubectlProc("apply", "-f", "./hack/dev-env/apps/autonomous-guestbook.yaml")
}

func (c agentLocal) startPrincipal(grid *agent.Grid) error {
	principalRedisAddress, err := c.redisAddress(grid)
	if err != nil {
		return err
	}

	proc := run.NewManagedProc(
		"go",
		"run",
		"github.com/argoproj-labs/argocd-agent/cmd/argocd-agent",
		"principal",
		"--allowed-namespaces='*'",
		"--kubecontext="+grid.Principal.ContextName,
		"--namespace="+grid.Principal.Namespace,
		"--auth=mtls:CN=([^,]+)",
	)
	proc.AddEnv("ARGOCD_PRINCIPAL_REDIS_SERVER_ADDRESS", principalRedisAddress)
	return proc.Run()
}

func (c agentLocal) redisAddress(grid *agent.Grid) (string, error) {
	json, err := grid.Principal.KubectlGetJson("svc", "argocd-redis")
	if err != nil {
		return "", err
	}
	found, err := json.SeekTo("status", "loadBalancer", "ingress", 0, "ip")
	if err != nil {
		return "", err
	}
	if !found {
		found, err = json.SeekTo("status", "loadBalancer", "ingress", 0, "hostname")
		if err != nil {
			return "", err
		}
	}

	var ipOrHost string
	err = json.Decode(&ipOrHost)
	if err != nil {
		return "", err
	}

	return ipOrHost + ":6379", nil
}
