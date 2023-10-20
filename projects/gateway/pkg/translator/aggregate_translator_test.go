package translator_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	. "github.com/solo-io/gloo/projects/gateway/pkg/translator"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/test/samples"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var _ = Describe("Aggregate translator", func() {
	var (
		ctx = context.TODO()

		snap    *gloov1snap.ApiSnapshot
		proxy   *gloov1.Proxy
		reports reporter.ResourceReports
		ns      = "namespace"
	)

	genProxyWithTranslatorOpts := func(opts Opts) {
		tx := NewDefaultTranslator(opts)
		proxy, reports = tx.Translate(ctx, "proxy-name", snap, snap.Gateways)
	}

	genProxyWithIsolatedVirtualHosts := func() {
		genProxyWithTranslatorOpts(Opts{
			WriteNamespace:                 ns,
			IsolateVirtualHostsBySslConfig: true,
		})
	}

	BeforeEach(func() {
		snap = samples.SimpleGlooSnapshot(ns)
	})

	It("Computes listener idempotently when provided different ssl configs", func() {
		gw1 := snap.Gateways[1]
		gw := gw1.GetHttpGateway()
		gw.VirtualServiceExpressions = nil
		gw.VirtualServiceSelector = nil
		gw.VirtualServices = append(gw.VirtualServices, &core.ResourceRef{
			Name:      "ssl-vs-0",
			Namespace: ns,
		}, &core.ResourceRef{
			Name:      "ssl-vs-1",
			Namespace: ns,
		}, &core.ResourceRef{
			Name:      "ssl-vs-2",
			Namespace: ns,
		}, &core.ResourceRef{
			Name:      "ssl-vs-3",
			Namespace: ns,
		}, &core.ResourceRef{
			Name:      "ssl-vs-4",
			Namespace: ns,
		})
		snap.Gateways = v1.GatewayList{gw1}

		snap.VirtualServices = append(snap.VirtualServices, &v1.VirtualService{
			VirtualHost: &v1.VirtualHost{},
			SslConfig: &ssl.SslConfig{
				SniDomains: []string{"sni-0"},
				// We have to add some other config since we merge configs where the only
				// difference is the SniDomains
				TransportSocketConnectTimeout: &durationpb.Duration{Seconds: 0},
			},
			DisplayName: "ssl-vs-0",
			Metadata: &core.Metadata{
				Name:      "ssl-vs-0",
				Namespace: ns,
			},
		}, &v1.VirtualService{
			VirtualHost: &v1.VirtualHost{},
			SslConfig: &ssl.SslConfig{
				SniDomains: []string{"sni-1"},
				// We have to add some other config since we merge configs where the only
				// difference is the SniDomains
				TransportSocketConnectTimeout: &durationpb.Duration{Seconds: 1},
			},
			DisplayName: "ssl-vs-1",
			Metadata: &core.Metadata{
				Name:      "ssl-vs-1",
				Namespace: ns,
			},
		}, &v1.VirtualService{
			VirtualHost: &v1.VirtualHost{},
			SslConfig: &ssl.SslConfig{
				SniDomains: []string{"sni-2"},
				// We have to add some other config since we merge configs where the only
				// difference is the SniDomains
				TransportSocketConnectTimeout: &durationpb.Duration{Seconds: 2},
			},
			DisplayName: "ssl-vs-2",
			Metadata: &core.Metadata{
				Name:      "ssl-vs-2",
				Namespace: ns,
			},
		}, &v1.VirtualService{
			VirtualHost: &v1.VirtualHost{},
			SslConfig: &ssl.SslConfig{
				SniDomains: []string{"sni-3"},
				// We have to add some other config since we merge configs where the only
				// difference is the SniDomains
				TransportSocketConnectTimeout: &durationpb.Duration{Seconds: 3},
			},
			DisplayName: "ssl-vs-3",
			Metadata: &core.Metadata{
				Name:      "ssl-vs-3",
				Namespace: ns,
			},
		}, &v1.VirtualService{
			VirtualHost: &v1.VirtualHost{},
			SslConfig: &ssl.SslConfig{
				SniDomains: []string{"sni-4"},
				// We have to add some other config since we merge configs where the only
				// difference is the SniDomains
				TransportSocketConnectTimeout: &durationpb.Duration{Seconds: 4},
			},
			DisplayName: "ssl-vs-4",
			Metadata: &core.Metadata{
				Name:      "ssl-vs-4",
				Namespace: ns,
			},
		})
		genProxyWithIsolatedVirtualHosts()
		proxyName := proxy.Metadata.Name
		aggregateTranslator := &AggregateTranslator{VirtualServiceTranslator: &VirtualServiceTranslator{}}
		// run 100 times to ensure idempotency
		// not sure if 100 times is valid; in anecdotal testing it tended to fail in under 20
		var originalOrder, currentOrder string
		for i := 0; i < 100; i++ {
			l := aggregateTranslator.ComputeListener(NewTranslatorParams(ctx, snap, reports), proxyName, snap.Gateways[0])
			Expect(l).NotTo(BeNil())
			Expect(l.GetAggregateListener())

			currentOrder = ""
			currentOrder += l.GetAggregateListener().HttpFilterChains[0].GetMatcher().GetSslConfig().GetSniDomains()[0]
			currentOrder += l.GetAggregateListener().HttpFilterChains[1].GetMatcher().GetSslConfig().GetSniDomains()[0]
			currentOrder += l.GetAggregateListener().HttpFilterChains[2].GetMatcher().GetSslConfig().GetSniDomains()[0]
			currentOrder += l.GetAggregateListener().HttpFilterChains[3].GetMatcher().GetSslConfig().GetSniDomains()[0]
			currentOrder += l.GetAggregateListener().HttpFilterChains[4].GetMatcher().GetSslConfig().GetSniDomains()[0]

			if originalOrder == "" {
				originalOrder = currentOrder
				// ensure that all sni domains (sni-1 through sni-5) are present though we do not care what order the hasher has output them in at least the first time.
				for i := 0; i < 5; i++ {
					Expect(originalOrder).To(ContainSubstring(fmt.Sprintf("sni-%d", i)))
				}
			}

			// demand that the order is deterministic
			Expect(currentOrder).To(Equal(originalOrder))
		}
	})

	Context("No virtual Services", func() {
		JustBeforeEach(func() {
			snap.VirtualServices = []*v1.VirtualService{}
		})

		It("Does not generate a listener", func() {
			aggregateTranslator := &AggregateTranslator{VirtualServiceTranslator: &VirtualServiceTranslator{}}
			genProxyWithIsolatedVirtualHosts()
			proxyName := proxy.Metadata.Name
			l := aggregateTranslator.ComputeListener(NewTranslatorParams(ctx, snap, reports), proxyName, snap.Gateways[0])
			Expect(l).To(BeNil())
			Expect(reports.ValidateStrict()).NotTo(HaveOccurred())
		})

		It("Does generates a listener if TranslateEmptyGateways is set", func() {
			ctx := settingsutil.WithSettings(ctx, &gloov1.Settings{
				Gateway: &gloov1.GatewayOptions{
					TranslateEmptyGateways: &wrapperspb.BoolValue{
						Value: true,
					},
				},
			})
			aggregateTranslator := &AggregateTranslator{VirtualServiceTranslator: &VirtualServiceTranslator{}}
			genProxyWithIsolatedVirtualHosts()
			proxyName := proxy.Metadata.Name
			l := aggregateTranslator.ComputeListener(NewTranslatorParams(ctx, snap, reports), proxyName, snap.Gateways[0])
			Expect(l).NotTo(BeNil())
			Expect(l.GetAggregateListener())
			Expect(reports.ValidateStrict()).NotTo(HaveOccurred())
		})
	})
})
