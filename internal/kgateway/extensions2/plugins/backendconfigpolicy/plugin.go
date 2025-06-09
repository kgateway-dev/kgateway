package backendconfigpolicy

import (
	"context"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	preserve_case_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/http/header_formatters/preserve_case/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_upstreams_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	skubeclient "istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"

	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	translatorutils "github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
)

const PreserveCasePlugin = "envoy.http.stateful_header_formatters.preserve_case"

type BackendConfigPolicyIR struct {
	ct                            time.Time
	connectTimeout                *durationpb.Duration
	perConnectionBufferLimitBytes *int
	tcpKeepalive                  *corev3.TcpKeepalive
	commonHttpProtocolOptions     *corev3.HttpProtocolOptions
	http1ProtocolOptions          *corev3.Http1ProtocolOptions
	sslConfig                     *envoyauth.UpstreamTlsContext
}

var logger = logging.New("backendconfigpolicy")

var _ ir.PolicyIR = &BackendConfigPolicyIR{}

func (d *BackendConfigPolicyIR) CreationTime() time.Time {
	return d.ct
}

func (d *BackendConfigPolicyIR) Equals(other any) bool {
	d2, ok := other.(*BackendConfigPolicyIR)
	if !ok {
		return false
	}

	if !d.ct.Equal(d2.ct) {
		return false
	}

	if (d.connectTimeout == nil) != (d2.connectTimeout == nil) {
		return false
	}
	if d.connectTimeout != nil && d2.connectTimeout != nil {
		if !proto.Equal(d.connectTimeout, d2.connectTimeout) {
			return false
		}
	}

	if (d.perConnectionBufferLimitBytes == nil) != (d2.perConnectionBufferLimitBytes == nil) {
		return false
	}
	if d.perConnectionBufferLimitBytes != nil && d2.perConnectionBufferLimitBytes != nil {
		if *d.perConnectionBufferLimitBytes != *d2.perConnectionBufferLimitBytes {
			return false
		}
	}

	if (d.tcpKeepalive == nil) != (d2.tcpKeepalive == nil) {
		return false
	}
	if d.tcpKeepalive != nil && d2.tcpKeepalive != nil {
		if !proto.Equal(d.tcpKeepalive, d2.tcpKeepalive) {
			return false
		}
	}

	if (d.commonHttpProtocolOptions == nil) != (d2.commonHttpProtocolOptions == nil) {
		return false
	}
	if d.commonHttpProtocolOptions != nil && d2.commonHttpProtocolOptions != nil {
		if !proto.Equal(d.commonHttpProtocolOptions, d2.commonHttpProtocolOptions) {
			return false
		}
	}

	if (d.http1ProtocolOptions == nil) != (d2.http1ProtocolOptions == nil) {
		return false
	}
	if d.http1ProtocolOptions != nil && d2.http1ProtocolOptions != nil {
		if !proto.Equal(d.http1ProtocolOptions, d2.http1ProtocolOptions) {
			return false
		}
	}

	if (d.sslConfig == nil) != (d2.sslConfig == nil) {
		return false
	}
	if d.sslConfig != nil && d2.sslConfig != nil {
		if !proto.Equal(d.sslConfig, d2.sslConfig) {
			return false
		}
	}

	return true
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

		policyIR, err := translate(commoncol, krtctx, b)
		errs := []error{}
		if err != nil {
			errs = append(errs, err)
		}
		return &ir.PolicyWrapper{
			ObjectSource: objSrc,
			Policy:       b,
			PolicyIR:     policyIR,
			TargetRefs:   pluginutils.TargetRefsToPolicyRefs(b.Spec.TargetRefs, b.Spec.TargetSelectors),
			Errors:       errs,
		}
	}, commoncol.KrtOpts.ToOptions("BackendConfigPolicyIRs")...)
	return extensionsplug.Plugin{
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			wellknown.BackendConfigPolicyGVK.GroupKind(): {
				Name:              "BackendConfigPolicy",
				Policies:          backendConfigPolicyCol,
				ProcessBackend:    processBackend,
				GetPolicyStatus:   getPolicyStatusFn(commoncol.CrudClient),
				PatchPolicyStatus: patchPolicyStatusFn(commoncol.CrudClient),
			},
		},
	}
}

func processBackend(_ context.Context, polir ir.PolicyIR, _ ir.BackendObjectIR, out *clusterv3.Cluster) {
	pol := polir.(*BackendConfigPolicyIR)
	if pol.connectTimeout != nil {
		out.ConnectTimeout = pol.connectTimeout
	}

	if pol.perConnectionBufferLimitBytes != nil {
		out.PerConnectionBufferLimitBytes = &wrapperspb.UInt32Value{Value: uint32(*pol.perConnectionBufferLimitBytes)}
	}

	if pol.tcpKeepalive != nil {
		out.UpstreamConnectionOptions = &clusterv3.UpstreamConnectionOptions{
			TcpKeepalive: pol.tcpKeepalive,
		}
	}

	if pol.commonHttpProtocolOptions != nil {
		if err := translatorutils.MutateHttpOptions(out, func(opts *envoy_upstreams_v3.HttpProtocolOptions) {
			opts.CommonHttpProtocolOptions = pol.commonHttpProtocolOptions
		}); err != nil {
			logger.Error("failed to apply common http protocol options", "error", err)
		}
	}

	if pol.http1ProtocolOptions != nil {
		if err := translatorutils.MutateHttpOptions(out, func(opts *envoy_upstreams_v3.HttpProtocolOptions) {
			opts.UpstreamProtocolOptions = &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_{
				ExplicitHttpConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig{
					ProtocolConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_HttpProtocolOptions{
						HttpProtocolOptions: pol.http1ProtocolOptions,
					},
				},
			}
		}); err != nil {
			logger.Error("failed to apply http1 protocol options", "error", err)
		}
	}

	if pol.sslConfig != nil {
		typedConfig, err := utils.MessageToAny(pol.sslConfig)
		if err != nil {
			logger.Error("failed to convert ssl config to any", "error", err)
			return
		}
		out.TransportSocket = &corev3.TransportSocket{
			Name: envoywellknown.TransportSocketTls,
			ConfigType: &corev3.TransportSocket_TypedConfig{
				TypedConfig: typedConfig,
			},
		}
	}
}

func translate(commoncol *common.CommonCollections, krtctx krt.HandlerContext, pol *v1alpha1.BackendConfigPolicy) (*BackendConfigPolicyIR, error) {
	ir := BackendConfigPolicyIR{
		ct: pol.CreationTimestamp.Time,
	}
	if pol.Spec.ConnectTimeout != nil {
		ir.connectTimeout = durationpb.New(pol.Spec.ConnectTimeout.Duration)
	}
	if pol.Spec.PerConnectionBufferLimitBytes != nil {
		ir.perConnectionBufferLimitBytes = pol.Spec.PerConnectionBufferLimitBytes
	}

	if pol.Spec.TCPKeepalive != nil {
		ir.tcpKeepalive = translateTCPKeepalive(pol.Spec.TCPKeepalive)
	}

	if pol.Spec.CommonHttpProtocolOptions != nil {
		ir.commonHttpProtocolOptions = translateCommonHttpProtocolOptions(pol.Spec.CommonHttpProtocolOptions)
	}

	if pol.Spec.Http1ProtocolOptions != nil {
		http1ProtocolOptions, err := translateHttp1ProtocolOptions(pol.Spec.Http1ProtocolOptions)
		if err != nil {
			logger.Error("failed to translate http1 protocol options", "error", err)
			return &ir, err
		}
		ir.http1ProtocolOptions = http1ProtocolOptions
	}

	if pol.Spec.SSLConfig != nil {
		sslConfig, err := translateSSLConfig(NewDefaultSecretGetter(commoncol.Secrets, krtctx), pol.Spec.SSLConfig, pol.Namespace)
		if err != nil {
			return &ir, err
		}
		ir.sslConfig = sslConfig
	}

	return &ir, nil
}

func translateTCPKeepalive(tcpKeepalive *v1alpha1.TCPKeepalive) *corev3.TcpKeepalive {
	out := &corev3.TcpKeepalive{}
	if tcpKeepalive.KeepAliveProbes != nil {
		out.KeepaliveProbes = &wrapperspb.UInt32Value{Value: uint32(*tcpKeepalive.KeepAliveProbes)}
	}
	if tcpKeepalive.KeepAliveTime != nil {
		out.KeepaliveTime = &wrapperspb.UInt32Value{Value: uint32(tcpKeepalive.KeepAliveTime.Duration.Seconds())}
	}
	if tcpKeepalive.KeepAliveInterval != nil {
		out.KeepaliveInterval = &wrapperspb.UInt32Value{Value: uint32(tcpKeepalive.KeepAliveInterval.Duration.Seconds())}
	}
	return out
}

func translateCommonHttpProtocolOptions(commonHttpProtocolOptions *v1alpha1.CommonHttpProtocolOptions) *corev3.HttpProtocolOptions {
	out := &corev3.HttpProtocolOptions{}
	if commonHttpProtocolOptions.MaxRequestsPerConnection != nil {
		out.MaxRequestsPerConnection = &wrapperspb.UInt32Value{Value: uint32(*commonHttpProtocolOptions.MaxRequestsPerConnection)}
	}
	if commonHttpProtocolOptions.IdleTimeout != nil {
		out.IdleTimeout = durationpb.New(commonHttpProtocolOptions.IdleTimeout.Duration)
	}

	if commonHttpProtocolOptions.MaxHeadersCount != nil {
		out.MaxHeadersCount = &wrapperspb.UInt32Value{Value: uint32(*commonHttpProtocolOptions.MaxHeadersCount)}
	}

	if commonHttpProtocolOptions.MaxStreamDuration != nil {
		out.MaxStreamDuration = durationpb.New(commonHttpProtocolOptions.MaxStreamDuration.Duration)
	}

	if commonHttpProtocolOptions.HeadersWithUnderscoresAction != nil {
		switch *commonHttpProtocolOptions.HeadersWithUnderscoresAction {
		case v1alpha1.HeadersWithUnderscoresActionAllow:
			out.HeadersWithUnderscoresAction = corev3.HttpProtocolOptions_ALLOW
		case v1alpha1.HeadersWithUnderscoresActionRejectRequest:
			out.HeadersWithUnderscoresAction = corev3.HttpProtocolOptions_REJECT_REQUEST
		case v1alpha1.HeadersWithUnderscoresActionDropHeader:
			out.HeadersWithUnderscoresAction = corev3.HttpProtocolOptions_DROP_HEADER
		}
	}
	return out
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
