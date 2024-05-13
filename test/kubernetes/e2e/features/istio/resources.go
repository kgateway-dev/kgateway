package istio

import (
	"fmt"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/projects/gloo/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1 "github.com/solo-io/solo-apis/pkg/api/gateway.solo.io/v1"
	soloapis_gloov1 "github.com/solo-io/solo-apis/pkg/api/gloo.solo.io/v1"
	"github.com/solo-io/solo-apis/pkg/api/gloo.solo.io/v1/core/matchers"
	soloapis_kubernetes "github.com/solo-io/solo-apis/pkg/api/gloo.solo.io/v1/options/kubernetes"
	"github.com/solo-io/solo-apis/pkg/api/gloo.solo.io/v1/ssl"
	gloocore "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var (
	EdgeApisRoutingResourcesFileName                = "edge-apis-routing.gen.yaml"
	DisableAutomtlsEdgeApisRoutingResourcesFileName = "disable-automtls-edge-apis-routing.gen.yaml"
	UpstreamSslConfigEdgeApisFileName               = "upstream-ssl-config-edge-apis.gen.yaml"

	httpbinSvc = &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "httpbin", Namespace: "httpbin"}}

	// GetGlooGatewayEdgeResources defines the Edge API resources for no sslConfig on Upstream
	GetGlooGatewayEdgeResources = func(installNamespace string, disableIstioAutoMtls, setSslConfig bool) []client.Object {
		var sslConfig *ssl.UpstreamSslConfig
		if setSslConfig {
			/*
				This should match the basic istio integration sslConfig:
					sslConfig:
					  alpnProtocols:
					  - istio
					  sds:
					    certificatesSecretName: istio_server_cert
					    clusterName: gateway_proxy_sds
					    targetUri: 127.0.0.1:8234
					    validationContextName: istio_validation_context
			*/
			sslConfig = &ssl.UpstreamSslConfig{
				AlpnProtocols: []string{"istio"},
				SslSecrets: &ssl.UpstreamSslConfig_Sds{
					Sds: &ssl.SDSConfig{
						CertificatesSecretName: constants.IstioCertSecret,
						SdsBuilder: &ssl.SDSConfig_ClusterName{
							ClusterName: constants.SdsClusterName,
						},
						TargetUri:             constants.SdsTargetURI,
						ValidationContextName: constants.IstioValidationContext,
					},
				},
			}
		}

		httpbinUpstream := &soloapis_gloov1.Upstream{
			TypeMeta: metav1.TypeMeta{
				Kind:       gloov1.UpstreamGVK.Kind,
				APIVersion: fmt.Sprintf("%s/%s", gloov1.UpstreamGVK.Group, gloov1.UpstreamGVK.Version),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin-upstream",
				Namespace: installNamespace,
			},
			Spec: soloapis_gloov1.UpstreamSpec{
				DisableIstioAutoMtls: &wrappers.BoolValue{Value: disableIstioAutoMtls},
				UpstreamType: &soloapis_gloov1.UpstreamSpec_Kube{
					Kube: &soloapis_kubernetes.UpstreamSpec{
						Selector: map[string]string{
							"app": "httpbin",
						},
						ServiceName:      httpbinSvc.GetName(),
						ServiceNamespace: httpbinSvc.GetNamespace(),
						ServicePort:      8000,
					},
				},
				SslConfig: sslConfig,
			},
		}

		headlessVs := &v1.VirtualService{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1.VirtualServiceGVK.Kind,
				APIVersion: fmt.Sprintf("%s/%s", v1.VirtualServiceGVK.Group, v1.VirtualServiceGVK.Version),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin-vs",
				Namespace: installNamespace,
			},
			Spec: v1.VirtualServiceSpec{
				VirtualHost: &v1.VirtualHost{
					Domains: []string{"httpbin"},
					Routes: []*v1.Route{{
						Matchers: []*matchers.Matcher{
							{
								PathSpecifier: &matchers.Matcher_Prefix{
									Prefix: "/",
								},
							},
						},
						Action: &v1.Route_RouteAction{
							RouteAction: &soloapis_gloov1.RouteAction{
								Destination: &soloapis_gloov1.RouteAction_Single{
									Single: &soloapis_gloov1.Destination{
										DestinationType: &soloapis_gloov1.Destination_Upstream{
											Upstream: &gloocore.ResourceRef{
												Name:      httpbinUpstream.Name,
												Namespace: httpbinUpstream.Namespace,
											},
										},
									},
								},
							},
						},
					}},
				},
			},
		}

		var resources []client.Object
		resources = append(resources, headlessVs, httpbinUpstream)
		return resources
	}
)
