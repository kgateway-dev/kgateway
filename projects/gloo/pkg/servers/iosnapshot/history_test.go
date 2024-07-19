package iosnapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/onsi/gomega/gstruct"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaykubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/controller/scheme"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	"github.com/solo-io/gloo/projects/gloo/api/external/solo/ratelimit"
	envoycorev3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	ratelimitv1alpha1 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	extauthkubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1/kube/apis/enterprise.gloo.solo.io/v1"
	graphqlv1beta1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/graphql/v1beta1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/cors"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	rlv1alpha1 "github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"
	crdv1 "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/solo.io/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
	apiv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

var (
	deploymentGvk = schema.GroupVersionKind{
		Group:   appsv1.GroupName,
		Version: "v1",
		Kind:    "Deployment",
	}
)

var _ = Describe("History", func() {

	var (
		ctx context.Context

		clientBuilder *fake.ClientBuilder
		xdsCache      cache.SnapshotCache
		history       History
	)

	BeforeEach(func() {
		ctx = context.Background()

		scheme := scheme.NewScheme()
		additionalSchemes := []func(s *runtime.Scheme) error{
			extauthkubev1.AddToScheme,
			rlv1alpha1.AddToScheme,
		}
		for _, add := range additionalSchemes {
			err := add(scheme)
			Expect(err).NotTo(HaveOccurred())
		}

		clientBuilder = fake.NewClientBuilder().WithScheme(scheme)
		xdsCache = &xds.MockXdsCache{}
		history = NewHistory(xdsCache,
			&v1.Settings{
				Metadata: &core.Metadata{
					Name:      "my-settings",
					Namespace: defaults.GlooSystem,
				},
			},
			KubeGatewayDefaultGVKs,
		)
	})

	Context("GetInputSnapshot", func() {

		It("Includes Settings", func() {
			// Settings CR is not part of the APISnapshot, but should be returned by the input snapshot endpoint

			returnedResources := getTypedInputSnapshot(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.HaveTypeMeta(v1.SettingsGVK),
				matchers.HaveObjectMeta(types.NamespacedName{
					Namespace: defaults.GlooSystem,
					// This matches the name of the Settings resource that we construct the History object with
					Name: "my-settings",
				}),
				gstruct.Ignore(),
			), "returned resources include Settings")
		})

		It("Includes Endpoints", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				Endpoints: v1.EndpointList{
					{
						Metadata: &core.Metadata{
							Name:      "ep-snap",
							Namespace: defaults.GlooSystem,
						},
						Address: "2.3.4.5",
						Upstreams: []*core.ResourceRef{
							{
								Name:      "us1",
								Namespace: "ns1",
							},
						},
					},
				},
			})

			returnedResources := getTypedInputSnapshot(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.HaveTypeMeta(v1.EndpointGVK),
				matchers.HaveObjectMeta(types.NamespacedName{
					Namespace: defaults.GlooSystem,
					Name:      "ep-snap",
				}),
				gstruct.Ignore(),
			), "returned resources include endpoints")
		})

		It("Excludes Secrets", func() {
			// TODO: We want to update the implementation to include secrets, but redact the contents of them

			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				Secrets: v1.SecretList{{
					Metadata: &core.Metadata{Name: "secret", Namespace: defaults.GlooSystem},
				}},
			})

			returnedResources := getTypedInputSnapshot(ctx, history)
			Expect(returnedResources).NotTo(matchers.ContainCustomResource(
				matchers.HaveTypeMeta(v1.SecretGVK),
				gstruct.Ignore(),
				gstruct.Ignore(),
			), "returned resources exclude secrets")
		})

		It("Excludes Artifacts", func() {
			// TODO: We want to update the implementation to include artifacts, but redact the contents of them

			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				Artifacts: v1.ArtifactList{
					{Metadata: &core.Metadata{Name: "artifact", Namespace: defaults.GlooSystem}},
				},
			})

			returnedResources := getTypedInputSnapshot(ctx, history)
			Expect(returnedResources).NotTo(matchers.ContainCustomResource(
				matchers.HaveTypeMeta(v1.ArtifactGVK),
				gstruct.Ignore(),
				gstruct.Ignore(),
			), "returned resources exclude artifacts")
		})

		It("Includes UpstreamGroups", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				UpstreamGroups: v1.UpstreamGroupList{
					{
						Metadata: &core.Metadata{
							Name:      "ug-snap",
							Namespace: defaults.GlooSystem,
						},
						Destinations: []*v1.WeightedDestination{
							{
								Destination: &v1.Destination{
									DestinationType: &v1.Destination_Kube{
										Kube: &v1.KubernetesServiceDestination{
											Ref: &core.ResourceRef{
												Name:      "dest",
												Namespace: "ns",
											},
										},
									},
								},
							},
						},
					},
				},
			})

			returnedResources := getTypedInputSnapshot(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.HaveTypeMeta(v1.UpstreamGroupGVK),
				matchers.HaveObjectMeta(types.NamespacedName{
					Namespace: defaults.GlooSystem,
					Name:      "ug-snap",
				}),
				gstruct.Ignore(),
			), "returned resources include upstream groups")
		})

		It("Includes Upstreams", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				Upstreams: v1.UpstreamList{
					{
						Metadata: &core.Metadata{
							Name:      "us-snap",
							Namespace: defaults.GlooSystem,
						},
						UpstreamType: &v1.Upstream_Kube{
							Kube: &kubernetes.UpstreamSpec{
								ServiceName:      "svc",
								ServiceNamespace: "ns",
								ServicePort:      uint32(50),
							},
						},
						DiscoveryMetadata: &v1.DiscoveryMetadata{
							Labels: map[string]string{
								"key": "val",
							},
						},
					},
				},
			})

			returnedResources := getTypedInputSnapshot(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.HaveTypeMeta(v1.UpstreamGVK),
				matchers.HaveObjectMeta(types.NamespacedName{
					Namespace: defaults.GlooSystem,
					Name:      "us-snap",
				}),
				gstruct.Ignore(),
			), "returned resources include upstreams")
		})

		It("Includes AuthConfigs", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				AuthConfigs: extauthv1.AuthConfigList{
					{
						Metadata: &core.Metadata{
							Name:      "ac-snap",
							Namespace: defaults.GlooSystem,
						},
						Configs: []*extauthv1.AuthConfig_Config{{
							AuthConfig: &extauthv1.AuthConfig_Config_Oauth{},
						}},
					},
				},
			})

			returnedResources := getTypedInputSnapshot(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.HaveTypeMeta(extauthv1.AuthConfigGVK),
				matchers.HaveObjectMeta(types.NamespacedName{
					Namespace: defaults.GlooSystem,
					Name:      "ac-snap",
				}),
				gstruct.Ignore(),
			), "returned resources include auth configs")
		})

		It("Includes RateLimitConfigs", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				Ratelimitconfigs: ratelimitv1alpha1.RateLimitConfigList{
					{
						RateLimitConfig: ratelimit.RateLimitConfig{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "rlc-snap",
								Namespace: defaults.GlooSystem,
							},
							Spec: rlv1alpha1.RateLimitConfigSpec{
								ConfigType: &rlv1alpha1.RateLimitConfigSpec_Raw_{
									Raw: &rlv1alpha1.RateLimitConfigSpec_Raw{
										Descriptors: []*rlv1alpha1.Descriptor{{
											Key:   "generic_key",
											Value: "foo",
											RateLimit: &rlv1alpha1.RateLimit{
												Unit:            rlv1alpha1.RateLimit_MINUTE,
												RequestsPerUnit: 1,
											},
										}},
										RateLimits: []*rlv1alpha1.RateLimitActions{{
											Actions: []*rlv1alpha1.Action{{
												ActionSpecifier: &rlv1alpha1.Action_GenericKey_{
													GenericKey: &rlv1alpha1.Action_GenericKey{
														DescriptorValue: "foo",
													},
												},
											}},
										}},
									},
								},
							},
						},
					},
				},
			})

			returnedResources := getTypedInputSnapshot(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.HaveTypeMeta(ratelimitv1alpha1.RateLimitConfigGVK),
				matchers.HaveObjectMeta(types.NamespacedName{
					Namespace: defaults.GlooSystem,
					Name:      "rlc-snap",
				}),
				gstruct.Ignore(),
			), "returned resources include rate limit configs")
		})

		It("Includes VirtualServices", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				VirtualServices: gatewayv1.VirtualServiceList{
					{
						Metadata: &core.Metadata{
							Name:      "vs-snap",
							Namespace: defaults.GlooSystem,
						},
						VirtualHost: &gatewayv1.VirtualHost{
							Domains: []string{"x", "y", "z"},
						},
					},
				},
			})

			returnedResources := getTypedInputSnapshot(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.HaveTypeMeta(gatewayv1.VirtualServiceGVK),
				matchers.HaveObjectMeta(types.NamespacedName{
					Namespace: defaults.GlooSystem,
					Name:      "vs-snap",
				}),
				gstruct.Ignore(),
			), "returned resources include virtual services")
		})

		It("Includes RouteTables", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				RouteTables: gatewayv1.RouteTableList{
					{
						Metadata: &core.Metadata{
							Name:      "rt-snap",
							Namespace: defaults.GlooSystem,
						},
						Routes: []*gatewayv1.Route{
							{
								Action: &gatewayv1.Route_DelegateAction{
									DelegateAction: &gatewayv1.DelegateAction{
										Name:      "a",
										Namespace: "b",
									},
								},
							},
						},
					},
				},
			})

			returnedResources := getTypedInputSnapshot(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.HaveTypeMeta(gatewayv1.RouteTableGVK),
				matchers.HaveObjectMeta(types.NamespacedName{
					Namespace: defaults.GlooSystem,
					Name:      "rt-snap",
				}),
				gstruct.Ignore(),
			), "returned resources include route tables")
		})

		It("Includes Gateways (Edge API)", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				Gateways: gatewayv1.GatewayList{
					{
						Metadata: &core.Metadata{
							Name:      "gw-snap",
							Namespace: defaults.GlooSystem,
						},
						BindAddress: "1.2.3.4",
						ProxyNames:  []string{"proxy1"},
					},
				},
			})

			returnedResources := getTypedInputSnapshot(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.HaveTypeMeta(gatewayv1.GatewayGVK),
				matchers.HaveObjectMeta(types.NamespacedName{
					Namespace: defaults.GlooSystem,
					Name:      "gw-snap",
				}),
				gstruct.Ignore(),
			), "returned resources include gateways")
		})

		It("Includes HttpGateways", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				HttpGateways: gatewayv1.MatchableHttpGatewayList{
					{
						Metadata: &core.Metadata{
							Name:      "hgw-snap",
							Namespace: defaults.GlooSystem,
						},
						Matcher: &gatewayv1.MatchableHttpGateway_Matcher{
							SourcePrefixRanges: []*envoycorev3.CidrRange{
								{
									AddressPrefix: "abc",
								},
							},
						},
					},
				},
			})

			returnedResources := getTypedInputSnapshot(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.HaveTypeMeta(gatewayv1.MatchableHttpGatewayGVK),
				matchers.HaveObjectMeta(types.NamespacedName{
					Namespace: defaults.GlooSystem,
					Name:      "hgw-snap",
				}),
				gstruct.Ignore(),
			), "returned resources include http gateways")
		})

		It("Includes TcpGateways", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				TcpGateways: gatewayv1.MatchableTcpGatewayList{
					{
						Metadata: &core.Metadata{
							Name:      "tgw-snap",
							Namespace: defaults.GlooSystem,
						},
						Matcher: &gatewayv1.MatchableTcpGateway_Matcher{
							PassthroughCipherSuites: []string{"a", "b", "c"},
						},
					},
				},
			})

			returnedResources := getTypedInputSnapshot(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.HaveTypeMeta(gatewayv1.MatchableTcpGatewayGVK),
				matchers.HaveObjectMeta(types.NamespacedName{
					Namespace: defaults.GlooSystem,
					Name:      "tgw-snap",
				}),
				gstruct.Ignore(),
			), "returned resources include tcp gateways")
		})

		It("Includes VirtualHostOptions", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				VirtualHostOptions: gatewayv1.VirtualHostOptionList{
					{
						Metadata: &core.Metadata{
							Name:      "vho-snap",
							Namespace: defaults.GlooSystem,
						},
						Options: &v1.VirtualHostOptions{
							HeaderManipulation: &headers.HeaderManipulation{
								RequestHeadersToRemove: []string{"header1"},
							},
							Cors: &cors.CorsPolicy{
								ExposeHeaders: []string{"header2"},
								AllowOrigin:   []string{"some-origin"},
							},
						},
					},
				},
			})

			returnedResources := getTypedInputSnapshot(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.HaveTypeMeta(gatewayv1.VirtualHostOptionGVK),
				matchers.HaveObjectMeta(types.NamespacedName{
					Namespace: defaults.GlooSystem,
					Name:      "vho-snap",
				}),
				gstruct.Ignore(),
			), "returned resources include virtual host options")
		})

		It("Includes RouteOptions", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				RouteOptions: gatewayv1.RouteOptionList{
					{
						Metadata: &core.Metadata{
							Name:      "rto-snap",
							Namespace: defaults.GlooSystem,
						},
						Options: &v1.RouteOptions{
							HeaderManipulation: &headers.HeaderManipulation{
								RequestHeadersToRemove: []string{"header1"},
							},
							Cors: &cors.CorsPolicy{
								ExposeHeaders: []string{"header2"},
								AllowOrigin:   []string{"some-origin"},
							},
						},
					},
				},
			})

			returnedResources := getTypedInputSnapshot(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.HaveTypeMeta(gatewayv1.RouteOptionGVK),
				matchers.HaveObjectMeta(types.NamespacedName{
					Namespace: defaults.GlooSystem,
					Name:      "rto-snap",
				}),
				gstruct.Ignore(),
			), "returned resources include virtual host options")
		})

		It("Includes GraphQLApis", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				GraphqlApis: graphqlv1beta1.GraphQLApiList{
					{
						Metadata: &core.Metadata{
							Name:      "gql-snap",
							Namespace: defaults.GlooSystem,
						},
						Schema: &graphqlv1beta1.GraphQLApi_ExecutableSchema{
							ExecutableSchema: &graphqlv1beta1.ExecutableSchema{
								SchemaDefinition: "definition string",
							},
						},
					},
				},
			})

			returnedResources := getTypedInputSnapshot(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.HaveTypeMeta(graphqlv1beta1.GraphQLApiGVK),
				matchers.HaveObjectMeta(types.NamespacedName{
					Namespace: defaults.GlooSystem,
					Name:      "gql-snap",
				}),
				gstruct.Ignore(),
			), "returned resources include graphql apis")
		})

		It("Excludes Proxies", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				Proxies: v1.ProxyList{
					{Metadata: &core.Metadata{Name: "proxy", Namespace: defaults.GlooSystem}},
				},
			})

			returnedResources := getTypedInputSnapshot(ctx, history)
			Expect(returnedResources).NotTo(matchers.ContainCustomResource(
				matchers.HaveTypeMeta(v1.ProxyGVK),
				gstruct.Ignore(),
				gstruct.Ignore(),
			), "returned resources exclude proxies")
		})

		It("Sorts resources by GVK", func() {
			// TODO
		})

		Context("kube gateway integration", func() {

			It("includes Kubernetes Gateway resources in all namespaces", func() {
				clientObjects := []client.Object{
					&apiv1.Gateway{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-gw",
							Namespace: "a",
						},
					},
					&apiv1.HTTPRoute{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-http-route",
							Namespace: "b",
						},
					},
					&apiv1.GatewayClass{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-gw-class",
							Namespace: "c",
						},
					},
					&apiv1beta1.ReferenceGrant{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-ref-grant",
							Namespace: "d",
						},
					},
					&v1alpha1.GatewayParameters{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-gwp",
							Namespace: "e",
						},
					},
					&gatewaykubev1.ListenerOption{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-lo",
							Namespace: "f",
						},
					},
					&gatewaykubev1.HttpListenerOption{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-hlo",
							Namespace: "g",
						},
					},
					&gatewaykubev1.RouteOption{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-rto",
							Namespace: "h",
						},
					},
					&gatewaykubev1.VirtualHostOption{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-vho",
							Namespace: "i",
						},
					},
					&extauthkubev1.AuthConfig{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-ac",
							Namespace: "j",
						},
					},
					&rlv1alpha1.RateLimitConfig{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-rlc",
							Namespace: "k",
						},
					},
				}
				setClientOnHistory(ctx, history, clientBuilder.WithObjects(clientObjects...))

				inputSnapshotBytes, err := history.GetInputSnapshot(ctx)
				Expect(err).NotTo(HaveOccurred())

				returnedResources := []crdv1.Resource{}
				err = json.Unmarshal(inputSnapshotBytes, &returnedResources)
				Expect(err).NotTo(HaveOccurred())

				expectContainsResource(returnedResources, wellknown.GatewayGVK, "a", "kube-gw")
				expectContainsResource(returnedResources, wellknown.GatewayClassGVK, "c", "kube-gw-class")
				expectContainsResource(returnedResources, wellknown.HTTPRouteGVK, "b", "kube-http-route")
				expectContainsResource(returnedResources, wellknown.ReferenceGrantGVK, "d", "kube-ref-grant")
				expectContainsResource(returnedResources, v1alpha1.GatewayParametersGVK, "e", "kube-gwp")
				expectContainsResource(returnedResources, gatewayv1.ListenerOptionGVK, "f", "kube-lo")
				expectContainsResource(returnedResources, gatewayv1.HttpListenerOptionGVK, "g", "kube-hlo")
				expectContainsResource(returnedResources, gatewayv1.RouteOptionGVK, "h", "kube-rto")
				expectContainsResource(returnedResources, gatewayv1.VirtualHostOptionGVK, "i", "kube-vho")
				expectContainsResource(returnedResources, extauthv1.AuthConfigGVK, "j", "kube-ac")
				expectContainsResource(returnedResources, ratelimitv1alpha1.RateLimitConfigGVK, "k", "kube-rlc")
			})

			It("does not use ApiSnapshot for shared resources", func() {
				// when kube gateway integration is enabled, we should get back all the shared resource types
				// from k8s rather than only the ones from the api snapshot
				setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
					RouteOptions: gatewayv1.RouteOptionList{
						{
							Metadata: &core.Metadata{
								Name:      "rto-snap",
								Namespace: defaults.GlooSystem,
							},
						},
					},
					VirtualHostOptions: gatewayv1.VirtualHostOptionList{
						{
							Metadata: &core.Metadata{
								Name:      "vho-snap",
								Namespace: defaults.GlooSystem,
							},
						},
					},
					AuthConfigs: extauthv1.AuthConfigList{
						{
							Metadata: &core.Metadata{
								Name:      "ac-snap",
								Namespace: defaults.GlooSystem,
							},
						},
					},
					Ratelimitconfigs: ratelimitv1alpha1.RateLimitConfigList{
						{
							RateLimitConfig: ratelimit.RateLimitConfig{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "rlc-snap",
									Namespace: defaults.GlooSystem,
								},
							},
						},
					},
				})

				// k8s resources on the cluster (in reality this would be a superset of the ones
				// contained in the apisnapshot above, but we use non-overlapping resource names
				// in this test just to show that we are getting the ones from k8s instead of the
				// snapshot)
				clientObjects := []client.Object{
					&gatewaykubev1.RouteOption{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-rto",
							Namespace: "h",
						},
					},
					&gatewaykubev1.VirtualHostOption{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-vho",
							Namespace: "i",
						},
					},
					&extauthkubev1.AuthConfig{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-ac",
							Namespace: "j",
						},
					},
					&rlv1alpha1.RateLimitConfig{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-rlc",
							Namespace: "k",
						},
					},
				}
				setClientOnHistory(ctx, history, clientBuilder.WithObjects(clientObjects...))

				returnedResources := getTypedInputSnapshot(ctx, history)

				// should contain the kube resources
				expectContainsResource(returnedResources, gatewayv1.RouteOptionGVK, "h", "kube-rto")
				expectContainsResource(returnedResources, gatewayv1.VirtualHostOptionGVK, "i", "kube-vho")
				expectContainsResource(returnedResources, extauthv1.AuthConfigGVK, "j", "kube-ac")
				expectContainsResource(returnedResources, ratelimitv1alpha1.RateLimitConfigGVK, "k", "kube-rlc")

				// should not contain the api snapshot resources
				expectDoesNotContainResource(returnedResources, gatewayv1.RouteOptionGVK, defaults.GlooSystem, "rto-snap")
				expectDoesNotContainResource(returnedResources, gatewayv1.VirtualHostOptionGVK, defaults.GlooSystem, "vho-snap")
				expectDoesNotContainResource(returnedResources, extauthv1.AuthConfigGVK, defaults.GlooSystem, "ac-snap")
				expectDoesNotContainResource(returnedResources, ratelimitv1alpha1.RateLimitConfigGVK, defaults.GlooSystem, "rlc-snap")
			})

			It("returns only relevant gvks", func() {
				clientObjects := []client.Object{
					&appsv1.Deployment{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-deploy",
							Namespace: "a",
						},
					},
				}

				setClientOnHistory(ctx, history, clientBuilder.WithObjects(clientObjects...))

				returnedResources := getTypedInputSnapshot(ctx, history)

				// a Deployment is not one of the resource types we return in the input snapshot by default, so
				// the deployment should not appear in the results
				Expect(containsResourceType(returnedResources, deploymentGvk)).To(BeFalse(),
					"input snapshot should not contain deployments")
			})

			It("respects extra kube gvks", func() {
				// create a new History that adds deployments to the kube input snapshot gvks

				gvks := []schema.GroupVersionKind{}
				gvks = append(gvks, KubeGatewayDefaultGVKs...)
				gvks = append(gvks, deploymentGvk)
				history = NewHistory(xdsCache,
					&v1.Settings{
						Metadata: &core.Metadata{
							Name:      "my-settings",
							Namespace: defaults.GlooSystem,
						},
					},
					gvks,
				)

				// create a deployment
				clientObjects := []client.Object{
					&appsv1.Deployment{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-deploy",
							Namespace: "a",
						},
					},
				}
				setClientOnHistory(ctx, history, clientBuilder.WithObjects(clientObjects...))

				returnedResources := getTypedInputSnapshot(ctx, history)
				Expect(returnedResources).To(matchers.ContainCustomResource(
					matchers.HaveTypeMeta(deploymentGvk),
					matchers.HaveObjectMeta(types.NamespacedName{
						Namespace: "a",
						Name:      "kube-deploy",
					}),
					gstruct.Ignore(),
				), "we should now see the deployment in the input snapshot results")
			})
		})
	})

	Context("GetProxySnapshot", func() {

		It("returns ApiSnapshot with _only_ Proxies", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				Proxies: v1.ProxyList{
					{Metadata: &core.Metadata{Name: "proxy-east", Namespace: defaults.GlooSystem}},
					{Metadata: &core.Metadata{Name: "proxy-west", Namespace: defaults.GlooSystem}},
				},
				Upstreams: v1.UpstreamList{
					{Metadata: &core.Metadata{Name: "upstream-east", Namespace: defaults.GlooSystem}},
					{Metadata: &core.Metadata{Name: "upstream-west", Namespace: defaults.GlooSystem}},
				},
			})

			returnedResources := getTypedProxySnapshot(ctx, history)
			Expect(returnedResources).To(And(
				matchers.ContainCustomResource(
					matchers.HaveTypeMeta(v1.ProxyGVK),
					matchers.HaveObjectMeta(types.NamespacedName{
						Namespace: defaults.GlooSystem,
						Name:      "proxy-east",
					}),
					gstruct.Ignore(),
				),
				matchers.ContainCustomResource(
					matchers.HaveTypeMeta(v1.ProxyGVK),
					matchers.HaveObjectMeta(types.NamespacedName{
						Namespace: defaults.GlooSystem,
						Name:      "proxy-west",
					}),
					gstruct.Ignore(),
				),
			))

			Expect(returnedResources).NotTo(matchers.ContainCustomResource(
				matchers.HaveTypeMeta(v1.UpstreamGVK),
				gstruct.Ignore(),
				gstruct.Ignore(),
			))
		})

	})

})

func getTypedInputSnapshot(ctx context.Context, history History) []crdv1.Resource {
	inputSnapshotBytes, err := history.GetInputSnapshot(ctx)
	Expect(err).NotTo(HaveOccurred())

	var returnedResources []crdv1.Resource
	err = json.Unmarshal(inputSnapshotBytes, &returnedResources)
	Expect(err).NotTo(HaveOccurred())

	return returnedResources
}

func getTypedProxySnapshot(ctx context.Context, history History) []crdv1.Resource {
	inputSnapshotBytes, err := history.GetProxySnapshot(ctx)
	Expect(err).NotTo(HaveOccurred())

	var returnedResources []crdv1.Resource
	err = json.Unmarshal(inputSnapshotBytes, &returnedResources)
	Expect(err).NotTo(HaveOccurred())

	return returnedResources
}

// setSnapshotOnHistory sets the ApiSnapshot on the history, and blocks until it has been processed
// This is a utility method to help developers write tests, without having to worry about the asynchronous
// nature of the `Set` API on the History
func setSnapshotOnHistory(ctx context.Context, history History, snap *v1snap.ApiSnapshot) {
	snap.Gateways = append(snap.Gateways, &gatewayv1.Gateway{
		// We append a custom Gateway to the Snapshot, and then use that object
		// to verify the Snapshot has been processed
		Metadata: &core.Metadata{Name: "gw-signal", Namespace: defaults.GlooSystem},
	})

	history.SetApiSnapshot(snap)

	eventuallyInputSnapshotContainsResource(ctx, history, gatewayv1.GatewayGVK, defaults.GlooSystem, "gw-signal")
}

// setClientOnHistory sets the Kubernetes Client on the history, and blocks until it has been processed
// This is a utility method to help developers write tests, without having to worry about the asynchronous
// nature of the `Set` API on the History
func setClientOnHistory(ctx context.Context, history History, builder *fake.ClientBuilder) {
	gwSignalObject := &apiv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw-signal",
			Namespace: defaults.GlooSystem,
		},
	}

	history.SetKubeGatewayClient(builder.WithObjects(gwSignalObject).Build())

	eventuallyInputSnapshotContainsResource(ctx, history, wellknown.GatewayGVK, defaults.GlooSystem, "gw-signal")
}

// check that the input snapshot eventually contains a resource with the given gvk, namespace, and name
func eventuallyInputSnapshotContainsResource(
	ctx context.Context,
	history History,
	gvk schema.GroupVersionKind,
	namespace string,
	name string) {
	Eventually(func(g Gomega) {
		returnedResources := getTypedInputSnapshot(ctx, history)

		g.Expect(containsResource(returnedResources, gvk, namespace, name)).To(BeTrue())
	}).
		WithPolling(time.Millisecond*100).
		WithTimeout(time.Second*5).
		Should(Succeed(), fmt.Sprintf("snapshot should eventually contain resource %v %s.%s", gvk, namespace, name))
}

// Deprecated: Prefer matchers.ContainCustomResource (gomega/matchers/custom_resource.go)
func expectContainsResource(
	resources []crdv1.Resource,
	gvk schema.GroupVersionKind,
	namespace string,
	name string) {
	Expect(resources).To(matchers.ContainCustomResource(
		matchers.HaveTypeMeta(gvk),
		matchers.HaveObjectMeta(types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}),
		gstruct.Ignore(),
	), fmt.Sprintf("results should contain %v %s.%s", gvk, namespace, name))
}

// Deprecated: Prefer matchers.ContainCustomResource (gomega/matchers/custom_resource.go)
func expectDoesNotContainResource(
	resources []crdv1.Resource,
	gvk schema.GroupVersionKind,
	namespace string,
	name string) {
	Expect(resources).NotTo(matchers.ContainCustomResource(
		matchers.HaveTypeMeta(gvk),
		matchers.HaveObjectMeta(types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}),
		gstruct.Ignore(),
	), fmt.Sprintf("results should not contain %v %s.%s", gvk, namespace, name))
}

// return true if the list of resources contains a resource with the given gvk, namespace, and name
func containsResource(
	resources []crdv1.Resource,
	gvk schema.GroupVersionKind,
	namespace string,
	name string) bool {
	return slices.ContainsFunc(resources, func(res crdv1.Resource) bool {
		return areGvksEqual(res.GroupVersionKind(), gvk) &&
			res.GetName() == name &&
			res.GetNamespace() == namespace
	})
}

// return true if the list of resources contains any resource with the given gvk
func containsResourceType(
	resources []crdv1.Resource,
	gvk schema.GroupVersionKind) bool {
	return slices.ContainsFunc(resources, func(res crdv1.Resource) bool {
		return areGvksEqual(res.GroupVersionKind(), gvk)
	})
}

func areGvksEqual(a, b schema.GroupVersionKind) bool {
	return a.Group == b.Group &&
		a.Version == b.Version &&
		a.Kind == b.Kind
}
