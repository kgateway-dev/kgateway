package proxy_syncer

import (
	"context"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

func TestGatewayBackendVariantBackendsRetainBackendPolicies(t *testing.T) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "backend-svc",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Port: 443,
			}},
		},
	}
	backendTLSPolicy := &gwv1.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "backend-tls",
			Namespace:  "default",
			Generation: 1,
		},
	}
	policyWrapper := ir.PolicyWrapper{
		ObjectSource: ir.ObjectSource{
			Group:     wellknown.BackendTLSPolicyGVK.Group,
			Kind:      wellknown.BackendTLSPolicyKind,
			Namespace: "default",
			Name:      "backend-tls",
		},
		Policy:   backendTLSPolicy,
		PolicyIR: &gatewayBackendVariantPolicyIR{},
		TargetRefs: []ir.PolicyRef{{
			Group: "",
			Kind:  "Service",
			Name:  "backend-svc",
		}},
	}

	mock := krttest.NewMock(t, []any{service, policyWrapper})
	services := krttest.GetMockCollection[*corev1.Service](mock)
	policyCol := krttest.GetMockCollection[ir.PolicyWrapper](mock)
	refGrants := krtcollections.NewRefGrantIndex(krttest.GetMockCollection[*gwv1b1.ReferenceGrant](mock))

	policyGK := wellknown.BackendTLSPolicyGVK.GroupKind()
	policies := krtcollections.NewPolicyIndex(krtutil.KrtOptions{}, sdk.ContributesPolicies{
		policyGK: {
			Policies: policyCol,
			ProcessBackend: func(ctx context.Context, polir ir.PolicyIR, backend ir.BackendObjectIR, out *envoyclusterv3.Cluster) {
				typedConfig, err := utils.MessageToAny(&envoytlsv3.UpstreamTlsContext{
					Sni: "backend.example.com",
					CommonTlsContext: &envoytlsv3.CommonTlsContext{
						ValidationContextType: &envoytlsv3.CommonTlsContext_ValidationContext{
							ValidationContext: &envoytlsv3.CertificateValidationContext{
								TrustedCa: &envoycorev3.DataSource{
									Specifier: &envoycorev3.DataSource_InlineString{
										InlineString: "trusted-ca",
									},
								},
							},
						},
					},
				})
				require.NoError(t, err)
				out.TransportSocket = &envoycorev3.TransportSocket{
					Name: envoywellknown.TransportSocketTls,
					ConfigType: &envoycorev3.TransportSocket_TypedConfig{
						TypedConfig: typedConfig,
					},
				}
			},
		},
	}, apisettings.Settings{})

	backendIndex := krtcollections.NewBackendIndex(krtutil.KrtOptions{}, policies, refGrants)
	serviceGK := schema.GroupKind{Kind: "Service"}
	backendIndex.AddBackends(serviceGK, serviceBackends(services))

	services.WaitUntilSynced(nil)
	policyCol.WaitUntilSynced(nil)
	require.Eventually(t, backendIndex.HasSynced, time.Second, 10*time.Millisecond)

	rawBackend, err := backendIndex.GetBackendFromRef(
		krt.TestingDummyContext{},
		ir.ObjectSource{
			Group:     gwv1.GroupVersion.Group,
			Kind:      "HTTPRoute",
			Namespace: "default",
			Name:      "route",
		},
		gwv1.BackendObjectReference{
			Name: "backend-svc",
			Port: ptr.To(gwv1.PortNumber(443)),
		},
	)
	require.NoError(t, err)
	require.NotNil(t, rawBackend)
	require.Len(t, rawBackend.AttachedPolicies.Policies[policyGK], 1)

	clone := rawBackend.CloneForGatewayBackendClientCertificate(
		ir.ObjectSource{
			Group:     gwv1.GroupVersion.Group,
			Kind:      "Gateway",
			Namespace: "default",
			Name:      "example-gateway",
		},
		&ir.GatewayBackendClientCertificateIR{
			Certificate: ir.TLSCertificate{
				CertChain:  []byte("gateway-cert"),
				PrivateKey: []byte("gateway-key"),
			},
		},
	)
	variantBackends := krt.NewStaticCollection(nil, []ir.BackendObjectIR{clone}, krt.WithName("GatewayBackendClientCertificateVariantBackends"))
	attachedVariantBackends, _ := backendIndex.AttachPoliciesToCollection(variantBackends, "GatewayBackendClientCertificateVariantBackendsWithPolicy")
	attachedVariantBackends.WaitUntilSynced(nil)

	attachedVariant := krt.FetchOne(krt.TestingDummyContext{}, attachedVariantBackends, krt.FilterKey(clone.ResourceName()))
	require.NotNil(t, attachedVariant)
	variantBackend := *attachedVariant
	require.NotNil(t, variantBackend)
	require.Len(t, variantBackend.AttachedPolicies.Policies[policyGK], 1)
	require.NotNil(t, variantBackend.GatewayBackendClientCertificate)

	translator := irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			serviceGK: {
				InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					return nil
				},
			},
		},
		ContributedPolicies: map[schema.GroupKind]sdk.PolicyPlugin{
			policyGK: {
				ProcessBackend: func(ctx context.Context, polir ir.PolicyIR, backend ir.BackendObjectIR, out *envoyclusterv3.Cluster) {
					typedConfig, err := utils.MessageToAny(&envoytlsv3.UpstreamTlsContext{
						Sni: "backend.example.com",
						CommonTlsContext: &envoytlsv3.CommonTlsContext{
							ValidationContextType: &envoytlsv3.CommonTlsContext_ValidationContext{
								ValidationContext: &envoytlsv3.CertificateValidationContext{
									TrustedCa: &envoycorev3.DataSource{
										Specifier: &envoycorev3.DataSource_InlineString{
											InlineString: "trusted-ca",
										},
									},
								},
							},
						},
					})
					require.NoError(t, err)
					out.TransportSocket = &envoycorev3.TransportSocket{
						Name: envoywellknown.TransportSocketTls,
						ConfigType: &envoycorev3.TransportSocket_TypedConfig{
							TypedConfig: typedConfig,
						},
					}
				},
			},
		},
	}

	cluster, err := translator.TranslateBackend(context.Background(), krt.TestingDummyContext{}, ir.UniqlyConnectedClient{}, variantBackend)
	require.NoError(t, err)
	require.NotNil(t, cluster)
	require.NotNil(t, cluster.TransportSocket)

	tlsContext := &envoytlsv3.UpstreamTlsContext{}
	require.NoError(t, cluster.TransportSocket.GetTypedConfig().UnmarshalTo(tlsContext))
	require.NotNil(t, tlsContext.GetCommonTlsContext().GetValidationContext())
	require.Len(t, tlsContext.GetCommonTlsContext().GetTlsCertificates(), 1)
	assert.Equal(t, "trusted-ca", tlsContext.GetCommonTlsContext().GetValidationContext().GetTrustedCa().GetInlineString())
	assert.Equal(t, "gateway-cert", tlsContext.GetCommonTlsContext().GetTlsCertificates()[0].GetCertificateChain().GetInlineString())
	assert.Equal(t, "gateway-key", tlsContext.GetCommonTlsContext().GetTlsCertificates()[0].GetPrivateKey().GetInlineString())
}

func serviceBackends(services krt.Collection[*corev1.Service]) krt.Collection[ir.BackendObjectIR] {
	return krt.NewManyCollection(services, func(kctx krt.HandlerContext, svc *corev1.Service) []ir.BackendObjectIR {
		backends := make([]ir.BackendObjectIR, 0, len(svc.Spec.Ports))
		for _, port := range svc.Spec.Ports {
			backend := ir.NewBackendObjectIR(ir.ObjectSource{
				Kind:      "Service",
				Namespace: svc.Namespace,
				Name:      svc.Name,
			}, port.Port, "")
			backend.Obj = svc
			backends = append(backends, backend)
		}
		return backends
	}, krt.WithName("ServiceBackends"))
}

type gatewayBackendVariantPolicyIR struct{}

func (g *gatewayBackendVariantPolicyIR) CreationTime() time.Time {
	return time.Time{}
}

func (g *gatewayBackendVariantPolicyIR) Equals(other any) bool {
	_, ok := other.(*gatewayBackendVariantPolicyIR)
	return ok
}
