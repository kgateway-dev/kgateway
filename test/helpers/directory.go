package helpers

import (
	"os"
	"path/filepath"
)

func SoloDirectory() string {
	return filepath.Join(os.Getenv("GOPATH"), "src", "github.com", "solo-io")
}
func GlooSoloDirectory() string {
	return filepath.Join(os.Getenv("GOPATH"), "src", "github.com", "solo-io", "gloo")
}

func HelmDirectory() string {
	return filepath.Join(SoloDirectory(), "gloo", "install", "helm", "gloo")
}

func LocalE2eDirectory() string {
	return filepath.Join(SoloDirectory(), "gloo", "test", "local_e2e")
}

func KubeE2eDirectory() string {
	return filepath.Join(SoloDirectory(), "gloo", "test", "kube_e2e")
}

func NomadE2eDirectory() string {
	return filepath.Join(SoloDirectory(), "gloo", "test", "nomad_e2e")
}

func CertsDirectory() string {
	return filepath.Join(KubeE2eDirectory(), "certs")
}

func ServerCert() string {
	return filepath.Join(CertsDirectory(), "root.crt")
}
func ServerKey() string {
	return filepath.Join(CertsDirectory(), "root.key")
}
