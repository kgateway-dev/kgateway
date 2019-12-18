package debug

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	installcmd "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install"

	"github.com/solo-io/gloo/pkg/cliutil/install"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
)

var _ = Describe("Debug", func() {
	BeforeEach(func() {
		helpers.UseMemoryClients()
	})

	Context("log debugger", func() {
		It("should output logs by default", func() {
			opts := options.Options{}
			opts.Metadata.Namespace = "gloo-system"
			var b bytes.Buffer
			w := bufio.NewWriter(&b)
			err := DebugLogs(&opts, w)
			Expect(err).NotTo(HaveOccurred())

			err = w.Flush()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a tar file at location specified in --file when --zip is enabled", func() {
			opts := options.Options{}
			opts.Metadata.Namespace = "gloo-system"
			opts.Top.File = "/tmp/log.tgz"
			opts.Top.Zip = true
			err := DebugLogs(&opts, ioutil.Discard)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(opts.Top.File)
			Expect(err).NotTo(HaveOccurred())

			err = os.RemoveAll(opts.Top.File)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("yaml dumper", func() {
		var (
			kubeCli        *install.MockKubectl
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
			tempFile, err := ioutil.TempFile("", "")
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(tempFile.Name())

			var cmds []string
			for _, kind := range importantKinds {
				cmds = append(cmds, fmt.Sprintf("get %s -oyaml -n test-namespace", kind))
			}

			// don't really care what the returned data is, just want len(cmd) lines returned
			expectedOutput = strings.Split(strings.Repeat("dummy-data-ignore_", len(cmds)), "_")

			kubeCli = install.NewMockKubectl(cmds, expectedOutput)

			err = DumpYaml(tempFile.Name(), "test-namespace", kubeCli)
			Expect(err).NotTo(HaveOccurred(), "Should be able to dump yaml without returning an error")

			writtenBytes, err := ioutil.ReadFile(tempFile.Name())

			Expect(err).NotTo(HaveOccurred(), "Should be able to read the temp yaml file")
			Expect(writtenBytes).NotTo(BeEmpty(), "Should have written a nonzero number of bytes")

			manifests := strings.Split(string(writtenBytes), "---")
			Expect(manifests).To(HaveLen(len(cmds)), "Should have written the same number of manifests as commands")
		})
	})
})
