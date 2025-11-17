package curl_test

import (
	"github.com/onsi/gomega/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
)

var _ = Describe("Curl", func() {

	Context("BuildArgs", func() {

		DescribeTable("it builds the args using the provided option",
			func(option curl.Option, expectedMatcher types.GomegaMatcher) {
				Expect(curl.BuildArgs(option)).To(expectedMatcher)
			},
			Entry("VerboseOutput",
				curl.VerboseOutput(),
				ContainElement("-v"),
			),
			Entry("IgnoreServerCert",
				curl.IgnoreServerCert(),
				ContainElement("-k"),
			),
			Entry("Silent",
				curl.Silent(),
				ContainElement("-s"),
			),
			Entry("WithHeadersOnly",
				curl.WithHeadersOnly(),
				ContainElement("-I"),
			),
			Entry("WithCaFile",
				curl.WithCaFile("caFile"),
				ContainElement("--cacert"),
			),
			Entry("WithBody",
				curl.WithBody("body"),
				ContainElement("-d"),
			),
			Entry("WithRetries",
				curl.WithRetries(1, 1, 1),
				ContainElements("--retry", "--retry-delay", "--retry-max-time"),
			),
			Entry("WithArgs",
				curl.WithArgs([]string{"--custom-args"}),
				ContainElement("--custom-args"),
			),
		)

	})

})
