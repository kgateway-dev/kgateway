package deployer

import (
	"fmt"

	"istio.io/api/annotation"
	"istio.io/api/label"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
)

// Inputs is the set of options used to configure gateway/inference pool deployment.
type Inputs struct {
	Dev                        bool
	IstioAutoMtlsEnabled       bool
	ControlPlane               ControlPlaneInfo
	ImageInfo                  *ImageInfo
	CommonCollections          *collections.CommonCollections
	GatewayClassName           string
	WaypointGatewayClassName   string
	AgentgatewayClassName      string
	AgentgatewayControllerName string
}

// UpdateSecurityContexts updates the security contexts in the gateway parameters.
// It adds the sysctl to allow the privileged ports if the gateway uses them.
func UpdateSecurityContexts(cfg *kgateway.KubernetesProxyConfig, ports []HelmPort) {
	if ptr.Deref(cfg.GetOmitDefaultSecurityContext(), false) {
		return
	}
	if usesPrivilegedPorts(ports) {
		allowPrivilegedPorts(cfg)
	}
}

// usesPrivilegedPorts checks the helm ports to see if any of them are less than 1024
func usesPrivilegedPorts(ports []HelmPort) bool {
	for _, p := range ports {
		if int32(*p.Port) < 1024 {
			return true
		}
	}
	return false
}

// allowPrivilegedPorts allows the use of privileged ports by appending the "net.ipv4.ip_unprivileged_port_start" sysctl with a value of 0
// to the PodTemplate.SecurityContext.Sysctls, or updating the value if it already exists.
func allowPrivilegedPorts(cfg *kgateway.KubernetesProxyConfig) {
	if cfg.PodTemplate == nil {
		cfg.PodTemplate = &kgateway.Pod{}
	}

	if cfg.PodTemplate.SecurityContext == nil {
		cfg.PodTemplate.SecurityContext = &corev1.PodSecurityContext{}
	}

	// If the sysctl already exists, update the value
	for i, sysctl := range cfg.PodTemplate.SecurityContext.Sysctls {
		if sysctl.Name == "net.ipv4.ip_unprivileged_port_start" {
			sysctl.Value = "0"
			cfg.PodTemplate.SecurityContext.Sysctls[i] = sysctl
			return
		}
	}

	// If the sysctl does not exist, append it
	cfg.PodTemplate.SecurityContext.Sysctls = append(cfg.PodTemplate.SecurityContext.Sysctls, corev1.Sysctl{
		Name:  "net.ipv4.ip_unprivileged_port_start",
		Value: "0",
	})
}

// InMemoryGatewayParametersConfig holds the configuration for creating in-memory GatewayParameters.
type InMemoryGatewayParametersConfig struct {
	ControllerName             string
	ClassName                  string
	ImageInfo                  *ImageInfo
	WaypointClassName          string
	AgwControllerName          string
	OmitDefaultSecurityContext bool
}

// GetInMemoryGatewayParameters returns an in-memory GatewayParameters for envoy-based gateways.
//
// This function must NOT be called for agentgateway controllers - agentgateway uses
// agwHelmValuesGenerator which has its own defaults. Calling this with the agentgateway
// controllerName indicates a bug in the routing logic.
//
// Priority order:
// 1. Waypoint class name (must check before envoy controller since waypoint uses the same controller)
// 2. Default gateway parameters (for envoy controller or any other controller)
//
// This allows users to define their own GatewayClass that acts very much like a
// built-in class but is not an exact name match.
func GetInMemoryGatewayParameters(cfg InMemoryGatewayParametersConfig) (*kgateway.GatewayParameters, error) {
	if cfg.ControllerName == cfg.AgwControllerName {
		return nil, fmt.Errorf("GetInMemoryGatewayParameters must not be called for agentgateway controller %q; "+
			"agentgateway gateways should use agwHelmValuesGenerator", cfg.ControllerName)
	}
	if cfg.ClassName == cfg.WaypointClassName {
		return defaultWaypointGatewayParameters(cfg.ImageInfo, cfg.OmitDefaultSecurityContext), nil
	}
	return defaultGatewayParameters(cfg.ImageInfo, cfg.OmitDefaultSecurityContext), nil
}

// defaultWaypointGatewayParameters returns an in-memory GatewayParameters with default values
// set for the waypoint deployment.
func defaultWaypointGatewayParameters(imageInfo *ImageInfo, omitDefaultSecurityContext bool) *kgateway.GatewayParameters {
	gwp := defaultGatewayParameters(imageInfo, omitDefaultSecurityContext)

	// Ensure Service is initialized before adding ports
	if gwp.Spec.Kube.Service == nil {
		gwp.Spec.Kube.Service = &kgateway.Service{}
	}

	gwp.Spec.Kube.Service.Type = ptr.To(corev1.ServiceTypeClusterIP)

	if gwp.Spec.Kube.Service.Ports == nil {
		gwp.Spec.Kube.Service.Ports = []kgateway.Port{}
	}

	// Similar to labeling in kubernetes, this is used to identify the service as a waypoint service.
	meshPort := kgateway.Port{
		Port: IstioWaypointPort,
	}
	gwp.Spec.Kube.Service.Ports = append(gwp.Spec.Kube.Service.Ports, meshPort)

	if gwp.Spec.Kube.PodTemplate == nil {
		gwp.Spec.Kube.PodTemplate = &kgateway.Pod{}
	}
	if gwp.Spec.Kube.PodTemplate.ExtraLabels == nil {
		gwp.Spec.Kube.PodTemplate.ExtraLabels = make(map[string]string)
	}
	gwp.Spec.Kube.PodTemplate.ExtraLabels[label.IoIstioDataplaneMode.Name] = "ambient"

	// do not have zTunnel resolve DNS for us - this can cause traffic loops when we're doing
	// outbound based on DNS service entries
	// TODO do we want this on the north-south gateway class as well?
	if gwp.Spec.Kube.PodTemplate.ExtraAnnotations == nil {
		gwp.Spec.Kube.PodTemplate.ExtraAnnotations = make(map[string]string)
	}
	gwp.Spec.Kube.PodTemplate.ExtraAnnotations[annotation.AmbientDnsCapture.Name] = "false"
	return gwp
}

// defaultGatewayParameters returns an in-memory GatewayParameters with the default values
// set for the gateway.
func defaultGatewayParameters(imageInfo *ImageInfo, omitDefaultSecurityContext bool) *kgateway.GatewayParameters {
	gwp := &kgateway.GatewayParameters{
		Spec: kgateway.GatewayParametersSpec{
			SelfManaged: nil,
			Kube: &kgateway.KubernetesProxyConfig{
				Service: &kgateway.Service{
					Type: (*corev1.ServiceType)(ptr.To(string(corev1.ServiceTypeLoadBalancer))),
				},
				PodTemplate: &kgateway.Pod{
					TerminationGracePeriodSeconds: ptr.To(int64(60)),
					GracefulShutdown: &kgateway.GracefulShutdownSpec{
						Enabled:          ptr.To(true),
						SleepTimeSeconds: ptr.To(int64(10)),
					},
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/ready",
								Port: intstr.FromInt(8082),
							},
						},
						InitialDelaySeconds: 0,
						PeriodSeconds:       10,
					},
					StartupProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/ready",
								Port: intstr.FromInt(8082),
							},
						},
						InitialDelaySeconds: 0,
						PeriodSeconds:       1,
						TimeoutSeconds:      2,
						FailureThreshold:    60,
						SuccessThreshold:    1,
					},
				},
				EnvoyContainer: &kgateway.EnvoyContainer{
					Bootstrap: &kgateway.EnvoyBootstrap{
						LogLevel: ptr.To("info"),
						DnsResolver: &kgateway.DnsResolver{
							UdpMaxQueries: ptr.To(int32(100)),
						},
					},
					Image: &kgateway.Image{
						Registry:   ptr.To(imageInfo.Registry),
						Tag:        ptr.To(imageInfo.Tag),
						Repository: ptr.To(EnvoyWrapperImage),
						PullPolicy: (*corev1.PullPolicy)(ptr.To(imageInfo.PullPolicy)),
					},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: ptr.To(false),
						ReadOnlyRootFilesystem:   ptr.To(true),
						RunAsNonRoot:             ptr.To(true),
						RunAsUser:                ptr.To[int64](10101),
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
					},
				},
				Stats: &kgateway.StatsConfig{
					Enabled:                 ptr.To(true),
					RoutePrefixRewrite:      ptr.To("/stats/prometheus?usedonly"),
					EnableStatsRoute:        ptr.To(true),
					StatsRoutePrefixRewrite: ptr.To("/stats"),
				},
				SdsContainer: &kgateway.SdsContainer{
					Image: &kgateway.Image{
						Registry:   ptr.To(imageInfo.Registry),
						Tag:        ptr.To(imageInfo.Tag),
						Repository: ptr.To(SdsImage),
						PullPolicy: (*corev1.PullPolicy)(ptr.To(imageInfo.PullPolicy)),
					},
					Bootstrap: &kgateway.SdsBootstrap{
						LogLevel: ptr.To("info"),
					},
				},
				Istio: &kgateway.IstioIntegration{
					IstioProxyContainer: &kgateway.IstioContainer{
						Image: &kgateway.Image{
							Registry:   ptr.To("docker.io/istio"),
							Repository: ptr.To("proxyv2"),
							Tag:        ptr.To("1.22.0"),
							PullPolicy: (*corev1.PullPolicy)(ptr.To(imageInfo.PullPolicy)),
						},
						LogLevel:              ptr.To("warning"),
						IstioDiscoveryAddress: ptr.To("istiod.istio-system.svc:15012"),
						IstioMetaMeshId:       ptr.To("cluster.local"),
						IstioMetaClusterId:    ptr.To("Kubernetes"),
					},
				},
				// Note: Agentgateway config is only added for agentgateway controller gateways
				// via defaultAgentgatewayParameters(). For envoy gateways, we leave this nil.
			},
		},
	}
	if omitDefaultSecurityContext {
		gwp.Spec.Kube.EnvoyContainer.SecurityContext = nil
	}
	return gwp.DeepCopy()
}
