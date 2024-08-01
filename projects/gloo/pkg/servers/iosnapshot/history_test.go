package iosnapshot

import (
	"context"
	"fmt"
	types2 "github.com/onsi/gomega/types"
	"github.com/solo-io/gloo/pkg/schemes"
	apiv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	"time"

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
			Cache: &xds.MockXdsCache{},
		}
	})

	FContext("NewHistory", func() {

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
					append(InputSnapshotGVKs, deploymentGvk), // include the Deployment GVK
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
					InputSnapshotGVKs, // do not include the Deployment GVK
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
			}

			history = NewHistory(
				historyFactorParams.Cache,
				historyFactorParams.Settings,
				clientBuilder.WithObjects(clientObjects...).Build(),
				InputSnapshotGVKs)
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

		Context("Gloo Kubernetes Gateway API Resources", func() {

			It("Includes GatewayParameters", func() {
				returnedResources := getInputSnapshotObjects(ctx, history)
				Expect(returnedResources).To(ContainElement(
					simpleObjectMatcher(v1alpha1.GatewayParametersGVK, types.NamespacedName{
						Name:      "kube-gwp",
						Namespace: "e",
					}),
				), fmt.Sprintf("results should contain %v %s.%s", v1alpha1.GatewayParametersGVK, "e", "kube-gwp"))
			})

		})

		Context("Gloo Gateway Policy Resources", func() {

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

		})

		Context("Gloo Resources", func() {

		})

		Context("Edge Gateway API Resources", func() {

		})

		It("Includes Settings", func() {
			// Settings CR is not part of the APISnapshot, but should be returned by the input snapshot endpoint

			returnedResources := getInputSnapshotObjects(ctx, history)
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

			returnedResources := getInputSnapshotObjects(ctx, history)
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

			returnedResources := getInputSnapshotObjects(ctx, history)
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

			returnedResources := getInputSnapshotObjects(ctx, history)
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

			returnedResources := getInputSnapshotObjects(ctx, history)
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

			returnedResources := getInputSnapshotObjects(ctx, history)
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

			returnedResources := getInputSnapshotObjects(ctx, history)
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

			returnedResources := getInputSnapshotObjects(ctx, history)
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

			returnedResources := getInputSnapshotObjects(ctx, history)
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

			returnedResources := getInputSnapshotObjects(ctx, history)
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

			returnedResources := getInputSnapshotObjects(ctx, history)
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

			returnedResources := getInputSnapshotObjects(ctx, history)
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

			returnedResources := getInputSnapshotObjects(ctx, history)
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

			returnedResources := getInputSnapshotObjects(ctx, history)
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

			returnedResources := getInputSnapshotObjects(ctx, history)
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

			returnedResources := getInputSnapshotObjects(ctx, history)
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

			returnedResources := getInputSnapshotObjects(ctx, history)
			Expect(returnedResources).NotTo(matchers.ContainCustomResourceType(v1.ProxyGVK), "returned resources exclude proxies")
		})

	})

	FContext("GetEdgeApiSnapshot", func() {

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

	FContext("GetProxySnapshot", func() {

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
		Expect(apiSnapshot.Gateways).To(ContainElement(
			skmatchers.MatchProto(gwSignal),
		))
	}).
		WithPolling(time.Millisecond*100).
		WithTimeout(time.Second*5).
		Should(Succeed(), fmt.Sprintf("snapshot should eventually contain resource %v %s", gatewayv1.GatewayGVK, gwSignal.GetMetadata().Ref().String()))
}

func simpleObjectMatcher(gvk schema.GroupVersionKind, namespacedName types.NamespacedName) types2.GomegaMatcher {
	return matchers.MatchClientObject(
		gvk,
		namespacedName,
		gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"ObjectMeta": matchers.HaveNilManagedFields(),
		})),
	)
}
