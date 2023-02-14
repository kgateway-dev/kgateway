package translator

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	ingresstype "github.com/solo-io/gloo/projects/ingress/pkg/api/ingress"
	"github.com/solo-io/gloo/projects/ingress/pkg/api/service"
	v1 "github.com/solo-io/gloo/projects/ingress/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	kubev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/yaml"
)

var _ = Describe("Translate", func() {
	var (
		ctx = context.Background()
	)

	It("creates the appropriate proxy object for the provided ingress objects", func() {
		testIngressTranslate := func(requireIngressClass bool) {

			namespace := "example"
			serviceName := "wow-service"
			servicePort := int32(8080)
			secretName := "areallygreatsecret"
			pathType := networkingv1.PathTypeImplementationSpecific
			ingress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ing",
					Namespace: namespace,
					Annotations: map[string]string{
						IngressClassKey: "gloo",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "wow.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: &pathType,
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: serviceName,
													Port: networkingv1.ServiceBackendPort{
														Number: servicePort,
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
			}
			ingressTls := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ing-tls",
					Namespace: namespace,
					Annotations: map[string]string{
						IngressClassKey: "gloo",
					},
				},
				Spec: networkingv1.IngressSpec{
					TLS: []networkingv1.IngressTLS{
						{
							Hosts:      []string{"wow.com"},
							SecretName: secretName,
						},
					},
					Rules: []networkingv1.IngressRule{
						{
							Host: "wow.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/basic",
											PathType: &pathType,
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: serviceName,
													Port: networkingv1.ServiceBackendPort{
														Number: servicePort,
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
			}
			ingressTls2 := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ing-tls-2",
					Namespace: namespace,
					Annotations: map[string]string{
						IngressClassKey: "gloo",
					},
				},
				Spec: networkingv1.IngressSpec{
					TLS: []networkingv1.IngressTLS{
						{
							Hosts:      []string{"wow.com"},
							SecretName: secretName,
						},
					},
					Rules: []networkingv1.IngressRule{
						{
							Host: "wow.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/longestpathshouldcomesecond",
											PathType: &pathType,
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: serviceName,
													Port: networkingv1.ServiceBackendPort{
														Number: servicePort,
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
			}
			if !requireIngressClass {
				ingress.Annotations = nil
				ingressTls.Annotations = nil
				ingressTls2.Annotations = nil
			}
			ingressRes, err := ingresstype.FromKube(ingress)
			Expect(err).NotTo(HaveOccurred())
			ingressResTls, err := ingresstype.FromKube(ingressTls)
			Expect(err).NotTo(HaveOccurred())
			ingressResTls2, err := ingresstype.FromKube(ingressTls2)
			Expect(err).NotTo(HaveOccurred())
			us := &gloov1.Upstream{
				Metadata: &core.Metadata{
					Namespace: namespace,
					Name:      "wow-upstream",
				},
				UpstreamType: &gloov1.Upstream_Kube{
					Kube: &kubernetes.UpstreamSpec{
						ServiceNamespace: namespace,
						ServiceName:      serviceName,
						ServicePort:      uint32(servicePort),
						Selector: map[string]string{
							"a": "b",
						},
					},
				},
			}
			usSubset := &gloov1.Upstream{
				Metadata: &core.Metadata{
					Namespace: namespace,
					Name:      "wow-upstream-subset",
				},
				UpstreamType: &gloov1.Upstream_Kube{
					Kube: &kubernetes.UpstreamSpec{
						ServiceName: serviceName,
						ServicePort: uint32(servicePort),
						Selector: map[string]string{
							"a": "b",
							"c": "d",
						},
					},
				},
			}
			snap := &v1.TranslatorSnapshot{
				Ingresses: v1.IngressList{ingressRes, ingressResTls, ingressResTls2},
				Upstreams: gloov1.UpstreamList{us, usSubset},
			}
			proxy := translateProxy(ctx, namespace, snap, requireIngressClass, "")

			Expect(proxy.String()).To(Equal((&gloov1.Proxy{
				Listeners: []*gloov1.Listener{
					{
						Name:        "http",
						BindAddress: "::",
						BindPort:    8080,
						ListenerType: &gloov1.Listener_HttpListener{
							HttpListener: &gloov1.HttpListener{
								VirtualHosts: []*gloov1.VirtualHost{
									{
										Name: "wow.com-http",
										Domains: []string{
											"wow.com",
											"wow.com:8080",
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
														Destination: &gloov1.RouteAction_Single{
															Single: &gloov1.Destination{
																DestinationType: &gloov1.Destination_Upstream{
																	Upstream: &core.ResourceRef{
																		Name:      "wow-upstream",
																		Namespace: "example",
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
					},
					{
						Name:        "https",
						BindAddress: "::",
						BindPort:    8443,
						ListenerType: &gloov1.Listener_HttpListener{
							HttpListener: &gloov1.HttpListener{
								VirtualHosts: []*gloov1.VirtualHost{
									{
										Name: "wow.com-https",
										Domains: []string{
											"wow.com",
											"wow.com:8443",
										},
										Routes: []*gloov1.Route{
											{
												Matchers: []*matchers.Matcher{{
													PathSpecifier: &matchers.Matcher_Regex{
														Regex: "/longestpathshouldcomesecond",
													},
												}},
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
															},
														},
													},
												},
											},
											{
												Matchers: []*matchers.Matcher{{
													PathSpecifier: &matchers.Matcher_Regex{
														Regex: "/basic",
													},
												}},
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
						SslConfigurations: []*ssl.SslConfig{
							{
								SslSecrets: &ssl.SslConfig_SecretRef{
									SecretRef: &core.ResourceRef{
										Name:      "areallygreatsecret",
										Namespace: "example",
									},
								},
								SniDomains: []string{"wow.com", "wow.com:8443"},
							},
						},
					},
				},
				Metadata: &core.Metadata{
					Name:      "ingress-proxy",
					Namespace: "example",
				},
			}).String()))
		}
		testIngressTranslate(true)
		testIngressTranslate(false)
	})

	It("handles multiple secrets correctly", func() {
		ingresses := func() v1.IngressList {
			var ingressList networkingv1.IngressList
			err := yaml.Unmarshal([]byte(ingressExampleYaml), &ingressList)
			Expect(err).NotTo(HaveOccurred())

			var ingresses v1.IngressList
			for _, item := range ingressList.Items {
				ingress, err := ingresstype.FromKube(&item)
				Expect(err).NotTo(HaveOccurred())
				ingresses = append(ingresses, ingress)
			}
			return ingresses
		}()

		us1 := &gloov1.Upstream{
			Metadata: &core.Metadata{Namespace: "gloo-system", Name: "amoeba-dev-api-gateway-amoeba-dev-8080"},
			UpstreamType: &gloov1.Upstream_Kube{
				Kube: &kubernetes.UpstreamSpec{
					ServiceNamespace: "amoeba-dev",
					ServiceName:      "api-gateway-amoeba-dev",
					ServicePort:      uint32(8080),
				},
			},
		}

		us2 := &gloov1.Upstream{
			Metadata: &core.Metadata{Namespace: "gloo-system", Name: "amoeba-dev-api-gateway-amoeba-dev-8080"},
			UpstreamType: &gloov1.Upstream_Kube{
				Kube: &kubernetes.UpstreamSpec{
					ServiceNamespace: "amoeba-dev",
					ServiceName:      "amoeba-ui",
					ServicePort:      uint32(8080),
				},
			},
		}
		snap := &v1.TranslatorSnapshot{
			Ingresses: ingresses,
			Upstreams: gloov1.UpstreamList{us1, us2},
		}

		proxy := translateProxy(ctx, "gloo-system", snap, false, "")

		Expect(proxy.Listeners).To(HaveLen(1))
		Expect(proxy.Listeners[0].SslConfigurations).To(Equal([]*ssl.SslConfig{
			{
				SslSecrets: &ssl.SslConfig_SecretRef{
					SecretRef: &core.ResourceRef{
						Name:      "amoeba-api-ingress-secret",
						Namespace: "amoeba-dev",
					},
				},
				SniDomains: []string{
					"api-dev.intellishift.com",
					"api-dev.intellishift.com:8443",
				},
			},
			{
				SslSecrets: &ssl.SslConfig_SecretRef{
					SecretRef: &core.ResourceRef{
						Name:      "amoeba-ui-ingress-secret",
						Namespace: "amoeba-dev",
					},
				},
				SniDomains: []string{
					"ui-dev.intellishift.com",
					"ui-dev.intellishift.com:8443",
				},
			},
		}))
	})

	It("produces a proxy for valid ingresses and ignores invalid ones", func() {

		namespace := "ns"

		svc := makeService("svc", namespace, "http", 8081)
		port := intstr.IntOrString{Type: intstr.Int, IntVal: 8081}

		us := makeUpstream("us", namespace, svc)

		host1 := "host1"

		ing1 := makeIng("ing1", namespace, "", host1, "svc", port)
		ing2 := makeIng("invalid-svc", namespace, "", "host2", "svc-that-doesnt-exist", port)

		proxy := translateProxy(ctx, "write-namespace", &v1.TranslatorSnapshot{
			Upstreams: []*gloov1.Upstream{us},
			Services:  []*v1.KubeService{svc},
			Ingresses: []*v1.Ingress{ing1, ing2},
		}, false, "")

		Expect(proxy.Listeners).To(HaveLen(1))
		vhosts := proxy.Listeners[0].GetHttpListener().GetVirtualHosts()
		Expect(vhosts).To(HaveLen(1))
		// expect only ing1 to have been translated
		Expect(vhosts[0].Domains).To(Equal([]string{host1, host1 + ":8080"}))
	})

	It("respects a custom ingress class", func() {

		customClass1 := "fancy"
		customClass2 := "pants"

		namespace := "ns"

		svc := makeService("svc", namespace, "http", 8081)
		port := intstr.IntOrString{Type: intstr.Int, IntVal: 8081}

		us := makeUpstream("us", namespace, svc)

		host1 := "host1"

		ing1 := makeIng("ing1", namespace, customClass1, host1, "svc", port)
		ing2 := makeIng("ing2", namespace, customClass2, "host2", "svc", port)

		proxy := translateProxy(ctx, "write-namespace", &v1.TranslatorSnapshot{
			Upstreams: []*gloov1.Upstream{us},
			Services:  []*v1.KubeService{svc},
			Ingresses: []*v1.Ingress{ing1, ing2},
		}, true, customClass1)

		Expect(proxy.Listeners).To(HaveLen(1))
		vhosts := proxy.Listeners[0].GetHttpListener().GetVirtualHosts()
		Expect(vhosts).To(HaveLen(1))
		// expect only ing1 to have been translated
		Expect(vhosts[0].Domains).To(Equal([]string{host1, host1 + ":8080"}))
	})

	It("supports named ports", func() {

		namespace := "ns"

		svc := makeService("svc", namespace, "http", 8081)
		port := intstr.IntOrString{Type: intstr.String, StrVal: "http"}

		us := makeUpstream("us", namespace, svc)

		ing1 := makeIng("ing1", namespace, "", "host", "svc", port)

		proxy := translateProxy(ctx, "write-namespace", &v1.TranslatorSnapshot{
			Upstreams: []*gloov1.Upstream{us},
			Services:  []*v1.KubeService{svc},
			Ingresses: []*v1.Ingress{ing1},
		}, false, "")

		Expect(proxy.Listeners).To(HaveLen(1))
		vhosts := proxy.Listeners[0].GetHttpListener().GetVirtualHosts()
		// successful translation
		Expect(vhosts).To(HaveLen(1))
	})
})

func getFirstPort(svc *kubev1.Service) int32 {
	return svc.Spec.Ports[0].Port
}

func makeIng(name, namespace, ingressClass, host string, svcName string, servicePort intstr.IntOrString) *v1.Ingress {
	backendPort := networkingv1.ServiceBackendPort{}
	if servicePort.Type == intstr.Int {
		backendPort.Number = servicePort.IntVal
	} else {
		backendPort.Name = servicePort.StrVal
	}
	pathType := networkingv1.PathTypeImplementationSpecific
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				IngressClassKey: ingressClass,
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: svcName,
											Port: backendPort,
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
	ingType, _ := ingresstype.FromKube(ing)
	return ingType
}

func makeService(name, namespace, servicePortName string, servicePort int32) *v1.KubeService {
	svc, _ := service.FromKube(&kubev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: kubev1.ServiceSpec{
			Ports: []kubev1.ServicePort{{
				Name: servicePortName,
				Port: servicePort,
			}},
		},
	})

	return svc
}

func makeUpstream(name, namespace string, svc *v1.KubeService) *gloov1.Upstream {
	kubeSvc, _ := service.ToKube(svc)
	return &gloov1.Upstream{
		Metadata: &core.Metadata{
			Namespace: namespace,
			Name:      name,
		},
		UpstreamType: &gloov1.Upstream_Kube{
			Kube: &kubernetes.UpstreamSpec{
				ServiceNamespace: namespace,
				ServiceName:      kubeSvc.Name,
				ServicePort:      uint32(getFirstPort(kubeSvc)),
			},
		},
	}
}

const ingressExampleYaml = `
items:
- apiVersion: networking.k8s.io/v1
  kind: Ingress
  metadata:
    annotations:
      certmanager.k8s.io/cluster-issuer: letsencrypt-prod
      kubernetes.io/ingress.class: gloo
    creationTimestamp: "2019-09-09T17:41:10Z"
    generation: 1
    name: amoeba-api-ingress
    namespace: amoeba-dev
    resourceVersion: "26972626"
    selfLink: /apis/networking/v1/namespaces/amoeba-dev/ingresses/amoeba-api-ingress
    uid: 02c06c8f-d329-11e9-bc54-ce36377988a4
  spec:
    rules:
    - host: api-dev.intellishift.com
      http:
        paths:
        - backend:
            service:
              name: api-gateway-amoeba-dev
              port:
                number: 8080
          path: /
          pathType: ImplementationSpecific
    tls:
    - hosts:
      - api-dev.intellishift.com
      secretName: amoeba-api-ingress-secret
  status:
    loadBalancer: {}
- apiVersion: networking.k8s.io/v1
  kind: Ingress
  metadata:
    annotations:
      certmanager.k8s.io/issuer: amoeba-letsencrypt
      kubernetes.io/ingress.class: gloo
    creationTimestamp: "2019-09-09T17:41:10Z"
    generation: 1
    name: amoeba-ui-ingress
    namespace: amoeba-dev
    resourceVersion: "26972628"
    selfLink: /apis/networking/v1/namespaces/amoeba-dev/ingresses/amoeba-ui-ingress
    uid: 02c9b69a-d329-11e9-bc54-ce36377988a4
  spec:
    rules:
    - host: ui-dev.intellishift.com
      http:
        paths:
        - backend:
            service:
              name: amoeba-ui
              port:
                number: 8080
          path: /
          pathType: ImplementationSpecific
    tls:
    - hosts:
      - ui-dev.intellishift.com
      secretName: amoeba-ui-ingress-secret
  status:
    loadBalancer: {}
kind: List
metadata:
  resourceVersion: ""
  selfLink: ""
`
