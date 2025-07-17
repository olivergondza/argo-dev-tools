package main

import (
	"os"
	"os/exec"
)

type kubeCluster struct {
	name string
	// trackerClose prevents program completion on interrupt
	trackerClose func()
}

func (cluster *kubeCluster) Close() {
	out(os.Stderr, "Closing kubeCluster")
	// cannot use NewManagedProc - run after main context is cancelled
	err := exec.Command("k3d", "cluster", "delete", cluster.name).Run()
	if err != nil {
		out(os.Stderr, "Failed to close kubeCluster: %s", err)
		return
	}
	out(os.Stderr, "Closed kubeCluster")
	cluster.trackerClose()
}

func startK3dCluster(name string) (c *kubeCluster, err error) {
	cluster := &kubeCluster{name, func() {}}
	// Delete eventual leftovers from previous runs
	cluster.Close()

	// Clean up even half provisioned resources in case startK3dCluster itself fails
	defer func() {
		if err != nil {
			cluster.Close()
		}
	}()

	// Create pseudo-task for cluster ownership, to prevent interruption before the cluster is Close()d
	// The context is actually not needed, just the task
	_, cluster.trackerClose = mainTt.useContext("k3d_cluster")

	mp := NewManagedProc(cmdCreateCluster(name, getOutboundIP())...)
	if err := mp.Run(); err != nil {
		return nil, err
	}
	//TODO: set -x KUBECONFIG /tmp/k3d--argo-cd--argo-clstr--kubeconfig.yaml
	//TODO: k3d kubeconfig get argo-clstr > $KUBECONFIG
	//
	//TODO: oc kubeCluster-info

	mp = NewManagedProc("kubectl", "create", "namespace", "argocd")
	if err := mp.Run(); err != nil {
		return nil, err
	}

	mp = NewManagedProc("kubectl", "config", "set-context", "--current", "--namespace=argocd")
	if err := mp.Run(); err != nil {
		return nil, err
	}

	return cluster, nil
}

func cmdCreateCluster(name string, ip string) []string {
	return []string{
		"k3d", "cluster", "create", name,
		"--wait",
		"--k3s-arg", "--disable=traefik@server:*",
		"--api-port", ip + ":6550",
		"-p", "443:443@loadbalancer",
	}
}
