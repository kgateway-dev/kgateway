package make_test

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMake(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Make Suite")
}

type EnvVar struct {
	Name, Value string
}

type MakeVar struct {
	Name, ExpectedValue string
}

func ExpectMakeVarsWithEnvVars(envVars []*EnvVar, makeVars []*MakeVar) {
	for _, envVar := range envVars {
		err := os.Setenv(envVar.Name, envVar.Value)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
	}
	for _, makeVar := range makeVars {
		cmd := exec.Command("make", "-C", "../..", fmt.Sprintf("print-%s", makeVar.Name))
		out, err := cmd.CombinedOutput()
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		output := strings.TrimSpace(string(out))

		ExpectWithOffset(1, output).To(ContainSubstring(makeVar.ExpectedValue))
	}
	for _, envVar := range envVars {
		err := os.Unsetenv(envVar.Name)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
	}
}
