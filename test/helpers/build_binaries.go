package helpers

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/onsi/ginkgo"

	"github.com/solo-io/gloo/pkg/log"
)

// builds binaries
func BuildBinaries(outputDirectory string, debug bool) error {
	if os.Getenv("SKIP_BUILD") == "1" {
		return nil
	}
	// make the gloo containers
	for _, component := range []string{"control-plane", "function-discovery", "kube-ingress-controller", "upstream-discovery"} {
		arg := component

		if debug {
			arg += "-debug"
		}

		cmd := exec.Command("make", arg)
		cmd.Dir = GlooSoloDirectory()
		cmd.Stdout = ginkgo.GinkgoWriter
		cmd.Stderr = ginkgo.GinkgoWriter
		if outputDirectory != "" {
			cmd.Env = append(cmd.Env, "OUTPUT_DIR="+outputDirectory)
		}
		if err := cmd.Run(); err != nil {
			return err
		}
	}

	for _, path := range []string{
		//filepath.Join(GlooSoloDirectory(), "hack", "auditor"),
		filepath.Join(KubeE2eDirectory(), "containers", "helloservice"),
		//filepath.Join(KubeE2eDirectory(), "containers", "testrunner"),
		//filepath.Join(KubeE2eDirectory(), "containers", "event-emitter"),
		//filepath.Join(KubeE2eDirectory(), "containers", "upstream-for-events"),
		filepath.Join(KubeE2eDirectory(), "containers", "grpc-test-service"),
	} {
		log.Debugf("TEST: building binary %v", path)
		cmd := exec.Command("make", "build")
		cmd.Dir = path
		cmd.Stdout = ginkgo.GinkgoWriter
		cmd.Stderr = ginkgo.GinkgoWriter
		if outputDirectory != "" {
			cmd.Env = append(cmd.Env, "OUTPUT_DIR="+outputDirectory)
		}
		if err := cmd.Run(); err != nil {
			return err
		}
		cmd = exec.Command("make", "clean")
		cmd.Dir = path
		cmd.Stdout = ginkgo.GinkgoWriter
		cmd.Stderr = ginkgo.GinkgoWriter
		cmd.Run()
	}
	return nil
}
