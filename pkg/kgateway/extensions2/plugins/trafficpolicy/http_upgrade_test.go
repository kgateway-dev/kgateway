package trafficpolicy

import (
	"testing"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
)

func TestConstructHTTPUpgrade(t *testing.T) {
	terminate := true
	out := &trafficPolicySpecIr{}

	err := constructHTTPUpgrade(kgateway.TrafficPolicySpec{
		HTTPUpgrade: []kgateway.ProtocolUpgradeConfig{
			{Type: "websocket"},
			{Type: "CONNECT", Connect: &kgateway.ConnectConfig{Terminate: &terminate}},
		},
	}, out)

	require.NoError(t, err)
	require.NotNil(t, out.httpUpgrade)
	require.Len(t, out.httpUpgrade.configs, 2)
	assert.Equal(t, "websocket", out.httpUpgrade.configs[0].GetUpgradeType())
	assert.Nil(t, out.httpUpgrade.configs[0].GetConnectConfig())
	assert.Equal(t, "CONNECT", out.httpUpgrade.configs[1].GetUpgradeType())
	assert.NotNil(t, out.httpUpgrade.configs[1].GetConnectConfig())
}

func TestConstructHTTPUpgradeProxiesConnectByDefault(t *testing.T) {
	out := &trafficPolicySpecIr{}

	err := constructHTTPUpgrade(kgateway.TrafficPolicySpec{
		HTTPUpgrade: []kgateway.ProtocolUpgradeConfig{{
			Type:    "CONNECT",
			Connect: &kgateway.ConnectConfig{},
		}},
	}, out)

	require.NoError(t, err)
	require.Len(t, out.httpUpgrade.configs, 1)
	assert.Equal(t, "CONNECT", out.httpUpgrade.configs[0].GetUpgradeType())
	assert.Nil(t, out.httpUpgrade.configs[0].GetConnectConfig())
}

func TestConstructHTTPUpgradeRejectsCaseInsensitiveDuplicates(t *testing.T) {
	out := &trafficPolicySpecIr{}
	err := constructHTTPUpgrade(kgateway.TrafficPolicySpec{
		HTTPUpgrade: []kgateway.ProtocolUpgradeConfig{
			{Type: "CONNECT"},
			{Type: "connect"},
		},
	}, out)

	require.EqualError(t, err, `duplicate HTTP upgrade type "connect"`)
	assert.Nil(t, out.httpUpgrade)
}

func TestConstructHTTPUpgradeRejectsBuffer(t *testing.T) {
	out := &trafficPolicySpecIr{}
	err := constructHTTPUpgrade(kgateway.TrafficPolicySpec{
		Buffer:      &kgateway.Buffer{MaxRequestSize: resource.NewQuantity(1024, resource.BinarySI)},
		HTTPUpgrade: []kgateway.ProtocolUpgradeConfig{{Type: "CONNECT"}},
	}, out)

	require.EqualError(t, err, "buffer cannot be used together with httpUpgrade")
	assert.Nil(t, out.httpUpgrade)
}

func TestConstructHTTPUpgradeAllowsDisabledBuffer(t *testing.T) {
	out := &trafficPolicySpecIr{}
	err := constructHTTPUpgrade(kgateway.TrafficPolicySpec{
		Buffer:      &kgateway.Buffer{Disable: &shared.PolicyDisable{}},
		HTTPUpgrade: []kgateway.ProtocolUpgradeConfig{{Type: "CONNECT"}},
	}, out)

	require.NoError(t, err)
	assert.NotNil(t, out.httpUpgrade)
}

func TestApplyHTTPUpgrade(t *testing.T) {
	action := &envoyroutev3.RouteAction{
		UpgradeConfigs: []*envoyroutev3.RouteAction_UpgradeConfig{
			{UpgradeType: "WebSocket"},
		},
	}
	terminate := &envoyroutev3.RouteAction_UpgradeConfig_ConnectConfig{}
	upgrade := &httpUpgradeIR{configs: []*envoyroutev3.RouteAction_UpgradeConfig{
		{UpgradeType: "websocket"},
		{UpgradeType: "CONNECT", ConnectConfig: terminate},
	}}

	applyHTTPUpgrade(upgrade, action)

	require.Len(t, action.GetUpgradeConfigs(), 2)
	assert.Equal(t, "websocket", action.GetUpgradeConfigs()[0].GetUpgradeType())
	assert.Equal(t, "CONNECT", action.GetUpgradeConfigs()[1].GetUpgradeType())
	assert.NotNil(t, action.GetUpgradeConfigs()[1].GetConnectConfig())
	assert.NotSame(t, terminate, action.GetUpgradeConfigs()[1].GetConnectConfig(), "IR proto must be cloned before mutation")
}

func TestHTTPUpgradeIREquals(t *testing.T) {
	base := &httpUpgradeIR{configs: []*envoyroutev3.RouteAction_UpgradeConfig{{UpgradeType: "CONNECT"}}}
	equal := &httpUpgradeIR{configs: []*envoyroutev3.RouteAction_UpgradeConfig{{UpgradeType: "CONNECT"}}}
	differentType := &httpUpgradeIR{configs: []*envoyroutev3.RouteAction_UpgradeConfig{{UpgradeType: "websocket"}}}
	differentConnectConfig := &httpUpgradeIR{configs: []*envoyroutev3.RouteAction_UpgradeConfig{{
		UpgradeType:   "CONNECT",
		ConnectConfig: &envoyroutev3.RouteAction_UpgradeConfig_ConnectConfig{},
	}}}

	assert.True(t, base.Equals(equal))
	assert.False(t, base.Equals(differentType))
	assert.False(t, base.Equals(differentConnectConfig))
	assert.False(t, base.Equals((*httpUpgradeIR)(nil)))
	assert.True(t, (*httpUpgradeIR)(nil).Equals((*httpUpgradeIR)(nil)))
}

func TestTrafficPolicyEqualsIncludesHTTPUpgrade(t *testing.T) {
	base := &TrafficPolicy{spec: trafficPolicySpecIr{
		httpUpgrade: &httpUpgradeIR{configs: []*envoyroutev3.RouteAction_UpgradeConfig{{UpgradeType: "CONNECT"}}},
	}}
	equal := &TrafficPolicy{spec: trafficPolicySpecIr{
		httpUpgrade: &httpUpgradeIR{configs: []*envoyroutev3.RouteAction_UpgradeConfig{{UpgradeType: "CONNECT"}}},
	}}
	different := &TrafficPolicy{spec: trafficPolicySpecIr{
		httpUpgrade: &httpUpgradeIR{configs: []*envoyroutev3.RouteAction_UpgradeConfig{{UpgradeType: "websocket"}}},
	}}

	assert.True(t, base.Equals(equal))
	assert.False(t, base.Equals(different))
}
