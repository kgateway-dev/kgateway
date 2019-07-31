package translator

import (
	"time"

	"github.com/solo-io/gloo/projects/knative/api/external/knative"
	v1alpha12 "github.com/solo-io/gloo/projects/knative/pkg/api/external/knative"

	"github.com/knative/serving/pkg/apis/networking/v1alpha1"
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
		proxy, errs := translateProxy(namespace, snap)
		Expect(errs).NotTo(HaveOccurred())
		Expect(proxy.Metadata.Name).To(Equal("knative-proxy"))
		Expect(proxy.Listeners).To(HaveLen(2))
		Expect(proxy.Listeners[0].Name).To(Equal("http"))
		Expect(proxy.Listeners[0].BindPort).To(Equal(uint32(80)))

		//utter.Dump(proxy)
		expected := &gloov1.Proxy{
			Listeners: []*gloov1.Listener{
				{
					Name:        string("http"),
					BindAddress: string("::"),
					BindPort:    uint32(0x50),
					ListenerType: &gloov1.Listener_HttpListener{
						HttpListener: &gloov1.HttpListener{
							VirtualHosts: []*gloov1.VirtualHost{
								{
									Name: string("champ.net-http"),
									Domains: []string{
										string("champ.net"),
										string("champ.net:80"),
									},
									Routes: []*gloov1.Route{
										{
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
															DestinationType: &gloov1.Destination_Kube{
																Kube: &gloov1.KubernetesServiceDestination{
																	Ref:  core.ResourceRef{Name: serviceName, Namespace: serviceNamespace},
																	Port: uint32(servicePort),
																},
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
																AdvancedTemplates: false,
																Extractors:        map[string]*transformation.Extraction(nil),
																Headers: map[string]*transformation.InjaTemplate{
																	string("add"): {
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
								{
									Name: string("petes.com-http"),
									Domains: []string{
										string("petes.com"),
										string("petes.com:80"),
									},
									Routes: []*gloov1.Route{
										{
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
															DestinationType: &gloov1.Destination_Kube{
																Kube: &gloov1.KubernetesServiceDestination{
																	Ref:  core.ResourceRef{Name: serviceName, Namespace: serviceNamespace},
																	Port: uint32(servicePort),
																},
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
																AdvancedTemplates: false,
																Extractors:        map[string]*transformation.Extraction(nil),
																Headers: map[string]*transformation.InjaTemplate{
																	string("add"): {
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
								{
									Name: string("pog.com-http"),
									Domains: []string{
										string("pog.com"),
										string("pog.com:80"),
									},
									Routes: []*gloov1.Route{
										{
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
															DestinationType: &gloov1.Destination_Kube{
																Kube: &gloov1.KubernetesServiceDestination{
																	Ref:  core.ResourceRef{Name: serviceName, Namespace: serviceNamespace},
																	Port: uint32(servicePort),
																},
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
																AdvancedTemplates: false,
																Extractors:        map[string]*transformation.Extraction(nil),
																Headers: map[string]*transformation.InjaTemplate{
																	string("add"): {
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
								{
									Name: string("zah.net-http"),
									Domains: []string{
										string("zah.net"),
										string("zah.net:80"),
									},
									Routes: []*gloov1.Route{
										{
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
															DestinationType: &gloov1.Destination_Kube{
																Kube: &gloov1.KubernetesServiceDestination{
																	Ref:  core.ResourceRef{Name: serviceName, Namespace: serviceNamespace},
																	Port: uint32(servicePort),
																},
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
																AdvancedTemplates: false,
																Extractors:        map[string]*transformation.Extraction(nil),
																Headers: map[string]*transformation.InjaTemplate{
																	string("add"): {
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
										{
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
															DestinationType: &gloov1.Destination_Kube{
																Kube: &gloov1.KubernetesServiceDestination{
																	Ref:  core.ResourceRef{Name: serviceName, Namespace: serviceNamespace},
																	Port: uint32(servicePort),
																},
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
																AdvancedTemplates: false,
																Extractors:        map[string]*transformation.Extraction(nil),
																Headers: map[string]*transformation.InjaTemplate{
																	string("add"): {
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
										{
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
															DestinationType: &gloov1.Destination_Kube{
																Kube: &gloov1.KubernetesServiceDestination{
																	Ref:  core.ResourceRef{Name: serviceName, Namespace: serviceNamespace},
																	Port: uint32(servicePort),
																},
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
																AdvancedTemplates: false,
																Extractors:        map[string]*transformation.Extraction(nil),
																Headers: map[string]*transformation.InjaTemplate{
																	string("add"): {
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
					SslConfigurations: []*gloov1.SslConfig(nil),
				},
				{
					Name:        string("https"),
					BindAddress: string("::"),
					BindPort:    uint32(0x1bb),
					ListenerType: &gloov1.Listener_HttpListener{
						HttpListener: &gloov1.HttpListener{
							VirtualHosts: []*gloov1.VirtualHost{
								{
									Name: string("petes.com-http"),
									Domains: []string{
										string("petes.com"),
										string("petes.com:443"),
									},
									Routes: []*gloov1.Route{
										{
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
															DestinationType: &gloov1.Destination_Kube{
																Kube: &gloov1.KubernetesServiceDestination{
																	Ref:  core.ResourceRef{Name: serviceName, Namespace: serviceNamespace},
																	Port: uint32(servicePort),
																},
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
																AdvancedTemplates: false,
																Extractors:        map[string]*transformation.Extraction(nil),
																Headers: map[string]*transformation.InjaTemplate{
																	string("add"): {
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
					SslConfigurations: []*gloov1.SslConfig{
						{
							SslSecrets: &gloov1.SslConfig_SecretRef{
								SecretRef: &core.ResourceRef{
									Name:      string("areallygreatsecret"),
									Namespace: string("example"),
								},
							},
							SniDomains: []string{
								string("petes.com"),
								string("petes.com:443"),
							},
						},
					},
				},
			},
			Metadata: core.Metadata{
				Name:      string("knative-proxy"),
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
