package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func init() {
	projectRegistry["cd"] = projectCd{}
}

type projectCd struct {
}

func (p projectCd) Name() string {
	return "cd"
}

func (p projectCd) Commands() []projectCommand {
	return []projectCommand{
		cdLocal{},
		cdE2e{},
	}
}

type cdLocal struct{}

func (c cdLocal) Run() error {
	cluster, err := startCluster()
	if err != nil {
		return err
	}
	// Interrupted
	if cluster == nil {
		return nil
	}
	defer cluster.Close()

	// oc -n argocd apply -f manifests/install.yaml
	if err := cluster.Kubectl("apply", "-f", "manifests/install.yaml").Run(); err != nil {
		return fmt.Errorf("failed deploying argo-cd manifests: %s", err)
	}

	argoCdSecret := c.waitForArgoCdAdminSecret(cluster)

	err = cluster.scaleToZero(
		"statefulset/argocd-application-controller",
		"deployment/argocd-dex-server",
		"deployment/argocd-repo-server",
		"deployment/argocd-server",
		"deployment/argocd-redis",
		"deployment/argocd-applicationset-controller",
		"deployment/argocd-notifications-controller",
	)
	if err != nil {
		return err
	}

	cmd := exec.Command("xclip")
	cmd.Stdin = strings.NewReader(argoCdSecret)
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("xclip failed: %s", err)
	}

	// The login will only work after the `make start-local` progressed enough - run in background
	// It will terminate itself on success, or die trying.
	go authenticateArgocdCli(argoCdSecret)

	makeStartLocal := []string{
		"make", "start-local",
		"ARGOCD_GPG_ENABLED=false",
		"ARGOCD_E2E_REPOSERVER_PORT=8088",
		"ARGOCD_APPLICATIONSET_CONTROLLER_ENABLE_PROGRESSIVE_SYNCS=true", // https://argo-cd.readthedocs.io/en/latest/operator-manual/applicationset/Progressive-Syncs/
	}
	// This is the meat - here we wait for ^C
	if err := osExecGoreman(makeStartLocal...); err != nil {
		return err
	}

	return nil
}

func (c cdLocal) waitForArgoCdAdminSecret(cluster *kubeCluster) string {
	for {
		secret, err := cluster.getInitialArgoCdAdminSecret()
		if err == nil {
			return secret
		}

		out(os.Stderr, "Waiting for Argo CD initialized...")
		time.Sleep(5 * time.Second)
	}
}

func authenticateArgocdCli(secret string) {
	for {
		mp := NewManagedProc("./dist/argocd", "login", "--plaintext", "localhost:8080", "--username=admin", "--password="+secret)
		err := mp.Run()
		if err == nil {
			break
		}

		out(os.Stderr, "Waiting for ./dist/argocd login...")
		time.Sleep(5 * time.Second)
	}

	out(os.Stderr, "./dist/argocd logged in!")
}

func (c *kubeCluster) getInitialArgoCdAdminSecret() (string, error) {
	proc := c.Kubectl(
		"get", "secret", "argocd-initial-admin-secret",
		"-o", "jsonpath={.data.password}",
	)
	stdoutBuffer := proc.CaptureStdout()
	err := proc.Run()
	if err != nil {
		return "", err
	}

	decoded, err := base64.StdEncoding.DecodeString(stdoutBuffer.String())
	if err != nil {
		return "", err
	}

	return string(decoded), nil
}

func (c *kubeCluster) scaleToZero(resources ...string) error {
	for _, resource := range resources {
		if err := c.Kubectl("scale", resource, "--replicas", "0").Run(); err != nil {
			return fmt.Errorf("failed scaling down %s in the dummy Argo CD deployment: %v", resource, err)
		}
	}
	return nil
}

func (c cdLocal) Name() string {
	return "local"
}

type cdE2e struct{}

func (c cdE2e) Run() error {
	cluster, err := startCluster()
	if err != nil {
		return err
	}
	// Interrupted
	if cluster == nil {
		return nil
	}
	defer cluster.Close()

	makeStartE2eLocal := []string{
		"make", "start-e2e-local",
		"ARGOCD_E2E_REPOSERVER_PORT=8088",
		"COVERAGE_ENABLED=true",
		"ARGOCD_FAKE_IN_CLUSTER=true",
		"ARGOCD_E2E_K3S=true",
	}
	// This is the meat - here we wait for ^C
	if err := osExecGoreman(makeStartE2eLocal...); err != nil {
		return err
	}

	return nil
}

func (c cdE2e) Name() string {
	return "e2e"
}
