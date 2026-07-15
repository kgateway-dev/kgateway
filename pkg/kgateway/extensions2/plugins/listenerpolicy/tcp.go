package listenerpolicy

import (
	"reflect"
	"slices"

	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// TcpListenerPolicyIr holds the partially translated TcpSettings of a ListenerPolicy.
// It is the L4 sibling of HttpListenerPolicyIr.
type TcpListenerPolicyIr struct {
	// For a better UX, we set the default serviceName for access logs to the envoy cluster name
	// (`<gateway-name>.<gateway-namespace>`). Since the gateway name can only be determined during
	// translation, the access log configs and policies are stored so that during translation the
	// default serviceName is set if not already provided and the final config is then marshalled.
	accessLogConfig   []proto.Message
	accessLogPolicies []kgateway.AccessLog
}

func (d *TcpListenerPolicyIr) Equals(in any) bool {
	d2, ok := in.(*TcpListenerPolicyIr)
	if !ok {
		return false
	}

	if !slices.EqualFunc(d.accessLogConfig, d2.accessLogConfig, func(log proto.Message, log2 proto.Message) bool {
		return proto.Equal(log, log2)
	}) {
		return false
	}
	if !slices.EqualFunc(d.accessLogPolicies, d2.accessLogPolicies, func(log kgateway.AccessLog, log2 kgateway.AccessLog) bool {
		return reflect.DeepEqual(log, log2)
	}) {
		return false
	}

	return true
}

func NewTcpListenerPolicy(krtctx krt.HandlerContext, commoncol *collections.CommonCollections, t *kgateway.TCPSettings, objSrc ir.ObjectSource) (*TcpListenerPolicyIr, []error) {
	if t == nil {
		return nil, nil
	}
	var errs []error
	accessLog, err := convertAccessLogConfig(t.AccessLog, commoncol, krtctx, objSrc)
	if err != nil {
		logger.Error("error translating tcp access log", "error", err)
		errs = append(errs, err)
	}

	return &TcpListenerPolicyIr{
		accessLogConfig:   accessLog,
		accessLogPolicies: t.AccessLog,
	}, errs
}
