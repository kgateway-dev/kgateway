package debug

import (
	"fmt"
	"os"
	"strings"

	"github.com/solo-io/gloo/pkg/cliutil/testutil"
	installcmd "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
)

var _ = Describe("Debug", func() {

	BeforeEach(func() {
		helpers.UseMemoryClients()
	})

	Context("yaml dumper", func() {
		var (
			kubeCli        *testutil.MockKubectl
			expectedOutput []string
			importantKinds = append(append([]string{}, installcmd.GlooNamespacedKinds...), installcmd.GlooCrdNames...)
		)

		BeforeEach(func() {
			expectedOutput = []string{}
		})

		AfterEach(func() {
			kubeCli = nil
		})

		It("should attempt to retrieve all the important Kinds", func() {
			tempFile, err := os.CreateTemp("", "")
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(tempFile.Name())

			var cmds []string
			for _, kind := range importantKinds {
				cmds = append(cmds, fmt.Sprintf("get %s -oyaml -n test-namespace", kind))
			}

			// don't really care what the returned data is, just want len(cmd) lines returned
			expectedOutput = strings.Split(strings.Repeat("dummy-data-ignore_", len(cmds)), "_")

			kubeCli = testutil.NewMockKubectl(cmds, expectedOutput)

			err = DumpYaml(tempFile.Name(), "test-namespace", kubeCli)
			Expect(err).NotTo(HaveOccurred(), "Should be able to dump yaml without returning an error")

			writtenBytes, err := os.ReadFile(tempFile.Name())

			Expect(err).NotTo(HaveOccurred(), "Should be able to read the temp yaml file")
			Expect(writtenBytes).NotTo(BeEmpty(), "Should have written a nonzero number of bytes")

			manifests := strings.Split(string(writtenBytes), "---")
			Expect(manifests).To(HaveLen(len(cmds)), "Should have written the same number of manifests as commands")
		})
	})
})
