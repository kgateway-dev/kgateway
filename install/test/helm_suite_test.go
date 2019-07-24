package test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/solo-io/go-utils/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/go-utils/manifesttestutils"
)

func TestHelm(t *testing.T) {
	RegisterFailHandler(Fail)
	testutils.RegisterPreFailHandler(
		func() {
			testutils.PrintTrimmedStack()
		})
	testutils.RegisterCommonFailHandlers()
	RunSpecs(t, "Helm Suite")
}

const (
	namespace = "gloo-system"
)

var (
	version      string
	testManifest TestManifest
)

func MustMake(dir string, args ...string) {
	makeCmd := exec.Command("make", args...)
	makeCmd.Dir = dir

	var b bytes.Buffer
	var be bytes.Buffer
	makeCmd.Stdout = &b
	makeCmd.Stderr = &be
	err := makeCmd.Run()

	if err != nil {
		fmt.Printf(b.String())
		fmt.Println("\nstderr:")
		fmt.Printf(be.String())
		fmt.Println()
	}
	Expect(err).NotTo(HaveOccurred())
}

var _ = SynchronizedBeforeSuite(
	func() []byte {
		MustMake(".", "-C", "../..", "install/gloo-gateway.yaml", "HELMFLAGS=--namespace "+namespace+" --set namespace.create=true  --set gatewayProxies.gatewayProxy.service.extraAnnotations.test=test --set gatewayProxies.gatewayProxy.tracing=trace:spec")
		return nil
	},
	func(_ []byte) {
		testManifest = NewTestManifest("../gloo-gateway.yaml")
		version = os.Getenv("TAGGED_VERSION")
		if version == "" {
			version = "dev"
		} else {
			version = version[1:]
		}
	})
