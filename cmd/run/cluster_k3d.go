package main

import "os"

type kubeCluster struct {
	name string
}

func (cluster *kubeCluster) Close() {
	if err := osExec(cmdDeleteCluster(cluster.name)...); err != nil {
		out(os.Stderr, "Failed to interrupt kubeCluster: %s", err)
	}
}

func startK3dCluster(name string) (c *kubeCluster, err error) {
	cluster := &kubeCluster{name}
	// Clean up half provisioned resources
	defer func() {
		if err != nil {
			cluster.Close()
		}
	}()

	// Delete eventual leftovers from previous runs
	cluster.Close()

	if err := osExec(cmdCreateCluster(name, getOutboundIP())...); err != nil {
		return nil, err
	}
	//TODO: set -x KUBECONFIG /tmp/k3d--argo-cd--argo-clstr--kubeconfig.yaml
	//TODO: k3d kubeconfig get argo-clstr > $KUBECONFIG
	//
	//TODO: oc kubeCluster-info

	if err := osExec("kubectl", "create", "namespace", "argocd"); err != nil {
		return nil, err
	}
	//if err := osExec("kubectl", "config", "set-context", "--current", "--namespace=argocd"); err != nil {
	//	return err
	//}

	//TODO kubectl project argocd; is it needed?

	return cluster, nil
}

func cmdDeleteCluster(name string) []string {
	return []string{"k3d", "cluster", "delete", name}
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
