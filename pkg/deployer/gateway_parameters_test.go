package deployer_test

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
)

// TestGetInMemoryGatewayParameters tests the priority order for determining gateway parameters.
// Priority order:
// 1. Waypoint class name (adds mesh port)
// 2. Default gateway parameters
func TestGetInMemoryGatewayParameters(t *testing.T) {
	imageInfo := &deployer.ImageInfo{
		Registry:   "test-registry",
		Tag:        "test-tag",
		PullPolicy: "IfNotPresent",
	}

	const (
		envoyController = "kgateway.dev/kgateway"
		waypointClass   = "waypoint"
	)

	tests := []struct {
		name                 string
		controllerName       string
		className            string
		expectedServicePorts int // waypoint has an extra port
		description          string
	}{
		{
			name:                 "waypoint class name",
			controllerName:       envoyController,
			className:            waypointClass,
			expectedServicePorts: 1, // waypoint adds a mesh port
			description:          "When class name matches waypoint, it should return waypoint parameters",
		},
		{
			name:                 "waypoint class name - different controller",
			controllerName:       "some.other/controller",
			className:            waypointClass,
			expectedServicePorts: 1, // waypoint adds a mesh port
			description:          "When class name matches waypoint with non-envoy controller, it should return waypoint parameters",
		},
		{
			name:                 "default - envoy controller",
			controllerName:       envoyController,
			className:            "some-other-class",
			expectedServicePorts: 0,
			description:          "When class name doesn't match waypoint, it should return default gateway parameters",
		},
		{
			name:                 "default - unknown controller and class",
			controllerName:       "some.other/controller",
			className:            "some-other-class",
			expectedServicePorts: 0,
			description:          "When neither class name matches waypoint, it should return default gateway parameters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := deployer.InMemoryGatewayParametersConfig{
				ControllerName:             tt.controllerName,
				ClassName:                  tt.className,
				ImageInfo:                  imageInfo,
				WaypointClassName:          waypointClass,
				OmitDefaultSecurityContext: false,
			}

			gwp, err := deployer.GetInMemoryGatewayParameters(cfg)
			if err != nil {
				t.Fatalf("GetInMemoryGatewayParameters returned error: %v", err)
			}

			if gwp == nil {
				t.Fatal("GetInMemoryGatewayParameters returned nil")
			}

			if gwp.Spec.Kube == nil {
				t.Fatal("GatewayParameters.Spec.Kube is nil")
			}

			// Check service ports for waypoint
			var servicePorts int
			if gwp.Spec.Kube.Service != nil && gwp.Spec.Kube.Service.Ports != nil {
				servicePorts = len(gwp.Spec.Kube.Service.Ports)
			}

			if servicePorts != tt.expectedServicePorts {
				t.Errorf("%s: service ports count = %d, want %d", tt.description, servicePorts, tt.expectedServicePorts)
			}
		})
	}
}
