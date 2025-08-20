package deployer

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func DeepMergeGatewayParameters(dst, src *v1alpha1.GatewayParameters) *v1alpha1.GatewayParameters {
	if src != nil && src.Spec.SelfManaged != nil {
		// The src override specifies a self-managed gateway, set this on the dst
		// and skip merging of kube fields that are irrelevant because of using
		// a self-managed gateway
		dst.Spec.SelfManaged = src.Spec.SelfManaged
		dst.Spec.Kube = nil
		return dst
	}

	// nil src override means just use dst
	if src == nil || src.Spec.Kube == nil {
		return dst
	}

	if dst == nil || dst.Spec.Kube == nil {
		return src
	}

	dstKube := dst.Spec.Kube
	srcKube := src.Spec.Kube

	dstKube.Deployment = deepMergeDeployment(dstKube.GetDeployment(), srcKube.GetDeployment())
	dstKube.EnvoyContainer = deepMergeEnvoyContainer(dstKube.GetEnvoyContainer(), srcKube.GetEnvoyContainer())
	dstKube.SdsContainer = deepMergeSdsContainer(dstKube.GetSdsContainer(), srcKube.GetSdsContainer())
	dstKube.PodTemplate = deepMergePodTemplate(dstKube.GetPodTemplate(), srcKube.GetPodTemplate())
	dstKube.Service = deepMergeService(dstKube.GetService(), srcKube.GetService())
	dstKube.ServiceAccount = deepMergeServiceAccount(dstKube.GetServiceAccount(), srcKube.GetServiceAccount())
	dstKube.Istio = deepMergeIstioIntegration(dstKube.GetIstio(), srcKube.GetIstio())
	dstKube.Stats = deepMergeStatsConfig(dstKube.GetStats(), srcKube.GetStats())
	dstKube.AiExtension = deepMergeAIExtension(dstKube.GetAiExtension(), srcKube.GetAiExtension())
	dstKube.FloatingUserId = MergePointers(dstKube.GetFloatingUserId(), srcKube.GetFloatingUserId())
	dstKube.AgentGateway = deepMergeAgentGateway(dstKube.GetAgentGateway(), srcKube.GetAgentGateway())

	return dst
}

// MergePointers will decide whether to use dst or src without dereferencing or recursing
func MergePointers[T any](dst, src *T) *T {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	// given non-nil src override, use that instead
	return src
}

// DeepMergeMaps will use dst if src is nil, src if dest is nil, or add all entries from src into dst
// if neither are nil
func DeepMergeMaps[keyT comparable, valT any](dst, src map[keyT]valT) map[keyT]valT {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil || len(src) == 0 {
		return src
	}

	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func DeepMergeSlices[T any](dst, src []T) []T {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil || len(src) == 0 {
		return src
	}

	dst = append(dst, src...)

	return dst
}

func OverrideSlices[T any](dst, src []T) []T {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	return src
}

// Check against base value
func MergeComparable[T comparable](dst, src T) T {
	var t T
	if src == t {
		return dst
	}

	return src
}

func deepMergeStatsConfig(dst *v1alpha1.StatsConfig, src *v1alpha1.StatsConfig) *v1alpha1.StatsConfig {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.Enabled = MergePointers(dst.GetEnabled(), src.GetEnabled())
	dst.RoutePrefixRewrite = MergeComparable(dst.GetRoutePrefixRewrite(), src.GetRoutePrefixRewrite())
	dst.EnableStatsRoute = MergeComparable(dst.GetEnableStatsRoute(), src.GetEnableStatsRoute())
	dst.StatsRoutePrefixRewrite = MergeComparable(dst.GetStatsRoutePrefixRewrite(), src.GetStatsRoutePrefixRewrite())

	return dst
}

func deepMergePodTemplate(dst, src *v1alpha1.Pod) *v1alpha1.Pod {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.ExtraLabels = DeepMergeMaps(dst.GetExtraLabels(), src.GetExtraLabels())
	dst.ExtraAnnotations = DeepMergeMaps(dst.GetExtraAnnotations(), src.GetExtraAnnotations())
	dst.SecurityContext = deepMergePodSecurityContext(dst.GetSecurityContext(), src.GetSecurityContext())
	dst.ImagePullSecrets = DeepMergeSlices(dst.GetImagePullSecrets(), src.GetImagePullSecrets())
	dst.NodeSelector = DeepMergeMaps(dst.GetNodeSelector(), src.GetNodeSelector())
	dst.Affinity = DeepMergeAffinity(dst.GetAffinity(), src.GetAffinity())
	dst.Tolerations = DeepMergeSlices(dst.GetTolerations(), src.GetTolerations())
	dst.GracefulShutdown = deepMergeGracefulShutdown(dst.GetGracefulShutdown(), src.GetGracefulShutdown())
	dst.TerminationGracePeriodSeconds = MergePointers(dst.TerminationGracePeriodSeconds, src.TerminationGracePeriodSeconds)
	dst.ReadinessProbe = deepMergeProbe(dst.GetReadinessProbe(), src.GetReadinessProbe())
	dst.LivenessProbe = deepMergeProbe(dst.GetLivenessProbe(), src.GetLivenessProbe())
	dst.TopologySpreadConstraints = DeepMergeSlices(dst.GetTopologySpreadConstraints(), src.GetTopologySpreadConstraints())

	return dst
}

func deepMergePodSecurityContext(dst, src *corev1.PodSecurityContext) *corev1.PodSecurityContext {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.SELinuxOptions = deepMergeSELinuxOptions(dst.SELinuxOptions, src.SELinuxOptions)
	dst.WindowsOptions = deepMergeWindowsSecurityContextOptions(dst.WindowsOptions, src.WindowsOptions)
	dst.RunAsUser = MergePointers(dst.RunAsUser, src.RunAsUser)
	dst.RunAsGroup = MergePointers(dst.RunAsGroup, src.RunAsGroup)
	dst.RunAsNonRoot = MergePointers(dst.RunAsNonRoot, src.RunAsNonRoot)
	dst.SupplementalGroups = DeepMergeSlices(dst.SupplementalGroups, src.SupplementalGroups)
	dst.FSGroup = MergePointers(dst.FSGroup, src.FSGroup)
	dst.Sysctls = DeepMergeSlices(dst.Sysctls, src.Sysctls)
	dst.FSGroupChangePolicy = MergePointers(dst.FSGroupChangePolicy, src.FSGroupChangePolicy)
	dst.SeccompProfile = deepMergeSeccompProfile(dst.SeccompProfile, src.SeccompProfile)

	return dst
}

func deepMergeSELinuxOptions(dst, src *corev1.SELinuxOptions) *corev1.SELinuxOptions {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.User = MergeComparable(dst.User, src.User)
	dst.Role = MergeComparable(dst.Role, src.Role)
	dst.Type = MergeComparable(dst.Type, src.Type)
	dst.Level = MergeComparable(dst.Level, src.Level)

	return dst
}

func deepMergeWindowsSecurityContextOptions(dst, src *corev1.WindowsSecurityContextOptions) *corev1.WindowsSecurityContextOptions {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.GMSACredentialSpecName = MergePointers(dst.GMSACredentialSpec, src.GMSACredentialSpec)
	dst.GMSACredentialSpec = MergePointers(dst.GMSACredentialSpec, src.GMSACredentialSpec)
	dst.RunAsUserName = MergePointers(dst.RunAsUserName, src.RunAsUserName)
	dst.HostProcess = MergePointers(dst.HostProcess, src.HostProcess)

	return dst
}

func deepMergeSeccompProfile(dst, src *corev1.SeccompProfile) *corev1.SeccompProfile {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.Type = MergeComparable(dst.Type, src.Type)
	dst.LocalhostProfile = MergePointers(dst.LocalhostProfile, src.LocalhostProfile)

	return dst
}

func DeepMergeAffinity(dst, src *corev1.Affinity) *corev1.Affinity {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.NodeAffinity = deepMergeNodeAffinity(dst.NodeAffinity, src.NodeAffinity)
	dst.PodAffinity = deepMergePodAffinity(dst.PodAffinity, src.PodAffinity)
	dst.PodAntiAffinity = deepMergePodAntiAffinity(dst.PodAntiAffinity, src.PodAntiAffinity)

	return dst
}

func deepMergeNodeAffinity(dst, src *corev1.NodeAffinity) *corev1.NodeAffinity {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.RequiredDuringSchedulingIgnoredDuringExecution = deepMergeNodeSelector(dst.RequiredDuringSchedulingIgnoredDuringExecution, src.RequiredDuringSchedulingIgnoredDuringExecution)
	dst.PreferredDuringSchedulingIgnoredDuringExecution = DeepMergeSlices(dst.PreferredDuringSchedulingIgnoredDuringExecution, src.PreferredDuringSchedulingIgnoredDuringExecution)

	return dst
}

func deepMergeNodeSelector(dst, src *corev1.NodeSelector) *corev1.NodeSelector {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.NodeSelectorTerms = DeepMergeSlices(dst.NodeSelectorTerms, src.NodeSelectorTerms)

	return dst
}

func deepMergePodAffinity(dst, src *corev1.PodAffinity) *corev1.PodAffinity {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.RequiredDuringSchedulingIgnoredDuringExecution = DeepMergeSlices(dst.RequiredDuringSchedulingIgnoredDuringExecution, src.RequiredDuringSchedulingIgnoredDuringExecution)
	dst.PreferredDuringSchedulingIgnoredDuringExecution = DeepMergeSlices(dst.PreferredDuringSchedulingIgnoredDuringExecution, src.PreferredDuringSchedulingIgnoredDuringExecution)

	return dst
}

func deepMergePodAntiAffinity(dst, src *corev1.PodAntiAffinity) *corev1.PodAntiAffinity {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.RequiredDuringSchedulingIgnoredDuringExecution = DeepMergeSlices(dst.RequiredDuringSchedulingIgnoredDuringExecution, src.RequiredDuringSchedulingIgnoredDuringExecution)
	dst.PreferredDuringSchedulingIgnoredDuringExecution = DeepMergeSlices(dst.PreferredDuringSchedulingIgnoredDuringExecution, src.PreferredDuringSchedulingIgnoredDuringExecution)

	return dst
}

func deepMergeProbe(dst, src *corev1.Probe) *corev1.Probe {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.ProbeHandler = deepMergeProbeHandler(dst.ProbeHandler, src.ProbeHandler)
	dst.InitialDelaySeconds = MergeComparable(dst.InitialDelaySeconds, src.InitialDelaySeconds)
	dst.TimeoutSeconds = MergeComparable(dst.TimeoutSeconds, src.TimeoutSeconds)
	dst.PeriodSeconds = MergeComparable(dst.PeriodSeconds, src.PeriodSeconds)
	dst.SuccessThreshold = MergeComparable(dst.SuccessThreshold, src.SuccessThreshold)
	dst.FailureThreshold = MergeComparable(dst.FailureThreshold, src.FailureThreshold)
	dst.TerminationGracePeriodSeconds = MergePointers(dst.TerminationGracePeriodSeconds, src.TerminationGracePeriodSeconds)

	return dst
}

func deepMergeProbeHandler(dst, src corev1.ProbeHandler) corev1.ProbeHandler {
	dst.Exec = deepMergeExecAction(dst.Exec, src.Exec)
	dst.HTTPGet = deepMergeHTTPGetAction(dst.HTTPGet, src.HTTPGet)
	dst.TCPSocket = deepMergeTCPSocketAction(dst.TCPSocket, src.TCPSocket)
	dst.GRPC = deepMergeGRPCAction(dst.GRPC, src.GRPC)

	return dst
}

func deepMergeExecAction(dst, src *corev1.ExecAction) *corev1.ExecAction {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	// Don't merge the command string as that can break the entire probe
	dst.Command = OverrideSlices(dst.Command, src.Command)

	return dst
}

func deepMergeHTTPGetAction(dst, src *corev1.HTTPGetAction) *corev1.HTTPGetAction {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.Path = MergeComparable(dst.Path, src.Path)
	dst.Port = mergeIntOrString(dst.Port, src.Port)
	dst.Host = MergeComparable(dst.Host, src.Host)
	dst.Scheme = MergeComparable(dst.Scheme, src.Scheme)
	dst.HTTPHeaders = DeepMergeSlices(dst.HTTPHeaders, src.HTTPHeaders)

	return dst
}

func mergeIntOrString(dst, src intstr.IntOrString) intstr.IntOrString {
	// Do not deep merge as this can cause a conflict between the name and number of the port to access on the container
	return MergeComparable(dst, src)
}

func deepMergeTCPSocketAction(dst, src *corev1.TCPSocketAction) *corev1.TCPSocketAction {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.Port = mergeIntOrString(dst.Port, src.Port)
	dst.Host = MergeComparable(dst.Host, src.Host)

	return dst
}

func deepMergeGRPCAction(dst, src *corev1.GRPCAction) *corev1.GRPCAction {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.Port = MergeComparable(dst.Port, src.Port)
	dst.Service = MergePointers(dst.Service, src.Service)

	return dst
}

func deepMergeGracefulShutdown(dst, src *v1alpha1.GracefulShutdownSpec) *v1alpha1.GracefulShutdownSpec {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.Enabled = MergePointers(dst.Enabled, src.Enabled)
	dst.SleepTimeSeconds = MergePointers(dst.SleepTimeSeconds, src.SleepTimeSeconds)

	return dst
}

func deepMergeService(dst, src *v1alpha1.Service) *v1alpha1.Service {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	if src.GetType() != nil {
		dst.Type = src.GetType()
	}

	if src.GetClusterIP() != nil {
		dst.ClusterIP = src.GetClusterIP()
	}

	dst.ExtraLabels = DeepMergeMaps(dst.GetExtraLabels(), src.GetExtraLabels())
	dst.ExtraAnnotations = DeepMergeMaps(dst.GetExtraAnnotations(), src.GetExtraAnnotations())
	dst.Ports = DeepMergeSlices(dst.GetPorts(), src.GetPorts())

	return dst
}

func deepMergeServiceAccount(dst, src *v1alpha1.ServiceAccount) *v1alpha1.ServiceAccount {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.ExtraLabels = DeepMergeMaps(dst.GetExtraLabels(), src.GetExtraLabels())
	dst.ExtraAnnotations = DeepMergeMaps(dst.GetExtraAnnotations(), src.GetExtraAnnotations())

	return dst
}

func deepMergeSdsContainer(dst, src *v1alpha1.SdsContainer) *v1alpha1.SdsContainer {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.Image = deepMergeImage(dst.GetImage(), src.GetImage())
	dst.SecurityContext = deepMergeSecurityContext(dst.GetSecurityContext(), src.GetSecurityContext())
	dst.Resources = DeepMergeResourceRequirements(dst.GetResources(), src.GetResources())
	dst.Bootstrap = deepMergeSdsBootstrap(dst.GetBootstrap(), src.GetBootstrap())

	return dst
}

func deepMergeSdsBootstrap(dst, src *v1alpha1.SdsBootstrap) *v1alpha1.SdsBootstrap {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	if src.GetLogLevel() != nil {
		dst.LogLevel = src.GetLogLevel()
	}

	return dst
}

func deepMergeIstioIntegration(dst, src *v1alpha1.IstioIntegration) *v1alpha1.IstioIntegration {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.IstioProxyContainer = deepMergeIstioContainer(dst.GetIstioProxyContainer(), src.GetIstioProxyContainer())
	dst.CustomSidecars = mergeCustomSidecars(dst.GetCustomSidecars(), src.GetCustomSidecars())

	return dst
}

// mergeCustomSidecars will decide whether to use dst or src custom sidecar containers
func mergeCustomSidecars(dst, src []corev1.Container) []corev1.Container {
	// nil src override means just use dst
	if len(src) == 0 {
		return dst
	}

	// given non-nil src override, use that instead
	return src
}

func deepMergeIstioContainer(dst, src *v1alpha1.IstioContainer) *v1alpha1.IstioContainer {
	// nil src override means just use dst
	if src == nil {
		return dst
	}
	if dst == nil {
		return src
	}

	dst.Image = deepMergeImage(dst.GetImage(), src.GetImage())
	dst.SecurityContext = deepMergeSecurityContext(dst.GetSecurityContext(), src.GetSecurityContext())
	dst.Resources = DeepMergeResourceRequirements(dst.GetResources(), src.GetResources())

	if src.GetLogLevel() != nil {
		dst.LogLevel = src.GetLogLevel()
	}

	if src.GetIstioDiscoveryAddress() != nil {
		dst.IstioDiscoveryAddress = src.GetIstioDiscoveryAddress()
	}

	if src.GetIstioMetaMeshId() != nil {
		dst.IstioMetaMeshId = src.GetIstioMetaMeshId()
	}

	if src.GetIstioMetaClusterId() != nil {
		dst.IstioMetaClusterId = src.GetIstioMetaClusterId()
	}

	return dst
}

func deepMergeEnvoyContainer(dst, src *v1alpha1.EnvoyContainer) *v1alpha1.EnvoyContainer {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.Bootstrap = deepMergeEnvoyBootstrap(dst.GetBootstrap(), src.GetBootstrap())
	dst.Image = deepMergeImage(dst.GetImage(), src.GetImage())
	dst.SecurityContext = deepMergeSecurityContext(dst.GetSecurityContext(), src.GetSecurityContext())
	dst.Resources = DeepMergeResourceRequirements(dst.GetResources(), src.GetResources())
	dst.Env = DeepMergeSlices(dst.GetEnv(), src.GetEnv())

	return dst
}

func deepMergeImage(dst, src *v1alpha1.Image) *v1alpha1.Image {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	if src.GetRegistry() != nil {
		dst.Registry = src.GetRegistry()
	}

	if src.GetRepository() != nil {
		dst.Repository = src.GetRepository()
	}

	if src.GetTag() != nil {
		dst.Tag = src.GetTag()
	}

	if src.GetDigest() != nil {
		dst.Digest = src.GetDigest()
	}

	if src.GetPullPolicy() != nil {
		dst.PullPolicy = src.GetPullPolicy()
	}

	return dst
}

func deepMergeEnvoyBootstrap(dst, src *v1alpha1.EnvoyBootstrap) *v1alpha1.EnvoyBootstrap {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}
	if src.GetLogLevel() != nil {
		dst.LogLevel = src.GetLogLevel()
	}

	dst.ComponentLogLevels = DeepMergeMaps(dst.GetComponentLogLevels(), src.GetComponentLogLevels())

	return dst
}

func DeepMergeResourceRequirements(dst, src *corev1.ResourceRequirements) *corev1.ResourceRequirements {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.Limits = DeepMergeMaps(dst.Limits, src.Limits)
	dst.Requests = DeepMergeMaps(dst.Requests, src.Requests)

	return dst
}

func deepMergeSecurityContext(dst, src *corev1.SecurityContext) *corev1.SecurityContext {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.Capabilities = deepMergeCapabilities(dst.Capabilities, src.Capabilities)
	dst.SELinuxOptions = deepMergeSELinuxOptions(dst.SELinuxOptions, src.SELinuxOptions)
	dst.WindowsOptions = deepMergeWindowsSecurityContextOptions(dst.WindowsOptions, src.WindowsOptions)
	dst.RunAsUser = MergePointers(dst.RunAsUser, src.RunAsUser)
	dst.RunAsGroup = MergePointers(dst.RunAsGroup, src.RunAsGroup)
	dst.RunAsNonRoot = MergePointers(dst.RunAsNonRoot, src.RunAsNonRoot)
	dst.Privileged = MergePointers(dst.Privileged, src.Privileged)
	dst.ReadOnlyRootFilesystem = MergePointers(dst.ReadOnlyRootFilesystem, src.ReadOnlyRootFilesystem)
	dst.AllowPrivilegeEscalation = MergePointers(dst.AllowPrivilegeEscalation, src.AllowPrivilegeEscalation)
	dst.ProcMount = MergePointers(dst.ProcMount, src.ProcMount)
	dst.SeccompProfile = deepMergeSeccompProfile(dst.SeccompProfile, src.SeccompProfile)

	return dst
}

func deepMergeCapabilities(dst, src *corev1.Capabilities) *corev1.Capabilities {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.Add = DeepMergeSlices(dst.Add, src.Add)
	dst.Drop = DeepMergeSlices(dst.Drop, src.Drop)

	return dst
}

func deepMergeDeployment(dst, src *v1alpha1.ProxyDeployment) *v1alpha1.ProxyDeployment {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	// Handle AtMostOneOf constraint for replicas and omitReplicas
	// If src has either field set, it takes precedence and we clear the other field
	if src.GetReplicas() != nil {
		dst.Replicas = src.GetReplicas()
		dst.OmitReplicas = nil // Clear omitReplicas when replicas is set
	} else if src.GetOmitReplicas() != nil {
		dst.OmitReplicas = src.GetOmitReplicas()
		dst.Replicas = nil // Clear replicas when omitReplicas is set
	} else {
		// src has neither field set, keep dst as is
		// (dst.Replicas and dst.OmitReplicas remain unchanged)
	}

	return dst
}

func deepMergeAIExtension(dst, src *v1alpha1.AiExtension) *v1alpha1.AiExtension {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.Enabled = MergePointers(dst.GetEnabled(), src.GetEnabled())
	dst.Image = deepMergeImage(dst.GetImage(), src.GetImage())
	dst.SecurityContext = deepMergeSecurityContext(dst.GetSecurityContext(), src.GetSecurityContext())
	dst.Resources = DeepMergeResourceRequirements(dst.GetResources(), src.GetResources())
	dst.Env = DeepMergeSlices(dst.GetEnv(), src.GetEnv())
	dst.Ports = DeepMergeSlices(dst.GetPorts(), src.GetPorts())
	dst.Stats = deepMergeAIExtensionStats(dst.GetStats(), src.GetStats())
	dst.Tracing = deepMergeAIExtensionTracing(dst.GetTracing(), src.GetTracing())
	return dst
}

func deepMergeAIExtensionTracing(dst, src *v1alpha1.AiExtensionTrace) *v1alpha1.AiExtensionTrace {
	// nil src override means just use dst
	if src == nil {
		return dst
	}
	if dst == nil {
		return src
	}
	dst.EndPoint = MergeComparable(dst.EndPoint, src.EndPoint)
	dst.Sampler = MergePointers(dst.Sampler, src.Sampler)
	dst.Timeout = MergePointers(dst.Timeout, src.Timeout)
	dst.Protocol = MergePointers(dst.Protocol, src.Protocol)
	return dst
}

func deepMergeAIExtensionStats(dst, src *v1alpha1.AiExtensionStats) *v1alpha1.AiExtensionStats {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.CustomLabels = DeepMergeSlices(dst.GetCustomLabels(), src.GetCustomLabels())

	return dst
}

func deepMergeAgentGateway(dst, src *v1alpha1.AgentGateway) *v1alpha1.AgentGateway {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.Enabled = MergePointers(dst.GetEnabled(), src.GetEnabled())
	dst.LogLevel = MergePointers(dst.GetLogLevel(), src.GetLogLevel())
	dst.Image = deepMergeImage(dst.GetImage(), src.GetImage())
	dst.SecurityContext = deepMergeSecurityContext(dst.GetSecurityContext(), src.GetSecurityContext())
	dst.Resources = DeepMergeResourceRequirements(dst.GetResources(), src.GetResources())
	dst.Env = DeepMergeSlices(dst.GetEnv(), src.GetEnv())
	dst.CustomConfigMapName = MergePointers(dst.GetCustomConfigMapName(), src.GetCustomConfigMapName())

	return dst
}
