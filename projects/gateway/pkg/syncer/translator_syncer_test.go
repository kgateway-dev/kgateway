package syncer

import (
	"context"
	"fmt"
	"sync"

	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector/singlereplica"

	"github.com/solo-io/gloo/pkg/utils/statusutils"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils/metrics"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/reconciler"
	gatewaymocks "github.com/solo-io/gloo/projects/gateway/pkg/translator/mocks"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/compress"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	gloomocks "github.com/solo-io/gloo/projects/gloo/pkg/mocks"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"go.uber.org/mock/gomock"
)

var _ = Describe("TranslatorSyncer", func() {

	var (
		fakeProxyClient *gloomocks.MockProxyClient
		mockReporter    *fakeReporter
		syncer          *statusSyncer

		statusClient resources.StatusClient
	)

	BeforeEach(func() {
		mockReporter = &fakeReporter{}
		ctrl := gomock.NewController(GinkgoT())
		fakeProxyClient = gloomocks.NewMockProxyClient(ctrl)
		statusClient = statusutils.GetStatusClientFromEnvOrDefault(defaults.GlooSystem)
		statusMetrics, err := metrics.NewConfigStatusMetrics(metrics.GetDefaultConfigStatusOptions())
		Expect(err).NotTo(HaveOccurred())
		curSyncer := newStatusSyncer(defaults.GlooSystem, fakeProxyClient, mockReporter, statusClient, statusMetrics, singlereplica.Identity())
		syncer = &curSyncer
	})

	getMapOnlyKey := func(r map[string]reporter.Report) string {
		Expect(r).To(HaveLen(1))
		for k := range r {
			return k
		}
		panic("unreachable")
	}

	It("should set status correctly", func() {
		acceptedProxy := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test", Namespace: defaults.GlooSystem},
		}
		statusClient.SetStatus(acceptedProxy, &core.Status{State: core.Status_Accepted})

		vs := &gatewayv1.VirtualService{
			Metadata: &core.Metadata{
				Name:      "vs",
				Namespace: defaults.GlooSystem,
			},
		}
		errs := reporter.ResourceReports{}
		errs.Accept(vs)

		desiredProxies := reconciler.GeneratedProxies{
			acceptedProxy: errs,
		}

		syncer.setCurrentProxies(desiredProxies, make(reconciler.InvalidProxies))
		syncer.setStatuses(gloov1.ProxyList{acceptedProxy})

		err := syncer.syncStatus(context.Background())
		Expect(err).NotTo(HaveOccurred())
		reportedKey := getMapOnlyKey(mockReporter.Reports())
		Expect(reportedKey).To(Equal(translator.UpstreamToClusterName(vs.GetMetadata().Ref())))
		Expect(mockReporter.Reports()[reportedKey]).To(BeEquivalentTo(errs[vs]))
		m := map[string]*core.Status{
			"*v1.Proxy.test_gloo-system": {State: core.Status_Accepted},
		}
		Expect(mockReporter.Statuses()[reportedKey]).To(BeEquivalentTo(m))
	})

	It("should set status correctly when resources are in both proxies", func() {
		reportContainsWarning := func(report reporter.Report, warning string) bool {
			for _, w := range report.Warnings {
				if w == warning {
					return true
				}
			}
			return false
		}

		acceptedProxy1 := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test1", Namespace: "gloo-system"},
		}
		statusClient.SetStatus(acceptedProxy1, &core.Status{State: core.Status_Accepted})

		acceptedProxy2 := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test2", Namespace: "gloo-system"},
		}
		statusClient.SetStatus(acceptedProxy2, &core.Status{State: core.Status_Accepted})

		errs1 := reporter.ResourceReports{}
		errs2 := reporter.ResourceReports{}
		expectedErr := reporter.ResourceReports{}

		rt := &gatewayv1.RouteTable{
			Metadata: &core.Metadata{
				Name:      "rt",
				Namespace: defaults.GlooSystem,
			},
		}
		errs1.AddWarning(rt, "warning 1")
		errs2.AddWarning(rt, "warning 2")
		expectedErr.AddWarning(rt, "warning 1")
		expectedErr.AddWarning(rt, "warning 2")

		desiredProxies := reconciler.GeneratedProxies{
			acceptedProxy1: errs1,
			acceptedProxy2: errs2,
		}

		syncer.setCurrentProxies(desiredProxies, make(reconciler.InvalidProxies))
		syncer.setStatuses(gloov1.ProxyList{acceptedProxy1, acceptedProxy2})

		err := syncer.syncStatus(context.Background())
		Expect(err).NotTo(HaveOccurred())

		reportedKey := getMapOnlyKey(mockReporter.Reports())
		Expect(reportedKey).To(Equal(translator.UpstreamToClusterName(rt.GetMetadata().Ref())))

		Expect(reportContainsWarning(mockReporter.Reports()[reportedKey], "warning 1")).To(BeTrue())
		Expect(reportContainsWarning(mockReporter.Reports()[reportedKey], "warning 2")).To(BeTrue())

		m := map[string]*core.Status{
			"*v1.Proxy.test2_gloo-system": {State: core.Status_Accepted},
			"*v1.Proxy.test1_gloo-system": {State: core.Status_Accepted},
		}
		Expect(mockReporter.Statuses()[reportedKey]).To(BeEquivalentTo(m))
	})

	It("should set status correctly when proxy is pending first", func() {
		desiredProxy := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test", Namespace: "gloo-system"},
		}
		pendingProxy := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test", Namespace: "gloo-system"},
		}
		statusClient.SetStatus(pendingProxy, &core.Status{State: core.Status_Pending})

		acceptedProxy := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test", Namespace: "gloo-system"},
		}
		statusClient.SetStatus(acceptedProxy, &core.Status{State: core.Status_Accepted})

		vs := &gatewayv1.VirtualService{}
		errs := reporter.ResourceReports{}
		errs.Accept(vs)

		desiredProxies := reconciler.GeneratedProxies{
			desiredProxy: errs,
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go syncer.syncStatusOnEmit(ctx)

		syncer.setCurrentProxies(desiredProxies, make(reconciler.InvalidProxies))

		fakeProxyClient.EXPECT().List("gloo-system", gomock.Any()).Return(gloov1.ProxyList{pendingProxy}, nil).Times(1)
		syncer.handleUpdatedProxies(ctx)
		syncer.setCurrentProxies(desiredProxies, make(reconciler.InvalidProxies))
		fakeProxyClient.EXPECT().List("gloo-system", gomock.Any()).Return(gloov1.ProxyList{acceptedProxy}, nil).Times(1)
		syncer.handleUpdatedProxies(ctx)
		Eventually(mockReporter.Reports, "5s", "0.5s").ShouldNot(BeEmpty())
		reportedKey := getMapOnlyKey(mockReporter.Reports())
		Expect(reportedKey).To(Equal(translator.UpstreamToClusterName(vs.GetMetadata().Ref())))
		Expect(mockReporter.Reports()[reportedKey]).To(BeEquivalentTo(errs[vs]))
		m := map[string]*core.Status{
			"*v1.Proxy.test_gloo-system": {State: core.Status_Accepted},
		}
		Eventually(func() map[string]*core.Status { return mockReporter.Statuses()[reportedKey] }, "5s", "0.5s").Should(BeEquivalentTo(m))
	})

	It("should retry setting the status if it first fails", func() {
		desiredProxy := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test", Namespace: "gloo-system"},
		}
		acceptedProxy := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test", Namespace: "gloo-system"},
		}
		statusClient.SetStatus(acceptedProxy, &core.Status{State: core.Status_Accepted})

		mockReporter.Err = fmt.Errorf("error")
		vs := &gatewayv1.VirtualService{}
		errs := reporter.ResourceReports{}
		errs.Accept(vs)

		desiredProxies := reconciler.GeneratedProxies{
			desiredProxy:  errs,
			acceptedProxy: errs,
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go syncer.syncStatusOnEmit(ctx)

		syncer.setCurrentProxies(desiredProxies, make(reconciler.InvalidProxies))
		fakeProxyClient.EXPECT().List("gloo-system", gomock.Any()).Return(gloov1.ProxyList{acceptedProxy}, nil).Times(1)
		syncer.handleUpdatedProxies(ctx)
		Eventually(mockReporter.Reports, "5s", "0.5s").ShouldNot(BeEmpty())
		reportedKey := getMapOnlyKey(mockReporter.Reports())
		Expect(reportedKey).To(Equal(translator.UpstreamToClusterName(vs.GetMetadata().Ref())))
		Expect(mockReporter.Reports()[reportedKey]).To(BeEquivalentTo(errs[vs]))
		m := map[string]*core.Status{
			"*v1.Proxy.test_gloo-system": {State: core.Status_Accepted},
		}
		Eventually(func() map[string]*core.Status { return mockReporter.Statuses()[reportedKey] }, "5s", "0.5s").Should(BeEquivalentTo(m))
	})

	It("should set status correctly when one proxy errors", func() {
		acceptedProxy := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test1", Namespace: "gloo-system"},
		}
		statusClient.SetStatus(acceptedProxy, &core.Status{State: core.Status_Accepted})

		rejectedProxy := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test2", Namespace: "gloo-system"},
		}
		statusClient.SetStatus(rejectedProxy, &core.Status{State: core.Status_Rejected})

		vs := &gatewayv1.VirtualService{}
		errs := reporter.ResourceReports{}
		errs.Accept(vs)

		desiredProxies := reconciler.GeneratedProxies{
			acceptedProxy: errs,
			rejectedProxy: errs,
		}

		syncer.setCurrentProxies(desiredProxies, make(reconciler.InvalidProxies))
		syncer.setStatuses(gloov1.ProxyList{acceptedProxy, rejectedProxy})

		err := syncer.syncStatus(context.Background())
		Expect(err).NotTo(HaveOccurred())

		reportedKey := getMapOnlyKey(mockReporter.Reports())
		Expect(reportedKey).To(Equal(translator.UpstreamToClusterName(vs.GetMetadata().Ref())))
		Expect(mockReporter.Reports()[reportedKey]).To(BeEquivalentTo(errs[vs]))

		m := map[string]*core.Status{
			"*v1.Proxy.test1_gloo-system": {State: core.Status_Accepted},
			"*v1.Proxy.test2_gloo-system": {State: core.Status_Rejected},
		}
		Expect(mockReporter.Statuses()[reportedKey]).To(BeEquivalentTo(m))
	})

	It("should set status correctly when one proxy errors but is irrelevant", func() {
		acceptedProxy := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test", Namespace: "gloo-system"},
		}
		statusClient.SetStatus(acceptedProxy, &core.Status{State: core.Status_Accepted})

		rejectedProxy := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test2", Namespace: "gloo-system"},
		}
		statusClient.SetStatus(rejectedProxy, &core.Status{State: core.Status_Rejected})

		vs := &gatewayv1.VirtualService{}
		errs := reporter.ResourceReports{}
		errs.Accept(vs)

		desiredProxies := reconciler.GeneratedProxies{
			acceptedProxy: errs,
			rejectedProxy: reporter.ResourceReports{},
		}

		syncer.setCurrentProxies(desiredProxies, make(reconciler.InvalidProxies))
		syncer.setStatuses(gloov1.ProxyList{acceptedProxy, rejectedProxy})

		err := syncer.syncStatus(context.Background())
		Expect(err).NotTo(HaveOccurred())

		reportedKey := getMapOnlyKey(mockReporter.Reports())
		Expect(reportedKey).To(Equal(translator.UpstreamToClusterName(vs.GetMetadata().Ref())))
		Expect(mockReporter.Reports()[reportedKey]).To(BeEquivalentTo(errs[vs]))

		m := map[string]*core.Status{
			"*v1.Proxy.test_gloo-system": {State: core.Status_Accepted},
		}
		Expect(mockReporter.Statuses()[reportedKey]).To(BeEquivalentTo(m))
	})

	It("should set status correctly when one proxy errors", func() {
		rejectedProxy1 := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test1", Namespace: "gloo-system"},
		}
		statusClient.SetStatus(rejectedProxy1, &core.Status{State: core.Status_Rejected})

		rejectedProxy2 := &gloov1.Proxy{
			Metadata: &core.Metadata{Name: "test2", Namespace: "gloo-system"},
		}
		statusClient.SetStatus(rejectedProxy2, &core.Status{State: core.Status_Rejected})

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

		syncer.setCurrentProxies(desiredProxies, make(reconciler.InvalidProxies))
		syncer.setStatuses(gloov1.ProxyList{rejectedProxy1, rejectedProxy2})

		err := syncer.syncStatus(context.Background())
		Expect(err).NotTo(HaveOccurred())

		mergedErrs := reporter.ResourceReports{}
		mergedErrs.Accept(vs)
		mergedErrs.AddError(vs, fmt.Errorf("invalid 1"))
		mergedErrs.AddError(vs, fmt.Errorf("invalid 2"))

		reportedKey := getMapOnlyKey(mockReporter.Reports())
		Expect(reportedKey).To(Equal(translator.UpstreamToClusterName(vs.GetMetadata().Ref())))
		Expect(mockReporter.Reports()[reportedKey]).To(BeEquivalentTo(mergedErrs[vs]))

		m := map[string]*core.Status{
			"*v1.Proxy.test2_gloo-system": {State: core.Status_Rejected},
			"*v1.Proxy.test1_gloo-system": {State: core.Status_Rejected},
		}
		Expect(mockReporter.Statuses()[reportedKey]).To(BeEquivalentTo(m))
	})

	Context("translator syncer", func() {
		var (
			mockTranslator *gatewaymocks.MockTranslator
			ctrl           *gomock.Controller

			ctx      context.Context
			settings *gloov1.Settings

			ts    *TranslatorSyncer
			snap  *gloov1snap.ApiSnapshot
			proxy *gloov1.Proxy
		)
		BeforeEach(func() {
			ctrl = gomock.NewController(GinkgoT())
			mockTranslator = gatewaymocks.NewMockTranslator(ctrl)
			settings = &gloov1.Settings{
				Gateway: &gloov1.GatewayOptions{
					CompressedProxySpec: true,
				},
			}
			ctx = context.Background()

			ts = &TranslatorSyncer{
				writeNamespace: "gloo-system",
				translator:     mockTranslator,
			}
			snap = &gloov1snap.ApiSnapshot{
				Gateways: gatewayv1.GatewayList{
					&gatewayv1.Gateway{},
				},
			}
			proxy = &gloov1.Proxy{
				Metadata: &core.Metadata{
					Name: "proxy",
				},
			}
		})
		AfterEach(func() {
			ctrl.Finish()
		})

		It("should compress proxy spec when setttings are set", func() {

			ctx = settingsutil.WithSettings(ctx, settings)

			mockTranslator.EXPECT().Translate(gomock.Any(), "gateway-proxy", snap, gomock.Any()).
				Return(proxy, nil)

			ts.GeneratedDesiredProxies(ctx, snap)

			Expect(proxy.Metadata.Annotations).To(HaveKeyWithValue(compress.CompressedKey, compress.CompressedValue))
		})

		It("should not compress proxy spec when setttings are not set", func() {
			mockTranslator.EXPECT().Translate(gomock.Any(), "gateway-proxy", snap, gomock.Any()).
				Return(proxy, nil)

			ts.GeneratedDesiredProxies(ctx, snap)

			Expect(proxy.Metadata.Annotations).NotTo(HaveKeyWithValue(compress.CompressedKey, compress.CompressedValue))
		})
		It("should truncate proxy status when limit is set", func() {

			mockTranslator.EXPECT().Translate(gomock.Any(), "gateway-proxy", snap, gomock.Any()).
				Return(proxy, nil)
			ts.proxyStatusMaxSize = "5"
			ts.GeneratedDesiredProxies(ctx, snap)
			Expect(proxy.Metadata.Annotations).To(HaveKeyWithValue(compress.ShortenKey, "5"))
		})
		It("should not truncate proxy status when limit is not set", func() {

			mockTranslator.EXPECT().Translate(gomock.Any(), "gateway-proxy", snap, gomock.Any()).
				Return(proxy, nil)
			ts.GeneratedDesiredProxies(ctx, snap)
			Expect(proxy.Metadata.Annotations).NotTo(HaveKey(compress.ShortenKey))
		})
	})

})

type fakeReporter struct {
	reports  map[string]reporter.Report
	statuses map[string]map[string]*core.Status
	lock     sync.Mutex
	Err      error
}

func (f *fakeReporter) Reports() map[string]reporter.Report {
	f.lock.Lock()
	defer f.lock.Unlock()
	return f.reports
}
func (f *fakeReporter) Statuses() map[string]map[string]*core.Status {
	f.lock.Lock()
	defer f.lock.Unlock()
	return f.statuses
}

func (f *fakeReporter) WriteReports(ctx context.Context, errs reporter.ResourceReports, subresourceStatuses map[string]*core.Status) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	fmt.Fprintf(GinkgoWriter, "WriteReports: %#v %#v", errs, subresourceStatuses)
	newreports := map[string]reporter.Report{}
	for k, v := range f.reports {
		newreports[k] = v
	}
	for k, v := range errs {
		newreports[translator.UpstreamToClusterName(k.GetMetadata().Ref())] = v
	}
	f.reports = newreports

	newstatus := map[string]map[string]*core.Status{}
	for k, v := range f.statuses {
		newstatus[k] = v
	}
	for k := range errs {
		newstatus[translator.UpstreamToClusterName(k.GetMetadata().Ref())] = subresourceStatuses
	}
	f.statuses = newstatus

	err := f.Err
	f.Err = nil
	return err
}

func (f *fakeReporter) StatusFromReport(report reporter.Report, subresourceStatuses map[string]*core.Status) *core.Status {
	return &core.Status{}
}
