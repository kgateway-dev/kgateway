package helpers

import (
	"context"

	"github.com/hashicorp/consul/api"
	api2 "github.com/hashicorp/vault/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/consul"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/vault"
)

var _ = Describe("Clients", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
	})

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
				MustProxyClient(ctx),
				MustSecretClient(ctx),
				MustSettingsClient(ctx),
				MustUpstreamClient(ctx),
				MustUpstreamGroupClient(ctx),
				MustVirtualServiceClient(ctx),
			} {
				Expect(client.BaseClient()).To(BeAssignableToTypeOf(&memory.ResourceClient{}))
			}
		})
	})
	Describe("UseConsulClients", func() {
		BeforeEach(func() {
			UseConsulClients(&api.Client{}, "", &api.QueryOptions{})
		})
		AfterEach(func() {
			UseDefaultClients()
		})
		It("returns Consul-based config clients", func() {

			type BaseClientGetter interface {
				BaseClient() clients.ResourceClient
			}
			for _, client := range []BaseClientGetter{
				MustProxyClient(ctx),
				MustSettingsClient(ctx),
				MustUpstreamClient(ctx),
				MustUpstreamGroupClient(ctx),
				MustVirtualServiceClient(ctx),
			} {
				Expect(client.BaseClient()).To(BeAssignableToTypeOf(&consul.ResourceClient{}))
			}
		})
	})
	Describe("UseVaultClients", func() {
		BeforeEach(func() {
			UseVaultClients(&api2.Client{}, "", "")
		})
		AfterEach(func() {
			UseDefaultClients()
		})
		It("returns Consul-based secret clients", func() {

			type BaseClientGetter interface {
				BaseClient() clients.ResourceClient
			}
			for _, client := range []BaseClientGetter{
				MustSecretClient(ctx),
			} {
				baseClient := client.BaseClient()
				Expect(baseClient).To(BeAssignableToTypeOf(&vault.ResourceClient{}))
			}
		})
	})
})
