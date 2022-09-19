package translator_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/durationpb"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	. "github.com/solo-io/gloo/projects/gateway/pkg/translator"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/test/samples"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var _ = FDescribe("Aggregate translator", func() {
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
			Name:      "ssl-vs",
			Namespace: ns,
		}, &core.ResourceRef{
			Name:      "different-ssl-vs",
			Namespace: ns,
		})
		snap.Gateways = v1.GatewayList{gw1}

		snap.VirtualServices = append(snap.VirtualServices, &v1.VirtualService{
			VirtualHost: &v1.VirtualHost{},
			SslConfig: &gloov1.SslConfig{
				SniDomains: []string{"sni"},
				// We have to add some other config since we merge configs where the only
				// difference is the SniDomains
				TransportSocketConnectTimeout: &durationpb.Duration{Seconds: 1},
			},
			DisplayName: "ssl-vs",
			Metadata: &core.Metadata{
				Name:      "ssl-vs",
				Namespace: ns,
			},
		}, &v1.VirtualService{
			VirtualHost: &v1.VirtualHost{},
			SslConfig: &gloov1.SslConfig{
				SniDomains: []string{"different-sni"},
			},
			DisplayName: "different-ssl-vs",
			Metadata: &core.Metadata{
				Name:      "different-ssl-vs",
				Namespace: ns,
			},
		})
		genProxyWithIsolatedVirtualHosts()
		proxyName := proxy.Metadata.Name
		aggregateTranslator := &AggregateTranslator{VirtualServiceTranslator: &VirtualServiceTranslator{}}
		// run 100 times to ensure idempotency
		// not sure if 100 times is valid; in anecdotal testing it tended to fail in under 20
		for i := 0; i < 100; i++ {
			l := aggregateTranslator.ComputeListener(NewTranslatorParams(ctx, snap, reports), proxyName, snap.Gateways[0])
			Expect(l).NotTo(BeNil())
			Expect(l.GetAggregateListener().HttpFilterChains[0].GetMatcher().GetSslConfig().GetSniDomains()[0]).To(Equal("sni"))
		}
	})
})
