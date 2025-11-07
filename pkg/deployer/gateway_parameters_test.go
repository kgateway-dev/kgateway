package deployer_test

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
)

// TestGetInMemoryGatewayParameters_ControllerNamePriority tests that the controller name
// takes priority over the class name when determining which gateway parameters to return.
func TestGetInMemoryGatewayParameters_ControllerNamePriority(t *testing.T) {
	imageInfo := &deployer.ImageInfo{
		Registry:   "test-registry",
		Tag:        "test-tag",
		PullPolicy: "IfNotPresent",
	}

	const (
		envoyController = "kgateway.dev/kgateway"
		agwController   = "kgateway.dev/agentgateway"
		waypointClass   = "waypoint"
	)

	tests := []struct {
		name                 string
		controllerName       string
		className            string
		expectedAgwEnabled   bool
		expectedServicePorts int // waypoint has an extra port
		description          string
	}{
		{
			name:                 "agentgateway controller name - ignores class name",
			controllerName:       agwController,
			className:            waypointClass, // should be ignored
			expectedAgwEnabled:   true,
			expectedServicePorts: 0,
			description:          "When controller name matches agentgateway, it should return agentgateway parameters regardless of class name",
		},
		{
			name:                 "waypoint class name takes priority over envoy controller",
			controllerName:       envoyController,
			className:            waypointClass, // waypoint class checked before controller
			expectedAgwEnabled:   false,
			expectedServicePorts: 1, // waypoint adds a mesh port
			description:          "When both envoy controller and waypoint class match, waypoint class takes priority",
		},
		{
			name:                 "waypoint class name - no controller match",
			controllerName:       "some.other/controller",
			className:            waypointClass,
			expectedAgwEnabled:   false,
			expectedServicePorts: 1, // waypoint adds a mesh port
			description:          "When controller name doesn't match known controllers, it should check class name for waypoint",
		},
		{
			name:                 "default - no matches",
			controllerName:       "some.other/controller",
			className:            "some-other-class",
			expectedAgwEnabled:   false,
			expectedServicePorts: 0,
			description:          "When neither controller name nor class name match, it should return default gateway parameters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := deployer.InMemoryGatewayParametersConfig{
				ControllerName:             tt.controllerName,
				ClassName:                  tt.className,
				ImageInfo:                  imageInfo,
				WaypointClassName:          waypointClass,
				AgwControllerName:          agwController,
				OmitDefaultSecurityContext: false,
			}

			gwp := deployer.GetInMemoryGatewayParameters(cfg)

			if gwp == nil {
				t.Fatal("GetInMemoryGatewayParameters returned nil")
			}

			// Check if agentgateway is enabled
			if gwp.Spec.Kube == nil {
				t.Fatal("GatewayParameters.Spec.Kube is nil")
			}

			agwEnabled := gwp.Spec.Kube.Agentgateway != nil &&
				gwp.Spec.Kube.Agentgateway.Enabled != nil &&
				*gwp.Spec.Kube.Agentgateway.Enabled

			if agwEnabled != tt.expectedAgwEnabled {
				t.Errorf("%s: agentgateway enabled = %v, want %v", tt.description, agwEnabled, tt.expectedAgwEnabled)
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
