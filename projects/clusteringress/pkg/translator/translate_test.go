package translator

import (
	"github.com/knative/serving/pkg/apis/networking/v1alpha1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/kubernetes"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	ingresstype "github.com/solo-io/gloo/projects/clusteringress/pkg/api/clusteringress"
	"github.com/solo-io/gloo/projects/clusteringress/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
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
						Hosts:      []string{"wow.com"},
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
			Clusteringresses: v1.ClusteringressesByNamespace{"hi": {ingressRes, ingressResTls}},
			Secrets:          gloov1.SecretsByNamespace{"hi": {secret}},
			Upstreams:        gloov1.UpstreamsByNamespace{"hi":{us, usSubset}},
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
														Upstream: core.ResourceRef{
															Name:      "wow-upstream",
															Namespace: "example",
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
						ListenerPlugins:      (*gloov1.ListenerPlugins)(nil),
						XXX_NoUnkeyedLiteral: struct{}{},
						XXX_unrecognized:     []uint8{},
						XXX_sizecache:        0,
					},
				},
				SslConfiguations:     []*gloov1.SslConfig{},
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
														Upstream: core.ResourceRef{
															Name:      "wow-upstream",
															Namespace: "example",
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
														Upstream: core.ResourceRef{
															Name:      "wow-upstream",
															Namespace: "example",
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
						ListenerPlugins:      (*gloov1.ListenerPlugins)(nil),
						XXX_NoUnkeyedLiteral: struct{}{},
						XXX_unrecognized:     []uint8{},
						XXX_sizecache:        0,
					},
				},
				SslConfiguations: []*gloov1.SslConfig{
					&gloov1.SslConfig{
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
