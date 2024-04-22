package status

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	"github.com/solo-io/gloo/projects/gateway2/translator/translatorutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"k8s.io/apimachinery/pkg/types"
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
			},
		}
		proxyTwoNameNs := types.NamespacedName{
			Name:      proxyTwo.Metadata.Name,
			Namespace: proxyTwo.Metadata.Namespace,
		}

		proxiesToQueue := v1.ProxyList{proxyOne, proxyTwo}
		pluginRegistry := &registry.PluginRegistry{}

		// Test QueueStatusForProxies method
		syncer.QueueStatusForProxies(proxiesToQueue, pluginRegistry)

		// Queue the proxy (this is invoked in the proxy syncer)
		// Access private field proxiesPerRegistry
		proxiesMap := syncer.(*statusSyncerFactory).proxiesPerRegistry[pluginRegistry]
		Expect(proxiesMap[proxyOneNameNs]).To(Equal(123))
		Expect(proxiesMap[proxyTwoNameNs]).To(Equal(123))

		// Handle the proxy reports only for proxy one (this is invoked as a callback in the envoy translator syncer)
		ctx := context.Background()
		proxiesWithReports := []translatorutils.ProxyWithReports{
			{
				Proxy: proxyOne,
				Reports: translatorutils.TranslationReports{
					ProxyReport:     &validation.ProxyReport{},
					ResourceReports: reporter.ResourceReports{},
				},
			},
		}
		syncer.HandleProxyReports(ctx, proxiesWithReports)

		// Ensure proxy one has been removed from the queue after handling reports, but proxy two is still present
		proxiesMap = syncer.(*statusSyncerFactory).proxiesPerRegistry[pluginRegistry]
		Expect(proxiesMap).ToNot(ContainElement(proxyOneNameNs)) // proxy one should be removed
		Expect(proxiesMap[proxyTwoNameNs]).To(Equal(123))        // proxy two should still be in the map
	})

	It("Can handle multiple proxies in one call", func() {
		syncer := NewStatusSyncerFactory()
		proxyOne := &v1.Proxy{
			Metadata: &core.Metadata{
				Name:      "proxy-one",
				Namespace: "test-namespace",
				Annotations: map[string]string{
					utils.ProxySyncId: "123",
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
			},
		}
		proxyTwoNameNs := types.NamespacedName{
			Name:      proxyTwo.Metadata.Name,
			Namespace: proxyTwo.Metadata.Namespace,
		}

		proxiesToQueue := v1.ProxyList{proxyOne, proxyTwo}
		pluginRegistry := &registry.PluginRegistry{}

		// Test QueueStatusForProxies method
		syncer.QueueStatusForProxies(proxiesToQueue, pluginRegistry)

		// Queue the proxy (this is invoked in the proxy syncer)
		// Access private field proxiesPerRegistry
		proxiesMap := syncer.(*statusSyncerFactory).proxiesPerRegistry[pluginRegistry]
		Expect(proxiesMap[proxyOneNameNs]).To(Equal(123))
		Expect(proxiesMap[proxyTwoNameNs]).To(Equal(123))

		// Handle the proxy reports only for proxy one (this is invoked as a callback in the envoy translator syncer)
		ctx := context.Background()
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

		// Ensure proxy one has been removed from the queue after handling reports, but proxy two is still present
		proxiesMap = syncer.(*statusSyncerFactory).proxiesPerRegistry[pluginRegistry]
		Expect(proxiesMap).ToNot(ContainElement(proxyOneNameNs))
		Expect(proxiesMap).ToNot(ContainElement(proxyTwoNameNs))
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
			},
		}
		oldProxy := &v1.Proxy{
			Metadata: &core.Metadata{
				Name:      oldestProxy.GetMetadata().GetName(),
				Namespace: oldestProxy.GetMetadata().GetNamespace(),
				Annotations: map[string]string{
					utils.ProxySyncId: "124",
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
			},
		}
		proxyNameNs := types.NamespacedName{
			Name:      oldestProxy.Metadata.Name,
			Namespace: oldestProxy.Metadata.Namespace,
		}

		proxiesToQueue := v1.ProxyList{oldestProxy, newProxy, oldProxy}
		pluginRegistry := &registry.PluginRegistry{}

		// Test QueueStatusForProxies method
		syncer.QueueStatusForProxies(proxiesToQueue, pluginRegistry)

		// Queue the proxy (this is invoked in the proxy syncer)
		// Access private field proxiesPerRegistry
		proxiesMap := syncer.(*statusSyncerFactory).proxiesPerRegistry[pluginRegistry]
		Expect(proxiesMap[proxyNameNs]).To(Equal(125))

		// Handle the proxy reports (this is invoked as a callback in the envoy translator syncer)
		ctx := context.Background()
		proxiesWithReports := []translatorutils.ProxyWithReports{
			{
				Proxy: oldestProxy,
				Reports: translatorutils.TranslationReports{
					ProxyReport:     &validation.ProxyReport{},
					ResourceReports: reporter.ResourceReports{},
				},
			},
			{
				Proxy: newProxy,
				Reports: translatorutils.TranslationReports{
					ProxyReport:     &validation.ProxyReport{},
					ResourceReports: reporter.ResourceReports{},
				},
			},
			{
				Proxy: oldProxy,
				Reports: translatorutils.TranslationReports{
					ProxyReport:     &validation.ProxyReport{},
					ResourceReports: reporter.ResourceReports{},
				},
			},
		}
		syncer.HandleProxyReports(ctx, proxiesWithReports)

		// Ensure the proxy has been removed from the queue after handling reports
		proxiesMap = syncer.(*statusSyncerFactory).proxiesPerRegistry[pluginRegistry]
		Expect(proxiesMap).ToNot(ContainElement(proxyNameNs))
	})
})
