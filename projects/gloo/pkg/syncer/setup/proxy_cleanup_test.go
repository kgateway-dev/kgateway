package setup

import (
	"context"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Clean up proxies", func() {
	var (
		proxyClient          v1.ProxyClient
		ctx                  context.Context
		settings             *v1.Settings
		managedProxyLabels   map[string]string
		unmanagedProxyLabels map[string]string
		gatewayProxy         *v1.Proxy
	)
	BeforeEach(func() {
		resourceClientFactory := &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}

		settings = &v1.Settings{
			ConfigSource: &v1.Settings_KubernetesConfigSource{
				KubernetesConfigSource: &v1.Settings_KubernetesCrds{},
			},
			Gateway: &v1.GatewayOptions{
				EnableGatewayController: &wrappers.BoolValue{Value: true},
				PersistProxySpec:        &wrappers.BoolValue{Value: false},
			},
		}
		managedProxyLabels = map[string]string{
			"created_by": "gloo-gateway-translator",
		}
		unmanagedProxyLabels = map[string]string{
			"created_by": "other-controller",
		}
		gatewayProxy = &v1.Proxy{
			Metadata: &core.Metadata{
				Name:      "test-proxy",
				Namespace: defaults.GlooSystem,
				Labels:    managedProxyLabels,
			},
		}
		proxyClient, _ = v1.NewProxyClient(ctx, resourceClientFactory)
		ctx = context.TODO()
	})
	It("Deletes proxies with the gateway label and leaves other proxies", func() {
		otherProxy := &v1.Proxy{
			Metadata: &core.Metadata{
				Name:      "test-proxy2",
				Namespace: defaults.GlooSystem,
				Labels:    unmanagedProxyLabels,
			},
		}
		proxyClient.Write(otherProxy, clients.WriteOpts{})
		proxyClient.Write(gatewayProxy, clients.WriteOpts{})
		deleteUnusedProxies(ctx, defaults.GlooSystem, proxyClient)
		remainingProxies, _ := proxyClient.List(defaults.GlooSystem, clients.ListOpts{})
		Expect(len(remainingProxies)).To(Equal(1))
		Expect(remainingProxies[0].GetMetadata().Ref()).To(Equal(otherProxy.GetMetadata().Ref()))
		otherProxy.Metadata.Labels = unmanagedProxyLabels
	})
	It("Does not delete proxies when persisting proxies is enabled", func() {
		proxyClient.Write(gatewayProxy, clients.WriteOpts{})
		settings.Gateway.PersistProxySpec = &wrappers.BoolValue{Value: true}
		params := bootstrap.NewConfigFactoryParams(
			settings,
			nil,
			nil,
			nil,
			nil,
		)
		err := doProxyCleanup(ctx, params, settings, defaults.GlooSystem)
		Expect(err).NotTo(HaveOccurred())
		remainingProxies, _ := proxyClient.List(defaults.GlooSystem, clients.ListOpts{})
		Expect(len(remainingProxies)).To(Equal(1))
	})
})
