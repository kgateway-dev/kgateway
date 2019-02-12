package translator_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gateway/pkg/translator"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const ns = "gloo-system"

var _ = Describe("Translator", func() {
	var (
		snap *v1.ApiSnapshot
	)
	BeforeEach(func() {
		snap = &v1.ApiSnapshot{
			Gateways: v1.GatewaysByNamespace{
				ns: v1.GatewayList{
					{
						Metadata: core.Metadata{Namespace: ns, Name: "name"},
						BindPort: 2,
					},
				},
			},
			VirtualServices: v1.VirtualServicesByNamespace{
				ns: v1.VirtualServiceList{
					{
						Metadata: core.Metadata{Namespace: ns, Name: "name"},
						VirtualHost: &gloov1.VirtualHost{
							Domains: []string{"d1.com"},
						},
					},
					{
						Metadata: core.Metadata{Namespace: ns, Name: "name2"},
						VirtualHost: &gloov1.VirtualHost{
							Domains: []string{"d2.com"},
						},
					},
				},
			},
		}
	})

	It("should translate proxy with default name", func() {

		proxy, errs := Translate(context.Background(), ns, snap)

		Expect(errs).To(HaveLen(3))
		Expect(errs.Validate()).NotTo(HaveOccurred())
		Expect(proxy.Metadata.Name).To(Equal(GatewayProxyName))
		Expect(proxy.Metadata.Namespace).To(Equal(ns))
	})

	It("should translate an empty gateway to have all vservices", func() {

		proxy, _ := Translate(context.Background(), ns, snap)

		Expect(proxy.Listeners).To(HaveLen(1))
		listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener
		Expect(listener.VirtualHosts).To(HaveLen(2))
	})

	It("should translate an gateway to only have its vservices", func() {
		snap.Gateways[ns][0].VirtualServices = []core.ResourceRef{snap.VirtualServices[ns][0].Metadata.Ref()}

		proxy, errs := Translate(context.Background(), ns, snap)

		Expect(errs.Validate()).NotTo(HaveOccurred())
		Expect(proxy).NotTo(BeNil())
		Expect(proxy.Listeners).To(HaveLen(1))
		listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener
		Expect(listener.VirtualHosts).To(HaveLen(1))
	})

	It("should translate two gateways with to one proxy with the same name", func() {
		snap.Gateways[ns] = append(snap.Gateways[ns], &v1.Gateway{Metadata: core.Metadata{Namespace: ns, Name: "name"}})

		proxy, errs := Translate(context.Background(), ns, snap)

		Expect(errs.Validate()).NotTo(HaveOccurred())
		Expect(proxy.Metadata.Name).To(Equal(GatewayProxyName))
		Expect(proxy.Metadata.Namespace).To(Equal(ns))
		Expect(proxy.Listeners).To(HaveLen(2))
	})

	Context("merge", func() {
		BeforeEach(func() {
			snap.VirtualServices[ns][1].VirtualHost.Domains = snap.VirtualServices[ns][0].VirtualHost.Domains
		})

		It("should translate 2 virtual services with the same domains to 1 virtual service", func() {

			proxy, errs := Translate(context.Background(), ns, snap)

			Expect(errs.Validate()).NotTo(HaveOccurred())
			Expect(proxy.Metadata.Name).To(Equal(GatewayProxyName))
			Expect(proxy.Metadata.Namespace).To(Equal(ns))
			Expect(proxy.Listeners).To(HaveLen(1))
			listener := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener
			Expect(listener.VirtualHosts).To(HaveLen(1))
		})

		It("should not error with one contains plugins", func() {
			snap.VirtualServices[ns][0].VirtualHost.VirtualHostPlugins = new(gloov1.VirtualHostPlugins)

			_, errs := Translate(context.Background(), ns, snap)

			Expect(errs.Validate()).NotTo(HaveOccurred())
		})

		It("should error with both having plugins", func() {
			snap.VirtualServices[ns][0].VirtualHost.VirtualHostPlugins = new(gloov1.VirtualHostPlugins)
			snap.VirtualServices[ns][1].VirtualHost.VirtualHostPlugins = new(gloov1.VirtualHostPlugins)

			_, errs := Translate(context.Background(), ns, snap)

			Expect(errs.Validate()).To(HaveOccurred())
		})

		It("should not error with one contains ssl config", func() {
			snap.VirtualServices[ns][0].SslConfig = new(gloov1.SslConfig)

			_, errs := Translate(context.Background(), ns, snap)

			Expect(errs.Validate()).NotTo(HaveOccurred())
		})

		It("should error with both having ssl config", func() {
			snap.VirtualServices[ns][0].SslConfig = new(gloov1.SslConfig)
			snap.VirtualServices[ns][1].SslConfig = new(gloov1.SslConfig)

			_, errs := Translate(context.Background(), ns, snap)

			Expect(errs.Validate()).To(HaveOccurred())
		})

	})

})
