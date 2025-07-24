package cluster

import (
	"github.com/argoproj/dev-tools/cmd/run/run"
	"os"
	"os/exec"
)

type KubeCluster struct {
	name      string
	namespace string
	// trackerClose prevents program completion on interrupt
	trackerClose func()
}

func NewK3dCluster(name string) (c *KubeCluster, err error) {
	cluster := &KubeCluster{name, "", func() {}}
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
	run.Out(os.Stderr, "Closing KubeCluster")
	// cannot use NewManagedProc - run after main context is cancelled
	err := exec.Command("k3d", "cluster", "delete", c.name).Run()
	if err != nil {
		run.Out(os.Stderr, "Failed to close KubeCluster: %s", err)
		return
	}
	run.Out(os.Stderr, "Closed KubeCluster")
	c.trackerClose()
}

func (c *KubeCluster) UseNs(ns string) error {
	mp := run.NewManagedProc("kubectl", "create", "namespace", ns)
	if err := mp.Run(); err != nil {
		return err
	}
	c.namespace = ns

	// Needed my the `make` targets
	mp = run.NewManagedProc("kubectl", "config", "set-context", "--current", "--namespace="+ns)
	if err := mp.Run(); err != nil {
		return err
	}

	return nil
}

func (c *KubeCluster) newCreateProc(ip string) *run.ManagedProc {
	return run.NewManagedProc(
		"k3d", "cluster", "create", c.name,
		"--wait",
		"--k3s-arg", "--disable=traefik@server:*",
		"--api-port", ip+":6550",
		"-p", "443:443@loadbalancer",
	)
}

func (c *KubeCluster) Kubectl(args ...string) *run.ManagedProc {
	if c.namespace == "" {
		panic("namespace not set for cluster " + c.name)
	}

	args = append([]string{"kubectl", "-n", c.namespace}, args...)
	return run.NewManagedProc(args...)
}
