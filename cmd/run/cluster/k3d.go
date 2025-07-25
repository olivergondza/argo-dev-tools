package cluster

import (
	"bytes"
	"github.com/argoproj/dev-tools/cmd/run/run"
	"os"
	"os/exec"

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

	run.Out(os.Stderr, "Closing KubeCluster")
	// cannot use NewManagedProc - run after main context is cancelled
	err := exec.Command("k3d", "cluster", "delete", c.Name).Run()
	if err != nil {
		run.Out(os.Stderr, "Failed to close KubeCluster: %s", err)
		return
	}
	run.Out(os.Stderr, "Closed KubeCluster")
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
		"k3d", "cluster", "create", c.Name,
		"--wait",
		"--k3s-arg", "--disable=traefik@server:*",
		//"--api-port", ip+":6550",
		//"-p", "443:443@loadbalancer",
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
	args = append(args, "get", "--output=json")
	proc := c.KubectlProc(args...)
	stdout := proc.CaptureStdout()

	err := proc.Run()
	if err != nil {
		return nil, err
	}

	reader := bytes.NewReader(stdout.Bytes())
	return jsonpath.NewDecoder(reader), nil
}
