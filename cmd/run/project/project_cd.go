package project

import (
	"encoding/base64"
	"fmt"
	"github.com/argoproj/dev-tools/cmd/run/cluster"
	"github.com/argoproj/dev-tools/cmd/run/outcolor"
	"github.com/argoproj/dev-tools/cmd/run/run"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

func init() {
	run.ProjectRegistry["cd"] = projectCd{}
}

type projectCd struct {
}

func (p projectCd) Name() string {
	return "cd"
}

func (p projectCd) Commands() []run.ProjectCommand {
	return []run.ProjectCommand{
		cdLocal{},
		cdE2e{},
	}
}

func (p projectCd) CheckRepo() error {
	return run.CheckMarker("Makefile", regexp.MustCompile("^PACKAGE=github.com/argoproj/argo-cd/"))
}

type cdLocal struct{}

func (c cdLocal) Run() error {
	cluster, err := startCluster("argocd")
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

	err = scaleToZero(cluster,
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

	mp := run.NewManagedProc(
		"make", "start-local",
		"ARGOCD_GPG_ENABLED=false",
		"ARGOCD_E2E_REPOSERVER_PORT=8088",
		"ARGOCD_APPLICATIONSET_CONTROLLER_ENABLE_PROGRESSIVE_SYNCS=true", // https://argo-cd.readthedocs.io/en/latest/operator-manual/applicationset/Progressive-Syncs/
	)
	mp.StdoutTransformer = outcolor.ColorizeGoreman
	// This is the meat - here we wait for ^C
	return mp.Run()
}

// waitForArgoCdAdminSecret returns the secret one it gets populated.
func (c cdLocal) waitForArgoCdAdminSecret(cluster *cluster.KubeCluster) string {
	for {
		secret, err := getInitialArgoCdAdminSecret(cluster)
		if err == nil {
			return secret
		}

		run.Out(os.Stderr, "Waiting for Argo CD initialized...")
		time.Sleep(5 * time.Second)
	}
}

func authenticateArgocdCli(secret string) {
	for {
		mp := run.NewManagedProc("./dist/argocd", "login", "--plaintext", "localhost:8080", "--username=admin", "--password="+secret)
		err := mp.Run()
		if err == nil {
			break
		}

		run.Out(os.Stderr, "Waiting for ./dist/argocd login...")
		time.Sleep(5 * time.Second)
	}

	run.Out(os.Stderr, "./dist/argocd logged in!")
}

func getInitialArgoCdAdminSecret(c *cluster.KubeCluster) (string, error) {
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

func scaleToZero(c *cluster.KubeCluster, resources ...string) error {
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
	cluster, err := startCluster("argocd")
	if err != nil {
		return err
	}
	// Interrupted
	if cluster == nil {
		return nil
	}
	defer cluster.Close()

	mp := run.NewManagedProc(
		"make", "start-e2e-local",
		"ARGOCD_E2E_REPOSERVER_PORT=8088",
		"COVERAGE_ENABLED=true",
		"ARGOCD_FAKE_IN_CLUSTER=true",
		"ARGOCD_E2E_K3S=true",
	)
	mp.StdoutTransformer = outcolor.ColorizeGoreman
	// This is the meat - here we wait for ^C
	return mp.Run()
}

func (c cdE2e) Name() string {
	return "e2e"
}
