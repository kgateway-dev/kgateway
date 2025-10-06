package deployer

import (
	"context"
	"log/slog"

	"istio.io/api/annotation"
	"istio.io/api/label"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
)

// Inputs is the set of options used to configure gateway/inference pool deployment.
type Inputs struct {
	Dev                      bool
	IstioAutoMtlsEnabled     bool
	ControlPlane             ControlPlaneInfo
	ImageInfo                *ImageInfo
	CommonCollections        *collections.CommonCollections
	GatewayClassName         string
	WaypointGatewayClassName string
	AgentgatewayClassName    string
}

type ExtraGatewayParameters struct {
	Group     string
	Kind      string
	Object    client.Object
	Generator HelmValuesGenerator
}

// UpdateSecurityContexts updates the security contexts in the gateway parameters.
// It applies the floating user ID if it is set and adds the sysctl to allow the privileged ports if the gateway uses them.
func UpdateSecurityContexts(cfg *v1alpha1.KubernetesProxyConfig, ports []HelmPort) {
	// If the floating user ID is set, unset the RunAsUser field from all security contexts
	if ptr.Deref(cfg.GetFloatingUserId(), false) {
		applyFloatingUserId(cfg)
	}

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
func allowPrivilegedPorts(cfg *v1alpha1.KubernetesProxyConfig) {
	if cfg.PodTemplate == nil {
		cfg.PodTemplate = &v1alpha1.Pod{}
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

// applyFloatingUserId (deprecated in favor of omitDefaultSecurityContext) will
// set the RunAsUser field from all security contexts to null assuming that the
// floatingUserId field is set. Will not create a securityContext, even an
// empty one -- only updates existing securityContexts.
func applyFloatingUserId(dstKube *v1alpha1.KubernetesProxyConfig) {
	logger.Log(context.Background(), slog.LevelWarn, "the field GatewayParameters.Spec.Kube.FloatingUserId is deprecated and will be removed in a future release; see if OmitDefaultSecurityContext fits your needs")

	podSecurityContext := dstKube.GetPodTemplate().GetSecurityContext()
	if podSecurityContext != nil {
		podSecurityContext.RunAsUser = nil
	}

	securityContexts := []*corev1.SecurityContext{
		dstKube.GetEnvoyContainer().GetSecurityContext(),
		dstKube.GetSdsContainer().GetSecurityContext(),
		dstKube.GetIstio().GetIstioProxyContainer().GetSecurityContext(),
		dstKube.GetAiExtension().GetSecurityContext(),
		dstKube.GetAgentgateway().GetSecurityContext(),
	}

	for _, securityContext := range securityContexts {
		if securityContext != nil {
			securityContext.RunAsUser = nil
		}
	}
}

// GetInMemoryGatewayParameters returns an in-memory GatewayParameters based on the name of the gateway class.
func GetInMemoryGatewayParameters(name string, imageInfo *ImageInfo, gatewayClassName, waypointClassName, agentgatewayClassName string, omitDefaultSecurityContext bool) *v1alpha1.GatewayParameters {
	switch name {
	case waypointClassName:
		return defaultWaypointGatewayParameters(imageInfo, omitDefaultSecurityContext)
	case gatewayClassName:
		return defaultGatewayParameters(imageInfo, omitDefaultSecurityContext)
	case agentgatewayClassName:
		return defaultAgentgatewayParameters(imageInfo, omitDefaultSecurityContext)
	default:
		return defaultGatewayParameters(imageInfo, omitDefaultSecurityContext)
	}
}

// defaultAgentgatewayParameters returns an in-memory GatewayParameters with default values
// set for the agentgateway deployment.
func defaultAgentgatewayParameters(imageInfo *ImageInfo, omitDefaultSecurityContext bool) *v1alpha1.GatewayParameters {
	gwp := defaultGatewayParameters(imageInfo, omitDefaultSecurityContext)
	gwp.Spec.Kube.Agentgateway.Enabled = ptr.To(true)
	gwp.Spec.Kube.PodTemplate.ReadinessProbe.HTTPGet.Path = "/healthz/ready"
	gwp.Spec.Kube.PodTemplate.ReadinessProbe.HTTPGet.Port = intstr.FromInt(15021)
	gwp.Spec.Kube.PodTemplate.StartupProbe.HTTPGet.Path = "/healthz/ready"
	gwp.Spec.Kube.PodTemplate.StartupProbe.HTTPGet.Port = intstr.FromInt(15021)
	gwp.Spec.Kube.PodTemplate.GracefulShutdown.Enabled = ptr.To(true)

	// Add pod security context with sysctls for agentgateway (unless omitting default security context)
	if !omitDefaultSecurityContext {
		if gwp.Spec.Kube.PodTemplate.SecurityContext == nil {
			gwp.Spec.Kube.PodTemplate.SecurityContext = &corev1.PodSecurityContext{}
		}
		gwp.Spec.Kube.PodTemplate.SecurityContext.Sysctls = []corev1.Sysctl{
			{
				Name:  "net.ipv4.ip_unprivileged_port_start",
				Value: "0",
			},
		}
	}

	return gwp
}

// defaultWaypointGatewayParameters returns an in-memory GatewayParameters with default values
// set for the waypoint deployment.
func defaultWaypointGatewayParameters(imageInfo *ImageInfo, omitDefaultSecurityContext bool) *v1alpha1.GatewayParameters {
	gwp := defaultGatewayParameters(imageInfo, omitDefaultSecurityContext)

	// Ensure Service is initialized before adding ports
	if gwp.Spec.Kube.Service == nil {
		gwp.Spec.Kube.Service = &v1alpha1.Service{}
	}

	gwp.Spec.Kube.Service.Type = ptr.To(corev1.ServiceTypeClusterIP)

	if gwp.Spec.Kube.Service.Ports == nil {
		gwp.Spec.Kube.Service.Ports = []v1alpha1.Port{}
	}

	// Similar to labeling in kubernetes, this is used to identify the service as a waypoint service.
	meshPort := v1alpha1.Port{
		Port: IstioWaypointPort,
	}
	gwp.Spec.Kube.Service.Ports = append(gwp.Spec.Kube.Service.Ports, meshPort)

	if gwp.Spec.Kube.PodTemplate == nil {
		gwp.Spec.Kube.PodTemplate = &v1alpha1.Pod{}
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
func defaultGatewayParameters(imageInfo *ImageInfo, omitDefaultSecurityContext bool) *v1alpha1.GatewayParameters {
	gwp := &v1alpha1.GatewayParameters{
		Spec: v1alpha1.GatewayParametersSpec{
			SelfManaged: nil,
			Kube: &v1alpha1.KubernetesProxyConfig{
				Deployment: &v1alpha1.ProxyDeployment{
					Replicas:     ptr.To[int32](1),
					OmitReplicas: ptr.To(false),
				},
				Service: &v1alpha1.Service{
					Type: (*corev1.ServiceType)(ptr.To(string(corev1.ServiceTypeLoadBalancer))),
				},
				PodTemplate: &v1alpha1.Pod{
					TerminationGracePeriodSeconds: ptr.To(int64(60)),
					GracefulShutdown: &v1alpha1.GracefulShutdownSpec{
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
						InitialDelaySeconds: 5,
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
				EnvoyContainer: &v1alpha1.EnvoyContainer{
					Bootstrap: &v1alpha1.EnvoyBootstrap{
						LogLevel: ptr.To("info"),
					},
					Image: &v1alpha1.Image{
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
							Add:  []corev1.Capability{"NET_BIND_SERVICE"},
						},
					},
				},
				Stats: &v1alpha1.StatsConfig{
					Enabled:                 ptr.To(true),
					RoutePrefixRewrite:      ptr.To("/stats/prometheus?usedonly"),
					EnableStatsRoute:        ptr.To(true),
					StatsRoutePrefixRewrite: ptr.To("/stats"),
				},
				SdsContainer: &v1alpha1.SdsContainer{
					Image: &v1alpha1.Image{
						Registry:   ptr.To(imageInfo.Registry),
						Tag:        ptr.To(imageInfo.Tag),
						Repository: ptr.To(SdsImage),
						PullPolicy: (*corev1.PullPolicy)(ptr.To(imageInfo.PullPolicy)),
					},
					Bootstrap: &v1alpha1.SdsBootstrap{
						LogLevel: ptr.To("info"),
					},
				},
				Istio: &v1alpha1.IstioIntegration{
					IstioProxyContainer: &v1alpha1.IstioContainer{
						Image: &v1alpha1.Image{
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
				AiExtension: &v1alpha1.AiExtension{
					Enabled: ptr.To(false),
					Image: &v1alpha1.Image{
						Repository: ptr.To(KgatewayAIContainerName),
						Registry:   ptr.To(imageInfo.Registry),
						Tag:        ptr.To(imageInfo.Tag),
						PullPolicy: (*corev1.PullPolicy)(ptr.To(imageInfo.PullPolicy)),
					},
				},
				Agentgateway: &v1alpha1.Agentgateway{
					Enabled:  ptr.To(false),
					LogLevel: ptr.To("info"),
					Image: &v1alpha1.Image{
						Registry:   ptr.To(AgentgatewayRegistry),
						Tag:        ptr.To(AgentgatewayDefaultTag),
						Repository: ptr.To(AgentgatewayImage),
						PullPolicy: (*corev1.PullPolicy)(ptr.To(imageInfo.PullPolicy)),
					},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: ptr.To(false),
						ReadOnlyRootFilesystem:   ptr.To(true),
						RunAsNonRoot:             ptr.To(true),
						RunAsUser:                ptr.To[int64](10101),
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
							Add:  []corev1.Capability{"NET_BIND_SERVICE"},
						},
					},
				},
			},
		},
	}
	if omitDefaultSecurityContext {
		gwp.Spec.Kube.EnvoyContainer.SecurityContext = nil
		gwp.Spec.Kube.Agentgateway.SecurityContext = nil
	}
	return gwp.DeepCopy()
}
