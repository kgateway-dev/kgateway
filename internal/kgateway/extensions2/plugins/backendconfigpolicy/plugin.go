package backendconfigpolicy

import (
	"context"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	preserve_case_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/http/header_formatters/preserve_case/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	skubeclient "istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
)

const PreserveCasePlugin = "envoy.http.stateful_header_formatters.preserve_case"

type BackendConfigPolicyIR struct {
	ct                            time.Time
	maxRequestsPerConnection      *int
	connectTimeout                *durationpb.Duration
	perConnectionBufferLimitBytes *int
	TCPKeepalive                  *corev3.TcpKeepalive
	commonHttpProtocolOptions     *corev3.HttpProtocolOptions
	http1ProtocolOptions          *corev3.Http1ProtocolOptions
}

var _ ir.PolicyIR = &BackendConfigPolicyIR{}

func (d *BackendConfigPolicyIR) CreationTime() time.Time {
	return d.ct
}

func (d *BackendConfigPolicyIR) Equals(other any) bool {
	d2, ok := other.(*BackendConfigPolicyIR)
	if !ok {
		return false
	}
	return d.maxRequestsPerConnection == d2.maxRequestsPerConnection
}

func registerTypes(ourCli versioned.Interface) {
	skubeclient.Register[*v1alpha1.BackendConfigPolicy](
		wellknown.BackendConfigPolicyGVR,
		wellknown.BackendConfigPolicyGVK,
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return ourCli.GatewayV1alpha1().BackendConfigPolicies(namespace).List(context.Background(), o)
		},
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return ourCli.GatewayV1alpha1().BackendConfigPolicies(namespace).Watch(context.Background(), o)
		},
	)
}
func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionsplug.Plugin {
	registerTypes(commoncol.OurClient)
	col := krt.WrapClient(kclient.NewFiltered[*v1alpha1.BackendConfigPolicy](
		commoncol.Client,
		kclient.Filter{ObjectFilter: commoncol.Client.ObjectFilter()},
	), commoncol.KrtOpts.ToOptions("BackendConfigPolicy")...)
	backendConfigPolicyCol := krt.NewCollection(col, func(krtctx krt.HandlerContext, b *v1alpha1.BackendConfigPolicy) *ir.PolicyWrapper {
		objSrc := ir.ObjectSource{
			Group:     wellknown.BackendConfigPolicyGVK.Group,
			Kind:      wellknown.BackendConfigPolicyGVK.Kind,
			Namespace: b.Namespace,
			Name:      b.Name,
		}

		policyIR, err := translate(b)
		return &ir.PolicyWrapper{
			ObjectSource: objSrc,
			Policy:       b,
			PolicyIR:     policyIR,
			TargetRefs:   convertTargetRefs(b.Spec.TargetRefs),
			Errors:       []error{err},
		}
	}, commoncol.KrtOpts.ToOptions("BackendConfigPolicyIRs")...)
	return extensionsplug.Plugin{
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			wellknown.BackendConfigPolicyGVK.GroupKind(): {
				Name:           "BackendConfigPolicy",
				Policies:       backendConfigPolicyCol,
				ProcessBackend: processBackend,
			},
		},
	}
}

func processBackend(_ context.Context, polir ir.PolicyIR, _ ir.BackendObjectIR, out *clusterv3.Cluster) {
	pol := polir.(*BackendConfigPolicyIR)
	if pol.maxRequestsPerConnection != nil {
		out.MaxRequestsPerConnection = &wrapperspb.UInt32Value{Value: uint32(*pol.maxRequestsPerConnection)}
	}

	if pol.connectTimeout != nil {
		out.ConnectTimeout = pol.connectTimeout
	}

	if pol.perConnectionBufferLimitBytes != nil {
		out.PerConnectionBufferLimitBytes = &wrapperspb.UInt32Value{Value: uint32(*pol.perConnectionBufferLimitBytes)}
	}

	if pol.TCPKeepalive != nil {
		out.UpstreamConnectionOptions = &clusterv3.UpstreamConnectionOptions{
			TcpKeepalive: pol.TCPKeepalive,
		}
	}

	if pol.commonHttpProtocolOptions != nil {
		out.CommonHttpProtocolOptions = pol.commonHttpProtocolOptions
	}

	if pol.http1ProtocolOptions != nil {
		out.HttpProtocolOptions = pol.http1ProtocolOptions
	}
}

func translate(pol *v1alpha1.BackendConfigPolicy) (*BackendConfigPolicyIR, error) {
	ir := &BackendConfigPolicyIR{}
	if pol.Spec.MaxRequestsPerConnection != nil {
		ir.maxRequestsPerConnection = pol.Spec.MaxRequestsPerConnection
	}
	if pol.Spec.ConnectTimeout != nil {
		timeout, err := time.ParseDuration(string(*pol.Spec.ConnectTimeout))
		if err != nil {
			return nil, err
		}
		ir.connectTimeout = durationpb.New(timeout)
	}
	if pol.Spec.PerConnectionBufferLimitBytes != nil {
		ir.perConnectionBufferLimitBytes = pol.Spec.PerConnectionBufferLimitBytes
	}

	if pol.Spec.TCPKeepalive != nil {
		tcpKeepalive, err := translateTCPKeepalive(pol.Spec.TCPKeepalive)
		if err != nil {
			return nil, err
		}
		ir.TCPKeepalive = tcpKeepalive
	}

	if pol.Spec.CommonHttpProtocolOptions != nil {
		commonHttpProtocolOptions, err := translateCommonHttpProtocolOptions(pol.Spec.CommonHttpProtocolOptions)
		if err != nil {
			return nil, err
		}
		ir.commonHttpProtocolOptions = commonHttpProtocolOptions
	}

	if pol.Spec.Http1ProtocolOptions != nil {
		http1ProtocolOptions, err := translateHttp1ProtocolOptions(pol.Spec.Http1ProtocolOptions)
		if err != nil {
			return nil, err
		}
		ir.http1ProtocolOptions = http1ProtocolOptions
	}

	return ir, nil
}

func translateTCPKeepalive(tcpKeepalive *v1alpha1.TCPKeepalive) (*corev3.TcpKeepalive, error) {
	out := &corev3.TcpKeepalive{}
	if tcpKeepalive.KeepAliveProbes != nil {
		out.KeepaliveProbes = &wrapperspb.UInt32Value{Value: uint32(*tcpKeepalive.KeepAliveProbes)}
	}
	if tcpKeepalive.KeepAliveTime != nil {
		keepAliveTime, err := time.ParseDuration(string(*tcpKeepalive.KeepAliveTime))
		if err != nil {
			return nil, err
		}
		out.KeepaliveTime = &wrapperspb.UInt32Value{Value: uint32(keepAliveTime.Seconds())}
	}
	if tcpKeepalive.KeepAliveInterval != nil {
		keepAliveInterval, err := time.ParseDuration(string(*tcpKeepalive.KeepAliveInterval))
		if err != nil {
			return nil, err
		}
		out.KeepaliveInterval = &wrapperspb.UInt32Value{Value: uint32(keepAliveInterval.Seconds())}
	}
	return out, nil
}

func translateCommonHttpProtocolOptions(commonHttpProtocolOptions *v1alpha1.CommonHttpProtocolOptions) (*corev3.HttpProtocolOptions, error) {
	out := &corev3.HttpProtocolOptions{}
	if commonHttpProtocolOptions.IdleTimeout != nil {
		idleTimeout, err := time.ParseDuration(string(*commonHttpProtocolOptions.IdleTimeout))
		if err != nil {
			return nil, err
		}
		out.IdleTimeout = durationpb.New(idleTimeout)
	}

	if commonHttpProtocolOptions.MaxHeadersCount != nil {
		out.MaxHeadersCount = &wrapperspb.UInt32Value{Value: uint32(*commonHttpProtocolOptions.MaxHeadersCount)}
	}

	if commonHttpProtocolOptions.MaxStreamDuration != nil {
		maxStreamDuration, err := time.ParseDuration(string(*commonHttpProtocolOptions.MaxStreamDuration))
		if err != nil {
			return nil, err
		}
		out.MaxStreamDuration = durationpb.New(maxStreamDuration)
	}

	if commonHttpProtocolOptions.HeadersWithUnderscoresAction != nil {
		switch *commonHttpProtocolOptions.HeadersWithUnderscoresAction {
		case v1alpha1.AllowHeadersWithUnderscores:
			out.HeadersWithUnderscoresAction = corev3.HttpProtocolOptions_ALLOW
		case v1alpha1.RejectRequestsHeadersWithUnderscores:
			out.HeadersWithUnderscoresAction = corev3.HttpProtocolOptions_REJECT_REQUEST
		case v1alpha1.DropHeadersWithUnderscores:
			out.HeadersWithUnderscoresAction = corev3.HttpProtocolOptions_DROP_HEADER
		}
	}
	return out, nil
}

func translateHttp1ProtocolOptions(http1ProtocolOptions *v1alpha1.Http1ProtocolOptions) (*corev3.Http1ProtocolOptions, error) {
	out := &corev3.Http1ProtocolOptions{}
	if http1ProtocolOptions.EnableTrailers != nil {
		out.EnableTrailers = *http1ProtocolOptions.EnableTrailers
	}

	if http1ProtocolOptions.OverrideStreamErrorOnInvalidHttpMessage != nil {
		out.OverrideStreamErrorOnInvalidHttpMessage = &wrapperspb.BoolValue{Value: *http1ProtocolOptions.OverrideStreamErrorOnInvalidHttpMessage}
	}

	if http1ProtocolOptions.HeaderFormat != nil {
		switch *http1ProtocolOptions.HeaderFormat {
		case v1alpha1.ProperCaseHeaderKeyFormat:
			out.HeaderKeyFormat = &corev3.Http1ProtocolOptions_HeaderKeyFormat{
				HeaderFormat: &corev3.Http1ProtocolOptions_HeaderKeyFormat_ProperCaseWords_{
					ProperCaseWords: &corev3.Http1ProtocolOptions_HeaderKeyFormat_ProperCaseWords{},
				},
			}
		case v1alpha1.PreserveCaseHeaderKeyFormat:
			typedConfig, err := utils.MessageToAny(&preserve_case_v3.PreserveCaseFormatterConfig{})
			if err != nil {
				return nil, err
			}
			out.HeaderKeyFormat = &corev3.Http1ProtocolOptions_HeaderKeyFormat{
				HeaderFormat: &corev3.Http1ProtocolOptions_HeaderKeyFormat_StatefulFormatter{
					StatefulFormatter: &corev3.TypedExtensionConfig{
						Name:        PreserveCasePlugin,
						TypedConfig: typedConfig,
					},
				},
			}
		}
	}
	return out, nil
}

// convertTargetRefs converts []v1alpha1.LocalPolicyTargetReference to []ir.PolicyRef
func convertTargetRefs(targetRefs []v1alpha1.LocalPolicyTargetReference) []ir.PolicyRef {
	refs := make([]ir.PolicyRef, 0, len(targetRefs))
	for _, targetRef := range targetRefs {
		refs = append(refs, ir.PolicyRef{
			Kind:  string(targetRef.Kind),
			Name:  string(targetRef.Name),
			Group: string(targetRef.Group),
		})
	}
	return refs
}
