package project

import (
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/argoproj/dev-tools/cmd/run/cluster"
	"github.com/argoproj/dev-tools/cmd/run/outcolor"
	"github.com/argoproj/dev-tools/cmd/run/run"
	"github.com/spf13/cobra"
)

func NewCDCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cd",
		Short: "Argo CD workflows",
	}

	cmd.AddCommand(newCDLocalCommand())
	cmd.AddCommand(newCDE2ECommand())

	return cmd
}

func newCDLocalCommand() *cobra.Command {
	opts := cdOpts{}
	cmd := &cobra.Command{
		Use:   "local",
		Short: "Run Argo CD locally",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := opts.checkPwd(); err != nil {
				return err
			}
			return opts.local()
		},
	}

	opts.registerFlags(cmd)

	return cmd
}

func newCDE2ECommand() *cobra.Command {
	opts := cdOpts{}
	cmd := &cobra.Command{
		Use:   "e2e",
		Short: "Run Argo CD e2e",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := opts.checkPwd(); err != nil {
				return err
			}
			return opts.e2e()
		},
	}

	opts.registerFlags(cmd)

	return cmd
}

type cdOpts struct {
	progressiveSync bool
	sourceHydrator  bool
	applyResources  []string
}

func (opts *cdOpts) registerFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&opts.sourceHydrator, "source-hydrator", false, "Enable source hydrator")
	cmd.Flags().BoolVar(&opts.progressiveSync, "progressive-sync", false, "Enable progressive sync")
	cmd.Flags().StringSliceVar(&opts.applyResources, "apply-resources", nil, "Specify resources to apply, namely AppProjects, Applications and AppSets")
}

func (opts *cdOpts) checkPwd() error {
	return run.CheckMarker("Makefile", regexp.MustCompile("^PACKAGE=github.com/argoproj/argo-cd/"))
}

func (opts *cdOpts) local() error {
	cluster, err := startCluster("argocd")
	if err != nil {
		return err
	}
	if cluster == nil {
		return nil
	}
	defer cluster.Close()

	manifestInstall := "manifests/install.yaml"
	if opts.sourceHydrator {
		manifestInstall = "manifests/install-with-hydrator.yaml"
	}

	if err := cluster.KubectlProc("create", "-f", manifestInstall).Run(); err != nil {
		return fmt.Errorf("failed deploying argo-cd manifests from %q: %s", manifestInstall, err)
	}

	if err := opts.doApplyResources(cluster); err != nil {
		return err
	}

	argoCdSecret := waitForArgoCdAdminSecret(cluster)

	phonyResources := []string{
		"statefulset/argocd-application-controller",
		"deployment/argocd-dex-server",
		"deployment/argocd-repo-server",
		"deployment/argocd-server",
		"deployment/argocd-redis",
		"deployment/argocd-applicationset-controller",
		"deployment/argocd-notifications-controller",
	}
	if opts.sourceHydrator {
		phonyResources = append(phonyResources, "deployment/argocd-commit-server")
	}
	if err := scaleToZero(cluster, phonyResources...); err != nil {
		return err
	}

	if err := run.CopyToClipboard(argoCdSecret); err != nil {
		return err
	}

	go authenticateArgocdCli(argoCdSecret)

	opArgs := []string{"make", "start-local",
		"ARGOCD_GPG_ENABLED=false",
		"ARGOCD_E2E_REPOSERVER_PORT=8088",
	}
	if opts.progressiveSync {
		opArgs = append(opArgs, "ARGOCD_APPLICATIONSET_CONTROLLER_ENABLE_PROGRESSIVE_SYNCS=true")
	}
	mp := run.NewManagedProc(opArgs...)
	mp.StdoutTransformer = outcolor.ColorizeGoreman
	return mp.Run()
}

func (opts *cdOpts) e2e() error {
	cluster, err := startCluster("argocd")
	if err != nil {
		return err
	}
	if cluster == nil {
		return nil
	}
	defer cluster.Close()

	go authenticateArgocdCli("password")

	mp := run.NewManagedProc(
		"make", "start-e2e-local",
		"ARGOCD_E2E_REPOSERVER_PORT=8088",
		"COVERAGE_ENABLED=true",
		"ARGOCD_FAKE_IN_CLUSTER=true",
		"ARGOCD_E2E_K3S=true",
	)
	mp.StdoutTransformer = outcolor.ColorizeGoreman
	return mp.Run()
}

func (opts *cdOpts) doApplyResources(cluster *cluster.KubeCluster) error {
	for _, resource := range opts.applyResources {
		fileInfo, err := os.Stat(resource)
		if err != nil {
			return fmt.Errorf("cannot use resource from path %q: %s", resource, err)
		}

		var files []string
		if !fileInfo.IsDir() {
			files = []string{resource}
		} else {
			entries, err := os.ReadDir(resource)
			if err != nil {
				return fmt.Errorf("failed reading directory %q: %s", resource, err)
			}
			for _, entry := range entries {
				newPath := resource + "/" + entry.Name()
				if entry.IsDir() {
					return fmt.Errorf("no nested resource directories %q", newPath)
				}
				files = append(files, newPath)
			}
		}

		for _, file := range files {
			if err := cluster.KubectlProc("create", "-f", file).Run(); err != nil {
				return fmt.Errorf("failed deploying argo-cd manifests from %q: %s", file, err)
			}
		}
	}

	return nil
}

func waitForArgoCdAdminSecret(cluster *cluster.KubeCluster) string {
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
		if err := mp.Run(); err == nil {
			break
		}

		run.Out(os.Stderr, "Waiting for ./dist/argocd login...")
		time.Sleep(5 * time.Second)
	}

	run.Out(os.Stderr, "./dist/argocd logged in!")
}

func getInitialArgoCdAdminSecret(c *cluster.KubeCluster) (string, error) {
	proc := c.KubectlProc(
		"get", "secret", "argocd-initial-admin-secret",
		"-o", "jsonpath={.data.password}",
	)
	stdoutBuffer := proc.CaptureStdout()
	if err := proc.Run(); err != nil {
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
		if err := c.KubectlProc("scale", resource, "--replicas", "0").Run(); err != nil {
			return fmt.Errorf("failed scaling down %s in the dummy Argo CD deployment: %v", resource, err)
		}
	}
	return nil
}
