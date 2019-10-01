package translator

import (
	"context"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/hostrewrite"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/shadowing"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/tracing"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/headers"

	"github.com/solo-io/gloo/projects/knative/api/external/knative"
	v1alpha12 "github.com/solo-io/gloo/projects/knative/pkg/api/external/knative"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/faultinjection"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/retries"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/transformation"
	v1 "github.com/solo-io/gloo/projects/knative/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"knative.dev/serving/pkg/apis/networking/v1alpha1"
)

var _ = Describe("Translate", func() {
	It("creates the appropriate proxy object for the provided ingress objects", func() {
		namespace := "example"
		serviceName := "peteszah-service"
		serviceNamespace := "peteszah-service-namespace"
		servicePort := int32(80)
		secretName := "areallygreatsecret"
		ingress := &v1alpha1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ing",
				Namespace: namespace,
			},
			Spec: v1alpha1.IngressSpec{
				Rules: []v1alpha1.IngressRule{
					{
						Hosts: []string{"petes.com", "zah.net", "mysvc.myns.svc.cluster.local", "mysvc.myns.example.com"},
						HTTP: &v1alpha1.HTTPIngressRuleValue{
							Paths: []v1alpha1.HTTPIngressPath{
								{
									Path: "/",
									Splits: []v1alpha1.IngressBackendSplit{
										{
											IngressBackend: v1alpha1.IngressBackend{
												ServiceName:      serviceName,
												ServiceNamespace: serviceNamespace,
												ServicePort: intstr.IntOrString{
													Type:   intstr.Int,
													IntVal: servicePort,
												},
											},
										},
									},
									AppendHeaders: map[string]string{"add": "me"},
									Timeout:       &metav1.Duration{Duration: time.Nanosecond}, // good luck
									Retries: &v1alpha1.HTTPRetry{
										Attempts:      14,
										PerTryTimeout: &metav1.Duration{Duration: time.Microsecond},
									},
								},
							},
						},
					},
					{
						Hosts: []string{"pog.com", "champ.net", "zah.net"},
						HTTP: &v1alpha1.HTTPIngressRuleValue{
							Paths: []v1alpha1.HTTPIngressPath{
								{
									Path: "/hay",
									Splits: []v1alpha1.IngressBackendSplit{
										{
											IngressBackend: v1alpha1.IngressBackend{
												ServiceName:      serviceName,
												ServiceNamespace: serviceNamespace,
												ServicePort: intstr.IntOrString{
													Type:   intstr.Int,
													IntVal: servicePort,
												},
											},
										},
									},
									AppendHeaders: map[string]string{"add": "me"},
									Timeout:       &metav1.Duration{Duration: time.Nanosecond}, // good luck
									Retries: &v1alpha1.HTTPRetry{
										Attempts:      14,
										PerTryTimeout: &metav1.Duration{Duration: time.Microsecond},
									},
								},
							},
						},
					},
				},
			},
		}
		ingressTls := &v1alpha1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ing-tls",
				Namespace: namespace,
			},
			Spec: v1alpha1.IngressSpec{
				TLS: []v1alpha1.IngressTLS{
					{
						Hosts:      []string{"petes.com"},
						SecretName: secretName,
					},
				},
				Rules: []v1alpha1.IngressRule{
					{
						Hosts: []string{"petes.com", "zah.net"},
						HTTP: &v1alpha1.HTTPIngressRuleValue{
							Paths: []v1alpha1.HTTPIngressPath{
								{
									Path: "/",
									Splits: []v1alpha1.IngressBackendSplit{
										{
											IngressBackend: v1alpha1.IngressBackend{
												ServiceName:      serviceName,
												ServiceNamespace: serviceNamespace,
												ServicePort: intstr.IntOrString{
													Type:   intstr.Int,
													IntVal: servicePort,
												},
											},
										},
									},
									AppendHeaders: map[string]string{"add": "me"},
									Timeout:       &metav1.Duration{Duration: time.Nanosecond}, // good luck
									Retries: &v1alpha1.HTTPRetry{
										Attempts:      14,
										PerTryTimeout: &metav1.Duration{Duration: time.Microsecond},
									},
								},
							},
						},
					},
				},
			},
		}
		ingressRes := &v1alpha12.Ingress{Ingress: knative.Ingress(*ingress)}
		ingressResTls := &v1alpha12.Ingress{Ingress: knative.Ingress(*ingressTls)}
		secret := &gloov1.Secret{
			Metadata: core.Metadata{Name: secretName, Namespace: namespace},
			Kind: &gloov1.Secret_Tls{
				Tls: &gloov1.TlsSecret{
					CertChain:  "",
					RootCa:     "",
					PrivateKey: "",
				},
			},
		}
		snap := &v1.TranslatorSnapshot{
			Ingresses: v1alpha12.IngressList{ingressRes, ingressResTls},
			Secrets:   gloov1.SecretList{secret},
		}
		proxy, errs := translateProxy(context.TODO(), "test", namespace, snap.Ingresses, snap.Secrets)
		Expect(errs).NotTo(HaveOccurred())
		Expect(proxy.Listeners).To(HaveLen(2))
		Expect(proxy.Listeners[0].Name).To(Equal("http"))
		Expect(proxy.Listeners[0].BindPort).To(Equal(uint32(80)))

		expected := &gloov1.Proxy{
			Listeners: []*gloov1.Listener{
				&gloov1.Listener{
					Name:        "http",
					BindAddress: "::",
					BindPort:    0x00000050,
					ListenerType: &gloov1.Listener_HttpListener{
						HttpListener: &gloov1.HttpListener{
							VirtualHosts: []*gloov1.VirtualHost{
								&gloov1.VirtualHost{
									Name: "example.ing-0",
									Domains: []string{
										"petes.com",
										"petes.com:80",
										"zah.net",
										"zah.net:80",
										"mysvc.myns.svc.cluster.local",
										"mysvc.myns.svc.cluster.local:80",
										"mysvc.myns.svc",
										"mysvc.myns.svc:80",
										"mysvc.myns",
										"mysvc.myns:80",
										"mysvc.myns.example.com",
										"mysvc.myns.example.com:80",
									},
									Routes: []*gloov1.Route{
										&gloov1.Route{
											Matcher: &gloov1.Matcher{
												PathSpecifier: &gloov1.Matcher_Regex{
													Regex: "/",
												},
											},
											Action: &gloov1.Route_RouteAction{
												RouteAction: &gloov1.RouteAction{
													Destination: &gloov1.RouteAction_Multi{
														Multi: &gloov1.MultiDestination{
															Destinations: []*gloov1.WeightedDestination{
																&gloov1.WeightedDestination{
																	Destination: &gloov1.Destination{
																		DestinationType: &gloov1.Destination_Kube{
																			Kube: &gloov1.KubernetesServiceDestination{
																				Ref: core.ResourceRef{
																					Name:      "peteszah-service",
																					Namespace: "peteszah-service-namespace",
																				},
																				Port: 0x00000050,
																			},
																		},
																		DestinationSpec: (*gloov1.DestinationSpec)(nil),
																		Subset:          (*gloov1.Subset)(nil),
																	},
																	Weight:                     0x00000064,
																	WeightedDestinationPlugins: (*gloov1.WeightedDestinationPlugins)(nil),
																	XXX_NoUnkeyedLiteral:       struct{}{},
																	XXX_sizecache:              0,
																},
															},
														},
													},
												},
											},
											RoutePlugins: &gloov1.RoutePlugins{
												Transformations: (*transformation.RouteTransformations)(nil),
												Faults:          (*faultinjection.RouteFaults)(nil),
												PrefixRewrite:   (*transformation.PrefixRewrite)(nil),
												Timeout:         durptr(1),
												Retries: &retries.RetryPolicy{
													RetryOn:       "",
													NumRetries:    0x0000000e,
													PerTryTimeout: durptr(1000),
												},
												Extensions: (*gloov1.Extensions)(nil),
												Tracing:    (*tracing.RouteTracingSettings)(nil),
												Shadowing:  (*shadowing.RouteShadowing)(nil),
												HeaderManipulation: &headers.HeaderManipulation{
													RequestHeadersToAdd: []*headers.HeaderValueOption{
														&headers.HeaderValueOption{
															Header: &headers.HeaderValue{
																Key:   "add",
																Value: "me",
															},
															Append: (*types.BoolValue)(nil),
														},
													},
												},
												HostRewrite: (*hostrewrite.HostRewrite)(nil),
											},
										},
									},
									VirtualHostPlugins: (*gloov1.VirtualHostPlugins)(nil),
									CorsPolicy:         (*gloov1.CorsPolicy)(nil),
								},
								&gloov1.VirtualHost{
									Name: "example.ing-1",
									Domains: []string{
										"pog.com",
										"pog.com:80",
										"champ.net",
										"champ.net:80",
										"zah.net",
										"zah.net:80",
									},
									Routes: []*gloov1.Route{
										&gloov1.Route{
											Matcher: &gloov1.Matcher{
												PathSpecifier: &gloov1.Matcher_Regex{
													Regex: "/hay",
												},
											},
											Action: &gloov1.Route_RouteAction{
												RouteAction: &gloov1.RouteAction{
													Destination: &gloov1.RouteAction_Multi{
														Multi: &gloov1.MultiDestination{
															Destinations: []*gloov1.WeightedDestination{
																&gloov1.WeightedDestination{
																	Destination: &gloov1.Destination{
																		DestinationType: &gloov1.Destination_Kube{
																			Kube: &gloov1.KubernetesServiceDestination{
																				Ref: core.ResourceRef{
																					Name:      "peteszah-service",
																					Namespace: "peteszah-service-namespace",
																				},
																				Port: 0x00000050,
																			},
																		},
																		DestinationSpec: (*gloov1.DestinationSpec)(nil),
																		Subset:          (*gloov1.Subset)(nil),
																	},
																	Weight:                     0x00000064,
																	WeightedDestinationPlugins: (*gloov1.WeightedDestinationPlugins)(nil),
																	XXX_NoUnkeyedLiteral:       struct{}{},
																	XXX_sizecache:              0,
																},
															},
														},
													},
												},
											},
											RoutePlugins: &gloov1.RoutePlugins{
												Transformations: (*transformation.RouteTransformations)(nil),
												Faults:          (*faultinjection.RouteFaults)(nil),
												PrefixRewrite:   (*transformation.PrefixRewrite)(nil),
												Timeout:         durptr(1),
												Retries: &retries.RetryPolicy{
													RetryOn:       "",
													NumRetries:    0x0000000e,
													PerTryTimeout: durptr(1000),
												},
												Extensions: (*gloov1.Extensions)(nil),
												Tracing:    (*tracing.RouteTracingSettings)(nil),
												Shadowing:  (*shadowing.RouteShadowing)(nil),
												HeaderManipulation: &headers.HeaderManipulation{
													RequestHeadersToAdd: []*headers.HeaderValueOption{
														&headers.HeaderValueOption{
															Header: &headers.HeaderValue{
																Key:   "add",
																Value: "me",
															},
															Append: (*types.BoolValue)(nil),
														},
													},
												},
												HostRewrite: (*hostrewrite.HostRewrite)(nil),
											},
										},
									},
									VirtualHostPlugins: (*gloov1.VirtualHostPlugins)(nil),
									CorsPolicy:         (*gloov1.CorsPolicy)(nil),
								},
							},
							ListenerPlugins: (*gloov1.HttpListenerPlugins)(nil),
						},
					},
					UseProxyProto: (*types.BoolValue)(nil),
					Plugins:       (*gloov1.ListenerPlugins)(nil),
				},
				&gloov1.Listener{
					Name:        "https",
					BindAddress: "::",
					BindPort:    0x000001bb,
					ListenerType: &gloov1.Listener_HttpListener{
						HttpListener: &gloov1.HttpListener{
							VirtualHosts: []*gloov1.VirtualHost{
								&gloov1.VirtualHost{
									Name: "example.ing-tls-0",
									Domains: []string{
										"petes.com",
										"petes.com:443",
										"zah.net",
										"zah.net:443",
									},
									Routes: []*gloov1.Route{
										&gloov1.Route{
											Matcher: &gloov1.Matcher{
												PathSpecifier: &gloov1.Matcher_Regex{
													Regex: "/",
												},
											},
											Action: &gloov1.Route_RouteAction{
												RouteAction: &gloov1.RouteAction{
													Destination: &gloov1.RouteAction_Multi{
														Multi: &gloov1.MultiDestination{
															Destinations: []*gloov1.WeightedDestination{
																&gloov1.WeightedDestination{
																	Destination: &gloov1.Destination{
																		DestinationType: &gloov1.Destination_Kube{
																			Kube: &gloov1.KubernetesServiceDestination{
																				Ref: core.ResourceRef{
																					Name:      "peteszah-service",
																					Namespace: "peteszah-service-namespace",
																				},
																				Port: 0x00000050,
																			},
																		},
																		DestinationSpec: (*gloov1.DestinationSpec)(nil),
																		Subset:          (*gloov1.Subset)(nil),
																	},
																	Weight:                    0x00000064,
																	WeighedDestinationPlugins: (*gloov1.WeightedDestinationPlugins)(nil),
																	XXX_NoUnkeyedLiteral:      struct{}{},
																	XXX_sizecache:             0,
																},
															},
														},
													},
												},
											},
											RoutePlugins: &gloov1.RoutePlugins{
												Transformations: (*transformation.RouteTransformations)(nil),
												Faults:          (*faultinjection.RouteFaults)(nil),
												PrefixRewrite:   (*transformation.PrefixRewrite)(nil),
												Timeout:         durptr(1),
												Retries: &retries.RetryPolicy{
													RetryOn:       "",
													NumRetries:    0x0000000e,
													PerTryTimeout: durptr(1000),
												},
												Extensions: (*gloov1.Extensions)(nil),
												Tracing:    (*tracing.RouteTracingSettings)(nil),
												Shadowing:  (*shadowing.RouteShadowing)(nil),
												HeaderManipulation: &headers.HeaderManipulation{
													RequestHeadersToAdd: []*headers.HeaderValueOption{
														&headers.HeaderValueOption{
															Header: &headers.HeaderValue{
																Key:   "add",
																Value: "me",
															},
															Append: (*types.BoolValue)(nil),
														},
													},
												},
												HostRewrite: (*hostrewrite.HostRewrite)(nil),
											},
										},
									},
									VirtualHostPlugins: (*gloov1.VirtualHostPlugins)(nil),
									CorsPolicy:         (*gloov1.CorsPolicy)(nil),
								},
							},
							ListenerPlugins: (*gloov1.HttpListenerPlugins)(nil),
						},
					},
					SslConfigurations: []*gloov1.SslConfig{
						&gloov1.SslConfig{
							SslSecrets: &gloov1.SslConfig_SecretRef{
								SecretRef: &core.ResourceRef{
									Name:      "areallygreatsecret",
									Namespace: "example",
								},
							},
							SniDomains: []string{
								"petes.com",
							},
							Parameters: (*gloov1.SslParameters)(nil),
						},
					},
					UseProxyProto: (*types.BoolValue)(nil),
					Plugins:       (*gloov1.ListenerPlugins)(nil),
				},
			},
			Status: core.Status{
				State:      0,
				Reason:     "",
				ReportedBy: "",
			},
			Metadata: core.Metadata{
				Name:            "test",
				Namespace:       "example",
				Cluster:         "",
				ResourceVersion: "",
			},
			XXX_NoUnkeyedLiteral: struct{}{},
			XXX_sizecache:        0,
		}

		Expect(proxy).To(Equal(expected))
	})
})

func durptr(d int) *time.Duration {
	dur := time.Duration(d)
	return &dur
}
