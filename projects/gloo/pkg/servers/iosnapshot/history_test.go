package iosnapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
	apiv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
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
		history = NewHistory(xdsCache, &v1.Settings{
			Metadata: &core.Metadata{
				Name:      "my-settings",
				Namespace: defaults.GlooSystem,
			},
		})
	})

	Context("GetInputSnapshot", func() {

		It("returns ApiSnapshot without sensitive data", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				Secrets: v1.SecretList{
					{Metadata: &core.Metadata{Name: "secret-east", Namespace: defaults.GlooSystem}},
					{Metadata: &core.Metadata{Name: "secret-west", Namespace: defaults.GlooSystem}},
				},
				Artifacts: v1.ArtifactList{
					{Metadata: &core.Metadata{Name: "artifact-east", Namespace: defaults.GlooSystem}},
					{Metadata: &core.Metadata{Name: "artifact-west", Namespace: defaults.GlooSystem}},
				},
			})

			inputSnapshotBytes, err := history.GetInputSnapshot(ctx)
			Expect(err).NotTo(HaveOccurred())

			returnedResources := []crdv1.Resource{}
			err = json.Unmarshal(inputSnapshotBytes, &returnedResources)
			Expect(err).NotTo(HaveOccurred())

			Expect(containsResourceType(returnedResources, v1.SecretGVK)).To(BeFalse(), "input snapshot should not contain secrets")
			Expect(containsResourceType(returnedResources, v1.ArtifactGVK)).To(BeFalse(), "input snapshot should not contain artifacts")
		})

		It("returns ApiSnapshot without Proxies", func() {
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

			inputSnapshotBytes, err := history.GetInputSnapshot(ctx)
			Expect(err).NotTo(HaveOccurred())

			returnedResources := []crdv1.Resource{}
			err = json.Unmarshal(inputSnapshotBytes, &returnedResources)
			Expect(err).NotTo(HaveOccurred())

			// proxies should not be included in input snapshot
			Expect(containsResourceType(returnedResources, v1.ProxyGVK)).To(BeFalse(), "input snapshot should not contain proxies")

			// upstreams should be included in input snapshot
			expectContainsResource(returnedResources, v1.UpstreamGVK, defaults.GlooSystem, "upstream-east")
			expectContainsResource(returnedResources, v1.UpstreamGVK, defaults.GlooSystem, "upstream-west")
		})

		It("returns all Edge api snapshot resources", func() {
			// make sure each resource type can be successfully converted from snapshot
			// to kubernetes format
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

			inputSnapshotBytes, err := history.GetInputSnapshot(ctx)
			Expect(err).NotTo(HaveOccurred())

			returnedResources := []crdv1.Resource{}
			err = json.Unmarshal(inputSnapshotBytes, &returnedResources)
			Expect(err).NotTo(HaveOccurred())

			expectContainsResource(returnedResources, v1.EndpointGVK, defaults.GlooSystem, "ep-snap")
			expectContainsResource(returnedResources, v1.UpstreamGroupGVK, defaults.GlooSystem, "ug-snap")
			expectContainsResource(returnedResources, v1.UpstreamGVK, defaults.GlooSystem, "us-snap")
			expectContainsResource(returnedResources, extauthv1.AuthConfigGVK, defaults.GlooSystem, "ac-snap")
			expectContainsResource(returnedResources, ratelimitv1alpha1.RateLimitConfigGVK, defaults.GlooSystem, "rlc-snap")
			expectContainsResource(returnedResources, gatewayv1.VirtualServiceGVK, defaults.GlooSystem, "vs-snap")
			expectContainsResource(returnedResources, gatewayv1.RouteTableGVK, defaults.GlooSystem, "rt-snap")
			expectContainsResource(returnedResources, gatewayv1.GatewayGVK, defaults.GlooSystem, "gw-snap")
			expectContainsResource(returnedResources, gatewayv1.VirtualHostOptionGVK, defaults.GlooSystem, "vho-snap")
			expectContainsResource(returnedResources, gatewayv1.RouteOptionGVK, defaults.GlooSystem, "rto-snap")
			expectContainsResource(returnedResources, gatewayv1.MatchableHttpGatewayGVK, defaults.GlooSystem, "hgw-snap")
			expectContainsResource(returnedResources, gatewayv1.MatchableTcpGatewayGVK, defaults.GlooSystem, "tgw-snap")
			expectContainsResource(returnedResources, graphqlv1beta1.GraphQLApiGVK, defaults.GlooSystem, "gql-snap")
		})

		It("returns Settings", func() {
			// settings is not part of the api snapshot, but should be returned by the input snapshot endpoint
			inputSnapshotBytes, err := history.GetInputSnapshot(ctx)
			Expect(err).NotTo(HaveOccurred())

			returnedResources := []crdv1.Resource{}
			err = json.Unmarshal(inputSnapshotBytes, &returnedResources)
			Expect(err).NotTo(HaveOccurred())

			expectContainsResource(returnedResources, v1.SettingsGVK, defaults.GlooSystem, "my-settings")
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

				inputSnapshotBytes, err := history.GetInputSnapshot(ctx)
				Expect(err).NotTo(HaveOccurred())

				returnedResources := []crdv1.Resource{}
				err = json.Unmarshal(inputSnapshotBytes, &returnedResources)
				Expect(err).NotTo(HaveOccurred())

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

			proxySnapshotBytes, err := history.GetProxySnapshot(ctx)
			Expect(err).NotTo(HaveOccurred())

			returnedResources := []crdv1.Resource{}
			err = json.Unmarshal(proxySnapshotBytes, &returnedResources)
			Expect(err).NotTo(HaveOccurred())

			expectContainsResource(returnedResources, v1.ProxyGVK, defaults.GlooSystem, "proxy-east")
			expectContainsResource(returnedResources, v1.ProxyGVK, defaults.GlooSystem, "proxy-west")

			Expect(containsResourceType(returnedResources, v1.UpstreamGVK)).To(BeFalse(), "proxy snapshot should not contain upstreams")
		})

	})

})

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
		TypeMeta: metav1.TypeMeta{},
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
		inputSnapshotBytes, err := history.GetInputSnapshot(ctx)
		g.Expect(err).NotTo(HaveOccurred())

		returnedResources := []crdv1.Resource{}
		err = json.Unmarshal(inputSnapshotBytes, &returnedResources)
		g.Expect(err).NotTo(HaveOccurred())

		g.Expect(containsResource(returnedResources, gvk, namespace, name)).To(BeTrue())
	}).
		WithPolling(time.Millisecond*100).
		WithTimeout(time.Second*5).
		Should(Succeed(), fmt.Sprintf("snapshot should eventually contain resource %v %s.%s", gvk, namespace, name))
}

func expectContainsResource(
	resources []crdv1.Resource,
	gvk schema.GroupVersionKind,
	namespace string,
	name string) {
	Expect(containsResource(resources, gvk, namespace, name)).
		To(BeTrue(), fmt.Sprintf("results should contain %v %s.%s", gvk, namespace, name))
}

func expectDoesNotContainResource(
	resources []crdv1.Resource,
	gvk schema.GroupVersionKind,
	namespace string,
	name string) {
	Expect(containsResource(resources, gvk, namespace, name)).
		To(BeFalse(), fmt.Sprintf("results should not contain %v %s.%s", gvk, namespace, name))
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
