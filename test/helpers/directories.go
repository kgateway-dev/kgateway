package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/onsi/gomega"
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

// returns absolute path to the currently executing directory
func GetCallerDirectory(skip ...int) (string, error) {
	s := 1
	if len(skip) > 0 {
		s = skip[0] + 1
	}
	_, callerFile, _, ok := runtime.Caller(s)
	if !ok {
		return "", fmt.Errorf("failed to get runtime.Caller")
	}
	callerDir := filepath.Dir(callerFile)

	return filepath.Abs(callerDir)
}

// returns absolute path to the currently executing directory
func MustReadFile(name string) []byte {
	dir, err := GetCallerDirectory(1)
	Must(err)
	b, err := os.ReadFile(filepath.Join(dir, name))
	Must(err)
	return b
}

func Must(err error) {
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
}
