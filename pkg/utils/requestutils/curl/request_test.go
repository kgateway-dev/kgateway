package curl_test

import (
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Curl", func() {

	Context("BuildArgs", func() {

		DescribeTable("it builds the args using the provided option",
			func(option curl.Option, expectedMatcher types.GomegaMatcher) {
				// requiredOptions is the set of curl.Option that is necessary for BuildArgsOrError
				// to not return an error
				requiredOptions := []curl.Option{
					curl.WithService("service"),
				}

				args := curl.BuildArgs(append(requiredOptions, option)...)
				Expect(args).To(expectedMatcher)
			},
			Entry("VerboseOutput",
				curl.VerboseOutput(),
				ContainElement("-v"),
			),
			Entry("AllowInsecure",
				curl.AllowInsecure(),
				ContainElement("-k"),
			),
			Entry("WithoutStats",
				curl.WithoutStats(),
				ContainElement("-s"),
			),
			Entry("WithReturnHeaders",
				curl.WithReturnHeaders(),
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
			Entry("SelfSigned",
				curl.SelfSigned(),
				ContainElement("-k"),
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
