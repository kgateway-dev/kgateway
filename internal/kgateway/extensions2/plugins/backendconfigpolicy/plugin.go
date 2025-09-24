package backendconfigpolicy

import (
	"context"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/proto"
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
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	pluginsdkutils "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmputils"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

const PreserveCasePlugin = "envoy.http.stateful_header_formatters.preserve_case"

type BackendConfigPolicyIR struct {
	ct                            time.Time
	connectTimeout                *durationpb.Duration
	perConnectionBufferLimitBytes *uint32
	tcpKeepalive                  *envoycorev3.TcpKeepalive
	commonHttpProtocolOptions     *envoycorev3.HttpProtocolOptions
	http1ProtocolOptions          *envoycorev3.Http1ProtocolOptions
	http2ProtocolOptions          *envoycorev3.Http2ProtocolOptions
	tlsConfig                     *envoytlsv3.UpstreamTlsContext
	loadBalancerConfig            *LoadBalancerConfigIR
	healthCheck                   *envoycorev3.HealthCheck
	outlierDetection              *envoyclusterv3.OutlierDetection
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

	if !proto.Equal(d.connectTimeout, d2.connectTimeout) {
		return false
	}

	if !cmputils.PointerValsEqual(d.perConnectionBufferLimitBytes, d2.perConnectionBufferLimitBytes) {
		return false
	}

	if !proto.Equal(d.tcpKeepalive, d2.tcpKeepalive) {
		return false
	}

	if !proto.Equal(d.commonHttpProtocolOptions, d2.commonHttpProtocolOptions) {
		return false
	}

	if !proto.Equal(d.http1ProtocolOptions, d2.http1ProtocolOptions) {
		return false
	}

	if !proto.Equal(d.http2ProtocolOptions, d2.http2ProtocolOptions) {
		return false
	}

	if !proto.Equal(d.tlsConfig, d2.tlsConfig) {
		return false
	}

	if !cmputils.CompareWithNils(d.loadBalancerConfig, d2.loadBalancerConfig, func(a, b *LoadBalancerConfigIR) bool {
		return a.Equals(b)
	}) {
		return false
	}

	if !proto.Equal(d.healthCheck, d2.healthCheck) {
		return false
	}

	if !proto.Equal(d.outlierDetection, d2.outlierDetection) {
		return false
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

func NewPlugin(ctx context.Context, commoncol *collections.CommonCollections, v validator.Validator) sdk.Plugin {
	registerTypes(commoncol.OurClient)
	col := krt.WrapClient(kclient.NewFiltered[*v1alpha1.BackendConfigPolicy](
		commoncol.Client,
		kclient.Filter{ObjectFilter: commoncol.Client.ObjectFilter()},
	), commoncol.KrtOpts.ToOptions("BackendConfigPolicy")...)
	backendConfigPolicyCol := krt.NewCollection(col, func(krtctx krt.HandlerContext, b *v1alpha1.BackendConfigPolicy) *ir.PolicyWrapper {
		policyIR, errs := translate(commoncol, krtctx, b)
		if err := validateXDS(ctx, policyIR, v, commoncol.Settings.ValidationMode); err != nil {
			errs = append(errs, err)
		}

		return &ir.PolicyWrapper{
			ObjectSource: ir.ObjectSource{
				Group:     wellknown.BackendConfigPolicyGVK.Group,
				Kind:      wellknown.BackendConfigPolicyGVK.Kind,
				Namespace: b.Namespace,
				Name:      b.Name,
			},
			Policy:     b,
			PolicyIR:   policyIR,
			TargetRefs: pluginsdkutils.TargetRefsToPolicyRefs(b.Spec.TargetRefs, b.Spec.TargetSelectors),
			Errors:     errs,
		}
	}, commoncol.KrtOpts.ToOptions("BackendConfigPolicyIRs")...)
	return sdk.Plugin{
		ContributesPolicies: map[schema.GroupKind]sdk.PolicyPlugin{
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

func processBackend(_ context.Context, polir ir.PolicyIR, backend ir.BackendObjectIR, out *envoyclusterv3.Cluster) {
	pol := polir.(*BackendConfigPolicyIR)
	if pol.connectTimeout != nil {
		out.ConnectTimeout = pol.connectTimeout
	}

	if pol.perConnectionBufferLimitBytes != nil {
		out.PerConnectionBufferLimitBytes = &wrapperspb.UInt32Value{Value: *pol.perConnectionBufferLimitBytes} //nolint:gosec // G115: kubebuilder validation ensures 0 <= value <= 4294967295, safe for uint32
	}

	if pol.tcpKeepalive != nil {
		out.UpstreamConnectionOptions = &envoyclusterv3.UpstreamConnectionOptions{
			TcpKeepalive: pol.tcpKeepalive,
		}
	}

	applyCommonHttpProtocolOptions(pol.commonHttpProtocolOptions, backend, out)
	applyHttp1ProtocolOptions(pol.http1ProtocolOptions, backend, out)
	applyHttp2ProtocolOptions(pol.http2ProtocolOptions, backend, out)

	if pol.tlsConfig != nil {
		typedConfig, err := utils.MessageToAny(pol.tlsConfig)
		if err != nil {
			logger.Error("failed to convert tls config to any", "error", err)
			return
		}
		out.TransportSocket = &envoycorev3.TransportSocket{
			Name: envoywellknown.TransportSocketTls,
			ConfigType: &envoycorev3.TransportSocket_TypedConfig{
				TypedConfig: typedConfig,
			},
		}
	}

	applyLoadBalancerConfig(pol.loadBalancerConfig, out)

	if pol.healthCheck != nil {
		out.HealthChecks = []*envoycorev3.HealthCheck{pol.healthCheck}
	}

	if pol.outlierDetection != nil {
		out.OutlierDetection = pol.outlierDetection
	}
}

func translate(
	commoncol *collections.CommonCollections,
	krtctx krt.HandlerContext,
	pol *v1alpha1.BackendConfigPolicy,
) (*BackendConfigPolicyIR, []error) {
	var errs []error
	ir := BackendConfigPolicyIR{
		ct: pol.CreationTimestamp.Time,
	}
	if pol.Spec.ConnectTimeout != nil {
		ir.connectTimeout = durationpb.New(pol.Spec.ConnectTimeout.Duration)
	}
	if pol.Spec.PerConnectionBufferLimitBytes != nil {
		bufferSize := uint32(*pol.Spec.PerConnectionBufferLimitBytes) //nolint:gosec // G115: kubebuilder validation ensures 0 <= value <= 4294967295, safe for uint32
		ir.perConnectionBufferLimitBytes = &bufferSize
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
			errs = append(errs, err)
		}
		ir.http1ProtocolOptions = http1ProtocolOptions
	}

	if pol.Spec.Http2ProtocolOptions != nil {
		ir.http2ProtocolOptions = translateHttp2ProtocolOptions(pol.Spec.Http2ProtocolOptions)
	}

	if pol.Spec.TLS != nil {
		tlsConfig, err := translateTLSConfig(NewDefaultSecretGetter(commoncol.Secrets, krtctx), pol.Spec.TLS, pol.Namespace)
		if err != nil {
			errs = append(errs, err)
		}
		ir.tlsConfig = tlsConfig
	}

	if pol.Spec.LoadBalancer != nil {
		loadBalancerConfig, err := translateLoadBalancerConfig(pol.Spec.LoadBalancer, pol.Name, pol.Namespace)
		if err != nil {
			errs = append(errs, err)
		}
		ir.loadBalancerConfig = loadBalancerConfig
	}

	if pol.Spec.HealthCheck != nil {
		ir.healthCheck = translateHealthCheck(pol.Spec.HealthCheck)
	}

	if pol.Spec.OutlierDetection != nil {
		ir.outlierDetection = translateOutlierDetection(pol.Spec.OutlierDetection)
	}

	return &ir, errs
}

func translateTCPKeepalive(tcpKeepalive *v1alpha1.TCPKeepalive) *envoycorev3.TcpKeepalive {
	out := &envoycorev3.TcpKeepalive{}
	if tcpKeepalive.KeepAliveProbes != nil {
		out.KeepaliveProbes = &wrapperspb.UInt32Value{Value: uint32(*tcpKeepalive.KeepAliveProbes)} //nolint:gosec // G115: kubebuilder validation ensures 0 <= value <= 4294967295, safe for uint32
	}
	if tcpKeepalive.KeepAliveTime != nil {
		out.KeepaliveTime = &wrapperspb.UInt32Value{Value: uint32(tcpKeepalive.KeepAliveTime.Duration.Seconds())}
	}
	if tcpKeepalive.KeepAliveInterval != nil {
		out.KeepaliveInterval = &wrapperspb.UInt32Value{Value: uint32(tcpKeepalive.KeepAliveInterval.Duration.Seconds())}
	}
	return out
}
