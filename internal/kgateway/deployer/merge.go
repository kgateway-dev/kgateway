package deployer

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

// mergePointers will decide whether to use dst or src without dereferencing or recursing
func mergePointers[T any](dst, src *T) *T {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	// given non-nil src override, use that instead
	return src
}

// deepMergeMaps will use dst if src is nil, src if dest is nil, or add all entries from src into dst
// if neither are nil
func deepMergeMaps[keyT comparable, valT any](dst, src map[keyT]valT) map[keyT]valT {
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

func deepMergeSlices[T any](dst, src []T) []T {
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

func overrideSlices[T any](dst, src []T) []T {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	return src
}

// Check against base value
func mergeComparable[T comparable](dst, src T) T {
	var t T
	if src == t {
		return dst
	}

	return src
}

func deepMergeGatewayParameters(dst, src *v1alpha1.GatewayParameters) *v1alpha1.GatewayParameters {
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
	dstKube.FloatingUserId = mergePointers(dstKube.GetFloatingUserId(), srcKube.GetFloatingUserId())
	dstKube.AgentGateway = deepMergeAgentGateway(dstKube.GetAgentGateway(), srcKube.GetAgentGateway())

	return dst
}

func deepMergeStatsConfig(dst *v1alpha1.StatsConfig, src *v1alpha1.StatsConfig) *v1alpha1.StatsConfig {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.Enabled = mergePointers(dst.GetEnabled(), src.GetEnabled())
	dst.RoutePrefixRewrite = mergeComparable(dst.GetRoutePrefixRewrite(), src.GetRoutePrefixRewrite())
	dst.EnableStatsRoute = mergeComparable(dst.GetEnableStatsRoute(), src.GetEnableStatsRoute())
	dst.StatsRoutePrefixRewrite = mergeComparable(dst.GetStatsRoutePrefixRewrite(), src.GetStatsRoutePrefixRewrite())

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

	dst.ExtraLabels = deepMergeMaps(dst.GetExtraLabels(), src.GetExtraLabels())
	dst.ExtraAnnotations = deepMergeMaps(dst.GetExtraAnnotations(), src.GetExtraAnnotations())
	dst.SecurityContext = deepMergePodSecurityContext(dst.GetSecurityContext(), src.GetSecurityContext())
	dst.ImagePullSecrets = deepMergeSlices(dst.GetImagePullSecrets(), src.GetImagePullSecrets())
	dst.NodeSelector = deepMergeMaps(dst.GetNodeSelector(), src.GetNodeSelector())
	dst.Affinity = deepMergeAffinity(dst.GetAffinity(), src.GetAffinity())
	dst.Tolerations = deepMergeSlices(dst.GetTolerations(), src.GetTolerations())
	dst.GracefulShutdown = deepMergeGracefulShutdown(dst.GetGracefulShutdown(), src.GetGracefulShutdown())
	dst.TerminationGracePeriodSeconds = mergePointers(dst.TerminationGracePeriodSeconds, src.TerminationGracePeriodSeconds)
	dst.ReadinessProbe = deepMergeProbe(dst.GetReadinessProbe(), src.GetReadinessProbe())
	dst.LivenessProbe = deepMergeProbe(dst.GetLivenessProbe(), src.GetLivenessProbe())

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
	dst.RunAsUser = mergePointers(dst.RunAsUser, src.RunAsUser)
	dst.RunAsGroup = mergePointers(dst.RunAsGroup, src.RunAsGroup)
	dst.RunAsNonRoot = mergePointers(dst.RunAsNonRoot, src.RunAsNonRoot)
	dst.SupplementalGroups = deepMergeSlices(dst.SupplementalGroups, src.SupplementalGroups)
	dst.FSGroup = mergePointers(dst.FSGroup, src.FSGroup)
	dst.Sysctls = deepMergeSlices(dst.Sysctls, src.Sysctls)
	dst.FSGroupChangePolicy = mergePointers(dst.FSGroupChangePolicy, src.FSGroupChangePolicy)
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

	dst.User = mergeComparable(dst.User, src.User)
	dst.Role = mergeComparable(dst.Role, src.Role)
	dst.Type = mergeComparable(dst.Type, src.Type)
	dst.Level = mergeComparable(dst.Level, src.Level)

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

	dst.GMSACredentialSpecName = mergePointers(dst.GMSACredentialSpec, src.GMSACredentialSpec)
	dst.GMSACredentialSpec = mergePointers(dst.GMSACredentialSpec, src.GMSACredentialSpec)
	dst.RunAsUserName = mergePointers(dst.RunAsUserName, src.RunAsUserName)
	dst.HostProcess = mergePointers(dst.HostProcess, src.HostProcess)

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

	dst.Type = mergeComparable(dst.Type, src.Type)
	dst.LocalhostProfile = mergePointers(dst.LocalhostProfile, src.LocalhostProfile)

	return dst
}

func deepMergeAffinity(dst, src *corev1.Affinity) *corev1.Affinity {
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
	dst.PreferredDuringSchedulingIgnoredDuringExecution = deepMergeSlices(dst.PreferredDuringSchedulingIgnoredDuringExecution, src.PreferredDuringSchedulingIgnoredDuringExecution)

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

	dst.NodeSelectorTerms = deepMergeSlices(dst.NodeSelectorTerms, src.NodeSelectorTerms)

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

	dst.RequiredDuringSchedulingIgnoredDuringExecution = deepMergeSlices(dst.RequiredDuringSchedulingIgnoredDuringExecution, src.RequiredDuringSchedulingIgnoredDuringExecution)
	dst.PreferredDuringSchedulingIgnoredDuringExecution = deepMergeSlices(dst.PreferredDuringSchedulingIgnoredDuringExecution, src.PreferredDuringSchedulingIgnoredDuringExecution)

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

	dst.RequiredDuringSchedulingIgnoredDuringExecution = deepMergeSlices(dst.RequiredDuringSchedulingIgnoredDuringExecution, src.RequiredDuringSchedulingIgnoredDuringExecution)
	dst.PreferredDuringSchedulingIgnoredDuringExecution = deepMergeSlices(dst.PreferredDuringSchedulingIgnoredDuringExecution, src.PreferredDuringSchedulingIgnoredDuringExecution)

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
	dst.InitialDelaySeconds = mergeComparable(dst.InitialDelaySeconds, src.InitialDelaySeconds)
	dst.TimeoutSeconds = mergeComparable(dst.TimeoutSeconds, src.TimeoutSeconds)
	dst.PeriodSeconds = mergeComparable(dst.PeriodSeconds, src.PeriodSeconds)
	dst.SuccessThreshold = mergeComparable(dst.SuccessThreshold, src.SuccessThreshold)
	dst.FailureThreshold = mergeComparable(dst.FailureThreshold, src.FailureThreshold)
	dst.TerminationGracePeriodSeconds = mergePointers(dst.TerminationGracePeriodSeconds, src.TerminationGracePeriodSeconds)

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
	dst.Command = overrideSlices(dst.Command, src.Command)

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

	dst.Path = mergeComparable(dst.Path, src.Path)
	dst.Port = mergeIntOrString(dst.Port, src.Port)
	dst.Host = mergeComparable(dst.Host, src.Host)
	dst.Scheme = mergeComparable(dst.Scheme, src.Scheme)
	dst.HTTPHeaders = deepMergeSlices(dst.HTTPHeaders, src.HTTPHeaders)

	return dst
}

func mergeIntOrString(dst, src intstr.IntOrString) intstr.IntOrString {
	// Do not deep merge as this can cause a conflict between the name and number of the port to access on the container
	return mergeComparable(dst, src)
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
	dst.Host = mergeComparable(dst.Host, src.Host)

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

	dst.Port = mergeComparable(dst.Port, src.Port)
	dst.Service = mergePointers(dst.Service, src.Service)

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

	dst.Enabled = mergePointers(dst.Enabled, src.Enabled)
	dst.SleepTimeSeconds = mergePointers(dst.SleepTimeSeconds, src.SleepTimeSeconds)

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

	dst.ExtraLabels = deepMergeMaps(dst.GetExtraLabels(), src.GetExtraLabels())
	dst.ExtraAnnotations = deepMergeMaps(dst.GetExtraAnnotations(), src.GetExtraAnnotations())
	dst.Ports = deepMergeSlices(dst.GetPorts(), src.GetPorts())

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

	dst.ExtraLabels = deepMergeMaps(dst.GetExtraLabels(), src.GetExtraLabels())
	dst.ExtraAnnotations = deepMergeMaps(dst.GetExtraAnnotations(), src.GetExtraAnnotations())

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
	dst.Resources = deepMergeResourceRequirements(dst.GetResources(), src.GetResources())
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
	dst.Resources = deepMergeResourceRequirements(dst.GetResources(), src.GetResources())

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
	dst.Resources = deepMergeResourceRequirements(dst.GetResources(), src.GetResources())

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

	dst.ComponentLogLevels = deepMergeMaps(dst.GetComponentLogLevels(), src.GetComponentLogLevels())

	return dst
}

func deepMergeResourceRequirements(dst, src *corev1.ResourceRequirements) *corev1.ResourceRequirements {
	// nil src override means just use dst
	if src == nil {
		return dst
	}

	if dst == nil {
		return src
	}

	dst.Limits = deepMergeMaps(dst.Limits, src.Limits)
	dst.Requests = deepMergeMaps(dst.Requests, src.Requests)

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
	dst.RunAsUser = mergePointers(dst.RunAsUser, src.RunAsUser)
	dst.RunAsGroup = mergePointers(dst.RunAsGroup, src.RunAsGroup)
	dst.RunAsNonRoot = mergePointers(dst.RunAsNonRoot, src.RunAsNonRoot)
	dst.Privileged = mergePointers(dst.Privileged, src.Privileged)
	dst.ReadOnlyRootFilesystem = mergePointers(dst.ReadOnlyRootFilesystem, src.ReadOnlyRootFilesystem)
	dst.AllowPrivilegeEscalation = mergePointers(dst.AllowPrivilegeEscalation, src.AllowPrivilegeEscalation)
	dst.ProcMount = mergePointers(dst.ProcMount, src.ProcMount)
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

	dst.Add = deepMergeSlices(dst.Add, src.Add)
	dst.Drop = deepMergeSlices(dst.Drop, src.Drop)

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

	dst.Replicas = mergePointers(dst.GetReplicas(), src.GetReplicas())

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

	dst.Enabled = mergePointers(dst.GetEnabled(), src.GetEnabled())
	dst.Image = deepMergeImage(dst.GetImage(), src.GetImage())
	dst.SecurityContext = deepMergeSecurityContext(dst.GetSecurityContext(), src.GetSecurityContext())
	dst.Resources = deepMergeResourceRequirements(dst.GetResources(), src.GetResources())
	dst.Env = deepMergeSlices(dst.GetEnv(), src.GetEnv())
	dst.Ports = deepMergeSlices(dst.GetPorts(), src.GetPorts())
	dst.Stats = deepMergeAIExtensionStats(dst.GetStats(), src.GetStats())

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

	dst.CustomLabels = deepMergeSlices(dst.GetCustomLabels(), src.GetCustomLabels())

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

	dst.Enabled = mergePointers(dst.GetEnabled(), src.GetEnabled())
	dst.LogLevel = mergePointers(dst.GetLogLevel(), src.GetLogLevel())

	return dst
}
