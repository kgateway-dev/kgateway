package glooctl_test

import (
	"bufio"
	"bytes"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/debug"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	gloodefaults "github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"io"
	"os"
	"path/filepath"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Debug", func() {

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

	It("should output logs by default", func() {
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

	It("should create a tar file at location specified in --file when --zip is enabled", func() {
		opts := options.Options{
			Metadata: core.Metadata{
				Namespace: gloodefaults.GlooSystem,
			},
			Top: options.Top{
				Zip: true,
			},
		}
		opts.Top.File = filepath.Join(tmpDir, "log.tgz")

		err := debug.DebugLogs(&opts, io.Discard)
		Expect(err).NotTo(HaveOccurred())

		_, err = os.Stat(opts.Top.File)
		Expect(err).NotTo(HaveOccurred())

		err = os.RemoveAll(opts.Top.File)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should create a text file at location specified in --file when --zip is not enabled", func() {
		opts := options.Options{
			Metadata: core.Metadata{
				Namespace: gloodefaults.GlooSystem,
			},
			Top: options.Top{
				Zip: false,
			},
		}
		opts.Top.File = filepath.Join(tmpDir, "log.txt")

		err := debug.DebugLogs(&opts, io.Discard)
		Expect(err).NotTo(HaveOccurred())

		_, err = os.Stat(opts.Top.File)
		Expect(err).NotTo(HaveOccurred())

		err = os.RemoveAll(opts.Top.File)
		Expect(err).NotTo(HaveOccurred())
	})
})
