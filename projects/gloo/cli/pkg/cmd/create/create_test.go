package create_test

import (
	"log"
	"os"

	"github.com/hashicorp/consul/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	"github.com/solo-io/gloo/test/services"
)

var _ = Describe("Create", func() {
	if os.Getenv("RUN_CONSUL_TESTS") != "1" {
		log.Print("This test downloads and runs consul and is disabled by default. To enable, set RUN_CONSUL_TESTS=1 in your env.")
		return
	}

	var (
		consulFactory  *services.ConsulFactory
		consulInstance *services.ConsulInstance
		client         *api.Client
	)

	BeforeSuite(func() {
		var err error
		consulFactory, err = services.NewConsulFactory()
		Expect(err).NotTo(HaveOccurred())
		client, err = api.NewClient(api.DefaultConfig())
		Expect(err).NotTo(HaveOccurred())

	})

	AfterSuite(func() {
		_ = consulFactory.Clean()
	})

	BeforeEach(func() {
		helpers.UseDefaultClients()
		var err error
		// Start Consul
		consulInstance, err = consulFactory.NewConsulInstance()
		Expect(err).NotTo(HaveOccurred())
		err = consulInstance.Run()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if consulInstance != nil {
			err := consulInstance.Clean()
			Expect(err).NotTo(HaveOccurred())
		}
		helpers.UseDefaultClients()
	})

	Context("consul storage backend", func() {
		It("does upstreams and upstreamGroups", func() {
			err := testutils.Glooctl("create upstream static" +
				" --static-hosts jsonplaceholder.typicode.com:80 " +
				"--name json-upstream --use-consul")
			Expect(err).NotTo(HaveOccurred())
			kv, _, err := client.KV().Get("gloo/gloo.solo.io/v1/Upstream/gloo-system/json-upstream", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(kv).NotTo(BeNil())

			err = testutils.Glooctl("create upstreamgroup test --namespace gloo-system --weighted-upstreams gloo-system.json-upstream=1 --use-consul")
			Expect(err).NotTo(HaveOccurred())
			kv, _, err = client.KV().Get("gloo/gloo.solo.io/v1/UpstreamGroup/gloo-system/test", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(kv).NotTo(BeNil())
		})
		It("does virtualServices", func() {
			err := testutils.Glooctl("create virtualservice --name test --domains foo.bar,baz.qux --use-consul")
			Expect(err).NotTo(HaveOccurred())
			kv, _, err := client.KV().Get("gloo/gateway.solo.io/v1/VirtualService/gloo-system/test", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(kv).NotTo(BeNil())
		})
	})
})
