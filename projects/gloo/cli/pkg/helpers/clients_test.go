package helpers

import (
	"github.com/hashicorp/consul/api"
	api2 "github.com/hashicorp/vault/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/consul"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/vault"
)

var _ = Describe("Clients", func() {
	Describe("UseMemoryClients", func() {
		BeforeEach(func() {
			UseMemoryClients()
		})
		AfterEach(func() {
			UseDefaultClients()
		})
		It("returns Memory-based resource clients", func() {

			type BaseClientGetter interface {
				BaseClient() clients.ResourceClient
			}
			for _, client := range []BaseClientGetter{
				MustProxyClient(),
				MustSecretClient(),
				MustSettingsClient(),
				MustUpstreamClient(),
				MustUpstreamGroupClient(),
				MustVirtualServiceClient(),
			} {
				Expect(client.BaseClient()).To(BeAssignableToTypeOf(&memory.ResourceClient{}))
			}
		})
	})
	Describe("UseConsulClients", func() {
		BeforeEach(func() {
			UseConsulClients(&api.Client{}, "")
		})
		AfterEach(func() {
			UseDefaultClients()
		})
		It("returns Consul-based config clients", func() {

			type BaseClientGetter interface {
				BaseClient() clients.ResourceClient
			}
			for _, client := range []BaseClientGetter{
				MustProxyClient(),
				MustSettingsClient(),
				MustUpstreamClient(),
				MustUpstreamGroupClient(),
				MustVirtualServiceClient(),
			} {
				Expect(client.BaseClient()).To(BeAssignableToTypeOf(&consul.ResourceClient{}))
			}
		})
	})
	Describe("UseVaultClients", func() {
		BeforeEach(func() {
			UseVaultClients(&api2.Client{}, "")
		})
		AfterEach(func() {
			UseDefaultClients()
		})
		It("returns Consul-based secret clients", func() {

			type BaseClientGetter interface {
				BaseClient() clients.ResourceClient
			}
			for _, client := range []BaseClientGetter{
				MustSecretClient(),
			} {
				Expect(client.BaseClient()).To(BeAssignableToTypeOf(&vault.ResourceClient{}))
			}
		})
	})
})
