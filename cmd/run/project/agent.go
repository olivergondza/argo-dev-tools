package project

import (
	"github.com/argoproj/dev-tools/cmd/run/project/agent"
	"github.com/argoproj/dev-tools/cmd/run/run"
	"regexp"
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

func (c agentLocal) Name() string {
	return "local"
}

func (c agentLocal) Run() error {
	grid, err := agent.NewGrid()
	if err != nil {
		return err
	}
	// Interrupted
	if grid == nil {
		return nil
	}
	defer func() {
		grid.PrintDetails(true)
		grid.Close()
	}()
	grid.PrintDetails(false)

	manifests, err := agent.NewManifests("./hack/dev-env/")
	if err != nil {
		return err
	}
	defer manifests.Close()

	err = manifests.InjectValues(&agent.ManifestData{
		LbNetPrefix:     "192.168.56.",
		PwdControlPlane: run.RandomPwdBase64(),
		PwdManaged:      run.RandomPwdBase64(),
		PwdAutonomous:   run.RandomPwdBase64(),
	})
	if err != nil {
		return err
	}

	err = grid.DeployControlPlane(manifests)
	if err != nil {
		return err
	}
	err = manifests.InjectManagedAddresses("argocd-redis", "192.168.56.222")
	if err != nil {
		return err
	}
	err = grid.DeployAgents(manifests)
	if err != nil {
		return err
	}
	err = manifests.GenerateAccountSecrets()
	if err != nil {
		return err
	}
	err = grid.WaitForAllPodsRunning()
	if err != nil {
		return err
	}

	_, err = c.startPrincipal(grid)
	if err != nil {
		return err
	}
	err = grid.WaitForAllPodsRunning()
	if err != nil {
		return err
	}

	c.deployTestApps(grid)

	// TODO
	if err := run.NewManagedProc("sleep", "inf").Run(); err != nil {
		return err
	}

	return nil
}

func (c agentLocal) deployTestApps(grid *agent.Grid) {
	grid.Managed.KubectlProc("apply", "-f", "./hack/dev-env/apps/managed-guestbook.yaml")
	grid.Autonomous.KubectlProc("apply", "-f", "./hack/dev-env/apps/autonomous-guestbook.yaml")
}

func (c agentLocal) startPrincipal(grid *agent.Grid) (string, error) {
	principalRedisAddress, err := principalRedisAddress(grid)
	if err != nil {
		return "", err
	}

	proc := run.NewManagedProc(
		"go",
		"run",
		"github.com/argoproj-labs/argocd-agent/cmd/argocd-agent",
		"principal",
		"--allowed-namespaces='*'",
		"--kubecontext="+grid.Principal.ContextName,
		"--namespace="+grid.Principal.Namespace,
		// TODO: Not sure if really not needed.
		// Avoids: [FATAL]: Error reading TLS config for resource proxy: error getting proxy certificate: could not read TLS secret argocd-agent-control-plane/argocd-agent-resource-proxy-tls: secrets "argocd-agent-resource-proxy-tls" not found
		"--enable-resource-proxy=false",
		"--auth=mtls:CN=([^,]+)",
	)
	proc.AddEnv("ARGOCD_PRINCIPAL_REDIS_SERVER_ADDRESS", principalRedisAddress)
	return principalRedisAddress, proc.Run()
}

func principalRedisAddress(grid *agent.Grid) (string, error) {
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
