package agent

import (
	"encoding/base64"
	"fmt"
	"github.com/argoproj/dev-tools/cmd/run/run"
	"github.com/exponent-io/jsonpath"
	"github.com/sethvargo/go-password/password"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type Manifests struct {
	tempDir string
}

type ManifestData struct {
	LbNetPrefix     string
	ArgocdRelease   string
	PwdControlPlane string
	PwdManaged      string
	PwdAutonomous   string
}

func NewManifests(from string) (*Manifests, error) {
	tempDir, err := os.MkdirTemp("", "argo-dev-tools-*")
	if err != nil {
		return nil, err
	}
	m := &Manifests{tempDir: tempDir}

	// Much shorter than GO impl
	err = run.NewManagedProc("rsync", "-a", from, tempDir).Run()
	if err != nil {
		m.Close()
		return nil, err
	}

	proc := run.NewManagedProc(
		"git", "clone",
		"--depth=1", "--branch=stable",
		"--config=advice.detachedHead=false", "--quiet", // no warnings to clutter output
		"https://github.com/argoproj/argo-cd",
	)
	proc.Dir(m.tempDir)
	if err = proc.Run(); err != nil {
		return nil, err
	}

	return m, nil
}

func (m *Manifests) Close() {
	err := os.RemoveAll(m.tempDir)
	if err != nil {
		log.Printf("Error removing destination directory %s: %v", m.tempDir, err)
	}
}

func (m *Manifests) Path(relative string) string {
	absolute := m.NewPath(relative)
	if err := m.checkExists(absolute); err != nil {
		panic(err)
	}
	return absolute
}

func (m *Manifests) NewPath(relative string) string {
	return filepath.Join(m.tempDir, relative)
}

func (m *Manifests) checkExists(absolute string) error {
	_, err := os.Stat(absolute)
	return err
}

func (m *Manifests) Replace(path string, search string, replace string) error {
	if err := m.checkExists(path); err != nil {
		return err
	}
	return run.NewManagedProc("sed", "-i.bak", "s~"+search+"~"+replace+"~g", path).Run()
}

func (m *Manifests) Append(path string, content string) error {
	if err := m.checkExists(path); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(content)
	return err
}

func (m *Manifests) Create(path string, format string, args ...interface{}) error {
	if err := m.checkExists(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = fmt.Fprintf(file, format, args)
	return err
}

func (m *Manifests) InjectValues(data *ManifestData) error {
	var err error
	data.ArgocdRelease, err = m.argocdRelease()

	err = m.injectLb(data)
	if err != nil {
		return err
	}

	err = m.injectReleaseTag(data, err)
	if err != nil {
		return err
	}

	err = m.injectArgoSecrets(data, err)
	if err != nil {
		return err
	}

	return nil
}

func (m *Manifests) injectArgoSecrets(data *ManifestData, err error) error {
	err = m.Append(m.Path("agent-managed/argocd-secret.yaml"), fmt.Sprintf(
		"data:\n    server.secretkey: %s\n",
		data.PwdManaged,
	))
	if err != nil {
		return err
	}
	err = m.Append(m.Path("agent-autonomous/argocd-secret.yaml"), fmt.Sprintf(
		"data:\n    server.secretkey: %s\n",
		data.PwdAutonomous,
	))
	if err != nil {
		return err
	}
	err = m.Append(m.Path("control-plane/argocd-secret.yaml"), fmt.Sprintf(
		"data:\n    admin.password: %s\n    admin.passwordMtime: '%s'\n",
		data.PwdControlPlane,
		base64.StdEncoding.EncodeToString([]byte(time.Now().Format("2006-01-02T15:04:05-0700"))),
	))
	if err != nil {
		return err
	}

	return nil
}

func (m *Manifests) injectReleaseTag(data *ManifestData, err error) error {
	kustomizations := []string{
		"control-plane/kustomization.yaml",
		"agent-autonomous/kustomization.yaml",
		"agent-managed/kustomization.yaml",
	}
	for _, kustomization := range kustomizations {
		err = m.Replace(m.Path(kustomization), "LatestReleaseTag", data.ArgocdRelease)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Manifests) injectLb(data *ManifestData) error {
	proc := run.NewManagedProc(
		"sed", "-i.bak", "-e", "/loadBalancerIP/s/192\\.168\\.56./"+data.LbNetPrefix+"/",
		m.Path("control-plane/redis-service.yaml"),
		m.Path("control-plane/repo-server-service.yaml"),
		m.Path("control-plane/server-service.yaml"),
		m.Path("agent-managed/redis-service.yaml"),
		m.Path("agent-autonomous/redis-service.yaml"),
	)
	return proc.Run()
}

func (m *Manifests) InjectManagedAddresses(redis string, repoServer string) error {
	file := m.Path("agent-managed/argocd-cmd-params-cm.yaml")
	err := m.Replace(file, "repo-server-address", repoServer)
	if err != nil {
		return err
	}
	err = m.Replace(file, "redis-server-address", redis)
	if err != nil {
		return err
	}
	return nil
}

func (m *Manifests) argocdRelease() (string, error) {
	resp, err := http.Get("https://api.github.com/repos/argoproj/argo-cd/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed fetching argo-cd version: %s", resp.Status)
	}

	jp := jsonpath.NewDecoder(resp.Body)
	found, err := jp.SeekTo("tag_name")
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("failed to find tag_name")
	}

	var tagName string
	err = jp.Decode(&tagName)
	if err != nil {
		return "", err
	}
	return tagName, nil
}

func (m *Manifests) GenerateAccountSecrets() error {
	err := os.Mkdir(m.NewPath("creds"), os.FileMode(0750))
	if err != nil {
		return err
	}

	// Create file so htpasswd can append to it.
	controlPlane := m.NewPath("creds/users.control-plane")
	file, err := os.Create(controlPlane)
	if err != nil {
		return err
	}
	file.Close() // Handle not needed, close immediately

	accounts := []string{"agent-managed", "agent-autonomous"}
	for _, account := range accounts {
		pwd := password.MustGenerate(64, 10, 10, false, true)
		proc := run.NewManagedProc("htpasswd", "-b", "-B", controlPlane, account, pwd)
		if err = proc.Run(); err != nil {
			return err
		}
		err = m.Create(m.NewPath("creds/creds."+account), "%s:%s", account, pwd)
		if err != nil {
			return err
		}
	}

	return nil
}
