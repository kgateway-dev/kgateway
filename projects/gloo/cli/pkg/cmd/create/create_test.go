package create_test

import (
	"context"
	"github.com/solo-io/gloo/test/ginkgo/decorators"
	"log"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	cliutils "github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	"github.com/solo-io/gloo/test/testutils"
)

var _ = Describe("Create", decorators.Consul, func() {
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
	})

	AfterEach(func() {
		helpers.UseDefaultClients()

		cancel()
	})

	Context("consul storage backend", func() {
		It("does upstreams and upstreamGroups", func() {
			err := cliutils.Glooctl("create upstream static" +
				" --static-hosts jsonplaceholder.typicode.com:80 " +
				"--name json-upstream --use-consul")
			Expect(err).NotTo(HaveOccurred())
			kv, _, err := client.KV().Get("gloo/gloo.solo.io/v1/Upstream/gloo-system/json-upstream", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(kv).NotTo(BeNil())

			err = cliutils.Glooctl("create upstreamgroup test --namespace gloo-system --weighted-upstreams gloo-system.json-upstream=1 --use-consul")
			Expect(err).NotTo(HaveOccurred())
			kv, _, err = client.KV().Get("gloo/gloo.solo.io/v1/UpstreamGroup/gloo-system/test", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(kv).NotTo(BeNil())
		})
		It("does virtualServices", func() {
			err := cliutils.Glooctl("create virtualservice --name test --domains foo.bar,baz.qux --use-consul")
			Expect(err).NotTo(HaveOccurred())
			kv, _, err := client.KV().Get("gloo/gateway.solo.io/v1/VirtualService/gloo-system/test", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(kv).NotTo(BeNil())
		})
	})
})
