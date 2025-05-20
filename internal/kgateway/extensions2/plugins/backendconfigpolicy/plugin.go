package backendconfigpolicy

import (
	"context"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
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
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
)

type BackendConfigPolicyIR struct {
	ct                            time.Time
	maxRequestsPerConnection      *int
	connectTimeout                *durationpb.Duration
	perConnectionBufferLimitBytes *int
	TCPKeepalive                  *corev3.TcpKeepalive

	// idleTimeout       *durationpb.Duration
	// maxHeadersCount   *int
	// maxStreamDuration *durationpb.Duration
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
	col := krt.WrapClient(kclient.New[*v1alpha1.BackendConfigPolicy](commoncol.Client), commoncol.KrtOpts.ToOptions("BackendConfigPolicy")...)
	backendConfigPolicyCol := krt.NewCollection(col, func(krtctx krt.HandlerContext, b *v1alpha1.BackendConfigPolicy) *ir.PolicyWrapper {
		objSrc := ir.ObjectSource{
			Group:     wellknown.BackendConfigPolicyGVK.Group,
			Kind:      wellknown.BackendConfigPolicyGVK.Kind,
			Namespace: b.Namespace,
			Name:      b.Name,
		}

		policyIR, errors := translate(b)

		return &ir.PolicyWrapper{
			ObjectSource: objSrc,
			Policy:       b,
			PolicyIR:     policyIR,
			TargetRefs:   convertTargetRefs(b.Spec.TargetRefs),
			Errors:       errors,
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

func processBackend(ctx context.Context, polir ir.PolicyIR, in ir.BackendObjectIR, out *clusterv3.Cluster) {
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
}

func translate(pol *v1alpha1.BackendConfigPolicy) (*BackendConfigPolicyIR, []error) {
	ir := &BackendConfigPolicyIR{}
	errors := []error{}
	if pol.Spec.MaxRequestsPerConnection != nil {
		ir.maxRequestsPerConnection = pol.Spec.MaxRequestsPerConnection
	}
	if pol.Spec.ConnectTimeout != nil {
		timeout, err := time.ParseDuration(string(*pol.Spec.ConnectTimeout))
		if err != nil {
			errors = append(errors, err)
		}
		ir.connectTimeout = durationpb.New(timeout)
	}
	if pol.Spec.PerConnectionBufferLimitBytes != nil {
		ir.perConnectionBufferLimitBytes = pol.Spec.PerConnectionBufferLimitBytes
	}

	tcpKeepalive := pol.Spec.TCPKeepalive
	if tcpKeepalive != nil {
		ir.TCPKeepalive = &corev3.TcpKeepalive{}
		if tcpKeepalive.KeepAliveProbes != nil {
			ir.TCPKeepalive.KeepaliveProbes = &wrapperspb.UInt32Value{Value: uint32(*tcpKeepalive.KeepAliveProbes)}
		}
		if tcpKeepalive.KeepAliveTime != nil {
			keepAliveTime, err := time.ParseDuration(string(*tcpKeepalive.KeepAliveTime))
			if err != nil {
				errors = append(errors, err)
			}
			ir.TCPKeepalive.KeepaliveTime = &wrapperspb.UInt32Value{Value: uint32(keepAliveTime.Seconds())}
		}
		if tcpKeepalive.KeepAliveInterval != nil {
			keepAliveInterval, err := time.ParseDuration(string(*tcpKeepalive.KeepAliveInterval))
			if err != nil {
				errors = append(errors, err)
			}
			ir.TCPKeepalive.KeepaliveInterval = &wrapperspb.UInt32Value{Value: uint32(keepAliveInterval.Seconds())}
		}
	}

	return ir, errors
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
