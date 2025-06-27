package backendtlspolicy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	envoy_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoymatcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/utils/ptr"

	eiutils "github.com/kgateway-dev/kgateway/v2/internal/envoyinit/pkg/utils"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"

	gwv1a3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	kgwellknown "github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	pluginutils "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/utils"
)

var (
	ErrConfigMapNotFound = errors.New("ConfigMap not found")

	ErrCreatingTLSConfig = errors.New("TLS config error")

	ErrParsingTLSConfig = errors.New("TLS config parse error")

	ErrInvalidValidationSpec = errors.New("invalid validation spec")
)

var (
	backendTlsPolicyGvr       = gwv1a3.SchemeGroupVersion.WithResource("backendtlspolicies")
	backendTlsPolicyGroupKind = kgwellknown.BackendTLSPolicyGVK
)

type backendTlsPolicy struct {
	ct              time.Time
	transportSocket *envoy_config_core_v3.TransportSocket
}

var _ ir.PolicyIR = &backendTlsPolicy{}

func (d *backendTlsPolicy) CreationTime() time.Time {
	return d.ct
}

func (d *backendTlsPolicy) Equals(in any) bool {
	d2, ok := in.(*backendTlsPolicy)
	if !ok {
		return false
	}
	return proto.Equal(d.transportSocket, d2.transportSocket)
}

func registerTypes() {
	kubeclient.Register[*gwv1a3.BackendTLSPolicy](
		backendTlsPolicyGvr,
		backendTlsPolicyGroupKind,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.GatewayAPI().GatewayV1alpha3().BackendTLSPolicies(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.GatewayAPI().GatewayV1alpha3().BackendTLSPolicies(namespace).Watch(context.Background(), o)
		},
	)
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionsplug.Plugin {
	registerTypes()
	inf := kclient.NewDelayedInformer[*gwv1a3.BackendTLSPolicy](
		commoncol.Client, backendTlsPolicyGvr, kubetypes.StandardInformer,
		kclient.Filter{ObjectFilter: commoncol.Client.ObjectFilter()},
	)
	col := krt.WrapClient(inf, commoncol.KrtOpts.ToOptions("BackendTLSPolicy")...)

	translate := buildTranslateFunc(ctx, commoncol.ConfigMaps)
	tlsPolicyCol := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *gwv1a3.BackendTLSPolicy) *ir.PolicyWrapper {
		tlsPolicyIR, err := translate(krtctx, i)
		pol := &ir.PolicyWrapper{
			ObjectSource: ir.ObjectSource{
				Group:     backendTlsPolicyGroupKind.Group,
				Kind:      backendTlsPolicyGroupKind.Kind,
				Namespace: i.Namespace,
				Name:      i.Name,
			},
			Policy:     i,
			PolicyIR:   tlsPolicyIR,
			TargetRefs: pluginutils.TargetRefsToPolicyRefsWithSectionNameV1Alpha2(i.Spec.TargetRefs),
		}
		if err != nil {
			pol.Errors = []error{err}
		}
		return pol
	}, commoncol.KrtOpts.ToOptions("BackendTLSPolicyIRs")...)

	return extensionsplug.Plugin{
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			backendTlsPolicyGroupKind.GroupKind(): {
				Name:              "BackendTLSPolicy",
				Policies:          tlsPolicyCol,
				ProcessBackend:    processBackend,
				GetPolicyStatus:   getPolicyStatusFn(commoncol.CrudClient),
				PatchPolicyStatus: patchPolicyStatusFn(commoncol.CrudClient),
			},
		},
	}
}

func processBackend(ctx context.Context, polir ir.PolicyIR, in ir.BackendObjectIR, out *clusterv3.Cluster) {
	tlsPol, ok := polir.(*backendTlsPolicy)
	if !ok {
		return
	}
	if tlsPol.transportSocket == nil {
		return
	}
	out.TransportSocket = tlsPol.transportSocket
}

func buildTranslateFunc(
	ctx context.Context,
	cfgmaps krt.Collection[*corev1.ConfigMap],
) func(krtctx krt.HandlerContext, i *gwv1a3.BackendTLSPolicy) (*backendTlsPolicy, error) {
	return func(krtctx krt.HandlerContext, policyCR *gwv1a3.BackendTLSPolicy) (*backendTlsPolicy, error) {
		spec := policyCR.Spec
		policyIr := backendTlsPolicy{
			ct: policyCR.CreationTimestamp.Time,
		}
		validationContext := &envoy_tls_v3.CertificateValidationContext{}
		validationContext.MatchTypedSubjectAltNames = convertSubjectAltNames(spec.Validation)
		var tlsContextDefault *envoy_tls_v3.UpstreamTlsContext
		switch {
		case ptr.Deref(spec.Validation.WellKnownCACertificates, "") == gwv1a3.WellKnownCACertificatesSystem:
			sdsValidationCtx := &envoy_tls_v3.SdsSecretConfig{
				Name: eiutils.SystemCaSecretName,
			}

			hostname := string(spec.Validation.Hostname)
			tlsContextDefault = &envoy_tls_v3.UpstreamTlsContext{
				CommonTlsContext: &envoy_tls_v3.CommonTlsContext{
					ValidationContextType: &envoy_tls_v3.CommonTlsContext_CombinedValidationContext{
						CombinedValidationContext: &envoy_tls_v3.CommonTlsContext_CombinedCertificateValidationContext{
							DefaultValidationContext:         validationContext,
							ValidationContextSdsSecretConfig: sdsValidationCtx,
						},
					},
				},
				Sni: hostname,
			}

		case len(spec.Validation.CACertificateRefs) > 0:

			certRef := spec.Validation.CACertificateRefs[0]
			nn := types.NamespacedName{
				Name:      string(certRef.Name),
				Namespace: policyCR.Namespace,
			}
			cfgmap := krt.FetchOne(krtctx, cfgmaps, krt.FilterObjectName(nn))
			if cfgmap == nil {
				err := fmt.Errorf("%w: %v", ErrConfigMapNotFound, nn)
				slog.Error("error fetching policy", "error", err, "policy_name", policyCR.Name)
				return &policyIr, err
			}
			var err error
			tlsContextDefault, err = ResolveUpstreamSslConfig(*cfgmap, validationContext, string(spec.Validation.Hostname))
			if err != nil {
				perr := fmt.Errorf("%w: %v", ErrCreatingTLSConfig, err)
				slog.Error("error resolving TLS config", "error", perr, "policy_name", policyCR.Name)
				return &policyIr, perr
			}
		default:
			return &policyIr, ErrInvalidValidationSpec
		}

		typedConfig, err := utils.MessageToAny(tlsContextDefault)
		if err != nil {
			slog.Error("error converting TLS config to proto", "error", err, "policy", policyCR.Name)
			return &policyIr, ErrParsingTLSConfig
		}
		policyIr.transportSocket = &envoy_config_core_v3.TransportSocket{
			Name: wellknown.TransportSocketTls,
			ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{
				TypedConfig: typedConfig,
			},
		}

		return &policyIr, nil
	}
}

func convertSubjectAltNames(validation gwv1a3.BackendTLSPolicyValidation) []*envoy_tls_v3.SubjectAltNameMatcher {
	if len(validation.SubjectAltNames) == 0 {
		hostname := string(validation.Hostname)
		if hostname != "" {
			return []*envoy_tls_v3.SubjectAltNameMatcher{{
				SanType: envoy_tls_v3.SubjectAltNameMatcher_DNS,
				Matcher: &envoymatcher.StringMatcher{
					MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: hostname},
				},
			}}
		}
	}

	matchers := make([]*envoy_tls_v3.SubjectAltNameMatcher, 0, len(validation.SubjectAltNames))
	for _, san := range validation.SubjectAltNames {
		switch san.Type {
		case gwv1a3.HostnameSubjectAltNameType:
			matchers = append(matchers, &envoy_tls_v3.SubjectAltNameMatcher{
				SanType: envoy_tls_v3.SubjectAltNameMatcher_DNS,
				Matcher: &envoymatcher.StringMatcher{
					MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: string(san.Hostname)},
				},
			})
		case gwv1a3.URISubjectAltNameType:
			matchers = append(matchers, &envoy_tls_v3.SubjectAltNameMatcher{
				SanType: envoy_tls_v3.SubjectAltNameMatcher_URI,
				Matcher: &envoymatcher.StringMatcher{
					MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: string(san.URI)},
				},
			})
		}
	}
	return matchers
}
