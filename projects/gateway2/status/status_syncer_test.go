package status

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	"github.com/solo-io/gloo/projects/gateway2/translator/translatorutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

var _ = Describe("Status Syncer", func() {

	It("should queue proxy, handle report and clean up after report is processed", func() {
		syncer := NewStatusSyncerFactory()
		proxyOne := &v1.Proxy{
			Metadata: &core.Metadata{
				Name:      "proxy-one",
				Namespace: "test-namespace",
				Annotations: map[string]string{
					utils.ProxySyncId: "123",
				},
				Labels: map[string]string{
					utils.ProxyTypeKey: utils.GatewayApiProxyValue,
				},
			},
		}
		proxyOneNameNs := types.NamespacedName{
			Name:      proxyOne.Metadata.Name,
			Namespace: proxyOne.Metadata.Namespace,
		}

		proxyTwo := &v1.Proxy{
			Metadata: &core.Metadata{
				Name:      "proxy-two",
				Namespace: "test-namespace",
				Annotations: map[string]string{
					utils.ProxySyncId: "123",
				},
				Labels: map[string]string{
					utils.ProxyTypeKey: utils.GatewayApiProxyValue,
				},
			},
		}
		proxyTwoNameNs := types.NamespacedName{
			Name:      proxyTwo.Metadata.Name,
			Namespace: proxyTwo.Metadata.Namespace,
		}

		proxiesToQueue := v1.ProxyList{proxyOne, proxyTwo}
		pluginRegistry := &registry.PluginRegistry{}
		ctx := context.Background()

		// Test QueueStatusForProxies method
		syncer.QueueStatusForProxies(ctx, proxiesToQueue, pluginRegistry, 123)

		// Queue the proxy (this is invoked in the proxy syncer)
		proxiesMap := syncer.(*statusSyncerFactory).resyncsPerProxy
		Expect(proxiesMap[proxyOneNameNs]).To(Equal(123))
		Expect(proxiesMap[proxyTwoNameNs]).To(Equal(123))
		registryMap := syncer.(*statusSyncerFactory).registryPerSync
		Expect(registryMap[123]).To(Equal(pluginRegistry))

		// Handle the proxy reports only for proxy one (this is invoked as a callback in the envoy translator syncer)
		proxyOneWithReports := []translatorutils.ProxyWithReports{
			{
				Proxy: proxyOne,
				Reports: translatorutils.TranslationReports{
					ProxyReport:     &validation.ProxyReport{},
					ResourceReports: reporter.ResourceReports{},
				},
			},
		}
		syncer.HandleProxyReports(ctx, proxyOneWithReports)

		// Ensure proxy one has been removed from the queue after handling reports, but proxy two is still present
		proxiesMap = syncer.(*statusSyncerFactory).resyncsPerProxy
		Expect(proxiesMap).ToNot(ContainElement(proxyOneNameNs)) // proxy one should be removed
		Expect(proxiesMap[proxyTwoNameNs]).To(Equal(123))        // proxy two should still be in the map
		Expect(registryMap[123]).To(Equal(pluginRegistry))       // registry should still be in map

		// Handle the proxy reports only for proxy two
		proxyTwoWithReports := []translatorutils.ProxyWithReports{
			{
				Proxy: proxyOne,
				Reports: translatorutils.TranslationReports{
					ProxyReport:     &validation.ProxyReport{},
					ResourceReports: reporter.ResourceReports{},
				},
			},
		}
		syncer.HandleProxyReports(ctx, proxyTwoWithReports)

		// Ensure both proxies are removed from the queue after handling reports
		proxiesMap = syncer.(*statusSyncerFactory).resyncsPerProxy
		Expect(proxiesMap).ToNot(ContainElement(proxyOneNameNs))
		Expect(proxiesMap).ToNot(ContainElement(proxyTwoNameNs))
		registryMap = syncer.(*statusSyncerFactory).registryPerSync
		Expect(registryMap).ToNot(ContainElement(123))
	})

	It("Can handle multiple proxies in one HandleProxyReports call", func() {
		syncer := NewStatusSyncerFactory()
		proxyOne := &v1.Proxy{
			Metadata: &core.Metadata{
				Name:      "proxy-one",
				Namespace: "test-namespace",
				Annotations: map[string]string{
					utils.ProxySyncId: "123",
				},
				Labels: map[string]string{
					utils.ProxyTypeKey: utils.GatewayApiProxyValue,
				},
			},
		}
		proxyOneNameNs := types.NamespacedName{
			Name:      proxyOne.Metadata.Name,
			Namespace: proxyOne.Metadata.Namespace,
		}

		proxyTwo := &v1.Proxy{
			Metadata: &core.Metadata{
				Name:      "proxy-two",
				Namespace: "test-namespace",
				Annotations: map[string]string{
					utils.ProxySyncId: "123",
				},
				Labels: map[string]string{
					utils.ProxyTypeKey: utils.GatewayApiProxyValue,
				},
			},
		}
		proxyTwoNameNs := types.NamespacedName{
			Name:      proxyTwo.Metadata.Name,
			Namespace: proxyTwo.Metadata.Namespace,
		}

		proxiesToQueue := v1.ProxyList{proxyOne, proxyTwo}
		pluginRegistry := &registry.PluginRegistry{}
		ctx := context.Background()

		// Test QueueStatusForProxies method
		syncer.QueueStatusForProxies(ctx, proxiesToQueue, pluginRegistry, 123)

		// Queue the proxy (this is invoked in the proxy syncer)
		proxiesMap := syncer.(*statusSyncerFactory).resyncsPerProxy
		Expect(proxiesMap[proxyOneNameNs]).To(Equal(123))
		Expect(proxiesMap[proxyTwoNameNs]).To(Equal(123))
		registryMap := syncer.(*statusSyncerFactory).registryPerSync
		Expect(registryMap[123]).To(Equal(pluginRegistry))

		// Handle multiple proxy reports
		proxiesWithReports := []translatorutils.ProxyWithReports{
			{
				Proxy: proxyOne,
				Reports: translatorutils.TranslationReports{
					ProxyReport:     &validation.ProxyReport{},
					ResourceReports: reporter.ResourceReports{},
				},
			},
			{
				Proxy: proxyTwo,
				Reports: translatorutils.TranslationReports{
					ProxyReport:     &validation.ProxyReport{},
					ResourceReports: reporter.ResourceReports{},
				},
			},
		}
		syncer.HandleProxyReports(ctx, proxiesWithReports)

		// Ensure both proxies are removed from the queue after handling reports
		proxiesMap = syncer.(*statusSyncerFactory).resyncsPerProxy
		Expect(proxiesMap).ToNot(ContainElement(proxyOneNameNs))
		Expect(proxiesMap).ToNot(ContainElement(proxyTwoNameNs))
		registryMap = syncer.(*statusSyncerFactory).registryPerSync
		Expect(registryMap).ToNot(ContainElement(123))
	})

	It("should only queue and process the most recent proxy", func() {
		syncer := NewStatusSyncerFactory()
		oldestProxy := &v1.Proxy{
			Metadata: &core.Metadata{
				Name:      "test-proxy",
				Namespace: "test-namespace",
				Annotations: map[string]string{
					utils.ProxySyncId: "123",
				},
				Labels: map[string]string{
					utils.ProxyTypeKey: utils.GatewayApiProxyValue,
				},
			},
		}
		oldProxy := &v1.Proxy{
			Metadata: &core.Metadata{
				Name:      oldestProxy.GetMetadata().GetName(),
				Namespace: oldestProxy.GetMetadata().GetNamespace(),
				Annotations: map[string]string{
					utils.ProxySyncId: "124",
				},
				Labels: map[string]string{
					utils.ProxyTypeKey: utils.GatewayApiProxyValue,
				},
			},
		}
		newProxy := &v1.Proxy{
			Metadata: &core.Metadata{
				Name:      oldestProxy.GetMetadata().GetName(),
				Namespace: oldestProxy.GetMetadata().GetNamespace(),
				Annotations: map[string]string{
					utils.ProxySyncId: "125",
				},
				Labels: map[string]string{
					utils.ProxyTypeKey: utils.GatewayApiProxyValue,
				},
			},
		}
		proxyNameNs := types.NamespacedName{
			Name:      oldestProxy.Metadata.Name,
			Namespace: oldestProxy.Metadata.Namespace,
		}

		proxiesToQueue123 := v1.ProxyList{oldestProxy}
		pluginRegistry123 := &registry.PluginRegistry{}

		proxiesToQueue124 := v1.ProxyList{oldProxy}
		pluginRegistry124 := &registry.PluginRegistry{}

		proxiesToQueue125 := v1.ProxyList{newProxy}
		pluginRegistry125 := &registry.PluginRegistry{}
		ctx := context.Background()

		// Each proxy is queued with a different registry per sync iteration
		syncer.QueueStatusForProxies(ctx, proxiesToQueue123, pluginRegistry123, 123)
		syncer.QueueStatusForProxies(ctx, proxiesToQueue124, pluginRegistry124, 124)
		syncer.QueueStatusForProxies(ctx, proxiesToQueue125, pluginRegistry125, 125)

		// Queue the proxy (this is invoked in the proxy syncer)
		proxiesMap := syncer.(*statusSyncerFactory).resyncsPerProxy
		Expect(proxiesMap[proxyNameNs]).To(Equal(125))
		registryMap := syncer.(*statusSyncerFactory).registryPerSync
		Expect(registryMap[125]).To(Equal(pluginRegistry125))

		// Handle the proxy reports (this is invoked as a callback in the envoy translator syncer)
		// Note: the proxies can be skipped, but should not be able to come in out of order (dpanic will be hit if order is wrong)
		oldProxiesWithReports := []translatorutils.ProxyWithReports{
			{
				Proxy: oldestProxy,
				Reports: translatorutils.TranslationReports{
					ProxyReport:     &validation.ProxyReport{},
					ResourceReports: reporter.ResourceReports{},
				},
			},
		}
		syncer.HandleProxyReports(ctx, oldProxiesWithReports)

		// Ensure only the latest proxy is still present
		proxiesMap = syncer.(*statusSyncerFactory).resyncsPerProxy
		Expect(proxiesMap[proxyNameNs]).To(Equal(125))
		registryMap = syncer.(*statusSyncerFactory).registryPerSync
		// ensure registry is cleared for all sync iterations
		Expect(registryMap).To(And(HaveKey(123), HaveKey(124), HaveKey(125)))

		newProxiesWithReports := []translatorutils.ProxyWithReports{
			{
				Proxy: newProxy,
				Reports: translatorutils.TranslationReports{
					ProxyReport:     &validation.ProxyReport{},
					ResourceReports: reporter.ResourceReports{},
				},
			},
		}
		syncer.HandleProxyReports(ctx, newProxiesWithReports)

		// Ensure the proxy has been removed from the queue after handling reports
		proxiesMap = syncer.(*statusSyncerFactory).resyncsPerProxy
		// ensure all proxies are removed from the queue
		Expect(proxiesMap).To(BeEmpty())
		registryMap = syncer.(*statusSyncerFactory).registryPerSync
		Expect(registryMap).ToNot(BeEmpty())
		// ensure registry is only cleared for processed sync iteration
		Expect(registryMap).To(And(HaveKey(123), HaveKey(124)))
	})
})
