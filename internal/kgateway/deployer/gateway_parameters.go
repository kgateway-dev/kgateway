package deployer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	api "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/helm"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
)

// ErrNoValidPorts is returned when no valid ports are found for the Gateway
var ErrNoValidPorts = errors.New("no valid ports")

func NewGatewayParameters(cli client.Client, inputs *deployer.Inputs) *GatewayParameters {
	return &GatewayParameters{
		cli:               cli,
		inputs:            inputs,
		knownGWParameters: []client.Object{&v1alpha1.GatewayParameters{}}, // always include default GatewayParameters
	}
}

type GatewayParameters struct {
	cli                         client.Client
	inputs                      *deployer.Inputs
	helmValuesGeneratorOverride deployer.HelmValuesGenerator
	knownGWParameters           []client.Object
}

// WithExtraGatewayParameters registers additional parameter object types that should be watched by the controller.
// This is separate from the generator override - it's purely for setting up watches.
func (gp *GatewayParameters) WithExtraGatewayParameters(objects ...client.Object) *GatewayParameters {
	gp.knownGWParameters = append(gp.knownGWParameters, objects...)
	return gp
}

type kGatewayParameters struct {
	cli    client.Client
	inputs *deployer.Inputs
}

func (gp *GatewayParameters) WithHelmValuesGeneratorOverride(generator deployer.HelmValuesGenerator) *GatewayParameters {
	gp.helmValuesGeneratorOverride = generator
	return gp
}

func LoadEnvoyChart() (*chart.Chart, error) {
	return loadChart(helm.EnvoyHelmChart)
}

func LoadAgentgatewayChart() (*chart.Chart, error) {
	return loadChart(helm.AgentgatewayHelmChart)
}

func GatewayGVKsToWatch(ctx context.Context, d *deployer.Deployer) ([]schema.GroupVersionKind, error) {
	return d.GetGvksToWatch(ctx, map[string]any{
		"gateway": map[string]any{
			"istio": map[string]any{
				"enabled": false,
			},
			"image": map[string]any{"repository": "placeholderGatewayGVKsToWatch"},
		},
	})
}

func (gp *GatewayParameters) AllKnownGatewayParameters() []client.Object {
	return slices.Clone(gp.knownGWParameters)
}

func (gp *GatewayParameters) IsSelfManaged(ctx context.Context, obj client.Object) (bool, error) {
	generator, err := gp.getHelmValuesGenerator(ctx, obj)
	if err != nil {
		return false, err
	}
	return generator.IsSelfManaged(ctx, obj)
}

func (gp *GatewayParameters) GetValues(ctx context.Context, obj client.Object) (map[string]any, error) {
	generator, err := gp.getHelmValuesGenerator(ctx, obj)
	if err != nil {
		return nil, err
	}

	return generator.GetValues(ctx, obj)
}

func GatewayReleaseNameAndNamespace(obj client.Object) (string, string) {
	return obj.GetName(), obj.GetNamespace()
}

func (gp *GatewayParameters) getHelmValuesGenerator(ctx context.Context, obj client.Object) (deployer.HelmValuesGenerator, error) {
	gw, ok := obj.(*api.Gateway)
	if !ok {
		return nil, fmt.Errorf("expected a Gateway resource, got %s", obj.GetObjectKind().GroupVersionKind().String())
	}

	if gp.helmValuesGeneratorOverride != nil {
		slog.Debug("using override HelmValuesGenerator for Gateway",
			"gateway_name", gw.GetName(),
			"gateway_namespace", gw.GetNamespace(),
		)
		return gp.helmValuesGeneratorOverride, nil
	}

	slog.Debug("using default HelmValuesGenerator for Gateway",
		"gateway_name", gw.GetName(),
		"gateway_namespace", gw.GetNamespace(),
	)
	return newKGatewayParameters(gp.cli, gp.inputs), nil
}

func newKGatewayParameters(cli client.Client, inputs *deployer.Inputs) *kGatewayParameters {
	return &kGatewayParameters{cli: cli, inputs: inputs}
}

func (h *kGatewayParameters) IsSelfManaged(ctx context.Context, obj client.Object) (bool, error) {
	gw, ok := obj.(*api.Gateway)
	if !ok {
		return false, fmt.Errorf("expected a Gateway resource, got %s", obj.GetObjectKind().GroupVersionKind().String())
	}

	gwParam, err := h.getGatewayParametersForGateway(ctx, gw)
	if err != nil {
		return false, err
	}
	return gwParam != nil && gwParam.Spec.SelfManaged != nil, nil
}

func (h *kGatewayParameters) GetValues(ctx context.Context, obj client.Object) (map[string]any, error) {
	gw, ok := obj.(*api.Gateway)
	if !ok {
		return nil, fmt.Errorf("expected a Gateway resource, got %s", obj.GetObjectKind().GroupVersionKind().String())
	}

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
	err = deployer.JsonConvert(vals, &jsonVals)
	return jsonVals, err
}

// getGatewayParametersForGateway returns the merged GatewayParameters object resulting from the default GwParams object and
// the GwParam object specifically associated with the given Gateway (if one exists).
func (k *kGatewayParameters) getGatewayParametersForGateway(ctx context.Context, gw *api.Gateway) (*v1alpha1.GatewayParameters, error) {
	// attempt to get the GatewayParameters name from the Gateway. If we can't find it,
	// we'll check for the default GWP for the GatewayClass.
	if gw.Spec.Infrastructure == nil || gw.Spec.Infrastructure.ParametersRef == nil {
		slog.Debug("no GatewayParameters found for Gateway, using default",
			"gateway_name", gw.GetName(),
			"gateway_namespace", gw.GetNamespace(),
		)
		return k.getDefaultGatewayParameters(ctx, gw)
	}

	gwpName := gw.Spec.Infrastructure.ParametersRef.Name
	if group := gw.Spec.Infrastructure.ParametersRef.Group; group != v1alpha1.GroupName {
		return nil, fmt.Errorf("invalid group %s for GatewayParameters", group)
	}
	if kind := gw.Spec.Infrastructure.ParametersRef.Kind; kind != api.Kind(wellknown.GatewayParametersGVK.Kind) {
		return nil, fmt.Errorf("invalid kind %s for GatewayParameters", kind)
	}

	// the GatewayParameters must live in the same namespace as the Gateway
	gwpNamespace := gw.GetNamespace()
	gwp := &v1alpha1.GatewayParameters{}
	err := k.cli.Get(ctx, client.ObjectKey{Namespace: gwpNamespace, Name: gwpName}, gwp)
	if err != nil {
		return nil, deployer.GetGatewayParametersForGatewayError(err, gwpNamespace, gwpName, gw.GetNamespace(), gw.GetName(), "Gateway")
	}

	defaultGwp, err := k.getDefaultGatewayParameters(ctx, gw)
	if err != nil {
		return nil, err
	}

	mergedGwp := defaultGwp
	if ptr.Deref(gwp.Spec.Kube.GetOmitDefaultSecurityContext(), false) {
		// Need to regenerate defaults with OmitDefaultSecurityContext=true
		gwc, err := getGatewayClassFromGateway(ctx, k.cli, gw)
		if err != nil {
			return nil, err
		}
		mergedGwp = deployer.GetInMemoryGatewayParameters(gwc.GetName(), k.inputs.ImageInfo, k.inputs.GatewayClassName, k.inputs.WaypointGatewayClassName, k.inputs.AgentgatewayClassName, true)
	}
	deployer.DeepMergeGatewayParameters(mergedGwp, gwp)
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
	// Our defaults depend on OmitDefaultSecurityContext, but these are the defaults
	// when not OmitDefaultSecurityContext:
	defaultGwp := deployer.GetInMemoryGatewayParameters(
		gwc.GetName(),
		k.inputs.ImageInfo,
		k.inputs.GatewayClassName,
		k.inputs.WaypointGatewayClassName,
		k.inputs.AgentgatewayClassName,
		false,
	)

	paramRef := gwc.Spec.ParametersRef
	if paramRef == nil {
		// when there is no parametersRef, just return the defaults
		return defaultGwp, nil
	}

	gwpName := paramRef.Name
	if gwpName == "" {
		err := errors.New("parametersRef.name cannot be empty when parametersRef is specified")
		slog.Error("could not get gateway parameters for gateway class",
			"error", err,
			"gatewayClassName", gwc.GetName(),
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
		return nil, deployer.GetGatewayParametersForGatewayClassError(
			err,
			gwpNamespace, gwpName,
			gwc.GetName(),
			"GatewayClass",
		)
	}

	// merge the explicit GatewayParameters with the defaults. this is
	// primarily done to ensure that the image registry and tag are
	// correctly set when they aren't overridden by the GatewayParameters.
	mergedGwp := defaultGwp
	if ptr.Deref(gwp.Spec.Kube.GetOmitDefaultSecurityContext(), false) {
		mergedGwp = deployer.GetInMemoryGatewayParameters(
			gwc.GetName(),
			k.inputs.ImageInfo,
			k.inputs.GatewayClassName,
			k.inputs.WaypointGatewayClassName,
			k.inputs.AgentgatewayClassName,
			true,
		)
	}
	deployer.DeepMergeGatewayParameters(mergedGwp, gwp)
	return mergedGwp, nil
}

func (k *kGatewayParameters) getValues(gw *api.Gateway, gwParam *v1alpha1.GatewayParameters) (*deployer.HelmConfig, error) {
	var err error
	irGW := deployer.GetGatewayIR(gw, k.inputs.CommonCollections)
	ports := deployer.GetPortsValues(irGW, gwParam, irGW.ControllerName == k.inputs.AgentgatewayControllerName)
	if len(ports) == 0 {
		return nil, ErrNoValidPorts
	}

	gtw := &deployer.HelmGateway{
		Name:             &gw.Name,
		GatewayName:      &gw.Name,
		GatewayNamespace: &gw.Namespace,
		GatewayClassName: ptr.To(string(gw.Spec.GatewayClassName)),
		Ports:            ports,
		Xds: &deployer.HelmXds{
			// The xds host/port MUST map to the Service definition for the Control Plane
			// This is the socket address that the Proxy will connect to on startup, to receive xds updates
			Host: &k.inputs.ControlPlane.XdsHost,
			Port: &k.inputs.ControlPlane.XdsPort,
			Tls: &deployer.HelmXdsTls{
				Enabled: ptr.To(k.inputs.ControlPlane.XdsTLS),
				CaCert:  ptr.To(k.inputs.ControlPlane.XdsTlsCaPath),
			},
		},
		AgwXds: &deployer.HelmXds{
			// The agentgateway xds host/port MUST map to the Service definition for the Control Plane
			// This is the socket address that the Proxy will connect to on startup, to receive xds updates
			Host: &k.inputs.ControlPlane.XdsHost,
			Port: &k.inputs.ControlPlane.AgwXdsPort,
			Tls: &deployer.HelmXdsTls{
				Enabled: ptr.To(k.inputs.ControlPlane.XdsTLS),
				CaCert:  ptr.To(k.inputs.ControlPlane.XdsTlsCaPath),
			},
		},
	}
	if i := gw.Spec.Infrastructure; i != nil {
		gtw.GatewayAnnotations = translateInfraMeta(i.Annotations)
		gtw.GatewayLabels = translateInfraMeta(i.Labels)
	}
	// construct the default values
	vals := &deployer.HelmConfig{
		Gateway: gtw,
	}

	// Inject xDS CA certificate into Helm values if TLS is enabled
	if k.inputs.ControlPlane.XdsTLS {
		if err := k.injectXdsCACertificate(vals); err != nil {
			return nil, fmt.Errorf("failed to inject xDS CA certificate: %w", err)
		}
	}

	// if there is no GatewayParameters, return the values as is
	if gwParam == nil {
		return vals, nil
	}

	// The security contexts may need to be updated if privileged ports are used.
	// This may affect both the PodSecurityContext and the SecurityContexts for the containers defined in gwParam
	// Note: this call may populate the PodSecurityContext and SecurityContext fields in the gateway parameters if they are null,
	// so this needs to happen before those kubeProxyConfig fields are extracted to local variables.
	deployer.UpdateSecurityContexts(gwParam.Spec.Kube, vals.Gateway.Ports)

	// extract all the custom values from the GatewayParameters
	// (note: if we add new fields to GatewayParameters, they will
	// need to be plumbed through here as well)

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
	if aiExtensionConfig != nil && aiExtensionConfig.GetEnabled() != nil && *aiExtensionConfig.GetEnabled() {
		slog.Warn("gatewayparameters spec.kube.aiExtension is deprecated in v2.1 and will be removed in v2.2. Use spec.kube.agentgateway instead.")
	}

	gateway := vals.Gateway

	// deployment values
	if deployConfig.GetReplicas() != nil {
		gateway.ReplicaCount = ptr.To(uint32(*deployConfig.GetReplicas())) // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	gateway.Strategy = deployConfig.GetStrategy()

	// service values
	gateway.Service = deployer.GetServiceValues(svcConfig)
	// serviceaccount values
	gateway.ServiceAccount = deployer.GetServiceAccountValues(svcAccountConfig)
	// pod template values
	gateway.ExtraPodAnnotations = podConfig.GetExtraAnnotations()
	gateway.ExtraPodLabels = podConfig.GetExtraLabels()
	gateway.ImagePullSecrets = podConfig.GetImagePullSecrets()
	gateway.PodSecurityContext = podConfig.GetSecurityContext()
	gateway.NodeSelector = podConfig.GetNodeSelector()
	gateway.Affinity = podConfig.GetAffinity()
	gateway.Tolerations = podConfig.GetTolerations()
	gateway.StartupProbe = podConfig.GetStartupProbe()
	gateway.ReadinessProbe = podConfig.GetReadinessProbe()
	gateway.LivenessProbe = podConfig.GetLivenessProbe()
	gateway.GracefulShutdown = podConfig.GetGracefulShutdown()
	gateway.TerminationGracePeriodSeconds = podConfig.GetTerminationGracePeriodSeconds()
	gateway.TopologySpreadConstraints = podConfig.GetTopologySpreadConstraints()
	gateway.ExtraVolumes = podConfig.GetExtraVolumes()

	// data plane container
	if agwConfig := kubeProxyConfig.GetAgentgateway(); ptr.Deref(agwConfig.GetEnabled(), false) {
		gateway.DataPlaneType = deployer.DataPlaneAgentgateway
		gateway.Resources = agwConfig.GetResources()
		gateway.SecurityContext = agwConfig.GetSecurityContext()
		gateway.Image = deployer.GetImageValues(agwConfig.GetImage())
		gateway.Env = agwConfig.GetEnv()
		gateway.ExtraVolumeMounts = agwConfig.ExtraVolumeMounts
		gateway.LogLevel = agwConfig.GetLogLevel()
		gateway.CustomConfigMapName = agwConfig.GetCustomConfigMapName()
	} else {
		gateway.DataPlaneType = deployer.DataPlaneEnvoy
		logLevel := envoyContainerConfig.GetBootstrap().GetLogLevel()
		gateway.LogLevel = logLevel
		compLogLevels := envoyContainerConfig.GetBootstrap().GetComponentLogLevels()
		compLogLevelStr, err := deployer.ComponentLogLevelsToString(compLogLevels)
		if err != nil {
			return nil, err
		}
		gateway.ComponentLogLevel = &compLogLevelStr
		gateway.Resources = envoyContainerConfig.GetResources()
		gateway.SecurityContext = envoyContainerConfig.GetSecurityContext()
		gateway.Image = deployer.GetImageValues(envoyContainerConfig.GetImage())
		gateway.Env = envoyContainerConfig.GetEnv()
		gateway.ExtraVolumeMounts = envoyContainerConfig.ExtraVolumeMounts
	}

	// istio values
	gateway.Istio = deployer.GetIstioValues(k.inputs.IstioAutoMtlsEnabled, istioConfig)
	gateway.SdsContainer = deployer.GetSdsContainerValues(sdsContainerConfig)
	gateway.IstioContainer = deployer.GetIstioContainerValues(istioContainerConfig)

	// ai values
	gateway.AIExtension, err = deployer.GetAIExtensionValues(aiExtensionConfig)
	if err != nil {
		return nil, err
	}

	gateway.Stats = deployer.GetStatsValues(statsConfig)

	return vals, nil
}

// injectXdsCACertificate reads the CA certificate from the control plane's mounted TLS Secret
// and injects it into the Helm values so it can be used by the proxy templates.
func (k *kGatewayParameters) injectXdsCACertificate(vals *deployer.HelmConfig) error {
	caCertPath := k.inputs.ControlPlane.XdsTlsCaPath
	if _, err := os.Stat(caCertPath); os.IsNotExist(err) {
		return fmt.Errorf("xDS TLS is enabled but CA certificate file not found at %s. "+
			"Ensure the xDS TLS secret is properly mounted and contains ca.crt", caCertPath,
		)
	}

	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		return fmt.Errorf("failed to read CA certificate from %s: %w", caCertPath, err)
	}
	if len(caCert) == 0 {
		return fmt.Errorf("CA certificate at %s is empty", caCertPath)
	}

	caCertStr := string(caCert)
	if vals.Gateway.Xds != nil && vals.Gateway.Xds.Tls != nil {
		vals.Gateway.Xds.Tls.CaCert = &caCertStr
	}
	if vals.Gateway.AgwXds != nil && vals.Gateway.AgwXds.Tls != nil {
		vals.Gateway.AgwXds.Tls.CaCert = &caCertStr
	}

	return nil
}

func getGatewayClassFromGateway(ctx context.Context, cli client.Client, gw *api.Gateway) (*api.GatewayClass, error) {
	if gw == nil {
		return nil, errors.New("nil Gateway")
	}
	if gw.Spec.GatewayClassName == "" {
		return nil, errors.New("GatewayClassName must not be empty")
	}

	gwc := &api.GatewayClass{}
	err := cli.Get(ctx, client.ObjectKey{Name: string(gw.Spec.GatewayClassName)}, gwc)
	if err != nil {
		return nil, fmt.Errorf("failed to get GatewayClass for Gateway %s/%s: %w", gw.GetName(), gw.GetNamespace(), err)
	}

	return gwc, nil
}

func translateInfraMeta[K ~string, V ~string](meta map[K]V) map[string]string {
	infra := make(map[string]string, len(meta))
	for k, v := range meta {
		if strings.HasPrefix(string(k), "gateway.networking.k8s.io/") {
			continue // ignore this prefix to avoid conflicts
		}
		infra[string(k)] = string(v)
	}
	return infra
}
