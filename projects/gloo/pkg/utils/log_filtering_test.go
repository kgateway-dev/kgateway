package utils_test

import (
	"io"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

const (
	testLogLevels = "LOGS: {\"level\":\"error\",\"ts\":1}\n{\"level\":\"error\",\"ts\":1}\n{\"level\":\"info\",\"ts\":1}\n{\"level\":\"warn\",\"ts\":1}"
)

var _ = Describe("Debug", func() {

	It("should be able to parse out all logs", func() {
		logs := io.NopCloser(strings.NewReader(testLogLevels))
		filteredLogs := utils.FilterLogLevel(logs, utils.LogLevelAll)
		Expect(filteredLogs.String()).To(Equal(testLogLevels + "\n"))
	})

	It("should be able to parse out error logs", func() {
		logs := io.NopCloser(strings.NewReader(testLogLevels))
		filteredLogs := utils.FilterLogLevel(logs, utils.LogLevelError)
		Expect(filteredLogs.String()).To(Equal("LOGS: {\"level\":\"error\",\"ts\":1}\n{\"level\":\"error\",\"ts\":1}\n"))
	})

})
