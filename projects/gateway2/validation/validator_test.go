package validation_test

import (
	"context"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/client-go/kubernetes/fake"

	. "github.com/onsi/gomega"

	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway2/validation"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	envoybuffer "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/buffer/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/faultinjection"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/sanitizer"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	mock_consul "github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul/mocks"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	gloovalidation "github.com/solo-io/gloo/projects/gloo/pkg/validation"
	"github.com/solo-io/gloo/test/samples"
	corev1 "github.com/solo-io/skv2/pkg/api/core.skv2.solo.io/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("Kube Gateway API Policy Validation Helper", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc

		ctrl     *gomock.Controller
		settings *v1.Settings
		vc       gloovalidation.ValidatorConfig
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		resourceClientFactory := &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}

		ctrl = gomock.NewController(T)
		kube := fake.NewSimpleClientset()
		kubeCoreCache, err := corecache.NewKubeCoreCache(context.Background(), kube)
		Expect(err).NotTo(HaveOccurred())

		opts := bootstrap.Opts{
			Settings:  settings,
			Secrets:   resourceClientFactory,
			Upstreams: resourceClientFactory,
			Consul: bootstrap.Consul{
				ConsulWatcher: mock_consul.NewMockConsulWatcher(ctrl), // just needed to activate the consul plugin
			},
			KubeClient:    kube,
			KubeCoreCache: kubeCoreCache,
		}
		registeredPlugins := registry.Plugins(opts)
		routeReplacingSanitizer, _ := sanitizer.NewRouteReplacingSanitizer(settings.GetGloo().GetInvalidConfigPolicy())
		xdsSanitizer := sanitizer.XdsSanitizers{
			sanitizer.NewUpstreamRemovingSanitizer(),
			routeReplacingSanitizer,
		}

		pluginRegistry := registry.NewPluginRegistry(registeredPlugins)

		translator := translator.NewTranslatorWithHasher(
			utils.NewSslConfigTranslator(),
			settings,
			pluginRegistry,
			translator.EnvoyCacheResourcesListToFnvHash,
		)
		vc = gloovalidation.ValidatorConfig{
			Ctx: context.Background(),
			GlooValidatorConfig: gloovalidation.GlooValidatorConfig{
				XdsSanitizer: xdsSanitizer,
				Translator:   translator,
				Settings:     settings,
			},
		}
	})

	AfterEach(func() {
		cancel()
	})

	It("validates and rejects a bad RouteOption", func() {
		gv := gloovalidation.NewValidator(vc)

		rtOpt := routeOptWithBadConfig()
		params := plugins.Params{
			Ctx:      ctx,
			Snapshot: samples.SimpleGlooSnapshot("gloo-system"),
		}
		proxies, _ := validation.TranslateK8sGatewayProxies(ctx, params.Snapshot, rtOpt)
		gv.Sync(ctx, params.Snapshot)
		rpt, err := gv.ValidateGloo(ctx, proxies[0], rtOpt, false)
		Expect(err).NotTo(HaveOccurred())
		err = validation.GetSimpleErrorFromGlooValidation(rpt, proxies[0])
		Expect(err).To(HaveOccurred())
		const faultErrorMsg = "Route Error: ProcessingError. Reason: *faultinjection.plugin: invalid abort status code '0', must be in range of [200,600)."
		Expect(err.Error()).To(ContainSubstring(faultErrorMsg))
		r := rpt[0]
		proxyResourceReport := r.ResourceReports[proxies[0]]
		Expect(proxyResourceReport.Errors.Error()).To(ContainSubstring(faultErrorMsg))
	})

	It("validates and accepts a good RouteOption", func() {
		gv := gloovalidation.NewValidator(vc)

		rtOpt := routeOptWithGoodConfig()
		params := plugins.Params{
			Ctx:      ctx,
			Snapshot: samples.SimpleGlooSnapshot("gloo-system"),
		}
		proxies, _ := validation.TranslateK8sGatewayProxies(ctx, params.Snapshot, rtOpt)
		gv.Sync(ctx, params.Snapshot)
		rpt, err := gv.ValidateGloo(ctx, proxies[0], rtOpt, false)
		Expect(err).NotTo(HaveOccurred())
		err = validation.GetSimpleErrorFromGlooValidation(rpt, proxies[0])
		Expect(err).NotTo(HaveOccurred())
		r := rpt[0]
		proxyResourceReport := r.ResourceReports[proxies[0]]
		Expect(proxyResourceReport.Errors).NotTo(HaveOccurred())
	})

	It("validates and a rejects a bad VirtualHostOption", func() {
		gv := gloovalidation.NewValidator(vc)

		params := plugins.Params{
			Ctx:      ctx,
			Snapshot: samples.SimpleGlooSnapshot("gloo-system"),
		}
		vhost := vHostOptWithBadConfig()
		proxies, _ := validation.TranslateK8sGatewayProxies(ctx, params.Snapshot, vhost)
		gv.Sync(ctx, params.Snapshot)
		rpt, err := gv.ValidateGloo(ctx, proxies[0], vhost, false)
		Expect(err).NotTo(HaveOccurred())
		err = validation.GetSimpleErrorFromGlooValidation(rpt, proxies[0])
		Expect(err).To(HaveOccurred())
		const bufferErrorMsg = "VirtualHost Error: ProcessingError. Reason: invalid virtual host [vhost] while processing plugin buffer: invalid BufferPerRoute.Buffer: embedded message failed validation | caused by: invalid Buffer.MaxRequestBytes: value is required and must not be nil."
		Expect(err.Error()).To(ContainSubstring(bufferErrorMsg))
		r := rpt[0]
		proxyResourceReport := r.ResourceReports[proxies[0]]
		Expect(proxyResourceReport.Errors.Error()).To(ContainSubstring(bufferErrorMsg))
	})

	It("validates and accepts a good VirtualHostOption", func() {
		gv := gloovalidation.NewValidator(vc)

		params := plugins.Params{
			Ctx:      ctx,
			Snapshot: samples.SimpleGlooSnapshot("gloo-system"),
		}
		vhost := vHostOptWithGoodConfig()
		proxies, _ := validation.TranslateK8sGatewayProxies(ctx, params.Snapshot, vhost)
		gv.Sync(ctx, params.Snapshot)
		rpt, err := gv.ValidateGloo(ctx, proxies[0], vhost, false)
		Expect(err).NotTo(HaveOccurred())
		err = validation.GetSimpleErrorFromGlooValidation(rpt, proxies[0])
		Expect(err).ToNot(HaveOccurred())
		r := rpt[0]
		proxyResourceReport := r.ResourceReports[proxies[0]]
		Expect(proxyResourceReport.Errors).NotTo(HaveOccurred())
	})
})

func vHostOptWithBadConfig() *sologatewayv1.VirtualHostOption {
	return &sologatewayv1.VirtualHostOption{
		TargetRefs: []*corev1.PolicyTargetReferenceWithSectionName{
			{
				Group:     gwv1.GroupVersion.Group,
				Kind:      wellknown.GatewayKind,
				Name:      "gw",
				Namespace: wrapperspb.String("default"),
			},
		},
		Options: &v1.VirtualHostOptions{
			BufferPerRoute: &envoybuffer.BufferPerRoute{
				Override: &envoybuffer.BufferPerRoute_Buffer{
					Buffer: &envoybuffer.Buffer{
						MaxRequestBytes: nil,
					},
				},
			},
		},
	}
}

func vHostOptWithGoodConfig() *sologatewayv1.VirtualHostOption {
	vHostOpt := proto.Clone(vHostOptWithBadConfig()).(*sologatewayv1.VirtualHostOption)
	vHostOpt.GetOptions().GetBufferPerRoute().GetBuffer().MaxRequestBytes = wrapperspb.UInt32(1024)
	return vHostOpt
}

func routeOptWithBadConfig() *sologatewayv1.RouteOption {
	return &sologatewayv1.RouteOption{
		Metadata: &core.Metadata{
			Name:      "policy",
			Namespace: "default",
		},
		TargetRefs: []*corev1.PolicyTargetReference{
			{
				Group:     gwv1.GroupVersion.Group,
				Kind:      wellknown.HTTPRouteKind,
				Name:      "my-route",
				Namespace: wrapperspb.String("default"),
			},
		},
		Options: &v1.RouteOptions{
			Faults: &faultinjection.RouteFaults{
				Abort: &faultinjection.RouteAbort{
					Percentage: 4.19,
				},
			},
		},
	}
}

func routeOptWithGoodConfig() *sologatewayv1.RouteOption {
	rtOpt := proto.Clone(routeOptWithBadConfig()).(*sologatewayv1.RouteOption)
	rtOpt.GetOptions().GetFaults().GetAbort().HttpStatus = 500
	return rtOpt
}
