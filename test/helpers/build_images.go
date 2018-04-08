package helpers

import (
	"fmt"
	"hash/crc32"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/onsi/ginkgo"

	"github.com/solo-io/gloo/pkg/log"
)

var imageTagStatic = ""

func ImageTag() string {
	if imageTagStatic != "" {
		return imageTagStatic
	}
	tag := os.Getenv("TEST_IMAGE_TAG")
	if tag == "" {
		if host, err := os.Hostname(); err == nil {
			tag = hash(host)
		} else {
			tag = RandString(4)
		}
	}

	imageTagStatic = "testing-" + tag
	return imageTagStatic
}

func hash(h string) string {
	crc32q := crc32.MakeTable(0xD5828281)
	return fmt.Sprintf("%08x", crc32.Checksum([]byte(h), crc32q))
}

// builds and pushes all docker containers needed for test
func BuildPushContainers(push, debug bool) error {
	if os.Getenv("SKIP_BUILD") == "1" {
		return nil
	}
	imageTag := ImageTag()
	os.Setenv("IMAGE_TAG", imageTag)

	// make the gloo containers
	for _, component := range []string{"control-plane", "function-discovery", "kube-ingress-controller", "kube-upstream-discovery"} {
		arg := component
		arg += "-docker"
		if push {
			arg += "-push"
		}

		if debug {
			arg += "-debug"
		}

		cmd := exec.Command("make", arg)
		cmd.Dir = GlooSoloDirectory()
		cmd.Stdout = ginkgo.GinkgoWriter
		cmd.Stderr = ginkgo.GinkgoWriter
		if err := cmd.Run(); err != nil {
			return err
		}
	}

	for _, path := range []string{
		filepath.Join(GlooSoloDirectory(), "hack", "auditor"),
		filepath.Join(E2eDirectory(), "containers", "helloservice"),
		filepath.Join(E2eDirectory(), "containers", "testrunner"),
		filepath.Join(E2eDirectory(), "containers", "event-emitter"),
		filepath.Join(E2eDirectory(), "containers", "upstream-for-events"),
		filepath.Join(E2eDirectory(), "containers", "grpc-test-service"),
	} {
		dockerUser := os.Getenv("DOCKER_USER")
		if dockerUser == "" {
			dockerUser = "soloio"
		}
		fullImage := dockerUser + "/" + filepath.Base(path) + ":" + ImageTag()
		log.Debugf("TEST: building fullImage %v", fullImage)
		cmd := exec.Command("make", "docker")
		cmd.Dir = path
		cmd.Stdout = ginkgo.GinkgoWriter
		cmd.Stderr = ginkgo.GinkgoWriter
		if err := cmd.Run(); err != nil {
			return err
		}
		if push {
			cmd = exec.Command("docker", "push", fullImage)
			cmd.Stdout = ginkgo.GinkgoWriter
			cmd.Stderr = ginkgo.GinkgoWriter
			if err := cmd.Run(); err != nil {
				return err
			}
		}
		cmd = exec.Command("make", "clean")
		cmd.Dir = path
		cmd.Stdout = ginkgo.GinkgoWriter
		cmd.Stderr = ginkgo.GinkgoWriter
		cmd.Run()
	}
	return nil
}
