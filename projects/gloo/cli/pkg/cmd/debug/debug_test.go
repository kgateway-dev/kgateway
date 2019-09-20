package debug

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"os"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
)

const (
	testLogLevels = "{\"level\":\"error\",\"ts\":1}\n{\"level\":\"info\",\"ts\":1}\n{\"level\":\"warn\",\"ts\":1}"
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
		err := DebugGlooSystemLogs(&opts, w)
		Expect(err).NotTo(HaveOccurred())

		err = w.Flush()
		Expect(err).NotTo(HaveOccurred())
	})

	It("should create a tar file at location specified in --file when --zip is enabled", func() {
		opts := options.Options{}
		opts.Metadata.Namespace = "gloo-system"
		opts.Top.File = "/tmp/log.tgz"
		opts.Top.Zip = true
		err := DebugGlooSystemLogs(&opts, ioutil.Discard)
		Expect(err).NotTo(HaveOccurred())

		_, err = os.Stat(opts.Top.File)
		Expect(err).NotTo(HaveOccurred())

		err = os.RemoveAll(opts.Top.File)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should be able to parse out all logs", func() {
		logs := ioutil.NopCloser(strings.NewReader(testLogLevels))
		filteredLogs := parseLogsFrom(logs, false)
		Expect(filteredLogs.String()).To(Equal(testLogLevels + "\n"))
	})

	It("should be able to parse out error logs", func() {
		logs := ioutil.NopCloser(strings.NewReader(testLogLevels))
		filteredLogs := parseLogsFrom(logs, true)
		Expect(filteredLogs.String()).To(Equal("{\"level\":\"error\",\"ts\":1}\n"))
	})

})
