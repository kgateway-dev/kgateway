package syncer

import (
	"context"
	"fmt"
	"time"

	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector/singlereplica"

	"github.com/solo-io/gloo/pkg/utils/statusutils"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils/metrics"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/reconciler"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var _ = Describe("TranslatorSyncer integration test", func() {

	var (
		ts                       gloov1snap.ApiSyncer
		baseVirtualServiceClient v1.VirtualServiceClient
		proxyClient              gloov1.ProxyClient
		vs                       *v1.VirtualService
		snapshot                 func() *gloov1snap.ApiSnapshot
		statusClient             resources.StatusClient

		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		memFactory := &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}

		gatewayClient, err := v1.NewGatewayClient(ctx, memFactory)
		Expect(err).NotTo(HaveOccurred())
		if err := gatewayClient.Register(); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}

		baseVirtualServiceClient, err = v1.NewVirtualServiceClient(ctx, memFactory)
		virtualServiceClient := &delayingVsClient{
			VirtualServiceClient: baseVirtualServiceClient,
			// delay vs write, to induce the bug
			SleepDuration: time.Second / 2,
		}
		Expect(err).NotTo(HaveOccurred())
		if err := virtualServiceClient.Register(); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		routeTableClient, err := v1.NewRouteTableClient(ctx, memFactory)
		Expect(err).NotTo(HaveOccurred())
		if err := routeTableClient.Register(); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}

		statusClient = statusutils.GetStatusClientFromEnvOrDefault(defaults.GlooSystem)
		statusMetrics, err := metrics.NewConfigStatusMetrics(metrics.GetDefaultConfigStatusOptions())
		Expect(err).NotTo(HaveOccurred())

		proxyClient, err = gloov1.NewProxyClient(ctx, memFactory)
		Expect(err).NotTo(HaveOccurred())
		proxyReconciler := reconciler.NewProxyReconciler(nil, proxyClient, statusClient)
		rpt := reporter.NewReporter("gateway", statusClient, gatewayClient.BaseClient(), virtualServiceClient.BaseClient(), routeTableClient.BaseClient())
		xlator := translator.NewDefaultTranslator(translator.Opts{
			WriteNamespace: defaults.GlooSystem,
		})
		ts = NewTranslatorSyncer(ctx, defaults.GlooSystem, proxyClient, proxyReconciler, rpt, xlator, statusClient, statusMetrics, singlereplica.Identity())

		vs = &v1.VirtualService{
			Metadata: &core.Metadata{
				Name:      "name",
				Namespace: "gloo-system",
			},
			VirtualHost: &v1.VirtualHost{
				Routes: []*v1.Route{
					{
						Matchers: []*matchers.Matcher{
							{
								PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/"},
							},
						},
						Action: &v1.Route_DirectResponseAction{
							DirectResponseAction: &gloov1.DirectResponseAction{
								Status: 200,
								Body:   "foo",
							},
						},
					},
				},
			},
		}
		_, err = baseVirtualServiceClient.Write(vs, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
		gw := &v1.Gateway{
			GatewayType: &v1.Gateway_HttpGateway{
				HttpGateway: &v1.HttpGateway{
					VirtualServices: []*core.ResourceRef{
						vs.Metadata.Ref(),
					},
				},
			},
			BindAddress: "::",
			BindPort:    8080,
			ProxyNames:  []string{"gateway-proxy"},
			Metadata: &core.Metadata{
				Name:      "gateway-proxy",
				Namespace: "gloo-system",
			},
		}
		_, err = gatewayClient.Write(gw, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
		snapshot = func() *gloov1snap.ApiSnapshot {
			vss, err := baseVirtualServiceClient.List("gloo-system", clients.ListOpts{})
			Expect(err).NotTo(HaveOccurred())
			gws, err := gatewayClient.List("gloo-system", clients.ListOpts{})
			Expect(err).NotTo(HaveOccurred())
			return &gloov1snap.ApiSnapshot{
				VirtualServices: vss,
				Gateways:        gws,
			}
		}

	})

	AfterEach(func() {
		cancel()
	})

	EventuallyProxyStatusInVs := func() gomega.AsyncAssertion {
		return Eventually(func() (core.Status_State, error) {
			newvs, err := baseVirtualServiceClient.Read(vs.Metadata.Namespace, vs.Metadata.Name, clients.ReadOpts{})
			if err != nil {
				return core.Status_Pending, err
			}

			subresource := statusClient.GetStatus(newvs).GetSubresourceStatuses()
			if subresource == nil {
				return core.Status_Pending, fmt.Errorf("no status")
			}
			proxyState := subresource["*v1.Proxy.gateway-proxy_gloo-system"]

			if proxyState == nil {
				return core.Status_Pending, fmt.Errorf("no state")
			}
			return proxyState.GetState(), nil
		})
	}

	EventuallyProxyStatus := func() gomega.AsyncAssertion {
		return Eventually(func() (core.Status_State, error) {
			proxy, err := proxyClient.Read("gloo-system", "gateway-proxy", clients.ReadOpts{})
			if err != nil {
				return core.Status_Pending, err
			}

			return statusClient.GetStatus(proxy).GetState(), nil
		})
	}

	AcceptProxy := func() {
		proxy, err := proxyClient.Read("gloo-system", "gateway-proxy", clients.ReadOpts{})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		statusClient.SetStatus(proxy, &core.Status{State: core.Status_Accepted})

		_, err = proxyClient.Write(proxy, clients.WriteOpts{OverwriteExisting: true})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
	}

	It("should set status correctly even when the status from the snapshot was not updated", func() {
		ts.Sync(ctx, snapshot())
		// wait for proxy to be written
		Eventually(func() (*gloov1.Proxy, error) {
			return proxyClient.Read("gloo-system", "gateway-proxy", clients.ReadOpts{})
		}).ShouldNot(BeNil())

		// write the proxy status.
		AcceptProxy()
		ts, _ := ts.(*TranslatorSyncer)
		ts.UpdateProxies(ctx)
		// wait for the proxy status to be written in the VS
		EventuallyProxyStatusInVs().Should(Equal(core.Status_Accepted))

		// re-sync, so that the snapshot has the updated status.
		// the translator will cache the updated status.
		ts.Sync(ctx, snapshot())
		// Second round of updates:
		// update the VS but adding a route to it (anything will do here)
		vs, err := baseVirtualServiceClient.Read(vs.Metadata.Namespace, vs.Metadata.Name, clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())
		vs.VirtualHost.Routes = append(vs.VirtualHost.Routes, vs.VirtualHost.Routes[0])
		_, err = baseVirtualServiceClient.Write(vs, clients.WriteOpts{OverwriteExisting: true})
		Expect(err).NotTo(HaveOccurred())

		// re-sync to process the new VS
		ts.Sync(ctx, snapshot())
		ts.UpdateProxies(ctx)
		// wait for proxy status to become pending
		EventuallyProxyStatus().Should(Equal(core.Status_Pending))

		// wait for the status propagate
		EventuallyProxyStatusInVs().Should(Equal(core.Status_Pending))

		// write the proxy status again to the same status as the one currently in the snapshot
		AcceptProxy()

		ts.UpdateProxies(ctx)
		// status should be accepted.
		// this tests the bug that we saw where the status stayed pending.
		// the vs sub resource status did not update,
		// as the last status is the same as the one from Sync
		EventuallyProxyStatusInVs().Should(Equal(core.Status_Accepted))
	})

})

type delayingVsClient struct {
	v1.VirtualServiceClient
	SleepDuration time.Duration
}

func (d *delayingVsClient) Write(resource *v1.VirtualService, opts clients.WriteOpts) (*v1.VirtualService, error) {
	time.Sleep(d.SleepDuration)
	return d.VirtualServiceClient.Write(resource, opts)
}
