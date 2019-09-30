package syncer_test

import (
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"

	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	. "github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"github.com/solo-io/solo-kit/pkg/errors"
)

var _ = Describe("Translate Proxy", func() {

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
			Metadata: core.Metadata{
				Namespace: "gloo-system",
				Name:      translator.GatewayProxyName,
			},
		}

		c := &mockXdsCache{}
		rep := reporter.NewReporter(ref, proxyClient, upstreamClient)

		xdsHasher := &xds.ProxyKeyHasher{}
		s := NewTranslatorSyncer(&mockTranslator{true}, c, xdsHasher, rep, false, nil)
		snap := &v1.ApiSnapshot{
			Proxies: v1.ProxyList{
				proxy,
			},
		}
		err = s.Sync(context.Background(), snap)
		Expect(err).NotTo(HaveOccurred())

		proxies, err := proxyClient.List(proxy.GetMetadata().Namespace, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(proxies).To(HaveLen(1))
		Expect(proxies[0]).To(BeAssignableToTypeOf(&v1.Proxy{}))
		Expect(proxies[0].(*v1.Proxy).Status).To(Equal(core.Status{
			State:      2,
			Reason:     "1 error occurred:\n\t* hi, how ya doin'?\n\n",
			ReportedBy: ref,
		}))

		// NilSnapshot is always consistent, so snapshot will always be set as part of endpoints update
		Expect(c.called).To(BeTrue())

		// update rv for proxy
		p1, err := proxyClient.Read(proxy.Metadata.Namespace, proxy.Metadata.Name, clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())
		snap.Proxies[0] = p1.(*v1.Proxy)

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

func (t *mockTranslator) Translate(params plugins.Params, proxy *v1.Proxy) (envoycache.Snapshot, reporter.ResourceReports, *validation.ProxyReport, error) {
	if t.reportErrs {
		rpts := reporter.ResourceReports{}
		rpts.AddError(proxy, errors.Errorf("hi, how ya doin'?"))
		return envoycache.NilSnapshot{}, rpts, &validation.ProxyReport{}, nil
	}
	return envoycache.NilSnapshot{}, nil, &validation.ProxyReport{}, nil
}

var _ envoycache.SnapshotCache = &mockXdsCache{}

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

func (c *mockXdsCache) GetSnapshot(node string) (envoycache.Snapshot, error) {
	return &envoycache.NilSnapshot{}, nil
}

func (*mockXdsCache) ClearSnapshot(node string) {
	panic("implement me")
}
