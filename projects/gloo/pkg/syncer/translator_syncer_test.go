package syncer_test

import (
	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
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

var _ = FDescribe("Translate Proxy", func() {

	var (
		xdsCache    *mockXdsCache
		sanitizer   *mockXdsSanitizer
		syncer      v1.ApiSyncer
		snap        *v1.ApiSnapshot
		settings    *v1.Settings
		proxyClient v1.ProxyClient
		proxyName   = "proxy-name"
		ref         = "syncer-test"
		ns          = "any-ns"
	)

	BeforeEach(func() {
		xdsCache = &mockXdsCache{}
		sanitizer = &mockXdsSanitizer{}

		resourceClientFactory := &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}

		proxyClient, _ = v1.NewProxyClient(resourceClientFactory)

		upstreamClient, err := resourceClientFactory.NewResourceClient(factory.NewResourceClientParams{ResourceType: &v1.Upstream{}})
		Expect(err).NotTo(HaveOccurred())

		proxy := &v1.Proxy{
			Metadata: core.Metadata{
				Namespace: ns,
				Name:      proxyName,
			},
		}

		settings = &v1.Settings{}

		rep := reporter.NewReporter(ref, proxyClient.BaseClient(), upstreamClient)

		xdsHasher := &xds.ProxyKeyHasher{}
		syncer = NewTranslatorSyncer(&mockTranslator{true}, xdsCache, xdsHasher, sanitizer, rep, false, nil, settings)
		snap = &v1.ApiSnapshot{
			Proxies: v1.ProxyList{
				proxy,
			},
		}
		err = syncer.Sync(context.Background(), snap)
		Expect(err).NotTo(HaveOccurred())

		proxies, err := proxyClient.List(proxy.GetMetadata().Namespace, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(proxies).To(HaveLen(1))
		Expect(proxies[0]).To(BeAssignableToTypeOf(&v1.Proxy{}))
		Expect(proxies[0].Status).To(Equal(core.Status{
			State:      2,
			Reason:     "1 error occurred:\n\t* hi, how ya doin'?\n\n",
			ReportedBy: ref,
		}))

		// NilSnapshot is always consistent, so snapshot will always be set as part of endpoints update
		Expect(xdsCache.called).To(BeTrue())

		// update rv for proxy
		p1, err := proxyClient.Read(proxy.Metadata.Namespace, proxy.Metadata.Name, clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())
		snap.Proxies[0] = p1

		syncer = NewTranslatorSyncer(&mockTranslator{false}, xdsCache, xdsHasher, sanitizer, rep, false, nil, settings)

		err = syncer.Sync(context.Background(), snap)
		Expect(err).NotTo(HaveOccurred())

	})

	It("writes the reports the translator spits out and calls SetSnapshot on the cache", func() {
		proxies, err := proxyClient.List(ns, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(proxies).To(HaveLen(1))
		Expect(proxies[0]).To(BeAssignableToTypeOf(&v1.Proxy{}))
		Expect(proxies[0].Status).To(Equal(core.Status{
			State:      1,
			ReportedBy: ref,
		}))

		Expect(xdsCache.called).To(BeTrue())
	})

	It("updates the cache with the sanitized snapshot", func() {
		sanitizer.snap = envoycache.NewEasyGenericSnapshot("easy")
		err := syncer.Sync(context.Background(), snap)
		Expect(err).NotTo(HaveOccurred())

		Expect(sanitizer.called).To(BeTrue())
		Expect(xdsCache.setSnap).To(BeEquivalentTo(sanitizer.snap))
	})

	It("uses listeners and routes from the previous snapshot when sanitization fails", func() {
		sanitizer.err = errors.Errorf("we ran out of coffee")

		oldXdsSnap := xds.NewSnapshotFromResources(
			envoycache.NewResources("", nil),
			envoycache.NewResources("", nil),
			envoycache.NewResources("", nil),
			envoycache.NewResources("old listeners from before the war", []envoycache.Resource{
				xds.NewEnvoyResource(&v2.Listener{}),
			}),
		)

		// return this old snapshot when the syncer asks for it
		xdsCache.getSnap = oldXdsSnap
		err := syncer.Sync(context.Background(), snap)
		Expect(err).NotTo(HaveOccurred())

		Expect(sanitizer.called).To(BeTrue())
		Expect(xdsCache.called).To(BeTrue())

		oldListeners := oldXdsSnap.GetResources(xds.ListenerType)
		newListeners := xdsCache.setSnap.GetResources(xds.ListenerType)

		Expect(oldListeners).To(Equal(newListeners))

		oldRoutes := oldXdsSnap.GetResources(xds.RouteType)
		newRoutes := xdsCache.setSnap.GetResources(xds.RouteType)

		Expect(oldRoutes).To(Equal(newRoutes))
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
	// snap that is set
	setSnap envoycache.Snapshot
	// snap that is returned
	getSnap envoycache.Snapshot
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

func (c *mockXdsCache) GetStatusKeys() []string {
	return []string{}
}

func (c *mockXdsCache) SetSnapshot(node string, snapshot envoycache.Snapshot) error {
	c.called = true
	c.setSnap = snapshot
	return nil
}

func (c *mockXdsCache) GetSnapshot(node string) (envoycache.Snapshot, error) {
	if c.getSnap != nil {
		return c.getSnap, nil
	}
	return &envoycache.NilSnapshot{}, nil
}

func (*mockXdsCache) ClearSnapshot(node string) {
	panic("implement me")
}

type mockXdsSanitizer struct {
	called bool
	snap   envoycache.Snapshot
	err    error
}

func (s *mockXdsSanitizer) SanitizeSnapshot(ctx context.Context, glooSnapshot *v1.ApiSnapshot, xdsSnapshot envoycache.Snapshot, reports reporter.ResourceReports) (envoycache.Snapshot, error) {
	s.called = true
	if s.snap != nil {
		return s.snap, nil
	}
	if s.err != nil {
		return nil, s.err
	}
	return xdsSnapshot, nil
}
