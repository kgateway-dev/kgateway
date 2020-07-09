package syncer

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/reconciler"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var _ = Describe("TranslatorSyncer", func() {
	var (
		watcher   *fakeWatcher
		freporter *fakeReporter
		syncer    statusSyncer
	)

	BeforeEach(func() {
		watcher = &fakeWatcher{}
		freporter = &fakeReporter{}
		syncer = newStatusSyncer("gloo-system", watcher, freporter)
	})

	It("should set status correctly", func() {
		proxy := &gloov1.Proxy{
			Metadata: core.Metadata{Name: "test", Namespace: "gloo-system"},
			Status:   core.Status{State: core.Status_Accepted},
		}
		vs := &gatewayv1.VirtualService{}
		errs := reporter.ResourceReports{}
		errs.Accept(vs)

		// reports.AddError(gw, fmt.Errorf("invalid virtual service ref %v", vs))

		desiredProxies := reconciler.GeneratedProxies{
			proxy: errs,
		}

		syncer.setCurrentProxies(desiredProxies)
		syncer.setStatuses(gloov1.ProxyList{proxy})

		err := syncer.syncStatus(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(freporter.E).To(Equal(errs))
		m := map[string]*core.Status{
			"*v1.Proxy.gloo-system.test": {State: core.Status_Accepted},
		}
		Expect(freporter.S[vs]).To(BeEquivalentTo(m))
	})

	It("should set status correctly when one proxy errors", func() {
		proxy := &gloov1.Proxy{
			Metadata: core.Metadata{Name: "test", Namespace: "gloo-system"},
			Status:   core.Status{State: core.Status_Accepted},
		}
		proxy2 := &gloov1.Proxy{
			Metadata: core.Metadata{Name: "test2", Namespace: "gloo-system"},
			Status:   core.Status{State: core.Status_Rejected},
		}
		vs := &gatewayv1.VirtualService{}
		errs := reporter.ResourceReports{}
		errs.Accept(vs)

		desiredProxies := reconciler.GeneratedProxies{
			proxy:  errs,
			proxy2: errs,
		}

		syncer.setCurrentProxies(desiredProxies)
		syncer.setStatuses(gloov1.ProxyList{proxy, proxy2})

		err := syncer.syncStatus(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(freporter.E).To(Equal(errs))
		m := map[string]*core.Status{
			"*v1.Proxy.gloo-system.test":  {State: core.Status_Accepted},
			"*v1.Proxy.gloo-system.test2": {State: core.Status_Rejected},
		}
		Expect(freporter.S[vs]).To(BeEquivalentTo(m))
	})

	It("should set status correctly when one proxy errors but is irrelevant", func() {
		proxy := &gloov1.Proxy{
			Metadata: core.Metadata{Name: "test", Namespace: "gloo-system"},
			Status:   core.Status{State: core.Status_Accepted},
		}
		proxy2 := &gloov1.Proxy{
			Metadata: core.Metadata{Name: "test2", Namespace: "gloo-system"},
			Status:   core.Status{State: core.Status_Rejected},
		}
		vs := &gatewayv1.VirtualService{}
		errs := reporter.ResourceReports{}
		errs.Accept(vs)

		desiredProxies := reconciler.GeneratedProxies{
			proxy:  errs,
			proxy2: reporter.ResourceReports{},
		}

		syncer.setCurrentProxies(desiredProxies)
		syncer.setStatuses(gloov1.ProxyList{proxy, proxy2})

		err := syncer.syncStatus(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(freporter.E).To(Equal(errs))
		m := map[string]*core.Status{
			"*v1.Proxy.gloo-system.test": {State: core.Status_Accepted},
		}
		Expect(freporter.S[vs]).To(BeEquivalentTo(m))
	})

})

type fakeWatcher struct {
	P chan gloov1.ProxyList
	E chan error
}

func (f *fakeWatcher) Watch(namespace string, opts clients.WatchOpts) (<-chan gloov1.ProxyList, <-chan error, error) {
	return f.P, f.E, nil
}

type fakeReporter struct {
	E reporter.ResourceReports
	S map[resources.InputResource]map[string]*core.Status
}

func (f *fakeReporter) WriteReports(ctx context.Context, errs reporter.ResourceReports, subresourceStatuses map[string]*core.Status) error {
	if f.E == nil {
		f.E = errs
	} else {
		f.E.Merge(errs)
	}
	if f.S == nil {
		f.S = map[resources.InputResource]map[string]*core.Status{}
	}
	for k := range errs {
		f.S[k] = subresourceStatuses
	}

	return nil
}
