package iosnapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	wellknownkube "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/wellknown"

	glookubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"

	corev1 "k8s.io/api/core/v1"

	skmatchers "github.com/solo-io/solo-kit/test/matchers"

	appsv1 "k8s.io/api/apps/v1"
	apiv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

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
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("History", func() {

	var (
		ctx context.Context

		clientBuilder *fake.ClientBuilder
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

		xdsCache := &xds.MockXdsCache{}
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

	Context("NewHistory", func() {

		var (
			deploymentGvk = schema.GroupVersionKind{
				Group:   appsv1.GroupName,
				Version: "v1",
				Kind:    "Deployment",
			}
		)

		When("Deployment GVK is included", func() {

			BeforeEach(func() {
				history = NewHistory(&xds.MockXdsCache{},
					&v1.Settings{
						Metadata: &core.Metadata{
							Name:      "my-settings",
							Namespace: defaults.GlooSystem,
						},
					},
					append(KubeGatewayDefaultGVKs, deploymentGvk), // include the Deployment GVK
				)

				clientObjects := []client.Object{
					&appsv1.Deployment{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-deploy",
							Namespace: "a",
						},
					},
				}
				setClientOnHistory(ctx, history, clientBuilder.WithObjects(clientObjects...))
			})

			It("GetInputSnapshot includes Deployments", func() {
				returnedResources := getInputSnapshotResources(ctx, history)
				Expect(returnedResources).To(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(deploymentGvk),
					matchers.MatchObjectMeta(types.NamespacedName{
						Namespace: "a",
						Name:      "kube-deploy",
					}),
					gstruct.Ignore(),
				), "we should now see the deployment in the input snapshot results")
			})

		})

		When("Deployment GVK is excluded", func() {

			BeforeEach(func() {
				history = NewHistory(&xds.MockXdsCache{},
					&v1.Settings{
						Metadata: &core.Metadata{
							Name:      "my-settings",
							Namespace: defaults.GlooSystem,
						},
					},
					KubeGatewayDefaultGVKs, // do not include the Deployment GVK
				)

				clientObjects := []client.Object{
					&appsv1.Deployment{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-deploy",
							Namespace: "a",
						},
					},
				}
				setClientOnHistory(ctx, history, clientBuilder.WithObjects(clientObjects...))
			})

			It("GetInputSnapshot excludes Deployments", func() {
				returnedResources := getInputSnapshotResources(ctx, history)
				Expect(returnedResources).NotTo(matchers.ContainCustomResourceType(deploymentGvk), "snapshot should not include the deployment")
			})
		})

	})

	Context("GetInputSnapshot", func() {

		It("Includes Settings", func() {
			// Settings CR is not part of the APISnapshot, but should be returned by the input snapshot endpoint

			returnedResources := getInputSnapshotResources(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.MatchTypeMeta(v1.SettingsGVK),
				matchers.MatchObjectMeta(types.NamespacedName{
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

			returnedResources := getInputSnapshotResources(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.MatchTypeMeta(v1.EndpointGVK),
				matchers.MatchObjectMeta(types.NamespacedName{
					Namespace: defaults.GlooSystem,
					Name:      "ep-snap",
				}),
				gstruct.Ignore(),
			), "returned resources include endpoints")
		})

		It("Includes Secrets (redacted)", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				Secrets: v1.SecretList{
					{
						Metadata: &core.Metadata{
							Name:      "secret",
							Namespace: defaults.GlooSystem,
							Annotations: map[string]string{
								corev1.LastAppliedConfigAnnotation: "last-applied-configuration",
								"safe-annotation":                  "safe-annotation-value",
							},
						},
						Kind: &v1.Secret_Tls{
							Tls: &v1.TlsSecret{
								CertChain:  "cert-chain",
								PrivateKey: "private-key",
								RootCa:     "root-ca",
								OcspStaple: nil,
							},
						},
					},
				},
			})

			returnedResources := getInputSnapshotResources(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				// When the Kubernetes Gateway integration is not enabled, the Secrets  are sourced from the
				// ApiSnapshot, and thus use the internal Gloo-defined Secret GVK.
				matchers.MatchTypeMeta(v1.SecretGVK),
				matchers.MatchObjectMeta(types.NamespacedName{
					Namespace: defaults.GlooSystem,
					Name:      "secret",
				}, gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Annotations": And(
						HaveKeyWithValue(corev1.LastAppliedConfigAnnotation, "<redacted>"),
						HaveKeyWithValue("safe-annotation", "safe-annotation-value"),
					),
				})),
				gstruct.PointTo(BeEmpty()), // entire secret spec should be nil
			), "returned resources include secrets")
		})

		It("Includes Artifacts (redacted)", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				Artifacts: v1.ArtifactList{
					{
						Metadata: &core.Metadata{
							Name:      "artifact",
							Namespace: defaults.GlooSystem,
							Annotations: map[string]string{
								corev1.LastAppliedConfigAnnotation: "last-applied-configuration",
								"safe-annotation":                  "safe-annotation-value",
							},
						},
						Data: map[string]string{
							"key":   "sensitive-data",
							"key-2": "sensitive-data",
						},
					},
				},
			})

			returnedResources := getInputSnapshotResources(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.MatchTypeMeta(v1.ArtifactGVK),
				matchers.MatchObjectMeta(types.NamespacedName{
					Namespace: defaults.GlooSystem,
					Name:      "artifact",
				}, gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Annotations": And(
						HaveKeyWithValue(corev1.LastAppliedConfigAnnotation, "<redacted>"),
						HaveKeyWithValue("safe-annotation", "safe-annotation-value"),
					),
				})),
				gstruct.PointTo(HaveKeyWithValue("data", And(
					HaveKeyWithValue("key", "<redacted>"),
					HaveKeyWithValue("key-2", "<redacted>"),
				))),
			), "returned resources include artifacts")
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

			returnedResources := getInputSnapshotResources(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.MatchTypeMeta(v1.UpstreamGroupGVK),
				matchers.MatchObjectMeta(types.NamespacedName{
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

			returnedResources := getInputSnapshotResources(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.MatchTypeMeta(v1.UpstreamGVK),
				matchers.MatchObjectMeta(types.NamespacedName{
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

			returnedResources := getInputSnapshotResources(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.MatchTypeMeta(extauthv1.AuthConfigGVK),
				matchers.MatchObjectMeta(types.NamespacedName{
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

			returnedResources := getInputSnapshotResources(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.MatchTypeMeta(ratelimitv1alpha1.RateLimitConfigGVK),
				matchers.MatchObjectMeta(types.NamespacedName{
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

			returnedResources := getInputSnapshotResources(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.MatchTypeMeta(gatewayv1.VirtualServiceGVK),
				matchers.MatchObjectMeta(types.NamespacedName{
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

			returnedResources := getInputSnapshotResources(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.MatchTypeMeta(gatewayv1.RouteTableGVK),
				matchers.MatchObjectMeta(types.NamespacedName{
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

			returnedResources := getInputSnapshotResources(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.MatchTypeMeta(gatewayv1.GatewayGVK),
				matchers.MatchObjectMeta(types.NamespacedName{
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

			returnedResources := getInputSnapshotResources(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.MatchTypeMeta(gatewayv1.MatchableHttpGatewayGVK),
				matchers.MatchObjectMeta(types.NamespacedName{
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

			returnedResources := getInputSnapshotResources(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.MatchTypeMeta(gatewayv1.MatchableTcpGatewayGVK),
				matchers.MatchObjectMeta(types.NamespacedName{
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

			returnedResources := getInputSnapshotResources(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.MatchTypeMeta(gatewayv1.VirtualHostOptionGVK),
				matchers.MatchObjectMeta(types.NamespacedName{
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

			returnedResources := getInputSnapshotResources(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.MatchTypeMeta(gatewayv1.RouteOptionGVK),
				matchers.MatchObjectMeta(types.NamespacedName{
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

			returnedResources := getInputSnapshotResources(ctx, history)
			Expect(returnedResources).To(matchers.ContainCustomResource(
				matchers.MatchTypeMeta(graphqlv1beta1.GraphQLApiGVK),
				matchers.MatchObjectMeta(types.NamespacedName{
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

			returnedResources := getInputSnapshotResources(ctx, history)
			Expect(returnedResources).NotTo(matchers.ContainCustomResourceType(v1.ProxyGVK), "returned resources exclude proxies")
		})

		When("Kubernetes Gateway integration is enabled", func() {

			It("Includes Gateways (Kubernetes API)", func() {
				setClientOnHistory(ctx, history, clientBuilder.WithObjects(
					&apiv1.Gateway{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-gw",
							Namespace: "a",
							ManagedFields: []metav1.ManagedFieldsEntry{{
								Manager: "manager",
							}},
						},
					}))

				returnedResources := getInputSnapshotResources(ctx, history)
				Expect(returnedResources).To(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(wellknown.GatewayGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "kube-gw",
						Namespace: "a",
					}, matchers.HaveNilManagedFields()),
					gstruct.Ignore(),
				), fmt.Sprintf("results should contain %v %s.%s", wellknown.GatewayGVK, "a", "kube-gw"))
			})

			It("Includes GatewayClasses", func() {
				setClientOnHistory(ctx, history, clientBuilder.WithObjects(
					&apiv1.GatewayClass{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-gw-class",
							Namespace: "c",
							ManagedFields: []metav1.ManagedFieldsEntry{{
								Manager: "manager",
							}},
						},
					}))

				returnedResources := getInputSnapshotResources(ctx, history)
				Expect(returnedResources).To(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(wellknown.GatewayClassGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "kube-gw-class",
						Namespace: "c",
					}, matchers.HaveNilManagedFields()),
					gstruct.Ignore(),
				), fmt.Sprintf("results should contain %v %s.%s", wellknown.GatewayClassGVK, "c", "kube-gw-class"))
			})

			It("Includes HTTPRoutes", func() {
				setClientOnHistory(ctx, history, clientBuilder.WithObjects(
					&apiv1.HTTPRoute{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-http-route",
							Namespace: "b",
							ManagedFields: []metav1.ManagedFieldsEntry{{
								Manager: "manager",
							}},
						},
					}))

				returnedResources := getInputSnapshotResources(ctx, history)
				Expect(returnedResources).To(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(wellknown.HTTPRouteGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "kube-http-route",
						Namespace: "b",
					}, matchers.HaveNilManagedFields()),
					gstruct.Ignore(),
				), fmt.Sprintf("results should contain %v %s.%s", wellknown.HTTPRouteGVK, "b", "kube-http-route"))
			})

			It("Includes ReferenceGrants", func() {
				setClientOnHistory(ctx, history, clientBuilder.WithObjects(
					&apiv1beta1.ReferenceGrant{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-ref-grant",
							Namespace: "d",
							ManagedFields: []metav1.ManagedFieldsEntry{{
								Manager: "manager",
							}},
						},
					}))

				returnedResources := getInputSnapshotResources(ctx, history)
				Expect(returnedResources).To(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(wellknown.ReferenceGrantGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "kube-ref-grant",
						Namespace: "d",
					}, matchers.HaveNilManagedFields()),
					gstruct.Ignore(),
				), fmt.Sprintf("results should contain %v %s.%s", wellknown.ReferenceGrantGVK, "d", "kube-ref-grant"))
			})

			It("Includes GatewayParameters", func() {
				setClientOnHistory(ctx, history, clientBuilder.WithObjects(
					&v1alpha1.GatewayParameters{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-gwp",
							Namespace: "e",
							ManagedFields: []metav1.ManagedFieldsEntry{{
								Manager: "manager",
							}},
						},
					}))

				returnedResources := getInputSnapshotResources(ctx, history)
				Expect(returnedResources).To(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(v1alpha1.GatewayParametersGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "kube-gwp",
						Namespace: "e",
					}, matchers.HaveNilManagedFields()),
					gstruct.Ignore(),
				), fmt.Sprintf("results should contain %v %s.%s", v1alpha1.GatewayParametersGVK, "e", "kube-gwp"))
			})

			It("Includes ListenerOptions", func() {
				setClientOnHistory(ctx, history, clientBuilder.WithObjects(
					&gatewaykubev1.ListenerOption{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-lo",
							Namespace: "f",
							ManagedFields: []metav1.ManagedFieldsEntry{{
								Manager: "manager",
							}},
						},
					}))

				returnedResources := getInputSnapshotResources(ctx, history)
				Expect(returnedResources).To(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(gatewayv1.ListenerOptionGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "kube-lo",
						Namespace: "f",
					}, matchers.HaveNilManagedFields()),
					gstruct.Ignore(),
				), fmt.Sprintf("results should contain %v %s.%s", gatewayv1.ListenerOptionGVK, "f", "kube-lo"))
			})

			It("Includes HttpListenerOptions", func() {
				setClientOnHistory(ctx, history, clientBuilder.WithObjects(
					&gatewaykubev1.HttpListenerOption{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-hlo",
							Namespace: "g",
							ManagedFields: []metav1.ManagedFieldsEntry{{
								Manager: "manager",
							}},
						},
					}))

				returnedResources := getInputSnapshotResources(ctx, history)
				Expect(returnedResources).To(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(gatewayv1.HttpListenerOptionGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "kube-hlo",
						Namespace: "g",
					}, matchers.HaveNilManagedFields()),
					gstruct.Ignore(),
				), fmt.Sprintf("results should contain %v %s.%s", gatewayv1.HttpListenerOptionGVK, "g", "kube-hlo"))
			})

			It("Includes RouteOptions", func() {
				setClientOnHistory(ctx, history, clientBuilder.WithObjects(
					&gatewaykubev1.RouteOption{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-rto",
							Namespace: "h",
							ManagedFields: []metav1.ManagedFieldsEntry{{
								Manager: "manager",
							}},
						},
					}))

				returnedResources := getInputSnapshotResources(ctx, history)
				Expect(returnedResources).To(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(gatewayv1.RouteOptionGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "kube-rto",
						Namespace: "h",
					}, matchers.HaveNilManagedFields()),
					gstruct.Ignore(),
				), fmt.Sprintf("results should contain %v %s.%s", gatewayv1.RouteOptionGVK, "h", "kube-rto"))
			})

			It("Includes VirtualHostOptions", func() {
				setClientOnHistory(ctx, history, clientBuilder.WithObjects(
					&gatewaykubev1.VirtualHostOption{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-vho",
							Namespace: "i",
							ManagedFields: []metav1.ManagedFieldsEntry{{
								Manager: "manager",
							}},
						},
					}))

				returnedResources := getInputSnapshotResources(ctx, history)
				Expect(returnedResources).To(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(gatewayv1.VirtualHostOptionGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "kube-vho",
						Namespace: "i",
					}, matchers.HaveNilManagedFields()),
					gstruct.Ignore(),
				), fmt.Sprintf("results should contain %v %s.%s", gatewayv1.VirtualHostOptionGVK, "i", "kube-vho"))
			})

			It("Includes AuthConfigs", func() {
				setClientOnHistory(ctx, history, clientBuilder.WithObjects(
					&extauthkubev1.AuthConfig{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-ac",
							Namespace: "j",
							ManagedFields: []metav1.ManagedFieldsEntry{{
								Manager: "manager",
							}},
						},
					}))

				returnedResources := getInputSnapshotResources(ctx, history)
				Expect(returnedResources).To(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(extauthv1.AuthConfigGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "kube-ac",
						Namespace: "j",
					}, matchers.HaveNilManagedFields()),
					gstruct.Ignore(),
				), fmt.Sprintf("results should contain %v %s.%s", extauthv1.AuthConfigGVK, "j", "kube-ac"))
			})

			It("Includes RateLimitConfigs", func() {
				setClientOnHistory(ctx, history, clientBuilder.WithObjects(
					&rlv1alpha1.RateLimitConfig{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-rlc",
							Namespace: "k",
							ManagedFields: []metav1.ManagedFieldsEntry{{
								Manager: "manager",
							}},
						},
					}))

				returnedResources := getInputSnapshotResources(ctx, history)
				Expect(returnedResources).To(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(ratelimitv1alpha1.RateLimitConfigGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "kube-rlc",
						Namespace: "k",
					}, matchers.HaveNilManagedFields()),
					gstruct.Ignore(),
				), fmt.Sprintf("results should contain %v %s.%s", ratelimitv1alpha1.RateLimitConfigGVK, "k", "kube-rlc"))
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
					Secrets: v1.SecretList{
						{
							Metadata: &core.Metadata{
								Name:      "secret-snap",
								Namespace: defaults.GlooSystem,
							},
						},
					},
					Upstreams: v1.UpstreamList{
						{
							Metadata: &core.Metadata{
								Name:      "upstream-snap",
								Namespace: defaults.GlooSystem,
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
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-secret",
							Namespace: "m",
						},
					},
					&glookubev1.Upstream{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-upstream",
							Namespace: "l",
						},
					},
				}
				setClientOnHistory(ctx, history, clientBuilder.WithObjects(clientObjects...))

				returnedResources := getInputSnapshotResources(ctx, history)

				// should contain the kube resources
				Expect(returnedResources).To(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(gatewayv1.RouteOptionGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "kube-rto",
						Namespace: "h",
					}, matchers.HaveNilManagedFields()),
					gstruct.Ignore(),
				), fmt.Sprintf("results should contain %v %s.%s", gatewayv1.RouteOptionGVK, "h", "kube-rto"))
				Expect(returnedResources).To(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(gatewayv1.VirtualHostOptionGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "kube-vho",
						Namespace: "i",
					}, matchers.HaveNilManagedFields()),
					gstruct.Ignore(),
				), fmt.Sprintf("results should contain %v %s.%s", gatewayv1.VirtualHostOptionGVK, "i", "kube-vho"))
				Expect(returnedResources).To(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(extauthv1.AuthConfigGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "kube-ac",
						Namespace: "j",
					}, matchers.HaveNilManagedFields()),
					gstruct.Ignore(),
				), fmt.Sprintf("results should contain %v %s.%s", extauthv1.AuthConfigGVK, "j", "kube-ac"))
				Expect(returnedResources).To(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(ratelimitv1alpha1.RateLimitConfigGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "kube-rlc",
						Namespace: "k",
					}, matchers.HaveNilManagedFields()),
					gstruct.Ignore(),
				), fmt.Sprintf("results should contain %v %s.%s", ratelimitv1alpha1.RateLimitConfigGVK, "k", "kube-rlc"))
				Expect(returnedResources).To(matchers.ContainCustomResource(
					// The Kubernetes Secret GVK uses an empty Group. This means that we cannot rely on the standard
					// HaveTypeMeta matcher, which will use the GroupVersion() method that returns "/{version}"
					gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"APIVersion": Equal("v1"),
						"Kind":       Equal("Secret"),
					}),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "kube-secret",
						Namespace: "m",
					}, matchers.HaveNilManagedFields()),
					BeNil(),
				), fmt.Sprintf("results should contain %v %s.%s", wellknownkube.SecretGVK, "m", "kube-secret"))
				Expect(returnedResources).To(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(v1.UpstreamGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "kube-upstream",
						Namespace: "l",
					}, matchers.HaveNilManagedFields()),
					gstruct.Ignore(),
				), fmt.Sprintf("results should contain %v %s.%s", v1.UpstreamGVK, "l", "kube-upstream"))

				// should not contain the api snapshot resources
				Expect(returnedResources).NotTo(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(gatewayv1.RouteOptionGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "rto-snap",
						Namespace: defaults.GlooSystem,
					}),
					gstruct.Ignore(),
				), fmt.Sprintf("results should not contain %v %s.%s", gatewayv1.RouteOptionGVK, defaults.GlooSystem, "rto-snap"))
				Expect(returnedResources).NotTo(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(gatewayv1.VirtualHostOptionGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "vho-snap",
						Namespace: defaults.GlooSystem,
					}),
					gstruct.Ignore(),
				), fmt.Sprintf("results should not contain %v %s.%s", gatewayv1.VirtualHostOptionGVK, defaults.GlooSystem, "vho-snap"))
				Expect(returnedResources).NotTo(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(extauthv1.AuthConfigGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "ac-snap",
						Namespace: defaults.GlooSystem,
					}),
					gstruct.Ignore(),
				), fmt.Sprintf("results should not contain %v %s.%s", extauthv1.AuthConfigGVK, defaults.GlooSystem, "ac-snap"))
				Expect(returnedResources).NotTo(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(ratelimitv1alpha1.RateLimitConfigGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "rlc-snap",
						Namespace: defaults.GlooSystem,
					}),
					gstruct.Ignore(),
				), fmt.Sprintf("results should not contain %v %s.%s", ratelimitv1alpha1.RateLimitConfigGVK, defaults.GlooSystem, "rlc-snap"))
				Expect(returnedResources).NotTo(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(v1.SecretGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "secret-snap",
						Namespace: defaults.GlooSystem,
					}),
					gstruct.Ignore(),
				), fmt.Sprintf("results should not contain %v %s.%s", v1.SecretGVK, defaults.GlooSystem, "secret-snap"))
				Expect(returnedResources).NotTo(matchers.ContainCustomResource(
					matchers.MatchTypeMeta(v1.UpstreamGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Name:      "upstream-snap",
						Namespace: defaults.GlooSystem,
					}),
					gstruct.Ignore(),
				), fmt.Sprintf("results should not contain %v %s.%s", v1.UpstreamGVK, defaults.GlooSystem, "upstream-snap"))
			})

		})
	})

	Context("GetEdgeApiSnapshot", func() {

		It("returns ApiSnapshot", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				Proxies: v1.ProxyList{
					{Metadata: &core.Metadata{Name: "proxy", Namespace: defaults.GlooSystem}},
				},
				Upstreams: v1.UpstreamList{
					{Metadata: &core.Metadata{Name: "upstream", Namespace: defaults.GlooSystem}},
				},
				Artifacts: v1.ArtifactList{
					{
						Metadata: &core.Metadata{
							Name:      "artifact",
							Namespace: defaults.GlooSystem,
							Annotations: map[string]string{
								corev1.LastAppliedConfigAnnotation: "last-applied-configuration",
								"safe-annotation":                  "safe-annotation-value",
							},
						},
						Data: map[string]string{
							"key": "sensitive-data",
						},
					},
				},
				Secrets: v1.SecretList{
					{
						Metadata: &core.Metadata{
							Name:      "secret",
							Namespace: defaults.GlooSystem,
							Annotations: map[string]string{
								corev1.LastAppliedConfigAnnotation: "last-applied-configuration",
								"safe-annotation":                  "safe-annotation-value",
							},
						},
						Kind: &v1.Secret_Tls{
							Tls: &v1.TlsSecret{
								CertChain:  "cert-chain",
								PrivateKey: "private-key",
								RootCa:     "root-ca",
								OcspStaple: nil,
							},
						},
					},
				},
			})

			snap := getEdgeApiSnapshot(ctx, history)
			Expect(snap.Proxies).To(ContainElement(
				skmatchers.MatchProto(&v1.Proxy{Metadata: &core.Metadata{Name: "proxy", Namespace: defaults.GlooSystem}}),
			))
			Expect(snap.Upstreams).To(ContainElement(
				skmatchers.MatchProto(&v1.Upstream{Metadata: &core.Metadata{Name: "upstream", Namespace: defaults.GlooSystem}}),
			))
			Expect(snap.Artifacts).To(ContainElement(
				skmatchers.MatchProto(&v1.Artifact{
					Metadata: &core.Metadata{
						Name:      "artifact",
						Namespace: defaults.GlooSystem,
						Annotations: map[string]string{
							corev1.LastAppliedConfigAnnotation: "<redacted>",
							"safe-annotation":                  "safe-annotation-value",
						},
					},
					Data: map[string]string{
						"key": "<redacted>",
					},
				}),
			), "artifacts are included and redacted")
			Expect(snap.Secrets).To(ContainElement(
				skmatchers.MatchProto(&v1.Secret{
					Metadata: &core.Metadata{
						Name:      "secret",
						Namespace: defaults.GlooSystem,
						Annotations: map[string]string{
							corev1.LastAppliedConfigAnnotation: "<redacted>",
							"safe-annotation":                  "safe-annotation-value",
						},
					},
					Kind: nil,
				}),
			), "secrets are included and redacted")
		})

	})

	Context("GetProxySnapshot", func() {

		It("returns ApiSnapshot with _only_ Proxies", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				Proxies: v1.ProxyList{
					{Metadata: &core.Metadata{Name: "proxy", Namespace: defaults.GlooSystem}},
				},
				Upstreams: v1.UpstreamList{
					{Metadata: &core.Metadata{Name: "upstream", Namespace: defaults.GlooSystem}},
				},
			})

			returnedResources := getProxySnapshotResources(ctx, history)
			Expect(returnedResources).To(And(
				matchers.ContainCustomResource(
					matchers.MatchTypeMeta(v1.ProxyGVK),
					matchers.MatchObjectMeta(types.NamespacedName{
						Namespace: defaults.GlooSystem,
						Name:      "proxy",
					}),
					gstruct.Ignore(),
				),
			))
			Expect(returnedResources).NotTo(matchers.ContainCustomResourceType(v1.UpstreamGVK), "non-proxy resources should be excluded")
		})

	})

})

func getInputSnapshotResources(ctx context.Context, history History) []crdv1.Resource {
	snapshotResponse := history.GetInputSnapshot(ctx)
	Expect(snapshotResponse.Error).NotTo(HaveOccurred())

	var returnedResources []crdv1.Resource
	dataJson, err := json.Marshal(snapshotResponse.Data)
	Expect(err).NotTo(HaveOccurred())

	err = json.Unmarshal(dataJson, &returnedResources)
	Expect(err).NotTo(HaveOccurred())

	return returnedResources
}

func getProxySnapshotResources(ctx context.Context, history History) []crdv1.Resource {
	snapshotResponse := history.GetProxySnapshot(ctx)
	Expect(snapshotResponse.Error).NotTo(HaveOccurred())

	var returnedResources []crdv1.Resource
	dataJson, err := json.Marshal(snapshotResponse.Data)
	Expect(err).NotTo(HaveOccurred())

	err = json.Unmarshal(dataJson, &returnedResources)
	Expect(err).NotTo(HaveOccurred())

	return returnedResources
}

func getEdgeApiSnapshot(ctx context.Context, history History) *v1snap.ApiSnapshot {
	snapshotResponse := history.GetEdgeApiSnapshot(ctx)
	Expect(snapshotResponse.Error).NotTo(HaveOccurred())

	var snap *v1snap.ApiSnapshot
	dataJson, err := json.Marshal(snapshotResponse.Data)
	Expect(err).NotTo(HaveOccurred())

	err = json.Unmarshal(dataJson, &snap)
	Expect(err).NotTo(HaveOccurred())

	return snap
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
	// We append a custom Gateway to the Snapshot, and then use that object
	// to verify the Snapshot has been processed
	gwSignalObject := &apiv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw-client-signal",
			Namespace: defaults.GlooSystem,
		},
	}

	history.SetKubeGatewayClient(builder.WithObjects(gwSignalObject).Build())

	eventuallyInputSnapshotContainsResource(ctx, history, wellknown.GatewayGVK, defaults.GlooSystem, "gw-client-signal")
}

// check that the input snapshot eventually contains a resource with the given gvk, namespace, and name
func eventuallyInputSnapshotContainsResource(
	ctx context.Context,
	history History,
	gvk schema.GroupVersionKind,
	namespace string,
	name string) {
	Eventually(func(g Gomega) {
		returnedResources := getInputSnapshotResources(ctx, history)
		g.Expect(returnedResources).To(matchers.ContainCustomResource(
			matchers.MatchTypeMeta(gvk),
			matchers.MatchObjectMeta(types.NamespacedName{
				Namespace: namespace,
				Name:      name,
			}),
			gstruct.Ignore(),
		), fmt.Sprintf("results should contain %v %s.%s", gvk, namespace, name))
	}).
		WithPolling(time.Millisecond*100).
		WithTimeout(time.Second*5).
		Should(Succeed(), fmt.Sprintf("snapshot should eventually contain resource %v %s.%s", gvk, namespace, name))
}
