package agent

import (
	"errors"
	"fmt"
	"github.com/argoproj/dev-tools/cmd/run/cluster"
	"github.com/argoproj/dev-tools/cmd/run/run"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	errFailedWaitingForRepoServerHostname = errors.New("failed waiting for repo-server hostname")
)

type Grid struct {
	Principal  *cluster.KubeCluster
	Managed    *cluster.KubeCluster
	Autonomous *cluster.KubeCluster
}

func (g *Grid) Close() {
	if g.Managed != nil {
		g.Managed.Close()
	}
	if g.Autonomous != nil {
		g.Autonomous.Close()
	}
	if g.Principal != nil {
		g.Principal.Close()
	}
}

// NewGrid start all the clusters needed for the grid
// It returns either all the clusters up, so users is responsible to Close() after, or no cluster up at all
func NewGrid() (*Grid, error) {
	err := run.CheckDocker()
	if err != nil {
		return nil, err
	}

	// Strat cluster in parallel
	var wg sync.WaitGroup
	acg := &Grid{}
	clusters := map[string]func(kubeCluster *cluster.KubeCluster){
		"argocd-agent-control-plane": func(c *cluster.KubeCluster) { acg.Principal = c },
		"argocd-agent-managed":       func(c *cluster.KubeCluster) { acg.Managed = c },
		"argocd-agent-autonomous":    func(c *cluster.KubeCluster) { acg.Autonomous = c },
	}
	wg.Add(len(clusters))
	run.Out(os.Stderr, "starting clusters")
	errorChan := make(chan error, 10)
	for clusterName, setter := range clusters {
		go func() {
			defer func() {
				run.Out(os.Stderr, "DONE with "+clusterName)
				wg.Done()
			}()

			clstr, err := cluster.NewK3dCluster(clusterName)
			if err != nil {
				errorChan <- err
				return
			}
			if clstr == nil {
				errorChan <- fmt.Errorf("interrupted while creating clusters")
				return
			}
			setter(clstr)
			// Putting cluster name as NS name, so error messages not mentioning cluster identity can be attributed to a given environment.
			err = clstr.CreateNs(clusterName)
			if err != nil {
				errorChan <- err
				return
			}

			//proc := clstr.KubectlProc("apply", "-f", "https://raw.githubusercontent.com/metallb/metallb/refs/tags/v0.15.2/config/manifests/metallb-native.yaml")
		}()
	}

	wg.Wait()

	// channel must be closed before it can be iterated
	// using loop as the channel is empty if all goes well
	close(errorChan)
	for err = range errorChan {
		acg.Close()
		return nil, err
	}

	return acg, nil
}

func (g *Grid) PrintDetails(verbose bool) {
	run.Out(os.Stderr, "Agent grid details:")

	header := func(c *cluster.KubeCluster) {
		run.Out(os.Stderr, "===")
		run.Out(os.Stderr, c.Name+":")
		run.Out(os.Stderr, "  context: "+c.ContextName)
		if verbose {
			err := c.KubectlProc("get", "pod,service,secret,deployment", "--all-namespaces").Run()
			if err != nil {
				run.Out(os.Stderr, "  error: "+err.Error())
			}
			proc := c.KubectlProc(
				"get", "pods", "--all-namespaces", "--no-headers", "--field-selector=status.phase!=Running",
				"-o=custom-columns='NAMESPACE:.metadata.namespace,NAME:.metadata.name,STATUS:.status.phase'",
			)
			stdout := proc.CaptureStdout()
			if err = proc.Run(); err != nil {
				run.Out(os.Stderr, "  error: "+err.Error())
			}
			for podSpec := range strings.Lines(stdout.String()) {
				run.Out(os.Stderr, podSpec+"\n")
				split := strings.Fields(podSpec)
				err = c.KubectlProc("describe", "-n", split[0], "pod", split[1]).Run()
				if err != nil {
					run.Out(os.Stderr, "  error: "+err.Error())
				}
			}
		}
	}

	header(g.Principal)
	header(g.Autonomous)
	header(g.Managed)
}

func (g *Grid) DeployControlPlane(manifests *Manifests) error {
	err := doubleApply(g.Principal, "apply", "-k", manifests.Path("/control-plane/"))
	if err != nil {
		return err
	}

	repoServerHostname, err := g.waitForRepoServerHostname()
	if err != nil {
		return err
	}
	// TODO
	_ = repoServerHostname
	return nil
}

func (g *Grid) DeployAgents(manifests *Manifests) error {
	err := doubleApply(g.Managed, "apply", "-k", manifests.Path("agent-managed"))
	if err != nil {
		return err
	}

	err = doubleApply(g.Autonomous, "apply", "-k", manifests.Path("agent-autonomous"))
	if err != nil {
		return err
	}

	return nil
}

func doubleApply(c *cluster.KubeCluster, args ...string) error {
	// Run 'kubectl apply' twice, to avoid the following error that occurs during the first invocation:
	// - 'error: resource mapping not found for name: "default" namespace: "" from "(...)": no matches for kind "AppProject" in version "argoproj.io/v1alpha1"'
	_ = c.KubectlProc(args...).Run()
	return c.KubectlProc(args...).Run()
}

func (g *Grid) waitForRepoServerHostname() (string, error) {
	for i := 0; i < 15; i++ {
		repoServerHostname, err := g.getRepoServerHostname()
		if err == nil {
			return repoServerHostname, nil
		}
		if !errors.Is(err, errFailedWaitingForRepoServerHostname) {
			return "", err
		}

		run.Out(os.Stderr, "Waiting for repo server hostname...")
		time.Sleep(2 * time.Second)
	}

	return "", errFailedWaitingForRepoServerHostname
}

func (g *Grid) getRepoServerHostname() (string, error) {
	json, err := g.Principal.KubectlGetJson("svc", "argocd-repo-server")
	if err != nil {
		return "", err
	}
	found, err := json.SeekTo("status", "loadBalancer", "ingress", 0, "ip")
	if err != nil {
		return "", err
	}
	if !found {
		return "", errFailedWaitingForRepoServerHostname
	}
	var hostname string
	err = json.Decode(&hostname)
	if err != nil {
		return "", err
	}

	return hostname, nil
}

func (g *Grid) WaitForAllPodsRunning() error {
	var wg sync.WaitGroup
	wg.Add(3)

	var err error
	go func() {
		defer wg.Done()
		err = g.Principal.WaitForAllPodsRunning()
	}()
	go func() {
		defer wg.Done()
		err = g.Managed.WaitForAllPodsRunning()
	}()
	go func() {
		defer wg.Done()
		err = g.Autonomous.WaitForAllPodsRunning()
	}()

	wg.Wait()
	return err // if any
}
