package helpers

import (
	"os"
	"path/filepath"
)

func GlooDir() string {
	return filepath.Join(os.Getenv("GOPATH"), "src", "github.com", "solo-io", "gloo")
}

func GlooTestContainersDir() string {
	return filepath.Join(GlooDir(), "test", "kube2e", "containers")
}

func GlooInstallDir() string {
	return filepath.Join(GlooDir(), "install")
}

func GlooHelmChartDir() string {
	return filepath.Join(GlooInstallDir(), "helm", "gloo")
}
