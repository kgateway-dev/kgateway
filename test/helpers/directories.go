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

func GlooTestArtifactsDir() string {
	return filepath.Join(GlooDir(), "test", "kube2e", "artifacts")
}

func GlooHelmChartDir() string {
	return filepath.Join(GlooDir(), "install", "helm", "gloo")
}
