package test

import (
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Helm Template Generation", func() {
	// Lines ending with whitespace causes malformatted config map (https://github.com/solo-io/gloo/issues/4645)
	It("Should not containing trailing whitespace", func() {
		out, err := exec.Command("helm", "template", "..").CombinedOutput()
		Expect(err).NotTo(HaveOccurred())

		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			Expect(strings.HasSuffix(line, " ")).To(BeFalse())
		}
	})
})
