package proxy_syncer

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
)

func TestXdsRejectionMessage(t *testing.T) {
	msg := xdsRejectionMessage(xds.GatewayXdsRejection{
		Gateway: types.NamespacedName{Namespace: "ns", Name: "gw"},
		Rejections: []xds.TypeRejection{
			{TypeURL: "envoy.config.cluster.v3.Cluster", Message: "bad cluster"},
			{TypeURL: "envoy.config.listener.v3.Listener", Message: "bad listener", Others: 2},
		},
	})
	require.Equal(t, "Envoy rejected the latest configuration and is serving the last accepted version: "+
		"envoy.config.cluster.v3.Cluster: bad cluster; "+
		"envoy.config.listener.v3.Listener: bad listener (and 2 other errors from other proxies)", msg)
}
