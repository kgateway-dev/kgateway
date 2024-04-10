package cmdutils

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("File", func() {

	Context("RunCommandOutputToFile", func() {

		var (
			tmpFile string
		)

		AfterEach(func() {
			_ = os.RemoveAll(tmpFile)
		})

		It("runs command to file, if file does exist", func() {
			f, err := os.CreateTemp("", "cmdutils-test")
			Expect(err).NotTo(HaveOccurred())

			tmpFile = f.Name()
			writeActiveProcessesToFileAndAssertOutput(context.Background(), tmpFile)
		})

		It("runs command to file, if file does NOT exist", func() {
			tmpFile = filepath.Join(os.TempDir(), "file-does-not-exist.txt")
			writeActiveProcessesToFileAndAssertOutput(context.Background(), tmpFile)
		})

	})

})

func writeActiveProcessesToFileAndAssertOutput(ctx context.Context, fileName string) {
	GinkgoHelper()

	cmdFn := RunCommandOutputToFile(
		Command(ctx, "ps", "-a").WithStdout(GinkgoWriter).WithStderr(GinkgoWriter),
		fileName)
	Expect(cmdFn()).NotTo(HaveOccurred(), "Can execute the function without an error")

	fileInfo, err := os.Stat(fileName)
	Expect(err).NotTo(HaveOccurred())
	Expect(fileInfo.Size()).NotTo(BeZero(), "process data was written to file")
}
