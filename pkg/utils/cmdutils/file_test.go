package cmdutils

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("File", func() {

	Context("RunCommandOutputToFileFunc", func() {

		var (
			cmd     Cmd
			tmpFile string
		)

		BeforeEach(func() {
			cmd = Command(context.Background(), "ps").WithStdout(GinkgoWriter).WithStderr(GinkgoWriter)
			tmpFile = ""
		})

		AfterEach(func() {
			_ = os.RemoveAll(tmpFile)
		})

		It("runs command to file, if file does exist", func() {
			f, err := os.CreateTemp("", "cmdutils")
			Expect(err).NotTo(HaveOccurred())
			tmpFile = f.Name()

			cmdFn := RunCommandOutputToFileFunc(cmd, tmpFile)
			Expect(cmdFn()).NotTo(HaveOccurred(), "Can execute the function without an error")

			fileInfo, err := os.Stat(tmpFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(fileInfo.Size()).NotTo(BeZero(), "process data was written to file")
		})

		It("runs command to file, if file does NOT exist", func() {
			tmpFile = filepath.Join(os.TempDir(), "file-does-not-exist.txt")

			cmdFn := RunCommandOutputToFileFunc(cmd, tmpFile)
			Expect(cmdFn()).NotTo(HaveOccurred(), "Can execute the function without an error")

			fileInfo, err := os.Stat(tmpFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(fileInfo.Size()).NotTo(BeZero(), "process data was written to file")
		})

	})

})
