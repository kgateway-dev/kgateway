package iosnapshot

import (
	"context"
	"fmt"
	"time"

	gomegatypes "github.com/onsi/gomega/types"
	"github.com/solo-io/gloo/pkg/schemes"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	apiv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	wellknownkube "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/wellknown"

	corev1 "k8s.io/api/core/v1"

	skmatchers "github.com/solo-io/solo-kit/test/matchers"

	"github.com/onsi/gomega/gstruct"
	"github.com/solo-io/gloo/test/gomega/matchers"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaykubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	ratelimitv1alpha1 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	extauthkubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1/kube/apis/enterprise.gloo.solo.io/v1"
	graphqlv1beta1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/graphql/v1beta1"
	graphqlkubev1beta1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/graphql/v1beta1/kube/apis/graphql.gloo.solo.io/v1beta1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	rlv1alpha1 "github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"
	crdv1 "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/solo.io/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

		historyFactorParams HistoryFactoryParameters
	)

	BeforeEach(func() {
		ctx = context.Background()
		clientBuilder = fake.NewClientBuilder().WithScheme(schemes.DefaultScheme())

		historyFactorParams = HistoryFactoryParameters{
			Settings: &v1.Settings{
				Metadata: &core.Metadata{
					Name:      "my-settings",
					Namespace: defaults.GlooSystem,
				},
			},
			Cache:                       &xds.MockXdsCache{},
			EnableK8sGatewayIntegration: true,
		}
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
				clientObjects := []client.Object{
					&appsv1.Deployment{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-deploy",
							Namespace: "a",
						},
						Spec: appsv1.DeploymentSpec{
							MinReadySeconds: 5,
						},
					},
				}

				history = NewHistory(
					historyFactorParams.Cache,
					historyFactorParams.Settings,
					clientBuilder.WithObjects(clientObjects...).Build(),
					append(CompleteInputSnapshotGVKs, deploymentGvk), // include the Deployment GVK
				)
			})

			It("GetInputSnapshot includes Deployments", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).To(ContainElement(matchers.MatchClientObject(
					deploymentGvk,
					types.NamespacedName{
						Namespace: "a",
						Name:      "kube-deploy",
					},
					gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Spec": Equal(appsv1.DeploymentSpec{
							MinReadySeconds: 5,
						}),
					})),
				)), "we should now see the deployment in the input snapshot results")
			})

		})

		When("Deployment GVK is excluded", func() {

			BeforeEach(func() {
				clientObjects := []client.Object{
					&appsv1.Deployment{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-deploy",
							Namespace: "a",
						},
					},
				}

				history = NewHistory(&xds.MockXdsCache{},
					&v1.Settings{
						Metadata: &core.Metadata{
							Name:      "my-settings",
							Namespace: defaults.GlooSystem,
						},
					},
					clientBuilder.WithObjects(clientObjects...).Build(),
					CompleteInputSnapshotGVKs, // do not include the Deployment GVK
				)
			})

			It("GetInputSnapshot excludes Deployments", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).NotTo(ContainElement(
					matchers.MatchClientObjectGvk(deploymentGvk),
				), "snapshot should not include the deployment")
			})
		})

	})

	Context("GetInputSnapshot", func() {

		BeforeEach(func() {
			clientObjects := []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-secret",
						Namespace: "secret",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
					Data: map[string][]byte{
						"key": []byte("sensitive-data"),
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-configmap",
						Namespace: "configmap",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
					Data: map[string]string{
						"key": "value",
					},
				},
				&apiv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-gw",
						Namespace: "a",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
				},
				&apiv1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-gw-class",
						Namespace: "c",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
				},
				&apiv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-http-route",
						Namespace: "b",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
				},
				&apiv1beta1.ReferenceGrant{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-ref-grant",
						Namespace: "d",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
				},
				&v1alpha1.GatewayParameters{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-gwp",
						Namespace: "e",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
				},
				&gatewaykubev1.ListenerOption{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-lo",
						Namespace: "f",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
				},
				&gatewaykubev1.HttpListenerOption{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-hlo",
						Namespace: "g",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
				},
				&gatewaykubev1.VirtualHostOption{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-vho",
						Namespace: "i",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
				},
				&gatewaykubev1.RouteOption{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-rto",
						Namespace: "h",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
				},
				&extauthkubev1.AuthConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-ac",
						Namespace: "j",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
				},
				&rlv1alpha1.RateLimitConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-rlc",
						Namespace: "k",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
				},
				&graphqlkubev1beta1.GraphQLApi{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-graphql",
						Namespace: "graphql",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
				},
				&gloov1.Settings{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-settings",
						Namespace: "settings",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
				},
				&gloov1.Upstream{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-upstream",
						Namespace: "upstream",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
				},
				&gloov1.UpstreamGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-upstreamgroup",
						Namespace: "upstreamgroup",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
				},
				&gloov1.Proxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-proxy",
						Namespace: "proxy",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
				},
				&gatewaykubev1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-edgegateway",
						Namespace: "edgegateway",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
				},
				&gatewaykubev1.MatchableHttpGateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-httpgateway",
						Namespace: "httpgateway",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
				},
				&gatewaykubev1.MatchableTcpGateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-tcpgateway",
						Namespace: "tcpgateway",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
				},
				&gatewaykubev1.VirtualService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-virtualservice",
						Namespace: "virtualservice",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
				},
				&gatewaykubev1.RouteTable{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-routetable",
						Namespace: "routetable",
						ManagedFields: []metav1.ManagedFieldsEntry{{
							Manager: "manager",
						}},
					},
				},
			}

			history = NewHistory(
				historyFactorParams.Cache,
				historyFactorParams.Settings,
				clientBuilder.WithObjects(clientObjects...).Build(),
				CompleteInputSnapshotGVKs)
		})

		Context("Kubernetes Core Resources", func() {

			It("Includes Secrets (redacted)", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).To(ContainElement(
					matchers.MatchClientObject(
						wellknownkube.SecretGVK,
						types.NamespacedName{
							Name:      "kube-secret",
							Namespace: "secret",
						},
						gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"ObjectMeta": matchers.HaveNilManagedFields(),
							"Data":       HaveKeyWithValue("key", []byte("<redacted>")),
						})),
					),
				), fmt.Sprintf("results should contain %v %s.%s", wellknownkube.SecretGVK, "secret", "kube-secret"))
			})

			It("Includes ConfigMaps", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).To(ContainElement(
					matchers.MatchClientObject(
						wellknownkube.ConfigMapGVK,
						types.NamespacedName{
							Name:      "kube-configmap",
							Namespace: "configmap",
						},
						gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"ObjectMeta": matchers.HaveNilManagedFields(),
							"Data":       HaveKeyWithValue("key", "value"),
						})),
					),
				), fmt.Sprintf("results should contain %v %s.%s", wellknownkube.ConfigMapGVK, "configmap", "kube-configmap"))
			})

		})

		Context("Kubernetes Gateway API Resources", func() {

			It("Includes Gateways (Kubernetes API)", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).To(ContainElement(
					simpleObjectMatcher(wellknown.GatewayGVK, types.NamespacedName{
						Name:      "kube-gw",
						Namespace: "a",
					}),
				), fmt.Sprintf("results should contain %v %s.%s", wellknown.GatewayGVK, "a", "kube-gw"))

			})

			It("Includes GatewayClasses", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).To(ContainElement(
					simpleObjectMatcher(wellknown.GatewayClassGVK, types.NamespacedName{
						Name:      "kube-gw-class",
						Namespace: "c",
					}),
				), fmt.Sprintf("results should contain %v %s.%s", wellknown.GatewayClassGVK, "c", "kube-gw-class"))
			})

			It("Includes HTTPRoutes", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).To(ContainElement(
					simpleObjectMatcher(wellknown.HTTPRouteGVK, types.NamespacedName{
						Name:      "kube-http-route",
						Namespace: "b",
					}),
				), fmt.Sprintf("results should contain %v %s.%s", wellknown.HTTPRouteGVK, "b", "kube-http-route"))
			})

			It("Includes ReferenceGrants", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).To(ContainElement(
					simpleObjectMatcher(wellknown.ReferenceGrantGVK, types.NamespacedName{
						Name:      "kube-ref-grant",
						Namespace: "d",
					}),
				), fmt.Sprintf("results should contain %v %s.%s", wellknown.ReferenceGrantGVK, "d", "kube-ref-grant"))
			})

		})

		Context("Gloo Kubernetes Gateway Integration Resources", func() {

			It("Includes GatewayParameters", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).To(ContainElement(
					simpleObjectMatcher(v1alpha1.GatewayParametersGVK, types.NamespacedName{
						Name:      "kube-gwp",
						Namespace: "e",
					}),
				), fmt.Sprintf("results should contain %v %s.%s", v1alpha1.GatewayParametersGVK, "e", "kube-gwp"))
			})

			It("Includes ListenerOptions", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).To(ContainElement(
					simpleObjectMatcher(gatewayv1.ListenerOptionGVK, types.NamespacedName{
						Name:      "kube-lo",
						Namespace: "f",
					}),
				), fmt.Sprintf("results should contain %v %s.%s", gatewayv1.ListenerOptionGVK, "f", "kube-lo"))
			})

			It("Includes HttpListenerOptions", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).To(ContainElement(
					simpleObjectMatcher(gatewayv1.HttpListenerOptionGVK, types.NamespacedName{
						Name:      "kube-hlo",
						Namespace: "g",
					}),
				), fmt.Sprintf("results should contain %v %s.%s", gatewayv1.HttpListenerOptionGVK, "g", "kube-hlo"))
			})

		})

		Context("Gloo Gateway Policy Resources", func() {

			It("Includes VirtualHostOptions", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).To(ContainElement(
					simpleObjectMatcher(gatewayv1.VirtualHostOptionGVK, types.NamespacedName{
						Name:      "kube-vho",
						Namespace: "i",
					}),
				), fmt.Sprintf("results should contain %v %s.%s", gatewayv1.VirtualHostOptionGVK, "i", "kube-vho"))
			})

			It("Includes RouteOptions", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).To(ContainElement(
					simpleObjectMatcher(gatewayv1.RouteOptionGVK, types.NamespacedName{
						Name:      "kube-rto",
						Namespace: "h",
					}),
				), fmt.Sprintf("results should contain %v %s.%s", gatewayv1.RouteOptionGVK, "h", "kube-rto"))
			})

		})

		Context("Enterprise Extension Resources", func() {

			It("Excludes AuthConfigs", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).NotTo(ContainElement(
					matchers.MatchClientObjectGvk(extauthv1.AuthConfigGVK),
				), fmt.Sprintf("results should not contain %v", extauthv1.AuthConfigGVK))
			})

			It("Excludes RateLimitConfigs", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).NotTo(ContainElement(
					matchers.MatchClientObjectGvk(ratelimitv1alpha1.RateLimitConfigGVK),
				), fmt.Sprintf("results should not contain %v", ratelimitv1alpha1.RateLimitConfigGVK))
			})

			It("Excludes GraphQLApis", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).NotTo(ContainElement(
					matchers.MatchClientObjectGvk(graphqlv1beta1.GraphQLApiGVK),
				), fmt.Sprintf("results should not contain %v", graphqlv1beta1.GraphQLApiGVK))
			})

		})

		Context("Gloo Resources", func() {

			It("Includes Settings", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).To(ContainElement(
					simpleObjectMatcher(v1.SettingsGVK, types.NamespacedName{
						Name:      "kube-settings",
						Namespace: "settings",
					}),
				), fmt.Sprintf("results should contain %v %s.%s", v1.SettingsGVK, "settings", "kube-settings"))
			})

			It("Excludes Endpoints", func() {
				// Endpoints are a type that are stored in-memory, but the ControlPlane
				// As a result, GetInputSnapshot does not attempt to return them

				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).NotTo(ContainElement(
					matchers.MatchClientObjectGvk(v1.EndpointGVK),
				), fmt.Sprintf("results should not contain %v", v1.EndpointGVK))
			})

			It("Includes Upstreams", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).To(ContainElement(
					simpleObjectMatcher(v1.UpstreamGVK, types.NamespacedName{
						Name:      "kube-upstream",
						Namespace: "upstream",
					}),
				), fmt.Sprintf("results should contain %v %s.%s", v1.UpstreamGVK, "upstream", "kube-upstream"))
			})

			It("Includes UpstreamGroups", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).To(ContainElement(
					simpleObjectMatcher(v1.UpstreamGroupGVK, types.NamespacedName{
						Name:      "kube-upstreamgroup",
						Namespace: "upstreamgroup",
					}),
				), fmt.Sprintf("results should contain %v %s.%s", v1.UpstreamGroupGVK, "upstreamgroup", "kube-upstreamgroup"))
			})

			It("Includes Proxies", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).To(ContainElement(
					simpleObjectMatcher(v1.ProxyGVK, types.NamespacedName{
						Name:      "kube-proxy",
						Namespace: "proxy",
					}),
				), fmt.Sprintf("results should contain %v %s.%s", v1.ProxyGVK, "proxy", "kube-proxy"))
			})

		})

		Context("Edge Gateway API Resources", func() {

			It("Includes Gateways", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).To(ContainElement(
					simpleObjectMatcher(gatewayv1.GatewayGVK, types.NamespacedName{
						Name:      "kube-edgegateway",
						Namespace: "edgegateway",
					}),
				), fmt.Sprintf("results should contain %v %s.%s", gatewayv1.GatewayGVK, "edgegateway", "kube-edgegateway"))
			})

			It("Includes HttpGateways", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).To(ContainElement(
					simpleObjectMatcher(gatewayv1.MatchableHttpGatewayGVK, types.NamespacedName{
						Name:      "kube-httpgateway",
						Namespace: "httpgateway",
					}),
				), fmt.Sprintf("results should contain %v %s.%s", gatewayv1.MatchableHttpGatewayGVK, "httpgateway", "kube-httpgateway"))
			})

			It("Includes TcpGateways", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).To(ContainElement(
					simpleObjectMatcher(gatewayv1.MatchableTcpGatewayGVK, types.NamespacedName{
						Name:      "kube-tcpgateway",
						Namespace: "tcpgateway",
					}),
				), fmt.Sprintf("results should contain %v %s.%s", gatewayv1.MatchableTcpGatewayGVK, "tcpgateway", "kube-tcpgateway"))
			})

			It("Includes VirtualServices", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).To(ContainElement(
					simpleObjectMatcher(gatewayv1.VirtualServiceGVK, types.NamespacedName{
						Name:      "kube-virtualservice",
						Namespace: "virtualservice",
					}),
				), fmt.Sprintf("results should contain %v %s.%s", gatewayv1.VirtualServiceGVK, "virtualservice", "kube-virtualservice"))
			})

			It("Includes RouteTables", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).To(ContainElement(
					simpleObjectMatcher(gatewayv1.RouteTableGVK, types.NamespacedName{
						Name:      "kube-routetable",
						Namespace: "routetable",
					}),
				), fmt.Sprintf("results should contain %v %s.%s", gatewayv1.RouteTableGVK, "routetable", "kube-routetable"))
			})

		})

	})

	Context("GetEdgeApiSnapshot", func() {

		BeforeEach(func() {
			history = NewHistory(
				historyFactorParams.Cache,
				historyFactorParams.Settings,
				clientBuilder.Build(), // no objects, because this API doesn't rely on the kube client
				CompleteInputSnapshotGVKs,
			)
		})

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

		BeforeEach(func() {
			history = NewHistory(
				historyFactorParams.Cache,
				historyFactorParams.Settings,
				clientBuilder.Build(), // no objects, because this API doesn't rely on the kube client
				CompleteInputSnapshotGVKs,
			)
		})

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

func getInputSnapshotObjects(ctx context.Context, history History) []client.Object {
	snapshotResponse := history.GetInputSnapshot(ctx)
	Expect(snapshotResponse.Error).NotTo(HaveOccurred())

	responseObjects, ok := snapshotResponse.Data.([]client.Object)
	Expect(ok).To(BeTrue())

	return responseObjects
}

func getProxySnapshotResources(ctx context.Context, history History) []crdv1.Resource {
	snapshotResponse := history.GetProxySnapshot(ctx)
	Expect(snapshotResponse.Error).NotTo(HaveOccurred())

	responseObjects, ok := snapshotResponse.Data.([]crdv1.Resource)
	Expect(ok).To(BeTrue())

	return responseObjects
}

func getEdgeApiSnapshot(ctx context.Context, history History) *v1snap.ApiSnapshot {
	snapshotResponse := history.GetEdgeApiSnapshot(ctx)
	Expect(snapshotResponse.Error).NotTo(HaveOccurred())

	response, ok := snapshotResponse.Data.(*v1snap.ApiSnapshot)
	Expect(ok).To(BeTrue())

	return response
}

// setSnapshotOnHistory sets the ApiSnapshot on the history, and blocks until it has been processed
// This is a utility method to help developers write tests, without having to worry about the asynchronous
// nature of the `Set` API on the History
func setSnapshotOnHistory(ctx context.Context, history History, snap *v1snap.ApiSnapshot) {
	gwSignal := &gatewayv1.Gateway{
		// We append a custom Gateway to the Snapshot, and then use that object
		// to verify the Snapshot has been processed
		Metadata: &core.Metadata{Name: "gw-signal", Namespace: defaults.GlooSystem},
	}

	snap.Gateways = append(snap.Gateways, gwSignal)
	history.SetApiSnapshot(snap)

	Eventually(func(g Gomega) {
		apiSnapshot := getEdgeApiSnapshot(ctx, history)
		g.Expect(apiSnapshot.Gateways).To(ContainElement(skmatchers.MatchProto(gwSignal)))
	}).
		WithPolling(time.Millisecond*100).
		WithTimeout(time.Second*5).
		Should(Succeed(), fmt.Sprintf("snapshot should eventually contain resource %v %s", gatewayv1.GatewayGVK, gwSignal.GetMetadata().Ref().String()))
}

func simpleObjectMatcher(gvk schema.GroupVersionKind, namespacedName types.NamespacedName) gomegatypes.GomegaMatcher {
	return matchers.MatchClientObject(
		gvk,
		namespacedName,
		gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"ObjectMeta": matchers.HaveNilManagedFields(),
		})),
	)
}
