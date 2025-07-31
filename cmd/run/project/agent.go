package project

import (
	"github.com/argoproj/dev-tools/cmd/run/cluster"
	"github.com/argoproj/dev-tools/cmd/run/project/agent"
	"github.com/argoproj/dev-tools/cmd/run/run"
	"regexp"
	"strconv"
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

func (al agentLocal) Name() string {
	return "local"
}

func (al agentLocal) Run() error {
	grid, err := agent.NewGrid()
	if err != nil {
		return err
	}
	// Interrupted
	if grid == nil {
		return nil
	}
	defer func() {
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
		grid.PrintDetails(true)
		return err
	}
	err = manifests.InjectManagedAddresses("argocd-redis", "192.168.56.222")
	if err != nil {
		return err
	}
	err = grid.DeployAgents(manifests)
	if err != nil {
		grid.PrintDetails(true)
		return err
	}
	err = manifests.GenerateAccountSecrets()
	if err != nil {
		return err
	}
	err = al.createPkiConfig(grid)
	if err != nil {
		return err
	}
	err = grid.WaitForAllPodsRunning()
	if err != nil {
		grid.PrintDetails(true)
		return err
	}

	_, err = al.startPrincipal(grid)
	if err != nil {
		grid.PrintDetails(true)
		return err
	}
	al.startAgents(grid)

	// TODO: Is is needed, al.start* start no pods?
	err = grid.WaitForAllPodsRunning()
	if err != nil {
		grid.PrintDetails(true)
		return err
	}

	al.deployTestApps(grid)

	// TODO
	if err := run.NewManagedProc("sleep", "inf").Run(); err != nil {
		return err
	}

	return nil
}

func (al agentLocal) deployTestApps(grid *agent.Grid) {
	grid.Managed.KubectlProc("apply", "-f", "./hack/dev-env/apps/managed-guestbook.yaml")
	grid.Autonomous.KubectlProc("apply", "-f", "./hack/dev-env/apps/autonomous-guestbook.yaml")
}

func (al agentLocal) startPrincipal(grid *agent.Grid) (string, error) {
	principalRedisAddress, err := principalRedisAddress(grid)
	if err != nil {
		return "", err
	}

	proc := run.NewManagedProc(
		"go",
		"run",
		"github.com/argoproj-labs/argocd-agent/cmd/argocd-agent",
		"principal",
		"--allowed-namespaces=*",
		"--kubecontext="+grid.ControlPlane.ContextName,
		"--namespace="+grid.ControlPlane.Namespace,
		"--log-level", "trace",
		"--auth=mtls:CN=([^,]+)",
	)
	proc.AddEnv("ARGOCD_PRINCIPAL_REDIS_SERVER_ADDRESS", principalRedisAddress)

	go func() {
		err := proc.Run()
		if err != nil {
			panic(err)
		}
	}()

	return principalRedisAddress, nil
}

func (al agentLocal) startAgents(grid *agent.Grid) {
	portHealth := 8002
	portMetrics := 8182
	c2m := map[*cluster.KubeCluster]string{
		grid.Autonomous: "autonomous",
		grid.Managed:    "managed",
	}
	for kubeCluster, mode := range c2m {
		proc := run.NewManagedProc(
			"go", "run", "github.com/argoproj-labs/argocd-agent/cmd/argocd-agent",
			"agent", "--agent-mode", mode,
			"--creds=mtls:any",
			"--server-address=127.0.0.1",
			"--insecure-tls",
			"--kubecontext", kubeCluster.ContextName,
			"--namespace="+kubeCluster.Namespace,
			"--log-level", "trace",
			"--healthz-port", strconv.Itoa(portHealth),
			"--metrics-port", strconv.Itoa(portMetrics),
		)
		go func() {
			err := proc.Run()
			if err != nil {
				panic(err)
			}
		}()
	}
}

func (al agentLocal) createPkiConfig(grid *agent.Grid) error {
	ip := run.GetOutboundIP()
	agentctl := "./dist/argocd-agentctl"
	var err error

	proc := run.NewManagedProc(
		agentctl, "pki", "init",
	)
	if err = proc.Run(); err != nil {
		return err
	}

	proc = run.NewManagedProc(
		agentctl, "pki", "issue", "principal", "--upsert",
		"--principal-context", grid.ControlPlane.ContextName,
		"--principal-namespace", grid.ControlPlane.Namespace,
		"--ip", "127.0.0.1,"+ip,
	)
	if err = proc.Run(); err != nil {
		return err
	}
	proc = run.NewManagedProc(
		agentctl, "pki", "issue", "resource-proxy", "--upsert",
		"--principal-context", grid.ControlPlane.ContextName,
		"--principal-namespace", grid.ControlPlane.Namespace,
		"--ip", "127.0.0.1,"+ip,
	)
	if err = proc.Run(); err != nil {
		return err
	}
	proc = run.NewManagedProc(
		agentctl, "jwt", "create-key", "--upsert",
		"--principal-context", grid.ControlPlane.ContextName,
		"--principal-namespace", grid.ControlPlane.Namespace,
	)
	if err = proc.Run(); err != nil {
		return err
	}

	n2c := map[string]*cluster.KubeCluster{
		"agent-managed":    grid.Managed,
		"agent-autonomous": grid.Autonomous,
	}

	for agentName, cluster := range n2c {
		proc = run.NewManagedProc(
			agentctl, "agent", "create", agentName,
			"--resource-proxy-username", agentName,
			"--resource-proxy-password", agentName,
			"--resource-proxy-server", ip+":9090",
		)
		if err = proc.Run(); err != nil {
			return err
		}
		proc = run.NewManagedProc(
			agentctl, "pki", "issue", "agent", agentName,
			"--agent-context", cluster.ContextName,
			"--agent-namespace", cluster.Namespace,
			"--upsert",
		)
		if err = proc.Run(); err != nil {
			return err
		}
	}
	return nil
}

func principalRedisAddress(grid *agent.Grid) (string, error) {
	json, err := grid.ControlPlane.KubectlGetJson("svc", "argocd-redis")
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
