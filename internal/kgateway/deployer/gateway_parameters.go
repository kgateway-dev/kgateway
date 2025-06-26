package deployer

import (
	"context"
	"slices"

	"github.com/rotisserie/eris"
	"istio.io/api/annotation"
	"istio.io/api/label"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	api "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

type HelmValuesGenerator interface {
	GetValues(ctx context.Context, gw *api.Gateway, inputs *Inputs) (map[string]any, error)
}

type ExtraGatewayParameters struct {
	Group     string
	Kind      string
	Object    client.Object
	Generator HelmValuesGenerator
}

func NewGatewayParameters(cli client.Client) *GatewayParameters {
	return &GatewayParameters{
		cli:               cli,
		knownGWParameters: []client.Object{&v1alpha1.GatewayParameters{}}, // always include default GatewayParameters
		extraHVGenerators: make(map[schema.GroupKind]HelmValuesGenerator),
	}
}

type GatewayParameters struct {
	cli               client.Client
	extraHVGenerators map[schema.GroupKind]HelmValuesGenerator
	knownGWParameters []client.Object
}

type kGatewayParameters struct {
	cli    client.Client
	inputs *Inputs
}

func (gp *GatewayParameters) WithExtraGatewayParameters(params ...ExtraGatewayParameters) *GatewayParameters {
	for _, p := range params {
		gp.knownGWParameters = append(gp.knownGWParameters, p.Object)
		group := p.Group
		kind := p.Kind
		gp.extraHVGenerators[schema.GroupKind{Group: group, Kind: kind}] = p.Generator
	}
	return gp
}

func (gp *GatewayParameters) AllKnownGatewayParameters() []client.Object {
	return slices.Clone(gp.knownGWParameters)
}

func (gp *GatewayParameters) GetValues(ctx context.Context, gw *api.Gateway, inputs *Inputs) (map[string]any, error) {
	logger := log.FromContext(ctx)

	ref, err := gp.getGatewayParametersRef(ctx, gw)
	if err != nil {
		return nil, err
	}

	if g, ok := gp.extraHVGenerators[ref]; ok {
		return g.GetValues(ctx, gw, inputs)
	}
	logger.V(1).Info("using default GatewayParameters for Gateway",
		"gatewayName", gw.GetName(),
		"gatewayNamespace", gw.GetNamespace(),
	)

	return newKGatewayParameters(gp.cli, inputs).GetValues(ctx, gw)
}

func (gp *GatewayParameters) getGatewayParametersRef(ctx context.Context, gw *api.Gateway) (schema.GroupKind, error) {
	logger := log.FromContext(ctx)

	// attempt to get the GatewayParameters name from the Gateway. If we can't find it,
	// we'll check for the default GWP for the GatewayClass.
	if gw.Spec.Infrastructure == nil || gw.Spec.Infrastructure.ParametersRef == nil {
		logger.V(1).Info("no GatewayParameters found for Gateway, using default",
			"gatewayName", gw.GetName(),
			"gatewayNamespace", gw.GetNamespace(),
		)
		return gp.getDefaultGatewayParametersRef(ctx, gw)
	}

	return schema.GroupKind{
			Group: string(gw.Spec.Infrastructure.ParametersRef.Group),
			Kind:  string(gw.Spec.Infrastructure.ParametersRef.Kind)},
		nil
}

func (gp *GatewayParameters) getDefaultGatewayParametersRef(ctx context.Context, gw *api.Gateway) (schema.GroupKind, error) {
	gwc, err := getGatewayClassFromGateway(ctx, gp.cli, gw)
	if err != nil {
		return schema.GroupKind{}, err
	}

	if gwc.Spec.ParametersRef != nil {
		return schema.GroupKind{
				Group: string(gwc.Spec.ParametersRef.Group),
				Kind:  string(gwc.Spec.ParametersRef.Kind)},
			nil
	}

	return schema.GroupKind{}, nil
}

func newKGatewayParameters(cli client.Client, inputs *Inputs) *kGatewayParameters {
	return &kGatewayParameters{cli: cli, inputs: inputs}
}

func (h *kGatewayParameters) GetValues(ctx context.Context, gw *api.Gateway) (map[string]any, error) {
	gwParam, err := h.getGatewayParametersForGateway(ctx, gw)
	if err != nil {
		return nil, err
	}
	// If this is a self-managed Gateway, skip gateway auto provisioning
	if gwParam != nil && gwParam.Spec.SelfManaged != nil {
		return nil, nil
	}
	vals, err := h.getValues(gw, gwParam)
	if err != nil {
		return nil, err
	}

	var jsonVals map[string]any
	err = jsonConvert(vals, &jsonVals)
	return jsonVals, err
}

// getGatewayParametersForGateway returns the merged GatewayParameters object resulting from the default GwParams object and
// the GwParam object specifically associated with the given Gateway (if one exists).
func (k *kGatewayParameters) getGatewayParametersForGateway(ctx context.Context, gw *api.Gateway) (*v1alpha1.GatewayParameters, error) {
	logger := log.FromContext(ctx)

	// attempt to get the GatewayParameters name from the Gateway. If we can't find it,
	// we'll check for the default GWP for the GatewayClass.
	if gw.Spec.Infrastructure == nil || gw.Spec.Infrastructure.ParametersRef == nil {
		logger.V(1).Info("no GatewayParameters found for Gateway, using default",
			"gatewayName", gw.GetName(),
			"gatewayNamespace", gw.GetNamespace(),
		)
		return k.getDefaultGatewayParameters(ctx, gw)
	}

	gwpName := gw.Spec.Infrastructure.ParametersRef.Name
	if group := gw.Spec.Infrastructure.ParametersRef.Group; group != v1alpha1.GroupName {
		return nil, eris.Errorf("invalid group %s for GatewayParameters", group)
	}
	if kind := gw.Spec.Infrastructure.ParametersRef.Kind; kind != api.Kind(wellknown.GatewayParametersGVK.Kind) {
		return nil, eris.Errorf("invalid kind %s for GatewayParameters", kind)
	}

	// the GatewayParameters must live in the same namespace as the Gateway
	gwpNamespace := gw.GetNamespace()
	gwp := &v1alpha1.GatewayParameters{}
	err := k.cli.Get(ctx, client.ObjectKey{Namespace: gwpNamespace, Name: gwpName}, gwp)
	if err != nil {
		return nil, getGatewayParametersError(err, gwpNamespace, gwpName, gw.GetNamespace(), gw.GetName(), "Gateway")
	}

	defaultGwp, err := k.getDefaultGatewayParameters(ctx, gw)
	if err != nil {
		return nil, err
	}

	mergedGwp := defaultGwp
	deepMergeGatewayParameters(mergedGwp, gwp)
	return mergedGwp, nil
}

// gets the default GatewayParameters associated with the GatewayClass of the provided Gateway
func (k *kGatewayParameters) getDefaultGatewayParameters(ctx context.Context, gw *api.Gateway) (*v1alpha1.GatewayParameters, error) {
	gwc, err := getGatewayClassFromGateway(ctx, k.cli, gw)
	if err != nil {
		return nil, err
	}
	return k.getGatewayParametersForGatewayClass(ctx, gwc)
}

// Gets the GatewayParameters object associated with a given GatewayClass.
func (k *kGatewayParameters) getGatewayParametersForGatewayClass(ctx context.Context, gwc *api.GatewayClass) (*v1alpha1.GatewayParameters, error) {
	logger := log.FromContext(ctx)

	defaultGwp := getInMemoryGatewayParameters(gwc.GetName(), k.inputs.ImageInfo)
	paramRef := gwc.Spec.ParametersRef
	if paramRef == nil {
		// when there is no parametersRef, just return the defaults
		return defaultGwp, nil
	}

	gwpName := paramRef.Name
	if gwpName == "" {
		err := eris.New("parametersRef.name cannot be empty when parametersRef is specified")
		logger.Error(err,
			"gatewayClassName", gwc.GetName(),
			"gatewayClassNamespace", gwc.GetNamespace(),
		)
		return nil, err
	}

	gwpNamespace := ""
	if paramRef.Namespace != nil {
		gwpNamespace = string(*paramRef.Namespace)
	}

	gwp := &v1alpha1.GatewayParameters{}
	err := k.cli.Get(ctx, client.ObjectKey{Namespace: gwpNamespace, Name: gwpName}, gwp)
	if err != nil {
		return nil, getGatewayParametersError(
			err,
			gwpNamespace, gwpName,
			gwc.GetNamespace(), gwc.GetName(),
			"GatewayClass",
		)
	}

	// merge the explicit GatewayParameters with the defaults. this is
	// primarily done to ensure that the image registry and tag are
	// correctly set when they aren't overridden by the GatewayParameters.
	mergedGwp := defaultGwp
	deepMergeGatewayParameters(mergedGwp, gwp)
	return mergedGwp, nil
}

func (k *kGatewayParameters) getValues(gw *api.Gateway, gwParam *v1alpha1.GatewayParameters) (*helmConfig, error) {
	gwKey := ir.ObjectSource{
		Group:     wellknown.GatewayGVK.GroupKind().Group,
		Kind:      wellknown.GatewayGVK.GroupKind().Kind,
		Name:      gw.GetName(),
		Namespace: gw.GetNamespace(),
	}
	irGW := k.inputs.CommonCollections.GatewayIndex.Gateways.GetKey(gwKey.ResourceName())
	if irGW == nil {
		irGW = gatewayFrom(gw)
	}

	// construct the default values
	vals := &helmConfig{
		Gateway: &helmGateway{
			Name:             &gw.Name,
			GatewayName:      &gw.Name,
			GatewayNamespace: &gw.Namespace,
			Ports:            getPortsValues(irGW, gwParam),
			Xds: &helmXds{
				// The xds host/port MUST map to the Service definition for the Control Plane
				// This is the socket address that the Proxy will connect to on startup, to receive xds updates
				Host: &k.inputs.ControlPlane.XdsHost,
				Port: &k.inputs.ControlPlane.XdsPort,
			},
		},
	}

	// if there is no GatewayParameters, return the values as is
	if gwParam == nil {
		return vals, nil
	}

	// extract all the custom values from the GatewayParameters
	// (note: if we add new fields to GatewayParameters, they will
	// need to be plumbed through here as well)

	// Apply the floating user ID if it is set
	if gwParam.Spec.Kube.GetFloatingUserId() != nil && *gwParam.Spec.Kube.GetFloatingUserId() {
		applyFloatingUserId(gwParam.Spec.Kube)
	}

	kubeProxyConfig := gwParam.Spec.Kube
	deployConfig := kubeProxyConfig.GetDeployment()
	podConfig := kubeProxyConfig.GetPodTemplate()
	envoyContainerConfig := kubeProxyConfig.GetEnvoyContainer()
	svcConfig := kubeProxyConfig.GetService()
	svcAccountConfig := kubeProxyConfig.GetServiceAccount()
	istioConfig := kubeProxyConfig.GetIstio()

	sdsContainerConfig := kubeProxyConfig.GetSdsContainer()
	statsConfig := kubeProxyConfig.GetStats()
	istioContainerConfig := istioConfig.GetIstioProxyContainer()
	aiExtensionConfig := kubeProxyConfig.GetAiExtension()
	agentGatewayConfig := kubeProxyConfig.GetAgentGateway()

	gateway := vals.Gateway
	// deployment values
	gateway.ReplicaCount = deployConfig.GetReplicas()

	// service values
	gateway.Service = getServiceValues(svcConfig)
	// serviceaccount values
	gateway.ServiceAccount = getServiceAccountValues(svcAccountConfig)
	// pod template values
	gateway.ExtraPodAnnotations = podConfig.GetExtraAnnotations()
	gateway.ExtraPodLabels = podConfig.GetExtraLabels()
	gateway.ImagePullSecrets = podConfig.GetImagePullSecrets()
	gateway.PodSecurityContext = podConfig.GetSecurityContext()
	gateway.NodeSelector = podConfig.GetNodeSelector()
	gateway.Affinity = podConfig.GetAffinity()
	gateway.Tolerations = podConfig.GetTolerations()
	gateway.ReadinessProbe = podConfig.GetReadinessProbe()
	gateway.LivenessProbe = podConfig.GetLivenessProbe()
	gateway.GracefulShutdown = podConfig.GetGracefulShutdown()
	gateway.TerminationGracePeriodSeconds = podConfig.GetTerminationGracePeriodSeconds()

	// envoy container values
	logLevel := envoyContainerConfig.GetBootstrap().GetLogLevel()
	compLogLevels := envoyContainerConfig.GetBootstrap().GetComponentLogLevels()
	gateway.LogLevel = logLevel
	compLogLevelStr, err := ComponentLogLevelsToString(compLogLevels)
	if err != nil {
		return nil, err
	}
	gateway.ComponentLogLevel = &compLogLevelStr

	agentgatewayEnabled := agentGatewayConfig.GetEnabled()
	if agentgatewayEnabled != nil && *agentgatewayEnabled {
		gateway.Resources = agentGatewayConfig.GetResources()
		gateway.SecurityContext = agentGatewayConfig.GetSecurityContext()
		gateway.Image = getImageValues(agentGatewayConfig.GetImage())
		gateway.Env = agentGatewayConfig.GetEnv()
	} else {
		gateway.Resources = envoyContainerConfig.GetResources()
		gateway.SecurityContext = envoyContainerConfig.GetSecurityContext()
		gateway.Image = getImageValues(envoyContainerConfig.GetImage())
		gateway.Env = envoyContainerConfig.GetEnv()
	}

	// istio values
	gateway.Istio = getIstioValues(k.inputs.IstioAutoMtlsEnabled, istioConfig)
	gateway.SdsContainer = getSdsContainerValues(sdsContainerConfig)
	gateway.IstioContainer = getIstioContainerValues(istioContainerConfig)

	// ai values
	gateway.AIExtension, err = getAIExtensionValues(aiExtensionConfig)
	if err != nil {
		return nil, err
	}

	// TODO(npolshak): Currently we are using the same chart for both data planes. Should revisit having a separate chart for agentgateway: https://github.com/kgateway-dev/kgateway/issues/11240
	// agentgateway integration values
	gateway.AgentGateway, err = getAgentGatewayValues(agentGatewayConfig)
	if err != nil {
		return nil, err
	}

	gateway.Stats = getStatsValues(statsConfig)

	return vals, nil
}

func getGatewayClassFromGateway(ctx context.Context, cli client.Client, gw *api.Gateway) (*api.GatewayClass, error) {
	if gw == nil {
		return nil, eris.New("nil Gateway")
	}
	if gw.Spec.GatewayClassName == "" {
		return nil, eris.New("GatewayClassName must not be empty")
	}

	gwc := &api.GatewayClass{}
	err := cli.Get(ctx, client.ObjectKey{Name: string(gw.Spec.GatewayClassName)}, gwc)
	if err != nil {
		return nil, eris.Errorf("failed to get GatewayClass for Gateway %s/%s", gw.GetName(), gw.GetNamespace())
	}

	return gwc, nil
}

// getInMemoryGatewayParameters returns an in-memory GatewayParameters based on the name of the gateway class.
func getInMemoryGatewayParameters(name string, imageInfo *ImageInfo) *v1alpha1.GatewayParameters {
	switch name {
	case wellknown.WaypointClassName:
		return defaultWaypointGatewayParameters(imageInfo)
	case wellknown.GatewayClassName:
		return defaultGatewayParameters(imageInfo)
	case wellknown.AgentGatewayClassName:
		return defaultAgentGatewayParameters(imageInfo)
	default:
		return defaultGatewayParameters(imageInfo)
	}
}

// defaultAgentGatewayParameters returns an in-memory GatewayParameters with default values
// set for the agentgateway deployment.
func defaultAgentGatewayParameters(imageInfo *ImageInfo) *v1alpha1.GatewayParameters {
	gwp := defaultGatewayParameters(imageInfo)
	gwp.Spec.Kube.AgentGateway.Enabled = ptr.To(true)
	return gwp
}

// defaultWaypointGatewayParameters returns an in-memory GatewayParameters with default values
// set for the waypoint deployment.
func defaultWaypointGatewayParameters(imageInfo *ImageInfo) *v1alpha1.GatewayParameters {
	gwp := defaultGatewayParameters(imageInfo)
	gwp.Spec.Kube.Service.Type = ptr.To(corev1.ServiceTypeClusterIP)

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
func defaultGatewayParameters(imageInfo *ImageInfo) *v1alpha1.GatewayParameters {
	return &v1alpha1.GatewayParameters{
		Spec: v1alpha1.GatewayParametersSpec{
			SelfManaged: nil,
			Kube: &v1alpha1.KubernetesProxyConfig{
				Deployment: &v1alpha1.ProxyDeployment{
					Replicas: ptr.To[uint32](1),
				},
				Service: &v1alpha1.Service{
					Type: (*corev1.ServiceType)(ptr.To(string(corev1.ServiceTypeLoadBalancer))),
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
				AgentGateway: &v1alpha1.AgentGateway{
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
}

func gatewayFrom(gw *api.Gateway) *ir.Gateway {
	out := &ir.Gateway{
		ObjectSource: ir.ObjectSource{
			Group:     api.SchemeGroupVersion.Group,
			Kind:      wellknown.GatewayKind,
			Namespace: gw.Namespace,
			Name:      gw.Name,
		},
		Obj:       gw,
		Listeners: make([]ir.Listener, 0, len(gw.Spec.Listeners)),
	}

	for _, l := range gw.Spec.Listeners {
		out.Listeners = append(out.Listeners, ir.Listener{
			Listener: l,
			Parent:   gw,
		})
	}
	return out
}
