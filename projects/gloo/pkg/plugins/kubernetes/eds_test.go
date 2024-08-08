package kubernetes

import (
	"context"
	"os"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	kubeplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	kubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	mock_kubernetes "github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes/mocks"
	mock_cache "github.com/solo-io/gloo/test/mocks/cache"
)

var _ = Describe("Eds", func() {
	var (
		controller        *gomock.Controller
		mockCache         *mock_cache.MockKubeCoreCache
		mockSharedFactory *mock_kubernetes.MockKubePluginSharedFactory

		ctx context.Context
	)
	BeforeEach(func() {
		controller = gomock.NewController(GinkgoT())
		mockCache = mock_cache.NewMockKubeCoreCache(controller)
		mockSharedFactory = mock_kubernetes.NewMockKubePluginSharedFactory(controller)
		ctx = context.Background()
		ctx = settingsutil.WithSettings(ctx, &v1.Settings{WatchNamespaces: []string{"foo"}})
	})

	AfterEach(func() {
		controller.Finish()
	})

	It("should ignore upstreams in non watched namesapces", func() {
		up := v1.NewUpstream("foo", "name")
		up.UpstreamType = &v1.Upstream_Kube{
			Kube: &kubev1.UpstreamSpec{
				ServiceName:      "name",
				ServiceNamespace: "bar",
			},
		}
		upstreamsToTrack := v1.UpstreamList{up}

		mockCache.EXPECT().NamespacedServiceLister("bar").Return(nil)

		watcher, err := newEndpointWatcherForUpstreams(func([]string) KubePluginSharedFactory { return mockSharedFactory }, mockCache, upstreamsToTrack, clients.WatchOpts{Ctx: ctx}, nil)
		Expect(err).NotTo(HaveOccurred())
		watcher.List("foo", clients.ListOpts{Ctx: ctx})
		Expect(func() {}).NotTo(Panic())
	})

	It("should default to watchNamespaces if no upstreams exist", func() {
		watchNamespaces := []string{"gloo-system"}
		_, err := newEndpointWatcherForUpstreams(func(namespaces []string) KubePluginSharedFactory {
			Expect(namespaces).To(Equal(watchNamespaces))
			return mockSharedFactory
		},
			mockCache, v1.UpstreamList{}, clients.WatchOpts{Ctx: ctx}, &v1.Settings{WatchNamespaces: watchNamespaces})
		Expect(err).NotTo(HaveOccurred())
	})

	Context("Istio integration", func() {

		It("isIstioInjectionEnabled should respond correctly to ENABLE_ISTIO_SIDECAR_ON_GATEWAY env var", func() {

			os.Setenv(constants.IstioInjectionEnabled, "true")
			istioEnabled, warnsToLog := isIstioInjectionEnabled()
			Expect(istioEnabled).To(BeTrue())
			Expect(warnsToLog).To(HaveLen(1), "expected to have 1 warning")
			Expect(warnsToLog).To(ContainElements(enableIstioSidecarOnGatewayDeprecatedWarning), "expected deprecation warning for enableIstioSidecarOnGateway")

			os.Setenv(constants.IstioInjectionEnabled, "TRUE")
			istioEnabled, warnsToLog = isIstioInjectionEnabled()
			Expect(istioEnabled).To(BeTrue())
			Expect(warnsToLog).To(HaveLen(1), "expected to have 1 warning")
			Expect(warnsToLog).To(ContainElements(enableIstioSidecarOnGatewayDeprecatedWarning), "expected deprecation warning for enableIstioSidecarOnGateway")

			os.Unsetenv(constants.IstioInjectionEnabled)
			istioEnabled, warnsToLog = isIstioInjectionEnabled()
			Expect(istioEnabled).To(BeFalse())
			Expect(warnsToLog).To(BeEmpty(), "expected to have no warning")

			os.Setenv(constants.IstioInjectionEnabled, "false")
			istioEnabled, warnsToLog = isIstioInjectionEnabled()
			Expect(istioEnabled).To(BeFalse())
			Expect(warnsToLog).To(BeEmpty(), "expected to have no warning")

		})

		It("should translate EDS metadata", func() {
			writeNamespace := "foo"
			up := v1.NewUpstream(writeNamespace, "name")
			up.UpstreamType = &v1.Upstream_Kube{
				Kube: &kubev1.UpstreamSpec{
					ServiceName:      "bar",
					ServiceNamespace: "foo",
					ServicePort:      9080,
					Selector:         map[string]string{"app": "bar"},
				},
			}

			endpoints, warnsToLog, errorsToLog := FilterEndpoints(ctx, // do not use for logging! return logging messages as strings and log them after hashing (see https://github.com/solo-io/gloo/issues/3761)
				writeNamespace,
				[]*corev1.Endpoints{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "bar",
							Namespace: "foo",
						},
						Subsets: []corev1.EndpointSubset{
							{
								Ports: []corev1.EndpointPort{
									{
										Port:     9080,
										Name:     "http",
										Protocol: "TCP",
									},
								},
								Addresses: []corev1.EndpointAddress{
									{
										IP: "10.244.0.14",
										TargetRef: &corev1.ObjectReference{
											Kind:      "Pod",
											Name:      "bar-7d4d7c7b4b-4z5zv",
											Namespace: "foo",
										},
									},
								},
							},
						},
					},
				},
				[]*corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "bar",
							Namespace: "foo",
						},
						Spec: corev1.ServiceSpec{
							Selector: map[string]string{"app": "bar"},
							Ports: []corev1.ServicePort{
								{
									Name:     "http",
									Port:     9080,
									Protocol: "TCP",
									TargetPort: intstr.IntOrString{
										Type:   intstr.Int,
										IntVal: 9080,
									},
								},
							},
						},
					},
				},
				[]*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "bar-7d4d7c7b4b-4z5zv",
							Namespace: "foo",
							Labels: map[string]string{
								"app":                       "bar",
								"security.istio.io/tlsMode": "istio",
							},
						},
						Spec: corev1.PodSpec{},
					},
				},
				map[*core.ResourceRef]*kubeplugin.UpstreamSpec{
					up.Metadata.Ref(): up.GetKube(),
				})

			Expect(endpoints).To(HaveLen(1), "expected to have 1 endpoint")
			Expect(warnsToLog).To(BeEmpty(), "expected no warnings")
			Expect(errorsToLog).To(BeEmpty(), "expected no errors")

			// Check endpoint has automtls metadata
			Expect(endpoints[0].Metadata.Labels).To(HaveKeyWithValue(constants.IstioTlsModeLabel, "istio"))
		})
	})

})
