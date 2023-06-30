package add_test

import (
	"context"
	"github.com/solo-io/gloo/test/ginkgo/decorators"
	"log"

	"github.com/hashicorp/consul/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	cliutils "github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	"github.com/solo-io/gloo/test/testutils"
)

var _ = Describe("Add", decorators.Consul, func() {
	if !testutils.IsEnvTruthy(testutils.RunConsulTests) {
		log.Print("This test downloads and runs consul and is disabled by default. To enable, set RUN_CONSUL_TESTS=1 in your env.")
		return
	}

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		helpers.UseDefaultClients()

		ctx, cancel = context.WithCancel(context.Background())

		consulInstance = consulFactory.MustConsulInstance()
		err := consulInstance.Run(ctx)
		Expect(err).NotTo(HaveOccurred())

		// wait for consul to start
		Eventually(func() error {
			_, err := client.KV().Put(&api.KVPair{Key: "test"}, nil)
			return err
		}).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		helpers.UseDefaultClients()

		cancel()
	})

	Context("consul storage backend", func() {
		It("works", func() {
			err := cliutils.Glooctl("add route " +
				"--path-prefix / " +
				"--dest-name petstore " +
				"--prefix-rewrite /api/pets " +
				"--use-consul")
			Expect(err).NotTo(HaveOccurred())
			kv, _, err := client.KV().Get("gloo/gateway.solo.io/v1/VirtualService/gloo-system/default", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(kv).NotTo(BeNil())
		})
	})
})
