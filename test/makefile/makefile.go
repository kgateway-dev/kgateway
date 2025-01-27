//go:build ignore

package makefile

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// MustMake runs a make command
func MustMake(dir string, args ...string) {
	makeCmd := exec.Command("make", args...)
	makeCmd.Dir = dir

	makeCmd.Stdout = GinkgoWriter
	makeCmd.Stderr = GinkgoWriter
	err := makeCmd.Run()

	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

// MustMakeReturnStdout runs a make command and returns the stdout output
func MustMakeReturnStdout(dir string, args ...string) string {
	makeCmd := exec.Command("make", args...)
	makeCmd.Dir = dir

	var stdout bytes.Buffer
	makeCmd.Stdout = &stdout

	makeCmd.Stderr = GinkgoWriter
	err := makeCmd.Run()

	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	return stdout.String()
}

// MustGetVersion returns the VERSION that will be used to build the chart
func MustGetVersion(dir string, args ...string) string {
	args = append(args, "print-VERSION") // use print-VERSION so version matches on forks
	output := MustMakeReturnStdout(dir, args...)
	lines := strings.Split(output, "\n")

	// output from a fork:
	// <[]string | len:4, cap:4>: [
	//	"make[1]: Entering directory '/workspace/gloo'",
	//	"<VERSION>",
	//	"make[1]: Leaving directory '/workspace/gloo'",
	//	"",
	// ]

	// output from the gloo repo:
	// <[]string | len:2, cap:2>: [
	//	"<VERSION>",
	//	"",
	// ]

	if len(lines) == 4 {
		// This is being executed from a fork
		return lines[1]
	}

	if len(lines) == 2 {
		// This is being executed from the Gloo repo
		return lines[0]
	}

	// Error loudly to prevent subtle failures
	Fail(fmt.Sprintf("print-VERSION output returned unknown format. %v", lines))
	return "version-not-found"
}
