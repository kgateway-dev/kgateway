package admincli_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/envoyutils/admincli"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/go-utils/threadsafe"
)

var _ = Describe("Client", func() {

	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("Client tests", func() {

		It("WithCurlOptions can append and override default curl.Option", func() {
			client := admincli.NewClient().WithCurlOptions(
				curl.WithRetries(5, 5, 5), // override
				curl.WithoutStats(),       // new value
			)

			curlCommand := client.Command(ctx).Run().PrettyCommand()
			Expect(curlCommand).To(And(
				ContainSubstring("\"--retry\" \"5\""),
				ContainSubstring("\"--retry-delay\" \"5\""),
				ContainSubstring("\"--retry-max-time\" \"5\""),
				ContainSubstring(" \"-s\""),
			))
		})

	})

	Context("Integration tests", func() {

		When("Admin API is reachable", func() {
			// We do not write additional integration tests for when the Admin API is reachable
			// This utility is used in our test/services/envoy.Instance, which is the core service
			// for our in-memory e2e (test/e2e) tests.
		})

		When("Admin API is not reachable", func() {

			It("emits an error to configured locations", func() {
				var (
					defaultOutputLocation, errLocation, outLocation threadsafe.Buffer
				)

				// Create a client that points to an address where Envoy is NOT running
				client := admincli.NewClient().
					WithReceiver(&defaultOutputLocation).
					WithCurlOptions(
						curl.WithScheme("http"),
						curl.WithService("localhost"),
						curl.WithPort(1111),
						// Since we expect this test to fail, we don't need to use all the reties that the client defaults to use
						curl.WithoutRetries(),
					)

				statsCmd := client.StatsCmd(ctx).
					WithStdout(&outLocation).
					WithStderr(&errLocation)

				err := statsCmd.Run().Cause()
				Expect(err).To(HaveOccurred(), "running the command should return an error")
				Expect(defaultOutputLocation.Bytes()).To(BeEmpty(), "defaultOutputLocation should not be used")
				Expect(outLocation.Bytes()).To(BeEmpty(), "failed request should not output to Stdout")
				Expect(string(errLocation.Bytes())).To(ContainSubstring("Failed to connect to localhost port 1111"), "failed request should output to Stderr")
			})
		})
	})
})
