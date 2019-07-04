package translator

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/kubernetes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	ingresstype "github.com/solo-io/gloo/projects/ingress/pkg/api/ingress"
	v1 "github.com/solo-io/gloo/projects/ingress/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var _ = Describe("Translate", func() {
	It("creates the appropriate proxy object for the provided ingress objects", func() {
		namespace := "example"
		serviceName := "wow-service"
		servicePort := int32(80)
		secretName := "areallygreatsecret"
		ingress := &extensions.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ing",
				Namespace: namespace,
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "gloo",
				},
			},
			Spec: extensions.IngressSpec{
				Rules: []extensions.IngressRule{
					{
						Host: "wow.com",
						IngressRuleValue: extensions.IngressRuleValue{
							HTTP: &extensions.HTTPIngressRuleValue{
								Paths: []extensions.HTTPIngressPath{
									{
										Path: "/",
										Backend: extensions.IngressBackend{
											ServiceName: serviceName,
											ServicePort: intstr.IntOrString{
												Type:   intstr.Int,
												IntVal: servicePort,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		ingressTls := &extensions.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ing-tls",
				Namespace: namespace,
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "gloo",
				},
			},
			Spec: extensions.IngressSpec{
				TLS: []extensions.IngressTLS{
					{
						Hosts:      []string{"wow.com"},
						SecretName: secretName,
					},
				},
				Rules: []extensions.IngressRule{
					{
						Host: "wow.com",
						IngressRuleValue: extensions.IngressRuleValue{
							HTTP: &extensions.HTTPIngressRuleValue{
								Paths: []extensions.HTTPIngressPath{
									{
										Path: "/basic",
										Backend: extensions.IngressBackend{
											ServiceName: serviceName,
											ServicePort: intstr.IntOrString{
												Type:   intstr.Int,
												IntVal: servicePort,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		ingressTls2 := &extensions.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ing-tls-2",
				Namespace: namespace,
				Annotations: map[string]string{
					"kubernetes.io/ingress.class": "gloo",
				},
			},
			Spec: extensions.IngressSpec{
				TLS: []extensions.IngressTLS{
					{
						Hosts:      []string{"wow.com"},
						SecretName: secretName,
					},
				},
				Rules: []extensions.IngressRule{
					{
						Host: "wow.com",
						IngressRuleValue: extensions.IngressRuleValue{
							HTTP: &extensions.HTTPIngressRuleValue{
								Paths: []extensions.HTTPIngressPath{
									{
										Path: "/longestpathshouldcomesecond",
										Backend: extensions.IngressBackend{
											ServiceName: serviceName,
											ServicePort: intstr.IntOrString{
												Type:   intstr.Int,
												IntVal: servicePort,
											},
										},
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
		ingressResTls2, err := ingresstype.FromKube(ingressTls2)
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
			Ingresses: v1.IngressList{ingressRes, ingressResTls, ingressResTls2},
			Secrets:   gloov1.SecretList{secret},
			Upstreams: gloov1.UpstreamList{us, usSubset},
		}
		proxy, errs := translateProxy(namespace, snap)
		Expect(errs).NotTo(HaveOccurred())
		//log.Printf("%v", proxy)
		Expect(proxy.String()).To(Equal((&gloov1.Proxy{
			Listeners: []*gloov1.Listener{
				&gloov1.Listener{
					Name:        "http",
					BindAddress: "::",
					BindPort:    0x00000050,
					ListenerType: &gloov1.Listener_HttpListener{
						HttpListener: &gloov1.HttpListener{
							VirtualHosts: []*gloov1.VirtualHost{
								&gloov1.VirtualHost{
									Name: "wow.com-http",
									Domains: []string{
										"wow.com",
									},
									Routes: []*gloov1.Route{
										&gloov1.Route{
											Matcher: &gloov1.Matcher{
												PathSpecifier: &gloov1.Matcher_Regex{
													Regex: "/",
												},
												Headers:              []*gloov1.HeaderMatcher{},
												QueryParameters:      []*gloov1.QueryParameterMatcher{},
												Methods:              []string{},
												XXX_NoUnkeyedLiteral: struct{}{},
												XXX_unrecognized:     []uint8{},
												XXX_sizecache:        0,
											},
											Action: &gloov1.Route_RouteAction{
												RouteAction: &gloov1.RouteAction{
													Destination: &gloov1.RouteAction_Single{
														Single: &gloov1.Destination{
															DestinationType: &gloov1.Destination_Upstream{
																Upstream: &core.ResourceRef{
																	Name:      "wow-upstream",
																	Namespace: "example",
																},
															},
															DestinationSpec:      (*gloov1.DestinationSpec)(nil),
															XXX_NoUnkeyedLiteral: struct{}{},
															XXX_unrecognized:     []uint8{},
															XXX_sizecache:        0,
														},
													},
													XXX_NoUnkeyedLiteral: struct{}{},
													XXX_unrecognized:     []uint8{},
													XXX_sizecache:        0,
												},
											},
											RoutePlugins:         (*gloov1.RoutePlugins)(nil),
											XXX_NoUnkeyedLiteral: struct{}{},
											XXX_unrecognized:     []uint8{},
											XXX_sizecache:        0,
										},
									},
									VirtualHostPlugins:   (*gloov1.VirtualHostPlugins)(nil),
									XXX_NoUnkeyedLiteral: struct{}{},
									XXX_unrecognized:     []uint8{},
									XXX_sizecache:        0,
								},
							},
							Plugins:      (*gloov1.HttpListenerPlugins)(nil),
							XXX_NoUnkeyedLiteral: struct{}{},
							XXX_unrecognized:     []uint8{},
							XXX_sizecache:        0,
						},
					},
					SslConfigurations:    []*gloov1.SslConfig{},
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     []uint8{},
					XXX_sizecache:        0,
				},
				&gloov1.Listener{
					Name:        "https",
					BindAddress: "::",
					BindPort:    0x000001bb,
					ListenerType: &gloov1.Listener_HttpListener{
						HttpListener: &gloov1.HttpListener{
							VirtualHosts: []*gloov1.VirtualHost{
								&gloov1.VirtualHost{
									Name: "wow.com-http",
									Domains: []string{
										"wow.com",
									},
									Routes: []*gloov1.Route{
										&gloov1.Route{
											Matcher: &gloov1.Matcher{
												PathSpecifier: &gloov1.Matcher_Regex{
													Regex: "/longestpathshouldcomesecond",
												},
												Headers:              []*gloov1.HeaderMatcher{},
												QueryParameters:      []*gloov1.QueryParameterMatcher{},
												Methods:              []string{},
												XXX_NoUnkeyedLiteral: struct{}{},
												XXX_unrecognized:     []uint8{},
												XXX_sizecache:        0,
											},
											Action: &gloov1.Route_RouteAction{
												RouteAction: &gloov1.RouteAction{
													Destination: &gloov1.RouteAction_Single{
														Single: &gloov1.Destination{
															DestinationType: &gloov1.Destination_Upstream{
																Upstream: &core.ResourceRef{
																	Name:      "wow-upstream",
																	Namespace: "example",
																},
															},
															DestinationSpec:      (*gloov1.DestinationSpec)(nil),
															XXX_NoUnkeyedLiteral: struct{}{},
															XXX_unrecognized:     []uint8{},
															XXX_sizecache:        0,
														},
													},
													XXX_NoUnkeyedLiteral: struct{}{},
													XXX_unrecognized:     []uint8{},
													XXX_sizecache:        0,
												},
											},
											RoutePlugins:         (*gloov1.RoutePlugins)(nil),
											XXX_NoUnkeyedLiteral: struct{}{},
											XXX_unrecognized:     []uint8{},
											XXX_sizecache:        0,
										},
										&gloov1.Route{
											Matcher: &gloov1.Matcher{
												PathSpecifier: &gloov1.Matcher_Regex{
													Regex: "/basic",
												},
												Headers:              []*gloov1.HeaderMatcher{},
												QueryParameters:      []*gloov1.QueryParameterMatcher{},
												Methods:              []string{},
												XXX_NoUnkeyedLiteral: struct{}{},
												XXX_unrecognized:     []uint8{},
												XXX_sizecache:        0,
											},
											Action: &gloov1.Route_RouteAction{
												RouteAction: &gloov1.RouteAction{
													Destination: &gloov1.RouteAction_Single{
														Single: &gloov1.Destination{
															DestinationType: &gloov1.Destination_Upstream{
																Upstream: &core.ResourceRef{
																	Name:      "wow-upstream",
																	Namespace: "example",
																},
															},
															DestinationSpec:      (*gloov1.DestinationSpec)(nil),
															XXX_NoUnkeyedLiteral: struct{}{},
															XXX_unrecognized:     []uint8{},
															XXX_sizecache:        0,
														},
													},
													XXX_NoUnkeyedLiteral: struct{}{},
													XXX_unrecognized:     []uint8{},
													XXX_sizecache:        0,
												},
											},
											RoutePlugins:         (*gloov1.RoutePlugins)(nil),
											XXX_NoUnkeyedLiteral: struct{}{},
											XXX_unrecognized:     []uint8{},
											XXX_sizecache:        0,
										},
									},
									VirtualHostPlugins:   (*gloov1.VirtualHostPlugins)(nil),
									XXX_NoUnkeyedLiteral: struct{}{},
									XXX_unrecognized:     []uint8{},
									XXX_sizecache:        0,
								},
							},
							Plugins:      (*gloov1.HttpListenerPlugins)(nil),
							XXX_NoUnkeyedLiteral: struct{}{},
							XXX_unrecognized:     []uint8{},
							XXX_sizecache:        0,
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
							SniDomains:           []string{"wow.com"},
							XXX_NoUnkeyedLiteral: struct{}{},
							XXX_unrecognized:     []uint8{},
							XXX_sizecache:        0,
						},
					},
					XXX_NoUnkeyedLiteral: struct{}{},
					XXX_unrecognized:     []uint8{},
					XXX_sizecache:        0,
				},
			},
			Status: core.Status{
				State:               0,
				Reason:              "",
				ReportedBy:          "",
				SubresourceStatuses: map[string]*core.Status{},
			},
			Metadata: core.Metadata{
				Name:            "ingress-proxy",
				Namespace:       "example",
				ResourceVersion: "",
				Labels:          map[string]string{},
				Annotations:     map[string]string{},
			},
			XXX_NoUnkeyedLiteral: struct{}{},
			XXX_unrecognized:     []uint8{},
			XXX_sizecache:        0,
		}).String()))
	})
})
