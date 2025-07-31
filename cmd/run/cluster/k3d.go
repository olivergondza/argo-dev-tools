package cluster

import (
	"bytes"
	"github.com/argoproj/dev-tools/cmd/run/run"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/exponent-io/jsonpath"
)

type KubeCluster struct {
	Name        string
	Namespace   string
	ContextName string
	// trackerClose prevents program completion on interrupt
	trackerClose func()
}

func NewK3dCluster(name string) (c *KubeCluster, err error) {
	cluster := &KubeCluster{name, "", "k3d-" + name, func() {}}
	// Delete eventual leftovers from previous runs
	cluster.Close()

	// Clean up even half provisioned resources in case NewK3dCluster itself fails
	defer func() {
		if err != nil {
			cluster.Close()
		}
	}()

	// Create pseudo-task for cluster ownership, to prevent interruption before the cluster is Close()d
	// The context is actually not needed, just the task
	_, cluster.trackerClose = run.MainTt.UseContext("k3d_cluster")

	mp := cluster.newCreateProc(run.GetOutboundIP())
	if err := mp.Run(); err != nil {
		return nil, err
	}

	return cluster, nil
}

func (c *KubeCluster) Close() {
	defer c.trackerClose()

	run.Out(os.Stderr, "Closing KubeCluster "+c.Name)
	// cannot use NewManagedProc - run after main context is cancelled
	err := exec.Command("k3d", "cluster", "delete", c.Name).Run()
	if err != nil {
		run.Out(os.Stderr, "Failed to close KubeCluster %s: %s", c.Name, err)
		return
	}
	run.Out(os.Stderr, "Closed KubeCluster "+c.Name)
}

func (c *KubeCluster) CreateNs(ns string) error {
	mp := run.NewManagedProc("kubectl", "--context", c.ContextName, "create", "namespace", ns)
	if err := mp.Run(); err != nil {
		return err
	}
	c.Namespace = ns

	return nil
}

func (c *KubeCluster) UseNs(ns string) error {
	if c.Namespace == "" {
		panic("namespace not set for cluster " + c.Name)
	}
	c.Namespace = ns

	// Needed by the `make` targets
	mp := run.NewManagedProc("kubectl", "config", "set-context", "--current", "--namespace="+ns)
	if err := mp.Run(); err != nil {
		return err
	}

	return nil
}

func (c *KubeCluster) newCreateProc(ip string) *run.ManagedProc {
	return run.NewManagedProc(
		"k3d", "cluster", "create",
		"--wait",
		"--k3s-arg", "--disable=traefik@server:*",
		//"--api-port", ip+":6550",
		//"-p", "443:443@loadbalancer",
		c.Name,
	)
}

func (c *KubeCluster) KubectlProc(args ...string) *run.ManagedProc {
	if c.Namespace == "" {
		panic("namespace not set for cluster " + c.Name)
	}

	args = append([]string{"kubectl", "--context", c.ContextName, "-n", c.Namespace}, args...)
	return run.NewManagedProc(args...)
}

func (c *KubeCluster) KubectlGetJson(args ...string) (*jsonpath.Decoder, error) {
	args = append([]string{"get", "--output=json"}, args...)
	proc := c.KubectlProc(args...)
	stdout := proc.CaptureStdout()

	err := proc.Run()
	if err != nil {
		return nil, err
	}

	reader := bytes.NewReader(stdout.Bytes())
	return jsonpath.NewDecoder(reader), nil
}

func (c *KubeCluster) WaitForAllPodsRunning() error {
	for {
		proc := c.KubectlProc(
			"get",
			"pods",
			"--all-namespaces",
			"--field-selector=status.phase!=Running",
			"--no-headers",
		)
		stdout := proc.CaptureStdout()
		err := proc.Run()
		if err != nil {
			return err
		}

		var problems []string
		for l := range strings.Lines(stdout.String()) {
			problems = append(problems, l)
		}
		if len(problems) == 0 {
			return nil
		}

		run.Out(
			os.Stderr, "Waiting for all pods to be running in %s. Waiting on:\n- %v",
			c.Name, strings.Join(problems, "- \n"),
		)
		time.Sleep(10 * time.Second)
	}
}
