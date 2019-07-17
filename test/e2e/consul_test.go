package e2e_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	consulconfig "github.com/hashicorp/consul/agent/config"
)

var _ = Describe("Consul", func() {

	var (
		ctx            context.Context
		cancel         context.CancelFunc
		testClients    services.TestClients
		consulInstance *services.ConsulInstance
	)

	BeforeEach(func() {

		// TODO(marco): revamp when writing final e2e tests for consul routing
		Skip("consul e2e tests are temporarily disabled")

		ctx, cancel = context.WithCancel(context.Background())
		// start consul first
		var err error
		consulInstance, err = consulFactory.NewConsulInstance()
		if err != nil {
			Skip("please have consul to run this test")
		}
		consulInstance.Run()
		t := services.RunGateway(ctx, true)
		testClients = t
	})

	AfterEach(func() {
		if consulInstance != nil {
			consulInstance.Clean()
		}
		cancel()
	})

	It("should detect service", func() {
		s := func(str string) *string { return &str }
		// add service to consul
		csvc := &consulconfig.ServiceDefinition{
			Name: s("consul-service"),
		}
		consulInstance.AddConfigFromStruct("svc.json", csvc)

		// wait and see that upstream discovery discovered it

		Eventually(func() (int, error) {
			var listopt clients.ListOpts
			listopt.Ctx = ctx
			upstreams, err := testClients.UpstreamClient.List(defaults.GlooSystem, listopt)
			if err != nil {
				return 0, err
			}

			var consulUpstreams int
			for _, upstream := range upstreams {
				if _, ok := upstream.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_Consul); ok {
					consulUpstreams++
				}
			}
			return consulUpstreams, nil
		}, "10s", ".5s").Should(Not(BeZero()))

	})
})
