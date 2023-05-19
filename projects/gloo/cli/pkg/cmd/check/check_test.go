package check_test

import (
	"context"
	"fmt"

	gatewaysoloiov1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Check", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		helpers.UseMemoryClients()
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
	})

	Context("glooctl check", func() {
		It("should error if resource has no status", func() {

			client := helpers.MustKubeClient()
			client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: defaults.GlooSystem,
				},
			}, metav1.CreateOptions{})

			appName := "default"
			client.AppsV1().Deployments("gloo-system").Create(ctx, &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      appName,
					Namespace: "gloo-system",
					Labels: map[string]string{
						"gloo": "gloo",
					},
				},
				Spec: appsv1.DeploymentSpec{},
			}, metav1.CreateOptions{})

			helpers.MustNamespacedSettingsClient(ctx, "gloo-system").Write(&v1.Settings{
				Metadata: &core.Metadata{
					Name:      "default",
					Namespace: "gloo-system",
				},
			}, clients.WriteOpts{})

			noStatusUpstream := &v1.Upstream{
				Metadata: &core.Metadata{
					Name:      "some-warning-upstream",
					Namespace: "gloo-system",
				},
			}
			_, usErr := helpers.MustNamespacedUpstreamClient(ctx, "gloo-system").Write(noStatusUpstream, clients.WriteOpts{})
			Expect(usErr).NotTo(HaveOccurred())

			noStatusUpstreamGroup := &v1.UpstreamGroup{
				Metadata: &core.Metadata{
					Name:      "some-warning-upstream-group",
					Namespace: "gloo-system",
				},
			}
			_, usgErr := helpers.MustNamespacedUpstreamGroupClient(ctx, "gloo-system").Write(noStatusUpstreamGroup, clients.WriteOpts{})
			Expect(usgErr).NotTo(HaveOccurred())

			noStatusAuthConfig := &extauthv1.AuthConfig{
				Metadata: &core.Metadata{
					Name:      "some-warning-auth-config",
					Namespace: "gloo-system",
				},
			}
			_, acErr := helpers.MustNamespacedAuthConfigClient(ctx, "gloo-system").Write(noStatusAuthConfig, clients.WriteOpts{})
			Expect(acErr).NotTo(HaveOccurred())

			noStatusVHO := &gatewaysoloiov1.VirtualHostOption{
				Metadata: &core.Metadata{
					Name:      "some-warning-virtual-host-option",
					Namespace: "gloo-system",
				},
			}
			_, vhoErr := helpers.MustNamespacedVirtualHostOptionClient(ctx, "gloo-system").Write(noStatusVHO, clients.WriteOpts{})
			Expect(vhoErr).NotTo(HaveOccurred())

			noStatusRouteOption := &gatewaysoloiov1.RouteOption{
				Metadata: &core.Metadata{
					Name:      "some-warning-route-option",
					Namespace: "gloo-system",
				},
			}
			_, roErr := helpers.MustNamespacedRouteOptionClient(ctx, "gloo-system").Write(noStatusRouteOption, clients.WriteOpts{})
			Expect(roErr).NotTo(HaveOccurred())

			noStatusVS := &gatewaysoloiov1.VirtualService{
				Metadata: &core.Metadata{
					Name:      "some-warning-virtual-service",
					Namespace: "gloo-system",
				},
			}
			_, vsErr := helpers.MustNamespacedVirtualServiceClient(ctx, "gloo-system").Write(noStatusVS, clients.WriteOpts{})
			Expect(vsErr).NotTo(HaveOccurred())

			noStatusGateway := &gatewaysoloiov1.Gateway{
				Metadata: &core.Metadata{
					Name:      "some-warning-gateway",
					Namespace: "gloo-system",
				},
			}
			_, gwErr := helpers.MustNamespacedGatewayClient(ctx, "gloo-system").Write(noStatusGateway, clients.WriteOpts{})
			Expect(gwErr).NotTo(HaveOccurred())

			noStatusProxy := &v1.Proxy{
				Metadata: &core.Metadata{
					Name:      "some-warning-proxy",
					Namespace: "gloo-system",
				},
			}
			_, proxyErr := helpers.MustNamespacedProxyClient(ctx, "gloo-system").Write(noStatusProxy, clients.WriteOpts{})
			Expect(proxyErr).NotTo(HaveOccurred())

			_, err := testutils.GlooctlOut("check -x xds-metrics")
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("Found upstream with no status: %s %s", noStatusUpstream.GetMetadata().GetNamespace(), noStatusUpstream.GetMetadata().GetName())))
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("Found upstream group with no status: %s %s", noStatusUpstreamGroup.GetMetadata().GetNamespace(), noStatusUpstreamGroup.GetMetadata().GetName())))
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("Found auth config with no status: %s %s", noStatusAuthConfig.GetMetadata().GetNamespace(), noStatusAuthConfig.GetMetadata().GetName())))
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("Found VirtualHostOption with no status: %s %s", noStatusVHO.GetMetadata().GetNamespace(), noStatusVHO.GetMetadata().GetName())))
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("Found RouteOption with no status: %s %s", noStatusRouteOption.GetMetadata().GetNamespace(), noStatusRouteOption.GetMetadata().GetName())))
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("Found gateway with no status: %s %s", noStatusGateway.GetMetadata().GetNamespace(), noStatusGateway.GetMetadata().GetName())))
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("Found virtual service with no status: %s %s", noStatusVS.GetMetadata().GetNamespace(), noStatusVS.GetMetadata().GetName())))
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("Found proxy with no status: %s %s", noStatusProxy.GetMetadata().GetNamespace(), noStatusProxy.GetMetadata().GetName())))
		})
	})
})
