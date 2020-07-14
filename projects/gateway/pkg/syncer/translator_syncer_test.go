package syncer

import (
	"context"
	"fmt"
	"sync"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/reconciler"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var _ = Describe("TranslatorSyncer", func() {
	var (
		fakeWatcher  = &fakeWatcher{}
		mockReporter *fakeReporter
		syncer       statusSyncer
	)

	BeforeEach(func() {
		mockReporter = &fakeReporter{}
		syncer = newStatusSyncer("gloo-system", fakeWatcher, mockReporter)
	})

	getMapOnlyKey := func(r map[core.ResourceRef]reporter.Report) core.ResourceRef {
		Expect(r).To(HaveLen(1))
		for k := range r {
			return k
		}
		panic("unreachable")
	}

	It("should set status correctly", func() {
		acceptedProxy := &gloov1.Proxy{
			Metadata: core.Metadata{Name: "test", Namespace: "gloo-system"},
			Status:   core.Status{State: core.Status_Accepted},
		}
		vs := &gatewayv1.VirtualService{}
		errs := reporter.ResourceReports{}
		errs.Accept(vs)

		desiredProxies := reconciler.GeneratedProxies{
			acceptedProxy: errs,
		}

		syncer.setCurrentProxies(desiredProxies)
		syncer.setStatuses(gloov1.ProxyList{acceptedProxy})

		err := syncer.syncStatus(context.Background())
		Expect(err).NotTo(HaveOccurred())
		reportedKey := getMapOnlyKey(mockReporter.Reports())
		Expect(reportedKey).To(BeEquivalentTo(vs.GetMetadata().Ref()))
		Expect(mockReporter.Reports()[reportedKey]).To(BeEquivalentTo(errs[vs]))
		m := map[string]*core.Status{
			"*v1.Proxy.gloo-system.test": {State: core.Status_Accepted},
		}
		Expect(mockReporter.Statuses()[reportedKey]).To(BeEquivalentTo(m))
	})

	It("should set status correctly when proxy is pending first", func() {
		desiredProxy := &gloov1.Proxy{
			Metadata: core.Metadata{Name: "test", Namespace: "gloo-system"},
		}
		pendingProxy := &gloov1.Proxy{
			Metadata: core.Metadata{Name: "test", Namespace: "gloo-system"},
			Status:   core.Status{State: core.Status_Pending},
		}
		acceptedProxy := &gloov1.Proxy{
			Metadata: core.Metadata{Name: "test", Namespace: "gloo-system"},
			Status:   core.Status{State: core.Status_Accepted},
		}
		vs := &gatewayv1.VirtualService{}
		errs := reporter.ResourceReports{}
		errs.Accept(vs)

		desiredProxies := reconciler.GeneratedProxies{
			desiredProxy: errs,
		}
		proxies := make(chan gloov1.ProxyList)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go syncer.watchProxiesFromChannel(ctx, proxies, nil)
		go syncer.syncStatusOnEmit(ctx)

		syncer.setCurrentProxies(desiredProxies)
		proxies <- gloov1.ProxyList{pendingProxy}
		proxies <- gloov1.ProxyList{acceptedProxy}

		Eventually(mockReporter.Reports, "5s", "0.5s").ShouldNot(BeEmpty())
		reportedKey := getMapOnlyKey(mockReporter.Reports())
		Expect(reportedKey).To(BeEquivalentTo(vs.GetMetadata().Ref()))
		Expect(mockReporter.Reports()[reportedKey]).To(BeEquivalentTo(errs[vs]))
		m := map[string]*core.Status{
			"*v1.Proxy.gloo-system.test": {State: core.Status_Accepted},
		}
		Eventually(func() map[string]*core.Status { return mockReporter.Statuses()[reportedKey] }, "5s", "0.5s").Should(BeEquivalentTo(m))
	})

	It("should set status correctly when one proxy errors", func() {
		acceptedProxy := &gloov1.Proxy{
			Metadata: core.Metadata{Name: "test", Namespace: "gloo-system"},
			Status:   core.Status{State: core.Status_Accepted},
		}
		rejectedProxy := &gloov1.Proxy{
			Metadata: core.Metadata{Name: "test2", Namespace: "gloo-system"},
			Status:   core.Status{State: core.Status_Rejected},
		}
		vs := &gatewayv1.VirtualService{}
		errs := reporter.ResourceReports{}
		errs.Accept(vs)

		desiredProxies := reconciler.GeneratedProxies{
			acceptedProxy: errs,
			rejectedProxy: errs,
		}

		syncer.setCurrentProxies(desiredProxies)
		syncer.setStatuses(gloov1.ProxyList{acceptedProxy, rejectedProxy})

		err := syncer.syncStatus(context.Background())
		Expect(err).NotTo(HaveOccurred())

		reportedKey := getMapOnlyKey(mockReporter.Reports())
		Expect(reportedKey).To(BeEquivalentTo(vs.GetMetadata().Ref()))
		Expect(mockReporter.Reports()[reportedKey]).To(BeEquivalentTo(errs[vs]))

		m := map[string]*core.Status{
			"*v1.Proxy.gloo-system.test":  {State: core.Status_Accepted},
			"*v1.Proxy.gloo-system.test2": {State: core.Status_Rejected},
		}
		Expect(mockReporter.Statuses()[reportedKey]).To(BeEquivalentTo(m))
	})

	It("should set status correctly when one proxy errors but is irrelevant", func() {
		acceptedProxy := &gloov1.Proxy{
			Metadata: core.Metadata{Name: "test", Namespace: "gloo-system"},
			Status:   core.Status{State: core.Status_Accepted},
		}
		rejectedProxy := &gloov1.Proxy{
			Metadata: core.Metadata{Name: "test2", Namespace: "gloo-system"},
			Status:   core.Status{State: core.Status_Rejected},
		}
		vs := &gatewayv1.VirtualService{}
		errs := reporter.ResourceReports{}
		errs.Accept(vs)

		desiredProxies := reconciler.GeneratedProxies{
			acceptedProxy: errs,
			rejectedProxy: reporter.ResourceReports{},
		}

		syncer.setCurrentProxies(desiredProxies)
		syncer.setStatuses(gloov1.ProxyList{acceptedProxy, rejectedProxy})

		err := syncer.syncStatus(context.Background())
		Expect(err).NotTo(HaveOccurred())

		reportedKey := getMapOnlyKey(mockReporter.Reports())
		Expect(reportedKey).To(BeEquivalentTo(vs.GetMetadata().Ref()))
		Expect(mockReporter.Reports()[reportedKey]).To(BeEquivalentTo(errs[vs]))

		m := map[string]*core.Status{
			"*v1.Proxy.gloo-system.test": {State: core.Status_Accepted},
		}
		Expect(mockReporter.Statuses()[reportedKey]).To(BeEquivalentTo(m))
	})

	It("should set status correctly when one proxy errors", func() {
		rejectedProxy1 := &gloov1.Proxy{
			Metadata: core.Metadata{Name: "test", Namespace: "gloo-system"},
			Status:   core.Status{State: core.Status_Rejected},
		}
		rejectedProxy2 := &gloov1.Proxy{
			Metadata: core.Metadata{Name: "test2", Namespace: "gloo-system"},
			Status:   core.Status{State: core.Status_Rejected},
		}
		vs := &gatewayv1.VirtualService{}
		errsProxy1 := reporter.ResourceReports{}
		errsProxy1.Accept(vs)
		errsProxy1.AddError(vs, fmt.Errorf("invalid 1"))
		errsProxy2 := reporter.ResourceReports{}
		errsProxy2.Accept(vs)
		errsProxy2.AddError(vs, fmt.Errorf("invalid 2"))
		desiredProxies := reconciler.GeneratedProxies{
			rejectedProxy1: errsProxy1,
			rejectedProxy2: errsProxy2,
		}

		syncer.setCurrentProxies(desiredProxies)
		syncer.setStatuses(gloov1.ProxyList{rejectedProxy1, rejectedProxy2})

		err := syncer.syncStatus(context.Background())
		Expect(err).NotTo(HaveOccurred())

		mergedErrs := reporter.ResourceReports{}
		mergedErrs.Accept(vs)
		mergedErrs.AddError(vs, fmt.Errorf("invalid 1"))
		mergedErrs.AddError(vs, fmt.Errorf("invalid 2"))

		reportedKey := getMapOnlyKey(mockReporter.Reports())
		Expect(reportedKey).To(BeEquivalentTo(vs.GetMetadata().Ref()))
		Expect(mockReporter.Reports()[reportedKey]).To(BeEquivalentTo(mergedErrs[vs]))

		m := map[string]*core.Status{
			"*v1.Proxy.gloo-system.test":  {State: core.Status_Rejected},
			"*v1.Proxy.gloo-system.test2": {State: core.Status_Rejected},
		}
		Expect(mockReporter.Statuses()[reportedKey]).To(BeEquivalentTo(m))
	})

})

type fakeWatcher struct {
}

func (f *fakeWatcher) Watch(namespace string, opts clients.WatchOpts) (<-chan gloov1.ProxyList, <-chan error, error) {
	return nil, nil, nil
}

type fakeReporter struct {
	reports  map[core.ResourceRef]reporter.Report
	statuses map[core.ResourceRef]map[string]*core.Status
	lock     sync.Mutex
}

func (f *fakeReporter) Reports() map[core.ResourceRef]reporter.Report {
	f.lock.Lock()
	defer f.lock.Unlock()
	return f.reports
}
func (f *fakeReporter) Statuses() map[core.ResourceRef]map[string]*core.Status {
	f.lock.Lock()
	defer f.lock.Unlock()
	return f.statuses
}

func (f *fakeReporter) WriteReports(ctx context.Context, errs reporter.ResourceReports, subresourceStatuses map[string]*core.Status) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	fmt.Fprintf(GinkgoWriter, "WriteReports: %#v %#v", errs, subresourceStatuses)
	newreports := map[core.ResourceRef]reporter.Report{}
	for k, v := range f.reports {
		newreports[k] = v
	}
	for k, v := range errs {
		newreports[k.GetMetadata().Ref()] = v
	}
	f.reports = newreports

	newstatus := map[core.ResourceRef]map[string]*core.Status{}
	for k, v := range f.statuses {
		newstatus[k] = v
	}
	for k := range errs {
		newstatus[k.GetMetadata().Ref()] = subresourceStatuses
	}
	f.statuses = newstatus
	return nil
}
