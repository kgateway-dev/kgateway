package deployer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"

	"istio.io/istio/pkg/kube/kclient"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/agentgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer/strategicpatch"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

var (
	ErrGatewayParametersNotAllowedForAgentgateway = errors.New(
		"GatewayParameters cannot be used with agentgateway Gateways when KGW_GWP_AGWP_COMPATIBILITY=false; " +
			"use AgentgatewayParameters instead",
	)
)

type agentgatewayParameters struct {
	agwParamClient kclient.Client[*agentgateway.AgentgatewayParameters]
	gwClassClient  kclient.Client[*gwv1.GatewayClass]
	inputs         *deployer.Inputs
}

func newAgentgatewayParameters(cli apiclient.Client, inputs *deployer.Inputs) *agentgatewayParameters {
	return &agentgatewayParameters{
		agwParamClient: kclient.NewFilteredDelayed[*agentgateway.AgentgatewayParameters](cli, wellknown.AgentgatewayParametersGVR, kclient.Filter{ObjectFilter: cli.ObjectFilter()}),
		gwClassClient:  kclient.NewFilteredDelayed[*gwv1.GatewayClass](cli, wellknown.GatewayClassGVR, kclient.Filter{ObjectFilter: cli.ObjectFilter()}),
		inputs:         inputs,
	}
}

func (a *agentgatewayParameters) GetCacheSyncHandlers() []cache.InformerSynced {
	return []cache.InformerSynced{a.agwParamClient.HasSynced, a.gwClassClient.HasSynced}
}

// GetAgentgatewayParametersForGateway returns the AgentgatewayParameters for
// the given Gateway. Be aware of GatewayParameters as well.  It first checks
// if the Gateway references an AgentgatewayParameters via
// infrastructure.parametersRef, then falls back to the GatewayClass's
// parametersRef.
func (a *agentgatewayParameters) GetAgentgatewayParametersForGateway(gw *gwv1.Gateway) (*agentgateway.AgentgatewayParameters, error) {
	if gw.Spec.Infrastructure != nil && gw.Spec.Infrastructure.ParametersRef != nil {
		ref := gw.Spec.Infrastructure.ParametersRef

		if ref.Group == agentgateway.GroupName && ref.Kind == gwv1.Kind(wellknown.AgentgatewayParametersGVK.Kind) {
			agwpName := ref.Name
			agwpNamespace := gw.GetNamespace() // AgentgatewayParameters must be in the same namespace

			agwp := a.agwParamClient.Get(agwpName, agwpNamespace)
			if agwp == nil {
				return nil, fmt.Errorf("AgentgatewayParameters %s/%s not found for Gateway %s/%s",
					agwpNamespace, agwpName, gw.GetNamespace(), gw.GetName())
			}

			slog.Debug("found AgentgatewayParameters for Gateway",
				"agwp_name", agwpName,
				"agwp_namespace", agwpNamespace,
				"gateway_name", gw.GetName(),
				"gateway_namespace", gw.GetNamespace(),
			)
			return agwp, nil
		}
	}

	return a.getAgentgatewayParametersFromGatewayClass(gw)
}

func (a *agentgatewayParameters) getAgentgatewayParametersFromGatewayClass(gw *gwv1.Gateway) (*agentgateway.AgentgatewayParameters, error) {
	gwc, err := a.getGatewayClassFromGateway(gw)
	if err != nil {
		return nil, err
	}

	if gwc.Spec.ParametersRef == nil {
		slog.Debug("no parametersRef on GatewayClass",
			"gatewayclass_name", gwc.GetName(),
		)
		return nil, nil
	}

	ref := gwc.Spec.ParametersRef

	if ref.Group != agentgateway.GroupName || string(ref.Kind) != wellknown.AgentgatewayParametersGVK.Kind {
		slog.Debug("the GatewayClass parametersRef is not an AgentgatewayParameters",
			"gatewayclass_name", gwc.GetName(),
			"group", ref.Group,
			"kind", ref.Kind,
		)
		return nil, nil
	}

	agwpName := ref.Name
	agwpNamespace := ""
	if ref.Namespace != nil {
		agwpNamespace = string(*ref.Namespace)
	}

	agwp := a.agwParamClient.Get(agwpName, agwpNamespace)
	if agwp == nil {
		return nil, fmt.Errorf("AgentgatewayParameters %s/%s not found for GatewayClass %s",
			agwpNamespace, agwpName, gwc.GetName())
	}

	slog.Debug("found AgentgatewayParameters from GatewayClass",
		"agwp_name", agwpName,
		"agwp_namespace", agwpNamespace,
		"gatewayclass_name", gwc.GetName(),
	)
	return agwp, nil
}

func (a *agentgatewayParameters) getGatewayClassFromGateway(gw *gwv1.Gateway) (*gwv1.GatewayClass, error) {
	if gw == nil {
		return nil, errors.New("nil Gateway")
	}
	if gw.Spec.GatewayClassName == "" {
		return nil, errors.New("gatewayClassName must not be empty")
	}

	gwc := a.gwClassClient.Get(string(gw.Spec.GatewayClassName), metav1.NamespaceNone)
	if gwc == nil {
		return nil, fmt.Errorf("failed to get GatewayClass %s for Gateway %s/%s",
			gw.Spec.GatewayClassName, gw.GetNamespace(), gw.GetName())
	}

	return gwc, nil
}

// AgentgatewayParametersApplier applies AgentgatewayParameters configurations and overlays.
type AgentgatewayParametersApplier struct {
	params *agentgateway.AgentgatewayParameters
}

// NewAgentgatewayParametersApplier creates a new applier from the resolved parameters.
func NewAgentgatewayParametersApplier(params *agentgateway.AgentgatewayParameters) *AgentgatewayParametersApplier {
	return &AgentgatewayParametersApplier{params: params}
}

// ApplyToHelmValues applies the AgentgatewayParameters configs to the helm
// values.  This is called before rendering the helm chart. (We render a helm
// chart, but we do not use helm beyond that point.)
func (a *AgentgatewayParametersApplier) ApplyToHelmValues(vals *deployer.HelmConfig) {
	if a.params == nil || vals == nil || vals.Gateway == nil {
		return
	}

	configs := a.params.Spec.AgentgatewayParametersConfigs

	if configs.Image != nil {
		if vals.Gateway.Image == nil {
			vals.Gateway.Image = &deployer.HelmImage{}
		}
		if configs.Image.Registry != nil {
			vals.Gateway.Image.Registry = configs.Image.Registry
		}
		if configs.Image.Repository != nil {
			vals.Gateway.Image.Repository = configs.Image.Repository
		}
		if configs.Image.Tag != nil {
			vals.Gateway.Image.Tag = configs.Image.Tag
		}
		if configs.Image.Digest != nil {
			vals.Gateway.Image.Digest = configs.Image.Digest
		}
		if configs.Image.PullPolicy != nil {
			vals.Gateway.Image.PullPolicy = (*string)(configs.Image.PullPolicy)
		}
	}

	if configs.Resources != nil {
		vals.Gateway.Resources = configs.Resources
	}

	// Apply logging.level as RUST_LOG first, then merge explicit env vars on top.
	// This ensures explicit env vars override logging.level if both specify RUST_LOG.
	if configs.Logging != nil {
		if configs.Logging.Level != "" {
			vals.Gateway.Env = mergeEnvVars(vals.Gateway.Env, []corev1.EnvVar{
				{Name: "RUST_LOG", Value: configs.Logging.Level},
			})
		}
		if configs.Logging.Format != "" {
			format := string(configs.Logging.Format)
			vals.Gateway.LogFormat = &format
			// NOTE: The Deployment needs to have a new rollout if the only
			// thing that changes is the ConfigMap. The usual solution with
			// Helm is an annotation on the Deployment with a hash of the
			// ConfigMap's contents, and that's what our helm chart does. See
			// https://helm.sh/docs/howto/charts_tips_and_tricks/#automatically-roll-deployments
		}
	}

	// Apply rawConfig if present - this will be merged with typed config in the helm template
	if configs.RawConfig != nil && configs.RawConfig.Raw != nil {
		var rawConfigMap map[string]any
		if err := json.Unmarshal(configs.RawConfig.Raw, &rawConfigMap); err == nil {
			vals.Gateway.RawConfig = rawConfigMap
		}
	}

	// Apply explicit environment variables last so they can override logging.level.
	vals.Gateway.Env = mergeEnvVars(vals.Gateway.Env, configs.Env)

	if configs.Shutdown != nil {
		vals.Gateway.TerminationGracePeriodSeconds = ptr.To(configs.Shutdown.Max)
		if vals.Gateway.GracefulShutdown == nil {
			vals.Gateway.GracefulShutdown = &kgateway.GracefulShutdownSpec{}
		}
		vals.Gateway.GracefulShutdown.SleepTimeSeconds = ptr.To(configs.Shutdown.Min)
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
func (a *AgentgatewayParametersApplier) ApplyOverlaysToObjects(objs []client.Object) error {
	if a.params == nil {
		return nil
	}
	applier := strategicpatch.NewOverlayApplier(a.params)
	return applier.ApplyOverlays(objs)
}

type agentgatewayParametersHelmValuesGenerator struct {
	agwParams     *agentgatewayParameters
	gwParamClient kclient.Client[*kgateway.GatewayParameters]
	inputs        *deployer.Inputs
}

func newAgentgatewayParametersHelmValuesGenerator(cli apiclient.Client, inputs *deployer.Inputs) *agentgatewayParametersHelmValuesGenerator {
	return &agentgatewayParametersHelmValuesGenerator{
		agwParams:     newAgentgatewayParameters(cli, inputs),
		gwParamClient: kclient.NewFilteredDelayed[*kgateway.GatewayParameters](cli, wellknown.GatewayParametersGVR, kclient.Filter{ObjectFilter: cli.ObjectFilter()}),
		inputs:        inputs,
	}
}

// GetValues returns helm values derived from AgentgatewayParameters.
func (g *agentgatewayParametersHelmValuesGenerator) GetValues(ctx context.Context, obj client.Object) (map[string]any, error) {
	gw, ok := obj.(*gwv1.Gateway)
	if !ok {
		return nil, fmt.Errorf("expected a Gateway resource, got %s", obj.GetObjectKind().GroupVersionKind().String())
	}

	resolved, err := g.resolveParameters(gw)
	if err != nil {
		return nil, err
	}

	omitDefaultSecurityContext := g.shouldOmitDefaultSecurityContext(resolved)

	vals, err := g.getDefaultAgentgatewayHelmValues(gw, omitDefaultSecurityContext)
	if err != nil {
		return nil, err
	}

	// Apply GWP if present (mutually exclusive with AGWP after resolution)
	if resolved.gwp != nil {
		g.applyGatewayParametersToHelmValues(resolved.gwp, vals)
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
		if err := injectXdsCACertificate(g.inputs.ControlPlane.XdsTlsCaPath, vals); err != nil {
			return nil, fmt.Errorf("failed to inject xDS CA certificate: %w", err)
		}
	}

	var jsonVals map[string]any
	err = deployer.JsonConvert(vals, &jsonVals)
	return jsonVals, err
}

// resolvedParameters holds the resolved parameters for a Gateway, supporting
// both GatewayClass-level and Gateway-level AgentgatewayParameters.
type resolvedParameters struct {
	// gatewayClassAGWP is the AgentgatewayParameters from the GatewayClass (if any).
	gatewayClassAGWP *agentgateway.AgentgatewayParameters
	// gatewayAGWP is the AgentgatewayParameters from the Gateway (if any).
	gatewayAGWP *agentgateway.AgentgatewayParameters
	// gwp is the GatewayParameters (from either Gateway or GatewayClass).
	// This is mutually exclusive with gatewayClassAGWP and gatewayAGWP.
	gwp *kgateway.GatewayParameters
}

// resolveParameters resolves the parameters for the Gateway.
// For AgentgatewayParameters, it returns both GatewayClass-level and Gateway-level
// separately to support ordered overlay merging (GatewayClass first, then Gateway).
// For GatewayParameters, Gateway-level completely replaces GatewayClass-level.
func (g *agentgatewayParametersHelmValuesGenerator) resolveParameters(gw *gwv1.Gateway) (*resolvedParameters, error) {
	result := &resolvedParameters{}

	// Get GatewayClass parameters first
	gwc := g.agwParams.gwClassClient.Get(string(gw.Spec.GatewayClassName), metav1.NamespaceNone)
	if gwc != nil && gwc.Spec.ParametersRef != nil {
		ref := gwc.Spec.ParametersRef

		// Check for AgentgatewayParameters on GatewayClass
		if ref.Group == agentgateway.GroupName && string(ref.Kind) == wellknown.AgentgatewayParametersGVK.Kind {
			agwpNamespace := ""
			if ref.Namespace != nil {
				agwpNamespace = string(*ref.Namespace)
			}
			agwp := g.agwParams.agwParamClient.Get(ref.Name, agwpNamespace)
			if agwp == nil {
				return nil, fmt.Errorf("AgentgatewayParameters %s/%s not found for GatewayClass %s",
					agwpNamespace, ref.Name, gwc.GetName())
			}
			result.gatewayClassAGWP = agwp
		} else if ref.Group == kgateway.GroupName && string(ref.Kind) == wellknown.GatewayParametersGVK.Kind {
			// Check if GatewayParameters is allowed for agentgateway Gateways
			if !g.inputs.GwpAgwpCompatibility {
				return nil, fmt.Errorf("GatewayClass %s references GatewayParameters %s: %w",
					gwc.GetName(), ref.Name, ErrGatewayParametersNotAllowedForAgentgateway)
			}

			gwpNamespace := ""
			if ref.Namespace != nil {
				gwpNamespace = string(*ref.Namespace)
			}
			gwp := g.gwParamClient.Get(ref.Name, gwpNamespace)
			if gwp == nil {
				return nil, fmt.Errorf("GatewayParameters %s/%s not found for GatewayClass %s",
					gwpNamespace, ref.Name, gwc.GetName())
			}
			result.gwp = gwp
		}
	}

	// Check if Gateway has its own parametersRef
	if gw.Spec.Infrastructure != nil && gw.Spec.Infrastructure.ParametersRef != nil {
		ref := gw.Spec.Infrastructure.ParametersRef

		// Check for AgentgatewayParameters
		if ref.Group == agentgateway.GroupName && ref.Kind == gwv1.Kind(wellknown.AgentgatewayParametersGVK.Kind) {
			agwp := g.agwParams.agwParamClient.Get(ref.Name, gw.GetNamespace())
			if agwp == nil {
				return nil, fmt.Errorf("AgentgatewayParameters %s/%s not found for Gateway %s/%s",
					gw.GetNamespace(), ref.Name, gw.GetNamespace(), gw.GetName())
			}
			result.gatewayAGWP = agwp
			// When Gateway has AGWP, it does NOT replace GatewayClass AGWP for overlays,
			// but it does replace GatewayClass GWP (they are different resource types).
			result.gwp = nil
			return result, nil
		}

		// Check for GatewayParameters
		if ref.Group == kgateway.GroupName && ref.Kind == gwv1.Kind(wellknown.GatewayParametersGVK.Kind) {
			// Check if GatewayParameters is allowed for agentgateway Gateways
			if !g.inputs.GwpAgwpCompatibility {
				return nil, fmt.Errorf("Gateway %s/%s references GatewayParameters %s: %w",
					gw.GetNamespace(), gw.GetName(), ref.Name, ErrGatewayParametersNotAllowedForAgentgateway)
			}

			gwp := g.gwParamClient.Get(ref.Name, gw.GetNamespace())
			if gwp == nil {
				return nil, fmt.Errorf("GatewayParameters %s/%s not found for Gateway %s/%s",
					gw.GetNamespace(), ref.Name, gw.GetNamespace(), gw.GetName())
			}
			// Gateway GWP replaces both GatewayClass AGWP and GWP
			result.gatewayClassAGWP = nil
			result.gwp = gwp
			return result, nil
		}

		return nil, fmt.Errorf("infrastructure.parametersRef on Gateway %s references unknown type: group=%s kind=%s",
			gw.GetName(), ref.Group, ref.Kind)
	}

	return result, nil
}

// shouldOmitDefaultSecurityContext checks if the resolved parameters specify
// omitDefaultSecurityContext. Only GatewayParameters supports this field currently.
func (g *agentgatewayParametersHelmValuesGenerator) shouldOmitDefaultSecurityContext(resolved *resolvedParameters) bool {
	// GatewayParameters has explicit omitDefaultSecurityContext field
	if resolved.gwp != nil && resolved.gwp.Spec.Kube != nil {
		return ptr.Deref(resolved.gwp.Spec.Kube.OmitDefaultSecurityContext, false)
	}
	// AgentgatewayParameters doesn't have this field - users can use deployment overlay instead
	return false
}

// applyGatewayParametersToHelmValues applies relevant fields from
// GatewayParameters to helm values.  This provides backward compatibility for
// users who configure agentgateway using GatewayParameters, not
// AgentgatewayParameters.
func (g *agentgatewayParametersHelmValuesGenerator) applyGatewayParametersToHelmValues(gwp *kgateway.GatewayParameters, vals *deployer.HelmConfig) {
	if gwp == nil || gwp.Spec.Kube == nil || vals.Gateway == nil {
		return
	}

	podConfig := gwp.Spec.Kube.GetPodTemplate()

	if extraAnnotations := podConfig.GetExtraAnnotations(); len(extraAnnotations) > 0 {
		if vals.Gateway.ExtraPodAnnotations == nil {
			vals.Gateway.ExtraPodAnnotations = make(map[string]string)
		}
		maps.Copy(vals.Gateway.ExtraPodAnnotations, extraAnnotations)
	}
	if extraLabels := podConfig.GetExtraLabels(); len(extraLabels) > 0 {
		if vals.Gateway.ExtraPodLabels == nil {
			vals.Gateway.ExtraPodLabels = make(map[string]string)
		}
		maps.Copy(vals.Gateway.ExtraPodLabels, extraLabels)
	}

	// Apply pod scheduling fields from PodTemplate
	if nodeSelector := podConfig.GetNodeSelector(); len(nodeSelector) > 0 {
		vals.Gateway.NodeSelector = nodeSelector
	}
	if affinity := podConfig.GetAffinity(); affinity != nil {
		vals.Gateway.Affinity = affinity
	}
	if tolerations := podConfig.GetTolerations(); len(tolerations) > 0 {
		vals.Gateway.Tolerations = tolerations
	}
	if topologySpreadConstraints := podConfig.GetTopologySpreadConstraints(); len(topologySpreadConstraints) > 0 {
		vals.Gateway.TopologySpreadConstraints = topologySpreadConstraints
	}
	if securityContext := podConfig.GetSecurityContext(); securityContext != nil {
		vals.Gateway.PodSecurityContext = securityContext
	}

	svcConfig := gwp.Spec.Kube.GetService()
	vals.Gateway.Service = deployer.GetServiceValues(svcConfig)

	svcAccountConfig := gwp.Spec.Kube.GetServiceAccount()
	vals.Gateway.ServiceAccount = deployer.GetServiceAccountValues(svcAccountConfig)

	// Apply agentgateway-specific config from GatewayParameters
	agwConfig := gwp.Spec.Kube.GetAgentgateway()
	if agwConfig != nil {
		if logLevel := agwConfig.GetLogLevel(); logLevel != nil {
			vals.Gateway.LogLevel = logLevel
		}
		if image := agwConfig.GetImage(); image != nil {
			vals.Gateway.Image = deployer.GetImageValues(image)
		}
		if resources := agwConfig.GetResources(); resources != nil {
			vals.Gateway.Resources = resources
		}
		if securityContext := agwConfig.GetSecurityContext(); securityContext != nil {
			vals.Gateway.SecurityContext = securityContext
		}
		if env := agwConfig.GetEnv(); len(env) > 0 {
			vals.Gateway.Env = env
		}
		if customConfigMapName := agwConfig.GetCustomConfigMapName(); customConfigMapName != nil {
			vals.Gateway.CustomConfigMapName = customConfigMapName
		}
		if extraVolumeMounts := agwConfig.ExtraVolumeMounts; len(extraVolumeMounts) > 0 {
			vals.Gateway.ExtraVolumeMounts = extraVolumeMounts
		}
	}
}

func (g *agentgatewayParametersHelmValuesGenerator) GetCacheSyncHandlers() []cache.InformerSynced {
	handlers := g.agwParams.GetCacheSyncHandlers()
	return append(handlers, g.gwParamClient.HasSynced)
}

// GetResolvedParametersForGateway returns both the GatewayClass-level and Gateway-level
// AgentgatewayParameters for the given Gateway. This allows callers to apply overlays
// in order (GatewayClass first, then Gateway).
func (g *agentgatewayParametersHelmValuesGenerator) GetResolvedParametersForGateway(gw *gwv1.Gateway) (*resolvedParameters, error) {
	return g.resolveParameters(gw)
}

func (g *agentgatewayParametersHelmValuesGenerator) getDefaultAgentgatewayHelmValues(gw *gwv1.Gateway, omitDefaultSecurityContext bool) (*deployer.HelmConfig, error) {
	irGW := deployer.GetGatewayIR(gw, g.inputs.CommonCollections)
	ports := deployer.GetPortsValues(irGW, nil, true) // true = agentgateway
	if len(ports) == 0 {
		return nil, ErrNoValidPorts
	}

	gtw := &deployer.HelmGateway{
		DataPlaneType:    deployer.DataPlaneAgentgateway,
		Name:             &gw.Name,
		GatewayName:      &gw.Name,
		GatewayNamespace: &gw.Namespace,
		GatewayClassName: func() *string {
			s := string(gw.Spec.GatewayClassName)
			return &s
		}(),
		Ports: ports,
		Xds: &deployer.HelmXds{
			Host: &g.inputs.ControlPlane.XdsHost,
			Port: &g.inputs.ControlPlane.XdsPort,
			Tls: &deployer.HelmXdsTls{
				Enabled: func() *bool { b := g.inputs.ControlPlane.XdsTLS; return &b }(),
				CaCert:  &g.inputs.ControlPlane.XdsTlsCaPath,
			},
		},
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
	gtw.DataPlaneType = deployer.DataPlaneAgentgateway

	gtw.TerminationGracePeriodSeconds = ptr.To(int64(60))
	gtw.GracefulShutdown = &kgateway.GracefulShutdownSpec{
		Enabled:          ptr.To(true),
		SleepTimeSeconds: ptr.To(int64(10)),
	}

	gtw.ReadinessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/healthz/ready",
				Port: intstr.FromInt(15021),
			},
		},
		PeriodSeconds: 10,
	}
	gtw.StartupProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/healthz/ready",
				Port: intstr.FromInt(15021),
			},
		},
		PeriodSeconds:    1,
		TimeoutSeconds:   2,
		FailureThreshold: 60,
		SuccessThreshold: 1,
	}

	if !omitDefaultSecurityContext {
		gtw.PodSecurityContext = &corev1.PodSecurityContext{
			Sysctls: []corev1.Sysctl{
				{
					Name:  "net.ipv4.ip_unprivileged_port_start",
					Value: "0",
				},
			},
		}
		gtw.SecurityContext = &corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr.To(false),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			ReadOnlyRootFilesystem: ptr.To(true),
			RunAsNonRoot:           ptr.To(true),
			RunAsUser:              ptr.To(int64(10101)),
		}
	}

	return &deployer.HelmConfig{Gateway: gtw}, nil
}
