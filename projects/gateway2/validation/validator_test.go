package validation_test

import (
	"context"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/utils/statusutils"
	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gateway2/controller/scheme"
	"github.com/solo-io/gloo/projects/gateway2/extensions"
	"github.com/solo-io/gloo/projects/gateway2/proxy_syncer"
	gwquery "github.com/solo-io/gloo/projects/gateway2/query"
	rtoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/routeoptions/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"
	"github.com/solo-io/gloo/projects/gateway2/validation"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	envoybuffer "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/buffer/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauth "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/faultinjection"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/sanitizer"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	mock_consul "github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul/mocks"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	validationutils "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	gloovalidation "github.com/solo-io/gloo/projects/gloo/pkg/validation"
	"github.com/solo-io/gloo/test/samples"
	corev1 "github.com/solo-io/skv2/pkg/api/core.skv2.solo.io/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"github.com/solo-io/solo-kit/test/matchers"

	k8scorev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("RouteOptionsPlugin", func() {
	var (
		ctx               context.Context
		cancel            context.CancelFunc
		sch               *runtime.Scheme
		authConfigClient  extauth.AuthConfigClient
		routeOptionClient sologatewayv1.RouteOptionClient
		statusReporter    reporter.StatusReporter
		inputChannels     *proxy_syncer.GatewayInputChannels

		ctrl              *gomock.Controller
		settings          *v1.Settings
		registeredPlugins []plugins.Plugin
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		sch = scheme.NewScheme()
		resourceClientFactory := &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}

		routeOptionClient, _ = sologatewayv1.NewRouteOptionClient(ctx, resourceClientFactory)
		authConfigClient, _ = extauth.NewAuthConfigClient(ctx, resourceClientFactory)
		statusClient := statusutils.GetStatusClientForNamespace("gloo-system")

		statusReporter = reporter.NewReporter("gloo-kube-gateway", statusClient, routeOptionClient.BaseClient())

		inputChannels = proxy_syncer.NewGatewayInputChannels()

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
		registeredPlugins = registry.Plugins(opts)
	})

	AfterEach(func() {
		cancel()
	})

	FIt("validates a RouteOption with a dummy proxy", func() {
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
		vc := gloovalidation.ValidatorConfig{
			Ctx: context.Background(),
			GlooValidatorConfig: gloovalidation.GlooValidatorConfig{
				XdsSanitizer: xdsSanitizer,
				Translator:   translator,
			},
		}
		gv := gloovalidation.NewValidator(vc)

		deps := []client.Object{svc(), gw(), httpRoute(), attachedRouteOption()}
		routeOptionClient.Write(attachedInternal(), clients.WriteOpts{})
		fakeClient := testutils.BuildIndexedFakeClient(deps, gwquery.IterateIndices, rtoptquery.IterateIndices)
		gwQueries := testutils.BuildGatewayQueriesWithClient(fakeClient)

		k8sGwExtensions, _ := extensions.NewK8sGatewayExtensions(ctx, extensions.K8sGatewayExtensionsFactoryParameters{
			Cl:                fakeClient,
			Scheme:            sch,
			AuthConfigClient:  authConfigClient,
			RouteOptionClient: routeOptionClient,
			StatusReporter:    statusReporter,
			KickXds:           inputChannels.Kick,
		})

		rtOpt := attachedInternal()
		validator := validation.ValidationHelper{
			K8sGwExtensions: k8sGwExtensions,
			GatewayQueries:  gwQueries,
			Cl:              fakeClient,
		}

		params := plugins.Params{
			Ctx:      context.Background(),
			Snapshot: samples.SimpleGlooSnapshot("gloo-system"),
		}
		proxies, _ := validator.TranslateK8sGatewayProxies(ctx, params.Snapshot, rtOpt)
		params.Snapshot.Proxies = proxies
		gv.Sync(ctx, params.Snapshot)
		rpt, err := gv.ValidateGloo(ctx, proxies[0], rtOpt, false)
		Expect(err).NotTo(HaveOccurred())
		r := rpt[0]
		Expect(r.Proxy).To(Equal(proxies[0]))
		Expect(r.ResourceReports).To(Equal(reporter.ResourceReports{}))
		Expect(r.ProxyReport).To(matchers.MatchProto(validationutils.MakeReport(proxies[0])))

		vhost := attachedVHostInternal()
		proxies, _ = validator.TranslateK8sGatewayProxies(ctx, params.Snapshot, vhost)
		params.Snapshot.Proxies = proxies
		gv.Sync(ctx, params.Snapshot)
		rpt, err = gv.ValidateGloo(ctx, proxies[0], vhost, false)
		Expect(err).NotTo(HaveOccurred())
		r = rpt[0]
		Expect(r.Proxy).To(Equal(proxies[0]))
		Expect(r.ResourceReports).To(Equal(reporter.ResourceReports{}))
		Expect(r.ProxyReport).To(matchers.MatchProto(validationutils.MakeReport(proxies[0])))
	})

	// It("validates a RouteOption", func() {
	// 	deps := []client.Object{svc(), gw(), httpRoute(), attachedRouteOption()}
	// 	routeOptionClient.Write(attachedInternal(), clients.WriteOpts{})
	// 	fakeClient := testutils.BuildIndexedFakeClient(deps, gwquery.IterateIndices, rtoptquery.IterateIndices)
	// 	gwQueries := testutils.BuildGatewayQueriesWithClient(fakeClient)

	// 	k8sGwExtensions, _ := extensions.NewK8sGatewayExtensions(ctx, extensions.K8sGatewayExtensionsFactoryParameters{
	// 		Cl:                fakeClient,
	// 		Scheme:            sch,
	// 		AuthConfigClient:  authConfigClient,
	// 		RouteOptionClient: routeOptionClient,
	// 		StatusReporter:    statusReporter,
	// 		KickXds:           inputChannels.Kick,
	// 	})

	// 	rtOpt := attachedInternal()
	// 	validator := validation.ValidationHelper{
	// 		K8sGwExtensions: k8sGwExtensions,
	// 		GatewayQueries:  gwQueries,
	// 		Cl:              fakeClient,
	// 	}
	// 	validator.TranslateK8sGatewayProxies(ctx, rtOpt)
	// })
})

func nsPtr(s string) *gwv1.Namespace {
	var ns gwv1.Namespace = gwv1.Namespace(s)
	return &ns
}

func portNumPtr(n int32) *gwv1.PortNumber {
	var pn gwv1.PortNumber = gwv1.PortNumber(n)
	return &pn
}

func svc() *k8scorev1.Service {
	return &k8scorev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "my-svc",
		},
		Spec: k8scorev1.ServiceSpec{
			Ports: []k8scorev1.ServicePort{
				{
					Port: 8080,
				},
			},
		},
	}
}

func httpRoute() *gwv1.HTTPRoute {
	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-route",
			Namespace: "default",
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name:      "my-gw",
						Namespace: nsPtr("default"),
					},
				},
			},
			Hostnames: []gwv1.Hostname{
				gwv1.Hostname("example.com"),
			},
			Rules: []gwv1.HTTPRouteRule{
				{
					BackendRefs: []gwv1.HTTPBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: "my-svc",
									Port: portNumPtr(8080),
								},
							},
						},
					},
				},
			},
		},
	}
}

func gw() *gwv1.Gateway {
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-gw",
			Namespace: "default",
		},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{
					Name:     "my-http-listener",
					Port:     gwv1.PortNumber(8080),
					Protocol: gwv1.HTTPProtocolType,
					// AllowedRoutes: &gwv1.AllowedRoutes{
					// 	Namespaces: &gwv1.RouteNamespaces{
					// 		gw
					// 	},
					// },
				},
			},
		},
	}
}

func attachedVirtualHostOption() *solokubev1.VirtualHostOption {
	return &solokubev1.VirtualHostOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "policy",
			Namespace: "default",
		},
		Spec: *attachedVHostInternal(),
	}
}

func attachedVHostInternal() *sologatewayv1.VirtualHostOption {
	return &sologatewayv1.VirtualHostOption{
		TargetRef: &corev1.PolicyTargetReferenceWithSectionName{
			Group:     gwv1.GroupVersion.Group,
			Kind:      wellknown.GatewayKind,
			Name:      "gw",
			Namespace: wrapperspb.String("default"),
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

func attachedRouteOption() *solokubev1.RouteOption {
	now := metav1.Now()
	return &solokubev1.RouteOption{
		TypeMeta: metav1.TypeMeta{
			Kind: sologatewayv1.RouteOptionGVK.Kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "policy",
			Namespace:         "default",
			CreationTimestamp: now,
		},
		Spec: *attachedInternal(),
	}
}

func attachedInternal() *sologatewayv1.RouteOption {
	return &sologatewayv1.RouteOption{
		Metadata: &core.Metadata{
			Name:      "policy",
			Namespace: "default",
		},
		TargetRef: &corev1.PolicyTargetReference{
			Group:     gwv1.GroupVersion.Group,
			Kind:      wellknown.HTTPRouteKind,
			Name:      "my-route",
			Namespace: wrapperspb.String("default"),
		},
		Options: &v1.RouteOptions{
			Faults: &faultinjection.RouteFaults{
				Abort: &faultinjection.RouteAbort{
					Percentage: 4.19,
					HttpStatus: 500,
				},
			},
		},
	}
}
