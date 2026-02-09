package deployer

import (
	"context"
	"encoding/json"
	"fmt"

	"helm.sh/helm/v3/pkg/chart"
	"istio.io/istio/pkg/kube/kclient"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/agentgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer/strategicpatch"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/helm"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

func LoadAgentgatewayChart() (*chart.Chart, error) {
	return loadChart(helm.AgentgatewayHelmChart)
}

// AgentgatewayParametersApplier applies AgentgatewayParameters configurations and overlays.
type AgentgatewayParametersApplier struct {
	params *agentgateway.AgentgatewayParameters
}

// NewAgentgatewayParametersApplier creates a new applier from the resolved parameters.
func NewAgentgatewayParametersApplier(params *agentgateway.AgentgatewayParameters) *AgentgatewayParametersApplier {
	return &AgentgatewayParametersApplier{params: params}
}

func setIfNonNil[T any](dst **T, src *T) {
	if src != nil {
		*dst = src
	}
}

// ApplyToHelmValues applies the AgentgatewayParameters configs to the helm
// values.  This is called before rendering the helm chart. (We render a helm
// chart, but we do not use helm beyond that point.)
func (a *AgentgatewayParametersApplier) ApplyToHelmValues(vals *deployer.AgentgatewayHelmConfig) {
	if a.params == nil || vals == nil || vals.Agentgateway == nil {
		return
	}

	configs := a.params.Spec.AgentgatewayParametersConfigs
	res := vals.Agentgateway

	// Do a manual merge of the fields.
	// Convert from agentgateway.Image to HelmImage
	if configs.Image != nil {
		if res.Image == nil {
			res.Image = &deployer.HelmImage{}
		}
		setIfNonNil(&res.Image.Tag, configs.Image.Tag)
		setIfNonNil(&res.Image.Registry, configs.Image.Registry)
		setIfNonNil(&res.Image.Repository, configs.Image.Repository)
		if configs.Image.PullPolicy != nil {
			pp := string(*configs.Image.PullPolicy)
			res.Image.PullPolicy = &pp
		}
		setIfNonNil(&res.Image.Digest, configs.Image.Digest)
	}
	// Merge resources field-by-field to preserve values from GatewayClass AGWP
	// when Gateway AGWP only sets some fields (e.g., GWC sets limits, GW sets requests).
	res.Resources = deployer.DeepMergeResourceRequirements(res.Resources, configs.Resources)
	setIfNonNil(&res.Shutdown, configs.Shutdown)
	// Merge Istio field-by-field to preserve values from GatewayClass AGWP
	// when Gateway AGWP only sets some fields (e.g., GWC sets caAddress, GW sets trustDomain).
	if configs.Istio != nil {
		if res.Istio == nil {
			res.Istio = &agentgateway.IstioSpec{}
		}
		setIfNonZero(&res.Istio.CaAddress, configs.Istio.CaAddress)
		setIfNonZero(&res.Istio.TrustDomain, configs.Istio.TrustDomain)
	}
	setIfNonNil(&res.RawConfig, configs.RawConfig)

	// Convert RawConfig from *apiextensionsv1.JSON to map[string]any
	if configs.RawConfig != nil && len(configs.RawConfig.Raw) > 0 {
		var rawConfigMap map[string]any
		if err := json.Unmarshal(configs.RawConfig.Raw, &rawConfigMap); err == nil && rawConfigMap != nil {
			res.RawConfig = rawConfigMap
		}
	}

	// Apply logging configuration
	if configs.Logging != nil {
		// Apply logging format
		if configs.Logging.Format != "" {
			format := string(configs.Logging.Format)
			res.LogFormat = &format
		}

		// Apply logging level as RUST_LOG env var
		if configs.Logging.Level != "" {
			// Prepend RUST_LOG so explicit env vars can override it
			rustLogEnv := corev1.EnvVar{Name: "RUST_LOG", Value: configs.Logging.Level}
			res.Env = append([]corev1.EnvVar{rustLogEnv}, res.Env...)
		}
	}

	// Apply explicit environment variables last so they can override logging.level.
	res.Env = mergeEnvVars(res.Env, configs.Env)

	// Apply Istio configuration - merge field-by-field to preserve values from
	// GatewayClass AGWP when Gateway AGWP only sets some fields.
	if configs.Istio != nil {
		if res.Istio == nil {
			res.Istio = &deployer.AgentgatewayHelmIstio{}
		}
		if configs.Istio.CaAddress != "" {
			res.Istio.CaAddress = ptr.To(configs.Istio.CaAddress)
		}
		if configs.Istio.TrustDomain != "" {
			res.Istio.TrustDomain = ptr.To(configs.Istio.TrustDomain)
		}
	}

	// Apply shutdown configuration
	if configs.Shutdown != nil {
		if res.Shutdown == nil {
			res.Shutdown = &deployer.AgentgatewayShutdown{}
		}
		res.Shutdown.Min = ptr.To(configs.Shutdown.Min)
		res.Shutdown.Max = ptr.To(configs.Shutdown.Max)
	}
}

// mergeEnvVars merges two slices of environment variables.
// Variables in 'override' take precedence over variables in 'base' with the same name.
// The order is preserved: base vars first (minus overridden ones), then override vars.
func mergeEnvVars(base, override []corev1.EnvVar) []corev1.EnvVar {
	if len(override) == 0 {
		return base
	}
	if len(base) == 0 {
		return override
	}

	// Build a set of names in override
	overrideNames := make(map[string]struct{}, len(override))
	for _, env := range override {
		overrideNames[env.Name] = struct{}{}
	}

	// Keep base vars that are not overridden
	result := make([]corev1.EnvVar, 0, len(base)+len(override))
	for _, env := range base {
		if _, exists := overrideNames[env.Name]; !exists {
			result = append(result, env)
		}
	}

	// Append all override vars
	result = append(result, override...)
	return result
}

// ApplyOverlaysToObjects applies the strategic-merge-patch overlays to rendered k8s objects.
// This is called after rendering the helm chart.
// It returns the (potentially modified) slice of objects, as new objects may be added
// (e.g., PodDisruptionBudget, HorizontalPodAutoscaler).
func (a *AgentgatewayParametersApplier) ApplyOverlaysToObjects(objs []client.Object) ([]client.Object, error) {
	if a.params == nil {
		return objs, nil
	}
	applier := strategicpatch.NewOverlayApplier(a.params)
	return applier.ApplyOverlays(objs)
}

type AgentgatewayGatewayParameters struct {
	agwParamClient kclient.Client[*agentgateway.AgentgatewayParameters]
	gwClassClient  kclient.Client[*gwv1.GatewayClass]
	inputs         *deployer.Inputs
}

func NewAgentgatewayGatewayParameters(cli apiclient.Client, inputs *deployer.Inputs) *AgentgatewayGatewayParameters {
	return &AgentgatewayGatewayParameters{
		agwParamClient: kclient.NewFilteredDelayed[*agentgateway.AgentgatewayParameters](cli, wellknown.AgentgatewayParametersGVR, kclient.Filter{ObjectFilter: cli.ObjectFilter()}),
		gwClassClient:  kclient.NewFilteredDelayed[*gwv1.GatewayClass](cli, wellknown.GatewayClassGVR, kclient.Filter{ObjectFilter: cli.ObjectFilter()}),
		inputs:         inputs,
	}
}

// GetValues returns helm values derived from AgentgatewayParameters.
func (g *AgentgatewayGatewayParameters) GetValues(ctx context.Context, obj client.Object) (map[string]any, error) {
	gw, ok := obj.(*gwv1.Gateway)
	if !ok {
		return nil, fmt.Errorf("expected a Gateway resource, got %s", obj.GetObjectKind().GroupVersionKind().String())
	}

	resolved, err := g.resolveParameters(gw)
	if err != nil {
		return nil, err
	}

	vals, err := g.getDefaultAgentgatewayHelmValues(gw)
	if err != nil {
		return nil, err
	}

	// Apply AGWP Configs in order: GatewayClass first, then Gateway on top.
	// This allows Gateway-level configs to override GatewayClass-level configs.
	if resolved.gatewayClassAGWP != nil {
		applier := NewAgentgatewayParametersApplier(resolved.gatewayClassAGWP)
		applier.ApplyToHelmValues(vals)
	}
	if resolved.gatewayAGWP != nil {
		applier := NewAgentgatewayParametersApplier(resolved.gatewayAGWP)
		applier.ApplyToHelmValues(vals)
	}

	if g.inputs.ControlPlane.XdsTLS {
		if err := injectXdsCACertificate(g.inputs.ControlPlane.XdsTlsCaPath, vals.Agentgateway.AgwXds); err != nil {
			return nil, fmt.Errorf("failed to inject xDS CA certificate: %w", err)
		}
	}

	var jsonVals map[string]any
	err = deployer.AgentgatewayJsonConvert(vals, &jsonVals)
	return jsonVals, err
}

// resolvedParameters holds the resolved parameters for a Gateway, supporting
// both GatewayClass-level and Gateway-level AgentgatewayParameters.
type resolvedParameters struct {
	// gatewayClassAGWP is the AgentgatewayParameters from the GatewayClass (if any).
	gatewayClassAGWP *agentgateway.AgentgatewayParameters
	// gatewayAGWP is the AgentgatewayParameters from the Gateway (if any).
	gatewayAGWP *agentgateway.AgentgatewayParameters
}

// resolveParameters resolves the AgentgatewayParameters for the Gateway.
// It returns both GatewayClass-level and Gateway-level
// separately to support ordered overlay merging (GatewayClass first, then Gateway).
func (g *AgentgatewayGatewayParameters) resolveParameters(gw *gwv1.Gateway) (*resolvedParameters, error) {
	result := &resolvedParameters{}

	// Get GatewayClass parameters first
	gwc := g.gwClassClient.Get(string(gw.Spec.GatewayClassName), metav1.NamespaceNone)
	if gwc != nil && gwc.Spec.ParametersRef != nil {
		ref := gwc.Spec.ParametersRef

		// Check for AgentgatewayParameters on GatewayClass
		if ref.Group == agentgateway.GroupName && string(ref.Kind) == wellknown.AgentgatewayParametersGVK.Kind {
			agwpNamespace := ""
			if ref.Namespace != nil {
				agwpNamespace = string(*ref.Namespace)
			}
			agwp := g.agwParamClient.Get(ref.Name, agwpNamespace)
			if agwp == nil {
				return nil, fmt.Errorf("for GatewayClass %s, AgentgatewayParameters %s/%s not found",
					gwc.GetName(), agwpNamespace, ref.Name)
			}
			result.gatewayClassAGWP = agwp
		} else {
			return nil, fmt.Errorf("the GatewayClass %s references parameters of a type other than AgentgatewayParameters: %s",
				gwc.GetName(), ref.Name)
		}
	}

	// Check if Gateway has its own parametersRef
	if gw.Spec.Infrastructure != nil && gw.Spec.Infrastructure.ParametersRef != nil {
		ref := gw.Spec.Infrastructure.ParametersRef

		if ref.Group == agentgateway.GroupName && ref.Kind == gwv1.Kind(wellknown.AgentgatewayParametersGVK.Kind) {
			agwp := g.agwParamClient.Get(ref.Name, gw.GetNamespace())
			if agwp == nil {
				return nil, fmt.Errorf("AgentgatewayParameters %s/%s not found for Gateway %s/%s",
					gw.GetNamespace(), ref.Name, gw.GetNamespace(), gw.GetName())
			}
			result.gatewayAGWP = agwp
			return result, nil
		}

		return nil, fmt.Errorf("infrastructure.parametersRef on Gateway %s/%s references unsupported type: group=%s kind=%s; use AgentgatewayParameters instead",
			gw.GetNamespace(), gw.GetName(), ref.Group, ref.Kind)
	}

	return result, nil
}

func (g *AgentgatewayGatewayParameters) GetCacheSyncHandlers() []cache.InformerSynced {
	return []cache.InformerSynced{g.gwClassClient.HasSynced, g.agwParamClient.HasSynced}
}

// PostProcessObjects implements deployer.ObjectPostProcessor.
// It applies AgentgatewayParameters overlays to the rendered objects.
// When both GatewayClass and Gateway have AgentgatewayParameters, the overlays
// are applied in order: GatewayClass first, then Gateway on top.
func (g *AgentgatewayGatewayParameters) PostProcessObjects(ctx context.Context, obj client.Object, rendered []client.Object) ([]client.Object, error) {
	gw, ok := obj.(*gwv1.Gateway)
	if !ok {
		return rendered, nil
	}

	resolved, err := g.GetResolvedParametersForGateway(gw)
	if err != nil {
		return rendered, nil
	}

	// Apply overlays in order: GatewayClass first, then Gateway.
	// This allows Gateway-level overlays to override GatewayClass-level overlays.
	if resolved.gatewayClassAGWP != nil {
		applier := NewAgentgatewayParametersApplier(resolved.gatewayClassAGWP)
		rendered, err = applier.ApplyOverlaysToObjects(rendered)
		if err != nil {
			return nil, err
		}
	}
	if resolved.gatewayAGWP != nil {
		applier := NewAgentgatewayParametersApplier(resolved.gatewayAGWP)
		rendered, err = applier.ApplyOverlaysToObjects(rendered)
		if err != nil {
			return nil, err
		}
	}

	return rendered, nil
}

// GetResolvedParametersForGateway returns both the GatewayClass-level and Gateway-level
// AgentgatewayParameters for the given Gateway. This allows callers to apply overlays
// in order (GatewayClass first, then Gateway).
func (g *AgentgatewayGatewayParameters) GetResolvedParametersForGateway(gw *gwv1.Gateway) (*resolvedParameters, error) {
	return g.resolveParameters(gw)
}

// resolveOverlayAppliers implements overlayResolver for Agentgateway.
func (g *AgentgatewayGatewayParameters) resolveOverlayAppliers(gw *gwv1.Gateway) (gwcApplier, gwApplier overlayApplier, err error) {
	resolved, err := g.resolveParameters(gw)
	if err != nil {
		return nil, nil, err
	}
	if resolved.gatewayClassAGWP != nil {
		gwcApplier = NewAgentgatewayParametersApplier(resolved.gatewayClassAGWP).ApplyOverlaysToObjects
	}
	if resolved.gatewayAGWP != nil {
		gwApplier = NewAgentgatewayParametersApplier(resolved.gatewayAGWP).ApplyOverlaysToObjects
	}
	return gwcApplier, gwApplier, nil
}

func (g *AgentgatewayGatewayParameters) getDefaultAgentgatewayHelmValues(gw *gwv1.Gateway) (*deployer.AgentgatewayHelmConfig, error) {
	irGW := deployer.GetGatewayIR(gw, g.inputs.CommonCollections)
	ports := deployer.GetPortsValues(irGW, nil, true) // true = agentgateway
	if len(ports) == 0 {
		return nil, ErrNoValidPorts
	}

	gtw := &deployer.AgentgatewayHelmGateway{
		Name:             &gw.Name,
		GatewayName:      &gw.Name,
		GatewayNamespace: &gw.Namespace,
		GatewayClassName: func() *string {
			s := string(gw.Spec.GatewayClassName)
			return &s
		}(),
		Ports: ports,
		AgwXds: &deployer.HelmXds{
			Host: &g.inputs.ControlPlane.XdsHost,
			Port: &g.inputs.ControlPlane.AgwXdsPort,
			Tls: &deployer.HelmXdsTls{
				Enabled: func() *bool { b := g.inputs.ControlPlane.XdsTLS; return &b }(),
				CaCert:  &g.inputs.ControlPlane.XdsTlsCaPath,
			},
		},
	}

	if i := gw.Spec.Infrastructure; i != nil {
		gtw.GatewayAnnotations = translateInfraMeta(i.Annotations)
		gtw.GatewayLabels = translateInfraMeta(i.Labels)
	}

	gtw.Image = &deployer.HelmImage{
		Registry:   ptr.To(deployer.AgentgatewayRegistry),
		Repository: ptr.To(deployer.AgentgatewayImage),
		Tag:        ptr.To(deployer.AgentgatewayDefaultTag),
		PullPolicy: ptr.To(""),
	}

	gtw.Service = &deployer.AgentgatewayHelmService{}
	// Extract loadBalancerIP from Gateway.spec.addresses and set it on the service
	if err := deployer.SetAgentgatewayLoadBalancerIPFromGateway(gw, gtw.Service); err != nil {
		return nil, err
	}

	return &deployer.AgentgatewayHelmConfig{Agentgateway: gtw}, nil
}
