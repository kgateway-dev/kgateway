package syncer_test

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"

	"context"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	. "github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/reporter"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/test/helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("GraphQLSyncer", func() {
	It("writes the reports the translator spits out and calls SetSnapshot on the cache", func() {
		ref := "syncer-test"
		resourceClientFactory := &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}
		proxyClient, err := resourceClientFactory.NewResourceClient(factory.NewResourceClientParams{ResourceType: &v1.Proxy{}})
		Expect(err).NotTo(HaveOccurred())

		upstreamClient, err := resourceClientFactory.NewResourceClient(factory.NewResourceClientParams{ResourceType: &v1.Upstream{}})
		Expect(err).NotTo(HaveOccurred())

		proxy := &v1.Proxy{
			Metadata: helpers.NewRandomMetadata(),
		}

		c := &mockXdsCache{}
		rep := reporter.NewReporter(ref, proxyClient, upstreamClient)

		xdsHasher := &xds.ProxyKeyHasher{}
		s := NewTranslatorSyncer(&mockTranslator{true}, c, xdsHasher, rep, false, nil)
		snap := &v1.ApiSnapshot{
			Proxies: map[string]v1.ProxyList{
				"": []*v1.Proxy{
					proxy,
				}},
		}
		err = s.Sync(context.Background(), snap)
		Expect(err).NotTo(HaveOccurred())

		proxies, err := proxyClient.List(proxy.GetMetadata().Namespace, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(proxies).To(HaveLen(1))
		Expect(proxies[0]).To(BeAssignableToTypeOf(&v1.Proxy{}))
		Expect(proxies[0].(*v1.Proxy).Status).To(Equal(core.Status{
			State:      2,
			Reason:     "hi, how ya doin'?",
			ReportedBy: ref,
		}))

		Expect(c.called).To(BeFalse())

		// update rv for proxy
		p1, err := proxyClient.Read(proxy.Metadata.Namespace, proxy.Metadata.Name, clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())
		snap.Proxies[""][0] = p1.(*v1.Proxy)

		s = NewTranslatorSyncer(&mockTranslator{false}, c, xdsHasher, rep, false, nil)
		err = s.Sync(context.Background(), snap)
		Expect(err).NotTo(HaveOccurred())

		proxies, err = proxyClient.List(proxy.GetMetadata().Namespace, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(proxies).To(HaveLen(1))
		Expect(proxies[0]).To(BeAssignableToTypeOf(&v1.Proxy{}))
		Expect(proxies[0].(*v1.Proxy).Status).To(Equal(core.Status{
			State:      1,
			ReportedBy: ref,
		}))

		Expect(c.called).To(BeTrue())
	})
})

type mockTranslator struct {
	reportErrs bool
}

func (t *mockTranslator) Translate(params plugins.Params, proxy *v1.Proxy) (envoycache.Snapshot, reporter.ResourceErrors, error) {
	if t.reportErrs {
		return envoycache.NilSnapshot{}, reporter.ResourceErrors{proxy: errors.Errorf("hi, how ya doin'?")}, nil
	}
	return envoycache.NilSnapshot{}, nil, nil
}

type mockXdsCache struct {
	called bool
}

func (*mockXdsCache) CreateWatch(envoycache.Request) (value chan envoycache.Response, cancel func()) {
	panic("implement me")
}

func (*mockXdsCache) Fetch(context.Context, envoycache.Request) (*envoycache.Response, error) {
	panic("implement me")
}

func (*mockXdsCache) GetStatusInfo(string) envoycache.StatusInfo {
	panic("implement me")
}

func (*mockXdsCache) GetStatusKeys() []string {
	panic("implement me")
}

func (c *mockXdsCache) SetSnapshot(node string, snapshot envoycache.Snapshot) error {
	c.called = true
	return nil
}

func (*mockXdsCache) ClearSnapshot(node string) {
	panic("implement me")
}
