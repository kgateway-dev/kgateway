package listenerpolicy

import (
	"testing"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestTranslateTCPKeepalive(t *testing.T) {
	tcpKeepalive := &kgateway.TCPKeepalive{
		KeepAliveProbes:   ptr.To[int32](3),
		KeepAliveTime:     &metav1.Duration{Duration: 20 * time.Minute},
		KeepAliveInterval: &metav1.Duration{Duration: 60 * time.Second},
	}

	got := translateTCPKeepalive(tcpKeepalive)
	want := &envoycorev3.TcpKeepalive{
		KeepaliveProbes:   wrapperspb.UInt32(3),
		KeepaliveTime:     wrapperspb.UInt32(uint32((20 * time.Minute).Seconds())),
		KeepaliveInterval: wrapperspb.UInt32(60),
	}

	require.True(t, proto.Equal(want, got))
}

func TestApplyListenerPluginTCPKeepalive(t *testing.T) {
	pass := &listenerPolicyPluginGwPass{}
	out := &envoylistenerv3.Listener{}
	want := &envoycorev3.TcpKeepalive{
		KeepaliveProbes:   wrapperspb.UInt32(5),
		KeepaliveTime:     wrapperspb.UInt32(120),
		KeepaliveInterval: wrapperspb.UInt32(30),
	}

	pass.ApplyListenerPlugin(&ir.ListenerContext{
		Port: 80,
		Policy: &ListenerPolicyIR{
			defaultPolicy: listenerPolicy{
				tcpKeepalive: want,
			},
		},
	}, out)

	require.True(t, proto.Equal(want, out.TcpKeepalive))
}
