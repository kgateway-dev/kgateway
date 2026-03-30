package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/pkg/features"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

func TestGetDefaultClassInfoAppliesParametersRefs(t *testing.T) {
	t.Parallel()

	const (
		standardClass = "kgateway"
		waypointClass = "kgateway-waypoint"
	)

	ns := gwv1.Namespace("control-plane")
	settings := &apisettings.Settings{
		EnableEnvoy:    true,
		EnableWaypoint: true,
		GatewayClassParametersRefs: apisettings.GatewayClassParametersRefs{
			standardClass: {
				Name:      "standard-gwp",
				Namespace: &ns,
			},
			waypointClass: {
				Name:      "waypoint-gwp",
				Namespace: &ns,
			},
		},
	}

	classInfos := GetDefaultClassInfo(
		settings,
		standardClass,
		waypointClass,
		"ctrl.kgateway.dev",
		nil,
	)

	require.NotNil(t, classInfos[standardClass])
	require.NotNil(t, classInfos[standardClass].ParametersRef)
	require.Equal(t, "standard-gwp", classInfos[standardClass].ParametersRef.Name)
	require.Equal(t, gwv1.Namespace("control-plane"), *classInfos[standardClass].ParametersRef.Namespace)
	require.Equal(t, gwv1.Group(wellknown.GatewayParametersGVK.Group), classInfos[standardClass].ParametersRef.Group)
	require.Equal(t, gwv1.Kind(wellknown.GatewayParametersGVK.Kind), classInfos[standardClass].ParametersRef.Kind)

	require.NotNil(t, classInfos[waypointClass])
	require.NotNil(t, classInfos[waypointClass].ParametersRef)
	require.Equal(t, "waypoint-gwp", classInfos[waypointClass].ParametersRef.Name)
	require.Equal(t, gwv1.Namespace("control-plane"), *classInfos[waypointClass].ParametersRef.Namespace)
	require.Equal(t, gwv1.Group(wellknown.GatewayParametersGVK.Group), classInfos[waypointClass].ParametersRef.Group)
	require.Equal(t, gwv1.Kind(wellknown.GatewayParametersGVK.Kind), classInfos[waypointClass].ParametersRef.Kind)
}

func TestGetDefaultClassInfoGatesHTTPSMisdirectedRequestsSupport(t *testing.T) {
	t.Parallel()

	const standardClass = "kgateway"

	tests := []struct {
		name    string
		enabled bool
		want    bool
	}{
		{
			name:    "disabled by default",
			enabled: false,
			want:    false,
		},
		{
			name:    "advertised when enabled",
			enabled: true,
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			settings := &apisettings.Settings{
				EnableEnvoy:                            true,
				EnableHTTPSListenerMisdirectedRequests: tt.enabled,
			}

			classInfos := GetDefaultClassInfo(
				settings,
				standardClass,
				"",
				"ctrl.kgateway.dev",
				nil,
			)

			require.NotNil(t, classInfos[standardClass])

			supportedNames := make(map[gwv1.FeatureName]struct{}, len(classInfos[standardClass].SupportedFeatures))
			for _, feature := range classInfos[standardClass].SupportedFeatures {
				supportedNames[feature.Name] = struct{}{}
			}

			_, ok := supportedNames[gwv1.FeatureName(features.SupportGatewayHTTPSListenerDetectMisdirectedRequests)]
			require.Equal(t, tt.want, ok)
		})
	}
}
