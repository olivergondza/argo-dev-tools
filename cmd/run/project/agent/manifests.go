package agent

import (
	"github.com/argoproj/dev-tools/cmd/run/run"
	"log"
	"os"
	"path/filepath"
)

type Manifests struct {
	tempDir string
}

func NewManifests(from string) (*Manifests, error) {
	tempDir, err := os.MkdirTemp("", "argo-dev-tools-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		err := os.RemoveAll(tempDir)
		if err != nil {
			log.Printf("Error removing temporary directory %s: %v", tempDir, err)
		}
	}()

	m := &Manifests{tempDir: tempDir}
	err = run.NewManagedProc("cp", "-r", from, tempDir).Run()
	if err != nil {
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

func (m *Manifests) Path(path string) string {
	return filepath.Join(m.tempDir, path)
}
