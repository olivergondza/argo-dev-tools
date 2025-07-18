package main

import (
	"os"
	"os/exec"
)

type kubeCluster struct {
	name      string
	namespace string
	// trackerClose prevents program completion on interrupt
	trackerClose func()
}

func NewK3dCluster(name string) (c *kubeCluster, err error) {
	cluster := &kubeCluster{name, "", func() {}}
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
	_, cluster.trackerClose = mainTt.useContext("k3d_cluster")

	mp := cluster.newCreateProc(getOutboundIP())
	if err := mp.Run(); err != nil {
		return nil, err
	}

	return cluster, nil
}

func (c *kubeCluster) Close() {
	out(os.Stderr, "Closing kubeCluster")
	// cannot use NewManagedProc - run after main context is cancelled
	err := exec.Command("k3d", "cluster", "delete", c.name).Run()
	if err != nil {
		out(os.Stderr, "Failed to close kubeCluster: %s", err)
		return
	}
	out(os.Stderr, "Closed kubeCluster")
	c.trackerClose()
}

func (c *kubeCluster) UseNs(ns string) error {
	mp := NewManagedProc("kubectl", "create", "namespace", ns)
	if err := mp.Run(); err != nil {
		return err
	}
	c.namespace = ns

	// Needed my the `make` targets
	mp = NewManagedProc("kubectl", "config", "set-context", "--current", "--namespace="+ns)
	if err := mp.Run(); err != nil {
		return err
	}

	return nil
}

func (c *kubeCluster) newCreateProc(ip string) *ManagedProc {
	return NewManagedProc(
		"k3d", "cluster", "create", c.name,
		"--wait",
		"--k3s-arg", "--disable=traefik@server:*",
		"--api-port", ip+":6550",
		"-p", "443:443@loadbalancer",
	)
}

func (c *kubeCluster) Kubectl(args ...string) *ManagedProc {
	if c.namespace == "" {
		panic("namespace not set for cluster " + c.name)
	}

	args = append([]string{"kubectl", "-n", c.namespace}, args...)
	return NewManagedProc(args...)
}
