package admincli_test

import (
	"context"

	"github.com/solo-io/gloo/pkg/utils/envoyutils/admincli"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

	Context("Unit tests", func() {

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
				client := admincli.NewClient(&defaultOutputLocation, []curl.Option{
					curl.WithScheme("http"),
					curl.WithService("localhost"),
					curl.WithPort(1111),
					// Since we expect this test to fail, we don't need to use all the reties that the client defaults to use
					curl.WithRetries(0, 0, 0),
				})

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
