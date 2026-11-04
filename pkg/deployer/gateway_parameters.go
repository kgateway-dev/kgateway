package deployer

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
)

// DefaultIstioProxyImageTag is the default Istio proxyv2 image tag used for auto mTLS cert-agent sidecars.
const DefaultIstioProxyImageTag = "1.31.0"

// Inputs is the set of options used to configure gateway/inference pool deployment.
type Inputs struct {
	Dev                  bool
	IstioAutoMtlsEnabled bool
	ControlPlane         ControlPlaneInfo
	ImageInfo            *ImageInfo
	CommonCollections    *collections.CommonCollections
	GatewayClassName     string
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
	OmitDefaultSecurityContext bool
}

// GetInMemoryGatewayParameters returns an in-memory GatewayParameters for envoy-based gateways.
func GetInMemoryGatewayParameters(cfg InMemoryGatewayParametersConfig) (*kgateway.GatewayParameters, error) {
	return defaultGatewayParameters(cfg.ImageInfo, cfg.OmitDefaultSecurityContext), nil
}

// defaultGatewayParameters returns an in-memory GatewayParameters with the default values
// set for the gateway.
func defaultGatewayParameters(imageInfo *ImageInfo, omitDefaultSecurityContext bool) *kgateway.GatewayParameters {
	gwp := &kgateway.GatewayParameters{
		Spec: kgateway.GatewayParametersSpec{
			SelfManaged: nil,
			Kube: &kgateway.KubernetesProxyConfig{
				Service: &kgateway.Service{
					Type: new(corev1.ServiceTypeLoadBalancer),
				},
				PodTemplate: &kgateway.Pod{
					TerminationGracePeriodSeconds: new(int64(60)),
					GracefulShutdown: &kgateway.GracefulShutdownSpec{
						Enabled:          new(true),
						SleepTimeSeconds: new(int64(10)),
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
						LogLevel: new("info"),
						DnsResolver: &kgateway.DnsResolver{
							UdpMaxQueries: new(int32(100)),
						},
					},
					Image: &kgateway.Image{
						Registry:   new(imageInfo.Registry),
						Tag:        new(imageInfo.Tag),
						Repository: new(wellknown.EnvoyWrapperImage),
						PullPolicy: (*corev1.PullPolicy)(new(imageInfo.PullPolicy)),
					},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: new(false),
						ReadOnlyRootFilesystem:   new(true),
						RunAsNonRoot:             new(true),
						RunAsUser:                new(int64(10101)),
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
					},
				},
				Stats: &kgateway.StatsConfig{
					Enabled:                 new(true),
					RoutePrefixRewrite:      new("/stats/prometheus?usedonly"),
					EnableStatsRoute:        new(true),
					StatsRoutePrefixRewrite: new("/stats"),
				},
				SdsContainer: &kgateway.SdsContainer{
					Image: &kgateway.Image{
						Registry:   new(imageInfo.Registry),
						Tag:        new(imageInfo.Tag),
						Repository: new(wellknown.SdsImage),
						PullPolicy: (*corev1.PullPolicy)(new(imageInfo.PullPolicy)),
					},
					Bootstrap: &kgateway.SdsBootstrap{
						LogLevel: new("info"),
					},
				},
				Istio: &kgateway.IstioIntegration{
					IstioProxyContainer: &kgateway.IstioContainer{
						Image: &kgateway.Image{
							Registry:   new("docker.io/istio"),
							Repository: new("proxyv2"),
							Tag:        new(DefaultIstioProxyImageTag),
							PullPolicy: (*corev1.PullPolicy)(new(imageInfo.PullPolicy)),
						},
						LogLevel:              new("warning"),
						IstioDiscoveryAddress: new("istiod.istio-system.svc:15012"),
						IstioMetaMeshId:       new("cluster.local"),
						IstioMetaClusterId:    new("Kubernetes"),
					},
				},
			},
		},
	}
	if omitDefaultSecurityContext {
		gwp.Spec.Kube.EnvoyContainer.SecurityContext = nil
	}
	return gwp.DeepCopy()
}
