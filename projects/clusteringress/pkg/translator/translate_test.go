package translator

import (
	"context"
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"

	"github.com/solo-io/gloo/projects/clusteringress/api/external/knative"
	v1alpha12 "github.com/solo-io/gloo/projects/clusteringress/pkg/api/external/knative"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/clusteringress/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/retries"
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
		ingressRes := &v1alpha12.ClusterIngress{ClusterIngress: knative.ClusterIngress(*ingress)}
		ingressResTls := &v1alpha12.ClusterIngress{ClusterIngress: knative.ClusterIngress(*ingressTls)}
		secret := &gloov1.Secret{
			Metadata: core.Metadata{Name: secretName, Namespace: namespace},
			Kind: &gloov1.Secret_Tls{
				Tls: &gloov1.TlsSecret{},
			},
		}
		snap := &v1.TranslatorSnapshot{
			Clusteringresses: v1alpha12.ClusterIngressList{ingressRes, ingressResTls},
			Secrets:          gloov1.SecretList{secret},
		}
		proxy, errs := translateProxy(context.TODO(), namespace, snap)
		Expect(errs).NotTo(HaveOccurred())
		Expect(proxy.Metadata.Name).To(Equal("clusteringress-proxy"))
		Expect(proxy.Listeners).To(HaveLen(2))
		Expect(proxy.Listeners[0].Name).To(Equal("http"))
		Expect(proxy.Listeners[0].BindPort).To(Equal(uint32(80)))

		expected := &gloov1.Proxy{
			Listeners: []*gloov1.Listener{
				{
					Name:        "http",
					BindAddress: "::",
					BindPort:    0x00000050,
					ListenerType: &gloov1.Listener_HttpListener{
						HttpListener: &gloov1.HttpListener{
							VirtualHosts: []*gloov1.VirtualHost{
								{
									Name: "example.ing-0",
									Domains: []string{
										"petes.com",
										"petes.com:80",
										"zah.net",
										"zah.net:80",
									},
									Routes: []*gloov1.Route{
										{
											Matchers: []*matchers.Matcher{{
												PathSpecifier: &matchers.Matcher_Regex{
													Regex: "/",
												},
											}},
											Action: &gloov1.Route_RouteAction{
												RouteAction: &gloov1.RouteAction{
													Destination: &gloov1.RouteAction_Multi{
														Multi: &gloov1.MultiDestination{
															Destinations: []*gloov1.WeightedDestination{
																{
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
																	},
																	Weight: 0x00000064,
																},
															},
														},
													},
												},
											},
											Options: &gloov1.RouteOptions{
												Timeout: durptr(1),
												Retries: &retries.RetryPolicy{
													NumRetries:    0x0000000e,
													PerTryTimeout: durptr(1000),
												},
												HeaderManipulation: &headers.HeaderManipulation{
													RequestHeadersToAdd: []*headers.HeaderValueOption{
														{
															Header: &headers.HeaderValue{
																Key:   "add",
																Value: "me",
															},
														},
													},
												},
											},
										},
									},
								},
								{
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
										{
											Matchers: []*matchers.Matcher{{
												PathSpecifier: &matchers.Matcher_Regex{
													Regex: "/hay",
												},
											}},
											Action: &gloov1.Route_RouteAction{
												RouteAction: &gloov1.RouteAction{
													Destination: &gloov1.RouteAction_Multi{
														Multi: &gloov1.MultiDestination{
															Destinations: []*gloov1.WeightedDestination{
																{
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
																	},
																	Weight: 0x00000064,
																},
															},
														},
													},
												},
											},
											Options: &gloov1.RouteOptions{
												Timeout: durptr(1),
												Retries: &retries.RetryPolicy{
													NumRetries:    0x0000000e,
													PerTryTimeout: durptr(1000),
												},
												HeaderManipulation: &headers.HeaderManipulation{
													RequestHeadersToAdd: []*headers.HeaderValueOption{
														{
															Header: &headers.HeaderValue{
																Key:   "add",
																Value: "me",
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Name:        "https",
					BindAddress: "::",
					BindPort:    0x000001bb,
					ListenerType: &gloov1.Listener_HttpListener{
						HttpListener: &gloov1.HttpListener{
							VirtualHosts: []*gloov1.VirtualHost{
								{
									Name: "example.ing-tls-0",
									Domains: []string{
										"petes.com",
										"petes.com:443",
										"zah.net",
										"zah.net:443",
									},
									Routes: []*gloov1.Route{
										{
											Matchers: []*matchers.Matcher{{
												PathSpecifier: &matchers.Matcher_Regex{
													Regex: "/",
												},
											}},
											Action: &gloov1.Route_RouteAction{
												RouteAction: &gloov1.RouteAction{
													Destination: &gloov1.RouteAction_Multi{
														Multi: &gloov1.MultiDestination{
															Destinations: []*gloov1.WeightedDestination{
																{
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
																	},
																	Weight: 0x00000064,
																},
															},
														},
													},
												},
											},
											Options: &gloov1.RouteOptions{
												Timeout: durptr(1),
												Retries: &retries.RetryPolicy{
													NumRetries:    0x0000000e,
													PerTryTimeout: durptr(1000),
												},
												HeaderManipulation: &headers.HeaderManipulation{
													RequestHeadersToAdd: []*headers.HeaderValueOption{
														{
															Header: &headers.HeaderValue{
																Key:   "add",
																Value: "me",
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
					SslConfigurations: []*gloov1.SslConfig{
						{
							SslSecrets: &gloov1.SslConfig_SecretRef{
								SecretRef: &core.ResourceRef{
									Name:      "areallygreatsecret",
									Namespace: "example",
								},
							},
							SniDomains: []string{
								"petes.com",
							},
						},
					},
				},
			},
			Status: core.Status{},
			Metadata: core.Metadata{
				Name:      "clusteringress-proxy",
				Namespace: "example",
			},
		}

		Expect(proxy).To(Equal(expected))
	})
})

func durptr(d int) *time.Duration {
	dur := time.Duration(d)
	return &dur
}
