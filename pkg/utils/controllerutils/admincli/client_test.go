package admincli_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/go-utils/threadsafe"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/controllerutils/admincli"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
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
				curl.WithRetries(1, 1, 1), // override
				curl.Silent(),             // new value
			)

			curlCommand := client.Command(ctx).PrettyCommand()
			Expect(curlCommand).To(And(
				ContainSubstring("\"--retry\" \"1\""),
				ContainSubstring("\"--retry-delay\" \"1\""),
				ContainSubstring("\"--retry-max-time\" \"1\""),
				ContainSubstring(" \"-s\""),
			))
		})

	})

	Context("Integration tests", func() {

		When("Admin API is reachable", func() {
			// We rely on e2e tests defined in /test/kubernetes/e2e/features/admin_server to verify this behavior
		})

		When("Admin API is not reachable", func() {

			It("emits an error to configured locations", func() {
				var (
					defaultOutputLocation, errLocation, outLocation threadsafe.Buffer
				)

				// Create a client that points to an address where Gloo is NOT running
				client := admincli.NewClient().
					WithReceiver(&defaultOutputLocation).
					WithCurlOptions(
						curl.WithScheme("http"),
						curl.WithHost("127.0.0.1"),
						curl.WithPort(1111),
						// Since we expect this test to fail, we don't need to use all the reties that the client defaults to use
						curl.WithoutRetries(),
					)

				inputSnapshotCmd := client.InputSnapshotCmd(ctx).
					WithStdout(&outLocation).
					WithStderr(&errLocation)

				err := inputSnapshotCmd.Run().Cause()
				Expect(err).To(HaveOccurred(), "running the command should return an error")
				Expect(defaultOutputLocation.Bytes()).To(BeEmpty(), "defaultOutputLocation should not be used")
				Expect(outLocation.Bytes()).To(BeEmpty(), "failed request should not output to Stdout")
				Expect(errLocation.String()).To(ContainSubstring("Failed to connect to 127.0.0.1 port 1111"), "failed request should output to Stderr")
			})
		})
	})
})
