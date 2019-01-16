package translator

import (
	"time"

	"github.com/knative/serving/pkg/apis/networking/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	ingresstype "github.com/solo-io/gloo/projects/clusteringress/pkg/api/clusteringress"
	"github.com/solo-io/gloo/projects/clusteringress/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/faultinjection"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/retries"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/transformation"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var _ = Describe("Translate", func() {
	It("creates the appropriate proxy object for the provided ingress objects", func() {
		namespace := "example"
		serviceName := "peteszah-service"
		servicePort := int32(80)
		secretName := "areallygreatsecret"
		ingress := &v1alpha1.ClusterIngress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ing",
				Namespace: namespace,
			},
			Spec: v1alpha1.IngressSpec{
				Rules: []v1alpha1.ClusterIngressRule{
					{
						Hosts: []string{"petes.com", "zah.net"},
						HTTP: &v1alpha1.HTTPClusterIngressRuleValue{
							Paths: []v1alpha1.HTTPClusterIngressPath{
								{
									Path: "/",
									Splits: []v1alpha1.ClusterIngressBackendSplit{
										{
											ClusterIngressBackend: v1alpha1.ClusterIngressBackend{
												ServiceName: serviceName,
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
						HTTP: &v1alpha1.HTTPClusterIngressRuleValue{
							Paths: []v1alpha1.HTTPClusterIngressPath{
								{
									Path: "/hay",
									Splits: []v1alpha1.ClusterIngressBackendSplit{
										{
											ClusterIngressBackend: v1alpha1.ClusterIngressBackend{
												ServiceName: serviceName,
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
		ingressTls := &v1alpha1.ClusterIngress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ing-tls",
				Namespace: namespace,
			},
			Spec: v1alpha1.IngressSpec{
				TLS: []v1alpha1.ClusterIngressTLS{
					{
						Hosts:      []string{"petes.com"},
						SecretName: secretName,
					},
				},
				Rules: []v1alpha1.ClusterIngressRule{
					{
						Hosts: []string{"petes.com", "zah.net"},
						HTTP: &v1alpha1.HTTPClusterIngressRuleValue{
							Paths: []v1alpha1.HTTPClusterIngressPath{
								{
									Path: "/",
									Splits: []v1alpha1.ClusterIngressBackendSplit{
										{
											ClusterIngressBackend: v1alpha1.ClusterIngressBackend{
												ServiceName: serviceName,
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
		ingressRes, err := ingresstype.FromKube(ingress)
		Expect(err).NotTo(HaveOccurred())
		ingressResTls, err := ingresstype.FromKube(ingressTls)
		Expect(err).NotTo(HaveOccurred())
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
		us := &gloov1.Upstream{
			Metadata: core.Metadata{
				Namespace: namespace,
				Name:      "wow-upstream",
			},
			UpstreamSpec: &gloov1.UpstreamSpec{
				UpstreamType: &gloov1.UpstreamSpec_Kube{
					Kube: &kubernetes.UpstreamSpec{
						ServiceNamespace: namespace,
						ServiceName:      serviceName,
						ServicePort:      uint32(servicePort),
						Selector: map[string]string{
							"a": "b",
						},
					},
				},
			},
		}
		usSubset := &gloov1.Upstream{
			Metadata: core.Metadata{
				Namespace: namespace,
				Name:      "wow-upstream-subset",
			},
			UpstreamSpec: &gloov1.UpstreamSpec{
				UpstreamType: &gloov1.UpstreamSpec_Kube{
					Kube: &kubernetes.UpstreamSpec{
						ServiceName: serviceName,
						ServicePort: uint32(servicePort),
						Selector: map[string]string{
							"a": "b",
							"c": "d",
						},
					},
				},
			},
		}
		snap := &v1.TranslatorSnapshot{
			Clusteringresses: v1.ClusterIngressList{ingressRes, ingressResTls},
			Secrets:          gloov1.SecretsByNamespace{"hi": {secret}},
			Upstreams:        gloov1.UpstreamsByNamespace{"hi": {us, usSubset}},
		}
		proxy, errs := translateProxy(namespace, snap)
		Expect(errs).NotTo(HaveOccurred())
		Expect(proxy.Metadata.Name).To(Equal("clusteringress-proxy"))
		Expect(proxy.Listeners).To(HaveLen(2))
		Expect(proxy.Listeners[0].Name).To(Equal("http"))
		Expect(proxy.Listeners[0].BindPort).To(Equal(uint32(80)))

		//utter.Dump(proxy)
		Expect(err).NotTo(HaveOccurred())
		expected := &gloov1.Proxy{
			Listeners: []*gloov1.Listener{
				&gloov1.Listener{
					Name:        string("http"),
					BindAddress: string("::"),
					BindPort:    uint32(0x50),
					ListenerType: &gloov1.Listener_HttpListener{
						HttpListener: &gloov1.HttpListener{
							VirtualHosts: []*gloov1.VirtualHost{
								&gloov1.VirtualHost{
									Name: string("champ.net-http"),
									Domains: []string{
										string("champ.net"),
									},
									Routes: []*gloov1.Route{
										&gloov1.Route{
											Matcher: &gloov1.Matcher{
												PathSpecifier: &gloov1.Matcher_Regex{
													Regex: string("/hay"),
												},
												Headers:         []*gloov1.HeaderMatcher(nil),
												QueryParameters: []*gloov1.QueryParameterMatcher(nil),
												Methods:         []string(nil),
											},
											Action: &gloov1.Route_RouteAction{
												RouteAction: &gloov1.RouteAction{
													Destination: &gloov1.RouteAction_Single{
														Single: &gloov1.Destination{
															Upstream: core.ResourceRef{
																Name:      string("wow-upstream-subset"),
																Namespace: string("example"),
															},
															DestinationSpec: (*gloov1.DestinationSpec)(nil),
														},
													},
												},
											},
											RoutePlugins: &gloov1.RoutePlugins{
												Transformations: &transformation.RouteTransformations{
													RequestTransformation: &transformation.Transformation{
														TransformationType: &transformation.Transformation_TransformationTemplate{
															TransformationTemplate: &transformation.TransformationTemplate{
																AdvancedTemplates: bool(false),
																Extractors:        map[string]*transformation.Extraction(nil),
																Headers: map[string]*transformation.InjaTemplate{
																	string("add"): &transformation.InjaTemplate{
																		Text: string("me"),
																	},
																},
																BodyTransformation: &transformation.TransformationTemplate_Passthrough{
																	Passthrough: &transformation.Passthrough{},
																},
															},
														},
													},
													ResponseTransformation: (*transformation.Transformation)(nil),
												},
												Faults:        (*faultinjection.RouteFaults)(nil),
												PrefixRewrite: (*transformation.PrefixRewrite)(nil),
												Timeout:       durptr(1),
												Retries: &retries.RetryPolicy{
													RetryOn:       string(""),
													NumRetries:    uint32(0xe),
													PerTryTimeout: durptr(1000),
												},
											},
										},
									},
								},
								&gloov1.VirtualHost{
									Name: string("petes.com-http"),
									Domains: []string{
										string("petes.com"),
									},
									Routes: []*gloov1.Route{
										&gloov1.Route{
											Matcher: &gloov1.Matcher{
												PathSpecifier: &gloov1.Matcher_Regex{
													Regex: string("/"),
												},
												Headers:         []*gloov1.HeaderMatcher(nil),
												QueryParameters: []*gloov1.QueryParameterMatcher(nil),
												Methods:         []string(nil),
											},
											Action: &gloov1.Route_RouteAction{
												RouteAction: &gloov1.RouteAction{
													Destination: &gloov1.RouteAction_Single{
														Single: &gloov1.Destination{
															Upstream: core.ResourceRef{
																Name:      string("wow-upstream-subset"),
																Namespace: string("example"),
															},
															DestinationSpec: (*gloov1.DestinationSpec)(nil),
														},
													},
												},
											},
											RoutePlugins: &gloov1.RoutePlugins{
												Transformations: &transformation.RouteTransformations{
													RequestTransformation: &transformation.Transformation{
														TransformationType: &transformation.Transformation_TransformationTemplate{
															TransformationTemplate: &transformation.TransformationTemplate{
																AdvancedTemplates: bool(false),
																Extractors:        map[string]*transformation.Extraction(nil),
																Headers: map[string]*transformation.InjaTemplate{
																	string("add"): &transformation.InjaTemplate{
																		Text: string("me"),
																	},
																},
																BodyTransformation: &transformation.TransformationTemplate_Passthrough{
																	Passthrough: &transformation.Passthrough{},
																},
															},
														},
													},
													ResponseTransformation: (*transformation.Transformation)(nil),
												},
												Faults:        (*faultinjection.RouteFaults)(nil),
												PrefixRewrite: (*transformation.PrefixRewrite)(nil),
												Timeout:       durptr(1),
												Retries: &retries.RetryPolicy{
													RetryOn:       string(""),
													NumRetries:    uint32(0xe),
													PerTryTimeout: durptr(1000),
												},
											},
										},
									},
								},
								&gloov1.VirtualHost{
									Name: string("pog.com-http"),
									Domains: []string{
										string("pog.com"),
									},
									Routes: []*gloov1.Route{
										&gloov1.Route{
											Matcher: &gloov1.Matcher{
												PathSpecifier: &gloov1.Matcher_Regex{
													Regex: string("/hay"),
												},
												Headers:         []*gloov1.HeaderMatcher(nil),
												QueryParameters: []*gloov1.QueryParameterMatcher(nil),
												Methods:         []string(nil),
											},
											Action: &gloov1.Route_RouteAction{
												RouteAction: &gloov1.RouteAction{
													Destination: &gloov1.RouteAction_Single{
														Single: &gloov1.Destination{
															Upstream: core.ResourceRef{
																Name:      string("wow-upstream-subset"),
																Namespace: string("example"),
															},
															DestinationSpec: (*gloov1.DestinationSpec)(nil),
														},
													},
												},
											},
											RoutePlugins: &gloov1.RoutePlugins{
												Transformations: &transformation.RouteTransformations{
													RequestTransformation: &transformation.Transformation{
														TransformationType: &transformation.Transformation_TransformationTemplate{
															TransformationTemplate: &transformation.TransformationTemplate{
																AdvancedTemplates: bool(false),
																Extractors:        map[string]*transformation.Extraction(nil),
																Headers: map[string]*transformation.InjaTemplate{
																	string("add"): &transformation.InjaTemplate{
																		Text: string("me"),
																	},
																},
																BodyTransformation: &transformation.TransformationTemplate_Passthrough{
																	Passthrough: &transformation.Passthrough{},
																},
															},
														},
													},
													ResponseTransformation: (*transformation.Transformation)(nil),
												},
												Faults:        (*faultinjection.RouteFaults)(nil),
												PrefixRewrite: (*transformation.PrefixRewrite)(nil),
												Timeout:       durptr(1),
												Retries: &retries.RetryPolicy{
													RetryOn:       string(""),
													NumRetries:    uint32(0xe),
													PerTryTimeout: durptr(1000),
												},
											},
										},
									},
								},
								&gloov1.VirtualHost{
									Name: string("zah.net-http"),
									Domains: []string{
										string("zah.net"),
									},
									Routes: []*gloov1.Route{
										&gloov1.Route{
											Matcher: &gloov1.Matcher{
												PathSpecifier: &gloov1.Matcher_Regex{
													Regex: string("/hay"),
												},
												Headers:         []*gloov1.HeaderMatcher(nil),
												QueryParameters: []*gloov1.QueryParameterMatcher(nil),
												Methods:         []string(nil),
											},
											Action: &gloov1.Route_RouteAction{
												RouteAction: &gloov1.RouteAction{
													Destination: &gloov1.RouteAction_Single{
														Single: &gloov1.Destination{
															Upstream: core.ResourceRef{
																Name:      string("wow-upstream-subset"),
																Namespace: string("example"),
															},
															DestinationSpec: (*gloov1.DestinationSpec)(nil),
														},
													},
												},
											},
											RoutePlugins: &gloov1.RoutePlugins{
												Transformations: &transformation.RouteTransformations{
													RequestTransformation: &transformation.Transformation{
														TransformationType: &transformation.Transformation_TransformationTemplate{
															TransformationTemplate: &transformation.TransformationTemplate{
																AdvancedTemplates: bool(false),
																Extractors:        map[string]*transformation.Extraction(nil),
																Headers: map[string]*transformation.InjaTemplate{
																	string("add"): &transformation.InjaTemplate{
																		Text: string("me"),
																	},
																},
																BodyTransformation: &transformation.TransformationTemplate_Passthrough{
																	Passthrough: &transformation.Passthrough{},
																},
															},
														},
													},
													ResponseTransformation: (*transformation.Transformation)(nil),
												},
												Faults:        (*faultinjection.RouteFaults)(nil),
												PrefixRewrite: (*transformation.PrefixRewrite)(nil),
												Timeout:       durptr(1),
												Retries: &retries.RetryPolicy{
													RetryOn:       string(""),
													NumRetries:    uint32(0xe),
													PerTryTimeout: durptr(1000),
												},
											},
										},
										&gloov1.Route{
											Matcher: &gloov1.Matcher{
												PathSpecifier: &gloov1.Matcher_Regex{
													Regex: string("/"),
												},
												Headers:         []*gloov1.HeaderMatcher(nil),
												QueryParameters: []*gloov1.QueryParameterMatcher(nil),
												Methods:         []string(nil),
											},
											Action: &gloov1.Route_RouteAction{
												RouteAction: &gloov1.RouteAction{
													Destination: &gloov1.RouteAction_Single{
														Single: &gloov1.Destination{
															Upstream: core.ResourceRef{
																Name:      string("wow-upstream-subset"),
																Namespace: string("example"),
															},
															DestinationSpec: (*gloov1.DestinationSpec)(nil),
														},
													},
												},
											},
											RoutePlugins: &gloov1.RoutePlugins{
												Transformations: &transformation.RouteTransformations{
													RequestTransformation: &transformation.Transformation{
														TransformationType: &transformation.Transformation_TransformationTemplate{
															TransformationTemplate: &transformation.TransformationTemplate{
																AdvancedTemplates: bool(false),
																Extractors:        map[string]*transformation.Extraction(nil),
																Headers: map[string]*transformation.InjaTemplate{
																	string("add"): &transformation.InjaTemplate{
																		Text: string("me"),
																	},
																},
																BodyTransformation: &transformation.TransformationTemplate_Passthrough{
																	Passthrough: &transformation.Passthrough{},
																},
															},
														},
													},
													ResponseTransformation: (*transformation.Transformation)(nil),
												},
												Faults:        (*faultinjection.RouteFaults)(nil),
												PrefixRewrite: (*transformation.PrefixRewrite)(nil),
												Timeout:       durptr(1),
												Retries: &retries.RetryPolicy{
													RetryOn:       string(""),
													NumRetries:    uint32(0xe),
													PerTryTimeout: durptr(1000),
												},
											},
										},
										&gloov1.Route{
											Matcher: &gloov1.Matcher{
												PathSpecifier: &gloov1.Matcher_Regex{
													Regex: string("/"),
												},
												Headers:         []*gloov1.HeaderMatcher(nil),
												QueryParameters: []*gloov1.QueryParameterMatcher(nil),
												Methods:         []string(nil),
											},
											Action: &gloov1.Route_RouteAction{
												RouteAction: &gloov1.RouteAction{
													Destination: &gloov1.RouteAction_Single{
														Single: &gloov1.Destination{
															Upstream: core.ResourceRef{
																Name:      string("wow-upstream-subset"),
																Namespace: string("example"),
															},
															DestinationSpec: (*gloov1.DestinationSpec)(nil),
														},
													},
												},
											},
											RoutePlugins: &gloov1.RoutePlugins{
												Transformations: &transformation.RouteTransformations{
													RequestTransformation: &transformation.Transformation{
														TransformationType: &transformation.Transformation_TransformationTemplate{
															TransformationTemplate: &transformation.TransformationTemplate{
																AdvancedTemplates: bool(false),
																Extractors:        map[string]*transformation.Extraction(nil),
																Headers: map[string]*transformation.InjaTemplate{
																	string("add"): &transformation.InjaTemplate{
																		Text: string("me"),
																	},
																},
																BodyTransformation: &transformation.TransformationTemplate_Passthrough{
																	Passthrough: &transformation.Passthrough{},
																},
															},
														},
													},
													ResponseTransformation: (*transformation.Transformation)(nil),
												},
												Faults:        (*faultinjection.RouteFaults)(nil),
												PrefixRewrite: (*transformation.PrefixRewrite)(nil),
												Timeout:       durptr(1),
												Retries: &retries.RetryPolicy{
													RetryOn:       string(""),
													NumRetries:    uint32(0xe),
													PerTryTimeout: durptr(1000),
												},
											},
										},
									},
								},
							},
						},
					},
					SslConfiguations: []*gloov1.SslConfig(nil),
				},
				&gloov1.Listener{
					Name:        string("https"),
					BindAddress: string("::"),
					BindPort:    uint32(0x1bb),
					ListenerType: &gloov1.Listener_HttpListener{
						HttpListener: &gloov1.HttpListener{
							VirtualHosts: []*gloov1.VirtualHost{
								&gloov1.VirtualHost{
									Name: string("petes.com-http"),
									Domains: []string{
										string("petes.com"),
									},
									Routes: []*gloov1.Route{
										&gloov1.Route{
											Matcher: &gloov1.Matcher{
												PathSpecifier: &gloov1.Matcher_Regex{
													Regex: string("/"),
												},
												Headers:         []*gloov1.HeaderMatcher(nil),
												QueryParameters: []*gloov1.QueryParameterMatcher(nil),
												Methods:         []string(nil),
											},
											Action: &gloov1.Route_RouteAction{
												RouteAction: &gloov1.RouteAction{
													Destination: &gloov1.RouteAction_Single{
														Single: &gloov1.Destination{
															Upstream: core.ResourceRef{
																Name:      string("wow-upstream-subset"),
																Namespace: string("example"),
															},
															DestinationSpec: (*gloov1.DestinationSpec)(nil),
														},
													},
												},
											},
											RoutePlugins: &gloov1.RoutePlugins{
												Transformations: &transformation.RouteTransformations{
													RequestTransformation: &transformation.Transformation{
														TransformationType: &transformation.Transformation_TransformationTemplate{
															TransformationTemplate: &transformation.TransformationTemplate{
																AdvancedTemplates: bool(false),
																Extractors:        map[string]*transformation.Extraction(nil),
																Headers: map[string]*transformation.InjaTemplate{
																	string("add"): &transformation.InjaTemplate{
																		Text: string("me"),
																	},
																},
																BodyTransformation: &transformation.TransformationTemplate_Passthrough{
																	Passthrough: &transformation.Passthrough{},
																},
															},
														},
													},
													ResponseTransformation: (*transformation.Transformation)(nil),
												},
												Faults:        (*faultinjection.RouteFaults)(nil),
												PrefixRewrite: (*transformation.PrefixRewrite)(nil),
												Timeout:       durptr(1),
												Retries: &retries.RetryPolicy{
													RetryOn:       string(""),
													NumRetries:    uint32(0xe),
													PerTryTimeout: durptr(1000),
												},
											},
										},
									},
								},
							},
						},
					},
					SslConfiguations: []*gloov1.SslConfig{
						&gloov1.SslConfig{
							SslSecrets: &gloov1.SslConfig_SecretRef{
								SecretRef: &core.ResourceRef{
									Name:      string("areallygreatsecret"),
									Namespace: string("example"),
								},
							},
							SniDomains: []string{
								string("petes.com"),
							},
						},
					},
				},
			},
			Metadata: core.Metadata{
				Name:      string("clusteringress-proxy"),
				Namespace: string("example"),
			},
		}
		Expect(proxy).To(Equal(expected))
	})
})

func durptr(d int) *time.Duration {
	dur := time.Duration(d)
	return &dur
}
