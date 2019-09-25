package debug

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
)

var _ = Describe("Debug", func() {
	BeforeEach(func() {
		helpers.UseMemoryClients()
	})

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
