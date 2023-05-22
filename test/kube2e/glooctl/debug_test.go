package glooctl_test

import (
	"bufio"
	"bytes"
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/debug"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	gloodefaults "github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"os"
	"path/filepath"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Debug", func() {

	// These tests formerly lived at: https://github.com/solo-io/gloo/blob/063dbf3ba7b7666d0111741c083b197364b14716/projects/gloo/cli/pkg/cmd/debug
	// They were migrated to this package since they depend on a k8s cluster

	Context("Logs", func() {

		Context("stdout", func() {

			It("should succeed", func() {
				opts := options.Options{
					Metadata: core.Metadata{
						Namespace: gloodefaults.GlooSystem,
					},
				}

				var b bytes.Buffer
				w := bufio.NewWriter(&b)

				err := debug.DebugLogs(&opts, w)
				Expect(err).NotTo(HaveOccurred())

				err = w.Flush()
				Expect(err).NotTo(HaveOccurred())
			})

			It("should not crash (logs)", func() {
				err := testutils.Glooctl("debug logs")
				Expect(err).NotTo(HaveOccurred())
			})

			It("should not crash (log)", func() {
				err := testutils.Glooctl("debug log")
				Expect(err).NotTo(HaveOccurred())
			})

		})

		Context("file", func() {

			var (
				tmpDir string
			)

			BeforeEach(func() {
				var err error
				tmpDir, err = os.MkdirTemp("", "testDir")
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				_ = os.RemoveAll(tmpDir)
			})

			It("should create a tar file at location specified in --file when --zip is enabled", func() {
				outputFile := filepath.Join(tmpDir, "log.tgz")

				err := testutils.Glooctl(fmt.Sprintf("debug logs -n %s --file %s --zip %s", gloodefaults.GlooSystem, outputFile, "true"))
				Expect(err).NotTo(HaveOccurred(), "glooctl command should have succeeded")

				_, err = os.Stat(outputFile)
				Expect(err).NotTo(HaveOccurred(), "Output file should have been generated")
			})

			It("should create a text file at location specified in --file when --zip is not enabled", func() {
				outputFile := filepath.Join(tmpDir, "log.txt")

				err := testutils.Glooctl(fmt.Sprintf("debug logs -n %s --file %s --zip %s", gloodefaults.GlooSystem, outputFile, "false"))
				Expect(err).NotTo(HaveOccurred(), "glooctl command should have succeeded")

				_, err = os.Stat(outputFile)
				Expect(err).NotTo(HaveOccurred(), "Output file should have been generated")
			})
		})

	})

})
