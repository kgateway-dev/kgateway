package listenerpolicy

import (
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

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
