package syncer_test

import (
	"context"
	"fmt"

	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector/singlereplica"

	gloo_translator "github.com/solo-io/gloo/projects/gloo/pkg/translator"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/statusutils"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils/metrics"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"github.com/solo-io/solo-kit/pkg/errors"
)

var _ = Describe("Translate Proxy", func() {

	var (
		ctx            context.Context
		cancel         context.CancelFunc
		xdsCache       *MockXdsCache
		sanitizer      *MockXdsSanitizer
		syncer         v1snap.ApiSyncer
		snap           *v1snap.ApiSnapshot
		settings       *v1.Settings
		upstreamClient clients.ResourceClient
		proxyClient    v1.ProxyClient
		proxyName      = "proxy-name"
		ns             = "any-ns"
		ref            = "syncer-test"
		statusClient   resources.StatusClient
		statusMetrics  metrics.ConfigStatusMetrics
	)

	BeforeEach(func() {
		var err error
		xdsCache = &MockXdsCache{}
		sanitizer = &MockXdsSanitizer{}
		ctx, cancel = context.WithCancel(context.Background())

		resourceClientFactory := &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}

		proxyClient, _ = v1.NewProxyClient(ctx, resourceClientFactory)

		upstreamClient, err = resourceClientFactory.NewResourceClient(ctx, factory.NewResourceClientParams{ResourceType: &v1.Upstream{}})
		Expect(err).NotTo(HaveOccurred())

		proxy := &v1.Proxy{
			Metadata: &core.Metadata{
				Namespace: ns,
				Name:      proxyName,
			},
		}

		settings = &v1.Settings{}

		statusClient = statusutils.GetStatusClientFromEnvOrDefault(ns)
		statusMetrics, err = metrics.NewConfigStatusMetrics(metrics.GetDefaultConfigStatusOptions())
		Expect(err).NotTo(HaveOccurred())

		rep := reporter.NewReporter(ref, statusClient, proxyClient.BaseClient(), upstreamClient)

		syncer = NewTranslatorSyncer(ctx, &mockTranslator{true, false, nil}, xdsCache, sanitizer, rep, false, nil, settings, statusMetrics, nil, proxyClient, "", singlereplica.Identity())
		snap = &v1snap.ApiSnapshot{
			Proxies: v1.ProxyList{
				proxy,
			},
		}
		_, err = proxyClient.Write(proxy, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
		err = syncer.Sync(context.Background(), snap)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() (bool, error) {
			proxies, err := proxyClient.List(ns, clients.ListOpts{})
			if err != nil {
				return false, err
			}
			if len(proxies) != 1 {
				return false, fmt.Errorf("expected 1 proxy, got %v", len(proxies))
			}
			expectedStatus := &core.Status{
				State:      2,
				Reason:     "1 error occurred:\n\t* hi, how ya doin'?\n\n",
				ReportedBy: ref,
			}
			return expectedStatus.Equal(statusClient.GetStatus(proxies[0])), nil
		}, "2s", "0.1s").Should(BeTrue())

		// NilSnapshot is always consistent, so snapshot will always be set as part of endpoints update
		Expect(xdsCache.Called).To(BeTrue())

		// update rv for proxy
		p1, err := proxyClient.Read(proxy.Metadata.Namespace, proxy.Metadata.Name, clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())
		snap.Proxies[0] = p1

		syncer = NewTranslatorSyncer(ctx, &mockTranslator{false, false, nil}, xdsCache, sanitizer, rep, false, nil, settings, statusMetrics, nil, proxyClient, "", singlereplica.Identity())

		err = syncer.Sync(context.Background(), snap)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() { cancel() })

	It("writes the reports the translator spits out and calls SetSnapshot on the cache", func() {

		Eventually(func() (bool, error) {
			proxies, err := proxyClient.List(ns, clients.ListOpts{})
			if err != nil {
				return false, err
			}
			if len(proxies) != 1 {
				return false, fmt.Errorf("expected 1 proxy, got %v", len(proxies))
			}
			expectedStatus := &core.Status{
				State:      1,
				ReportedBy: ref,
			}
			return expectedStatus.Equal(statusClient.GetStatus(proxies[0])), nil
		}, "2s", "0.1s").Should(BeTrue())

		Expect(xdsCache.Called).To(BeTrue())
	})

	It("updates the cache with the sanitized snapshot", func() {
		sanitizer.Snap = envoycache.NewEasyGenericSnapshot("easy")
		err := syncer.Sync(context.Background(), snap)
		Expect(err).NotTo(HaveOccurred())

		Expect(sanitizer.Called).To(BeTrue())
		Expect(xdsCache.SetSnap).To(BeEquivalentTo(sanitizer.Snap))
	})
})

var _ = Describe("Translate multiple proxies with errors", func() {

	var (
		ctx            context.Context
		cancel         context.CancelFunc
		xdsCache       *MockXdsCache
		sanitizer      *MockXdsSanitizer
		syncer         v1snap.ApiSyncer
		snap           *v1snap.ApiSnapshot
		settings       *v1.Settings
		proxyClient    v1.ProxyClient
		upstreamClient v1.UpstreamClient
		proxyName      = "proxy-name"
		upstreamName   = "upstream-name"
		ns             = "any-ns"
		ref            = "syncer-test"
		statusClient   resources.StatusClient
		statusMetrics  metrics.ConfigStatusMetrics
	)

	proxiesShouldHaveErrors := func(proxies v1.ProxyList, numProxies int) {
		Expect(proxies).To(HaveLen(numProxies))
		for _, proxy := range proxies {
			Expect(proxy).To(BeAssignableToTypeOf(&v1.Proxy{}))
			Expect(statusClient.GetStatus(proxy)).To(Equal(&core.Status{
				State:      2,
				Reason:     "1 error occurred:\n\t* hi, how ya doin'?\n\n",
				ReportedBy: ref,
			}))

		}

	}
	writeUniqueErrsToUpstreams := func() {
		// Re-writes existing upstream to have an annotation
		// which triggers a unique error to be written from each proxy's mockTranslator
		upstreams, err := upstreamClient.List(ns, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(upstreams).To(HaveLen(1))

		us := upstreams[0]
		// This annotation causes the translator mock to generate a unique error per proxy on each upstream
		us.Metadata.Annotations = map[string]string{"uniqueErrPerProxy": "true"}
		_, err = upstreamClient.Write(us, clients.WriteOpts{OverwriteExisting: true})
		Expect(err).NotTo(HaveOccurred())
		snap.Upstreams = upstreams
		err = syncer.Sync(context.Background(), snap)
		Expect(err).NotTo(HaveOccurred())
	}

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		var err error
		xdsCache = &MockXdsCache{}
		sanitizer = &MockXdsSanitizer{}

		resourceClientFactory := &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}

		proxyClient, _ = v1.NewProxyClient(context.Background(), resourceClientFactory)

		usClient, err := resourceClientFactory.NewResourceClient(context.Background(), factory.NewResourceClientParams{ResourceType: &v1.Upstream{}})
		Expect(err).NotTo(HaveOccurred())

		proxy1 := &v1.Proxy{
			Metadata: &core.Metadata{
				Namespace: ns,
				Name:      proxyName + "1",
			},
		}
		proxy2 := &v1.Proxy{
			Metadata: &core.Metadata{
				Namespace: ns,
				Name:      proxyName + "2",
			},
		}

		us := &v1.Upstream{
			Metadata: &core.Metadata{
				Name:      upstreamName,
				Namespace: ns,
			},
		}

		settings = &v1.Settings{}

		statusClient = statusutils.GetStatusClientFromEnvOrDefault(ns)
		statusMetrics, err = metrics.NewConfigStatusMetrics(metrics.GetDefaultConfigStatusOptions())
		Expect(err).NotTo(HaveOccurred())

		rep := reporter.NewReporter(ref, statusClient, proxyClient.BaseClient(), usClient)

		syncer = NewTranslatorSyncer(ctx, &mockTranslator{true, true, nil}, xdsCache, sanitizer, rep, false, nil, settings, statusMetrics, nil, proxyClient, "", singlereplica.Identity())
		snap = &v1snap.ApiSnapshot{
			Proxies: v1.ProxyList{
				proxy1,
				proxy2,
			},
			Upstreams: v1.UpstreamList{
				us,
			},
		}

		_, err = usClient.Write(us, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
		_, err = proxyClient.Write(proxy1, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
		_, err = proxyClient.Write(proxy2, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
		err = syncer.Sync(context.Background(), snap)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() (bool, error) {
			proxies, err := proxyClient.List(proxy1.GetMetadata().Namespace, clients.ListOpts{})
			if err != nil {
				return false, err
			}
			if len(proxies) != 2 {
				return false, fmt.Errorf("expected 2 proxies, got %v", len(proxies))
			}
			expectedStatus := &core.Status{
				State:      2,
				Reason:     "1 error occurred:\n\t* hi, how ya doin'?\n\n",
				ReportedBy: ref,
			}
			return expectedStatus.Equal(statusClient.GetStatus(proxies[0])), nil
		}, "2s", "0.1s").Should(BeTrue())

		// NilSnapshot is always consistent, so snapshot will always be set as part of endpoints update
		Expect(xdsCache.Called).To(BeTrue())

		upstreamClient, err = v1.NewUpstreamClient(context.Background(), resourceClientFactory)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cancel()
	})

	It("handles reporting errors on multiple proxies sharing an upstream reporting 2 different errors", func() {
		// Testing the scenario where we have multiple proxies,
		// each of which should report a different unique error on an upstream.

		proxies, err := proxyClient.List(ns, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())

		proxiesShouldHaveErrors(proxies, 2)

		writeUniqueErrsToUpstreams()

		Eventually(func() (bool, error) {
			upstreams, err := upstreamClient.List(ns, clients.ListOpts{})
			if err != nil {
				return false, err
			}
			expectedStatus := &core.Status{
				State:      2,
				Reason:     "2 errors occurred:\n\t* upstream is bad - determined by proxy-name1\n\t* upstream is bad - determined by proxy-name2\n\n",
				ReportedBy: ref,
			}
			return expectedStatus.Equal(statusClient.GetStatus(upstreams[0])), nil
		}, "2s", "0.1s").Should(BeTrue())

		Expect(xdsCache.Called).To(BeTrue())
	})

	It("handles reporting errors on multiple proxies sharing an upstream, each reporting the same upstream error", func() {
		// Testing the scenario where we have multiple proxies,
		// each of which should report the same error on an upstream.
		proxies, err := proxyClient.List(ns, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())
		proxiesShouldHaveErrors(proxies, 2)

		upstreams, err := upstreamClient.List(ns, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(upstreams).To(HaveLen(1))
		Expect(statusClient.GetStatus(upstreams[0])).To(Equal(&core.Status{
			State:      2,
			Reason:     "1 error occurred:\n\t* generic upstream error\n\n",
			ReportedBy: ref,
		}))

		Expect(xdsCache.Called).To(BeTrue())
	})
})

type mockTranslator struct {
	reportErrs         bool
	reportUpstreamErrs bool // Adds an error to every upstream in the snapshot
	currentSnapshot    envoycache.Snapshot
}

func (t *mockTranslator) Translate(params plugins.Params, proxy *v1.Proxy) (envoycache.Snapshot, reporter.ResourceReports, *validation.ProxyReport) {
	if t.reportErrs {
		rpts := reporter.ResourceReports{}
		rpts.AddError(proxy, errors.Errorf("hi, how ya doin'?"))
		if t.reportUpstreamErrs {
			for _, upstream := range params.Snapshot.Upstreams {
				if upstream.Metadata.Annotations["uniqueErrPerProxy"] == "true" {
					rpts.AddError(upstream, errors.Errorf("upstream is bad - determined by %s", proxy.Metadata.Name))
				} else {
					rpts.AddError(upstream, errors.Errorf("generic upstream error"))
				}
			}
		}
		if t.currentSnapshot != nil {
			return t.currentSnapshot, rpts, &validation.ProxyReport{}
		}
		return envoycache.NilSnapshot{}, rpts, &validation.ProxyReport{}
	}
	if t.currentSnapshot != nil {
		return t.currentSnapshot, nil, &validation.ProxyReport{}
	}
	return envoycache.NilSnapshot{}, nil, &validation.ProxyReport{}
}

var (
	_ envoycache.SnapshotCache   = new(MockXdsCache)
	_ gloo_translator.Translator = new(mockTranslator)
)
