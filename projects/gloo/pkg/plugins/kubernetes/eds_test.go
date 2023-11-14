package kubernetes

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	kubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	mock_kubernetes "github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes/mocks"
	mock_cache "github.com/solo-io/gloo/test/mocks/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"go.uber.org/mock/gomock"
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

		watcher, err := newEndpointWatcherForUpstreams(func([]string) KubePluginSharedFactory { return mockSharedFactory }, mockCache, "foo", upstreamsToTrack, clients.WatchOpts{Ctx: ctx}, nil)
		Expect(err).NotTo(HaveOccurred())
		watcher.List("foo", clients.ListOpts{Ctx: ctx})
		Expect(func() {}).NotTo(Panic())

	})

	Context("Istio integration", func() {

		It("isIstioIntegrationEnabled should respond correctly to ENABLE_ISTIO_INTEGRATION env var", func() {

			os.Setenv("ENABLE_ISTIO_INTEGRATION", "true")
			Expect(isIstioIntegrationEnabled()).To(BeTrue())

			os.Setenv("ENABLE_ISTIO_INTEGRATION", "TRUE")
			Expect(isIstioIntegrationEnabled()).To(BeTrue())

			os.Unsetenv("ENABLE_ISTIO_INTEGRATION")
			Expect(isIstioIntegrationEnabled()).To(BeFalse())

			os.Setenv("ENABLE_ISTIO_INTEGRATION", "false")
			Expect(isIstioIntegrationEnabled()).To(BeFalse())
		})
	})

})
