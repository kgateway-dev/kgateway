package deployer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
	"istio.io/istio/pkg/kube/kclient"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer/strategicpatch"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/helm"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

var (
	// ErrNoValidPorts is returned when no valid ports are found for the Gateway
	ErrNoValidPorts = errors.New("no valid ports")

	// ErrNotFound is returned when a requested resource is not found
	ErrNotFound = errors.New("resource not found")
)

func NewGatewayParameters(cli apiclient.Client, inputs *deployer.Inputs) *GatewayParameters {
	gp := &GatewayParameters{
		inputs: inputs,
	}

	// Only create the kgateway parameters client if Envoy is enabled
	if inputs.CommonCollections.Settings.EnableEnvoy {
		gp.kgwParameters = newkgatewayParameters(cli, inputs)
	}

	return gp
}

type GatewayParameters struct {
	inputs                      *deployer.Inputs
	helmValuesGeneratorOverride deployer.HelmValuesGenerator
	kgwParameters               *kgatewayParameters
}

type kgatewayParameters struct {
	gwParamClient kclient.Client[*kgateway.GatewayParameters]
	gwClassClient kclient.Client[*gwv1.GatewayClass]
	inputs        *deployer.Inputs
}

func (gp *GatewayParameters) WithHelmValuesGeneratorOverride(generator deployer.HelmValuesGenerator) *GatewayParameters {
	gp.helmValuesGeneratorOverride = generator
	return gp
}

// GetGatewayParametersClient returns the GatewayParameters client if Envoy is enabled, nil otherwise.
// This allows the reconciler to reuse the same client for watching changes.
func (gp *GatewayParameters) GetGatewayParametersClient() kclient.Client[*kgateway.GatewayParameters] {
	if gp.kgwParameters != nil {
		return gp.kgwParameters.gwParamClient
	}
	return nil
}

func LoadEnvoyChart() (*chart.Chart, error) {
	return loadChart(helm.EnvoyHelmChart)
}

func (gp *GatewayParameters) GetValues(ctx context.Context, obj client.Object) (map[string]any, error) {
	generator, err := gp.getHelmValuesGenerator(obj)
	if err != nil {
		return nil, err
	}

	return generator.GetValues(ctx, obj)
}

func (gp *GatewayParameters) GetCacheSyncHandlers() []cache.InformerSynced {
	if gp.helmValuesGeneratorOverride != nil {
		return gp.helmValuesGeneratorOverride.GetCacheSyncHandlers()
	}

	var handlers []cache.InformerSynced
	if gp.kgwParameters != nil {
		handlers = append(handlers, gp.kgwParameters.GetCacheSyncHandlers()...)
	}
	return handlers
}

// PostProcessObjects implements deployer.ObjectPostProcessor.
// It applies GatewayParameters overlays to the rendered objects.
// When both GatewayClass and Gateway have parameters, the overlays
// are applied in order: GatewayClass first, then Gateway on top.
func (gp *GatewayParameters) PostProcessObjects(ctx context.Context, obj client.Object, rendered []client.Object) ([]client.Object, error) {
	// Check if override implements ObjectPostProcessor and delegate to it
	if gp.helmValuesGeneratorOverride != nil {
		if postProcessor, ok := gp.helmValuesGeneratorOverride.(deployer.ObjectPostProcessor); ok {
			return postProcessor.PostProcessObjects(ctx, obj, rendered)
		}
	}

	gw, ok := obj.(*gwv1.Gateway)
	if !ok {
		return rendered, nil
	}

	if gp.kgwParameters == nil {
		// Envoy not enabled; skip overlays (not an error since overlays are optional).
		return rendered, nil
	}
	resolved := gp.kgwParameters.resolveParametersForOverlays(gw)

	// Apply overlays in order: GatewayClass first, then Gateway.
	if resolved.gatewayClassGWP != nil {
		applier := strategicpatch.NewOverlayApplierFromGatewayParameters(resolved.gatewayClassGWP)
		var err error
		rendered, err = applier.ApplyOverlays(rendered)
		if err != nil {
			return nil, err
		}
	}
	if resolved.gatewayGWP != nil {
		applier := strategicpatch.NewOverlayApplierFromGatewayParameters(resolved.gatewayGWP)
		var err error
		rendered, err = applier.ApplyOverlays(rendered)
		if err != nil {
			return nil, err
		}
	}

	return rendered, nil
}

func GatewayReleaseNameAndNamespace(obj client.Object) (string, string) {
	// A helm release is never installed, only a template is generated, so the name doesn't matter
	// Use a hard-coded name to avoid going over the 53 character name limit
	return "release-name-placeholder", obj.GetNamespace()
}

func (gp *GatewayParameters) getHelmValuesGenerator(obj client.Object) (deployer.HelmValuesGenerator, error) {
	gw, ok := obj.(*gwv1.Gateway)
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

	if gp.kgwParameters == nil {
		return nil, fmt.Errorf("no parameter clients available")
	}
	slog.Debug("using default HelmValuesGenerator for Gateway",
		"gateway_name", gw.GetName(),
		"gateway_namespace", gw.GetNamespace(),
	)
	return gp.kgwParameters, nil
}

func newkgatewayParameters(cli apiclient.Client, inputs *deployer.Inputs) *kgatewayParameters {
	return &kgatewayParameters{
		gwParamClient: kclient.NewFilteredDelayed[*kgateway.GatewayParameters](cli, wellknown.GatewayParametersGVR, kclient.Filter{ObjectFilter: cli.ObjectFilter()}),
		gwClassClient: kclient.NewFilteredDelayed[*gwv1.GatewayClass](cli, wellknown.GatewayClassGVR, kclient.Filter{ObjectFilter: cli.ObjectFilter()}),
		inputs:        inputs,
	}
}

func (h *kgatewayParameters) GetValues(ctx context.Context, obj client.Object) (map[string]any, error) {
	gw, ok := obj.(*gwv1.Gateway)
	if !ok {
		return nil, fmt.Errorf("expected a Gateway resource, got %s", obj.GetObjectKind().GroupVersionKind().String())
	}

	gwParam, err := h.getGatewayParametersForGateway(gw)
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

func (k *kgatewayParameters) GetCacheSyncHandlers() []cache.InformerSynced {
	return []cache.InformerSynced{k.gwClassClient.HasSynced, k.gwParamClient.HasSynced}
}

// getGatewayParametersForGateway returns the merged GatewayParameters object resulting from the default GwParams object and
// the GwParam object specifically associated with the given Gateway (if one exists).
func (k *kgatewayParameters) getGatewayParametersForGateway(gw *gwv1.Gateway) (*kgateway.GatewayParameters, error) {
	// attempt to get the GatewayParameters name from the Gateway. If we can't find it,
	// we'll check for the default GWP for the GatewayClass.
	if gw.Spec.Infrastructure == nil || gw.Spec.Infrastructure.ParametersRef == nil {
		slog.Debug("no GatewayParameters found for Gateway, using default",
			"gateway_name", gw.GetName(),
			"gateway_namespace", gw.GetNamespace(),
		)
		return k.getDefaultGatewayParameters(gw)
	}

	ref := gw.Spec.Infrastructure.ParametersRef

	gwpName := ref.Name
	if group := ref.Group; group != kgateway.GroupName {
		return nil, fmt.Errorf("invalid group %s for GatewayParameters", group)
	}
	if kind := ref.Kind; kind != gwv1.Kind(wellknown.GatewayParametersGVK.Kind) {
		return nil, fmt.Errorf("invalid kind %s for GatewayParameters", kind)
	}

	// the GatewayParameters must live in the same namespace as the Gateway
	gwpNamespace := gw.GetNamespace()
	gwp := k.gwParamClient.Get(gwpName, gwpNamespace)
	if gwp == nil {
		return nil, deployer.GetGatewayParametersForGatewayError(ErrNotFound, gwpNamespace, gwpName, gw.GetNamespace(), gw.GetName(), "Gateway")
	}

	defaultGwp, err := k.getDefaultGatewayParameters(gw)
	if err != nil {
		return nil, err
	}

	mergedGwp := defaultGwp
	if ptr.Deref(gwp.Spec.Kube.GetOmitDefaultSecurityContext(), false) {
		// Clear the security context from the defaults to match the behavior of
		// GetInMemoryGatewayParameters with OmitDefaultSecurityContext=true.
		// This preserves GatewayClass params (like replicas) while still honoring
		// the Gateway's omitDefaultSecurityContext setting.
		if mergedGwp.Spec.Kube != nil && mergedGwp.Spec.Kube.EnvoyContainer != nil {
			mergedGwp.Spec.Kube.EnvoyContainer.SecurityContext = nil
		}
	}
	deployer.DeepMergeGatewayParameters(mergedGwp, gwp)
	return mergedGwp, nil
}

// gets the default GatewayParameters associated with the GatewayClass of the provided Gateway
func (k *kgatewayParameters) getDefaultGatewayParameters(gw *gwv1.Gateway) (*kgateway.GatewayParameters, error) {
	gwc, err := getGatewayClassFromGateway(k.gwClassClient, gw)
	if err != nil {
		return nil, err
	}
	return k.getGatewayParametersForGatewayClass(gwc)
}

// Gets the GatewayParameters object associated with a given GatewayClass.
func (k *kgatewayParameters) getGatewayParametersForGatewayClass(gwc *gwv1.GatewayClass) (*kgateway.GatewayParameters, error) {
	// Our defaults depend on OmitDefaultSecurityContext, but these are the defaults
	// when not OmitDefaultSecurityContext:
	defaultGwp, err := deployer.GetInMemoryGatewayParameters(deployer.InMemoryGatewayParametersConfig{
		ControllerName:             string(gwc.Spec.ControllerName),
		ClassName:                  gwc.GetName(),
		ImageInfo:                  k.inputs.ImageInfo,
		WaypointClassName:          k.inputs.WaypointGatewayClassName,
		OmitDefaultSecurityContext: false,
	})
	if err != nil {
		return nil, err
	}

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

	gwp := k.gwParamClient.Get(gwpName, gwpNamespace)
	if gwp == nil {
		return nil, deployer.GetGatewayParametersForGatewayClassError(
			ErrNotFound,
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
		mergedGwp, err = deployer.GetInMemoryGatewayParameters(deployer.InMemoryGatewayParametersConfig{
			ControllerName:             string(gwc.Spec.ControllerName),
			ClassName:                  gwc.GetName(),
			ImageInfo:                  k.inputs.ImageInfo,
			WaypointClassName:          k.inputs.WaypointGatewayClassName,
			OmitDefaultSecurityContext: true,
		})
		if err != nil {
			return nil, err
		}
	}
	deployer.DeepMergeGatewayParameters(mergedGwp, gwp)
	return mergedGwp, nil
}

func (k *kgatewayParameters) getValues(gw *gwv1.Gateway, gwParam *kgateway.GatewayParameters) (*deployer.HelmConfig, error) {
	irGW := deployer.GetGatewayIR(gw, k.inputs.CommonCollections)
	ports := deployer.GetPortsValues(irGW, gwParam)
	if len(ports) == 0 {
		return nil, ErrNoValidPorts
	}

	gtw := &deployer.HelmGateway{
		Name:             &gw.Name,
		FullnameOverride: &gw.Name,
		GatewayName:      &gw.Name,
		GatewayNamespace: &gw.Namespace,
		GatewayClassName: new(string(gw.Spec.GatewayClassName)),
		Ports:            ports,
		Xds: &deployer.HelmXds{
			// The xds host/port MUST map to the Service definition for the Control Plane
			// This is the socket address that the Proxy will connect to on startup, to receive xds updates
			Host: &k.inputs.ControlPlane.XdsHost,
			Port: &k.inputs.ControlPlane.XdsPort,
			Tls: &deployer.HelmXdsTls{
				Enabled: new(k.inputs.ControlPlane.XdsTLS),
				CaCert:  new(k.inputs.ControlPlane.XdsTlsCaPath),
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
		if err := injectXdsCACertificate(k.inputs.ControlPlane.XdsTlsCaPath, vals); err != nil {
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

	gateway := vals.Gateway

	// deployment values
	if deployConfig.GetReplicas() != nil {
		gateway.ReplicaCount = new(uint32(*deployConfig.GetReplicas())) // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	gateway.Strategy = deployConfig.GetStrategy()

	// service values
	gateway.Service = deployer.GetServiceValues(svcConfig)
	// Extract loadBalancerIP from Gateway.spec.addresses and set it on the service if service type is LoadBalancer
	if err := deployer.SetLoadBalancerIPFromGateway(gw, gateway.Service); err != nil {
		return nil, err
	}
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
	gateway.PriorityClassName = podConfig.GetPriorityClassName()

	gateway.DataPlaneType = deployer.DataPlaneEnvoy
	logLevel := envoyContainerConfig.GetBootstrap().GetLogLevel()
	gateway.LogLevel = logLevel
	compLogLevels := envoyContainerConfig.GetBootstrap().GetComponentLogLevels()
	compLogLevelStr, err := deployer.ComponentLogLevelsToString(compLogLevels)
	if err != nil {
		return nil, err
	}
	gateway.ComponentLogLevel = &compLogLevelStr

	// Extract DNS resolver configuration
	dnsResolverConfig := envoyContainerConfig.GetBootstrap().GetDnsResolver()
	if dnsResolverConfig != nil {
		var udpMaxQueries *int32
		if maybeMaxQ := ptr.Deref(dnsResolverConfig.GetUdpMaxQueries(), 0); maybeMaxQ > 0 {
			udpMaxQueries = &maybeMaxQ
		}
		gateway.DnsResolver = &deployer.HelmDnsResolver{
			UdpMaxQueries: udpMaxQueries,
		}
	}

	gateway.Resources = envoyContainerConfig.GetResources()
	gateway.SecurityContext = envoyContainerConfig.GetSecurityContext()
	gateway.Image = deployer.GetImageValues(envoyContainerConfig.GetImage())
	gateway.Env = envoyContainerConfig.GetEnv()
	gateway.ExtraVolumeMounts = envoyContainerConfig.ExtraVolumeMounts

	// istio values
	gateway.Istio = deployer.GetIstioValues(k.inputs.IstioAutoMtlsEnabled, istioConfig)
	gateway.SdsContainer = deployer.GetSdsContainerValues(sdsContainerConfig)
	gateway.IstioContainer = deployer.GetIstioContainerValues(istioContainerConfig)

	gateway.Stats = deployer.GetStatsValues(statsConfig)

	return vals, nil
}

// resolvedKgatewayParameters holds the resolved parameters for a Gateway, supporting
// both GatewayClass-level and Gateway-level GatewayParameters for overlay application.
type resolvedKgatewayParameters struct {
	// gatewayClassGWP is the GatewayParameters from the GatewayClass (if any).
	gatewayClassGWP *kgateway.GatewayParameters
	// gatewayGWP is the GatewayParameters from the Gateway (if any).
	gatewayGWP *kgateway.GatewayParameters
}

// resolveParametersForOverlays resolves the GatewayParameters for the Gateway.
// It returns both GatewayClass-level and Gateway-level parameters separately
// to support ordered overlay merging (GatewayClass first, then Gateway).
// Unlike getGatewayParametersForGateway, this does NOT merge the parameters.
func (k *kgatewayParameters) resolveParametersForOverlays(gw *gwv1.Gateway) *resolvedKgatewayParameters {
	result := &resolvedKgatewayParameters{}

	// Get GatewayClass parameters first
	gwc := k.gwClassClient.Get(string(gw.Spec.GatewayClassName), metav1.NamespaceNone)
	if gwc != nil && gwc.Spec.ParametersRef != nil {
		ref := gwc.Spec.ParametersRef

		// Check for GatewayParameters on GatewayClass
		if ref.Group == kgateway.GroupName && string(ref.Kind) == wellknown.GatewayParametersGVK.Kind {
			gwpNamespace := ""
			if ref.Namespace != nil {
				gwpNamespace = string(*ref.Namespace)
			}
			gwp := k.gwParamClient.Get(ref.Name, gwpNamespace)
			if gwp != nil {
				result.gatewayClassGWP = gwp
			}
		}
	}

	// Check if Gateway has its own parametersRef
	if gw.Spec.Infrastructure != nil && gw.Spec.Infrastructure.ParametersRef != nil {
		ref := gw.Spec.Infrastructure.ParametersRef

		if ref.Group == kgateway.GroupName && ref.Kind == gwv1.Kind(wellknown.GatewayParametersGVK.Kind) {
			gwp := k.gwParamClient.Get(ref.Name, gw.GetNamespace())
			if gwp != nil {
				result.gatewayGWP = gwp
			}
		}
	}

	return result
}

func getGatewayClassFromGateway(cli kclient.Client[*gwv1.GatewayClass], gw *gwv1.Gateway) (*gwv1.GatewayClass, error) {
	if gw == nil {
		return nil, errors.New("nil Gateway")
	}
	if gw.Spec.GatewayClassName == "" {
		return nil, errors.New("GatewayClassName must not be empty")
	}

	gwc := cli.Get(string(gw.Spec.GatewayClassName), metav1.NamespaceNone)
	if gwc == nil {
		return nil, fmt.Errorf("failed to get GatewayClass for Gateway %s/%s", gw.GetName(), gw.GetNamespace())
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
