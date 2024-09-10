package deployer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/helm"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"golang.org/x/exp/slices"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	api "sigs.k8s.io/gateway-api/apis/v1"
)

var (
	GetGatewayParametersError = eris.New("could not retrieve GatewayParameters")
	getGatewayParametersError = func(err error, gwpNamespace, gwpName, gwNamespace, gwName, resourceType string) error {
		wrapped := eris.Wrap(err, GetGatewayParametersError.Error())
		return eris.Wrapf(wrapped, "(%s.%s) for %s (%s.%s)",
			gwpNamespace, gwpName, resourceType, gwNamespace, gwName)
	}
	NilDeployerInputsErr = eris.New("nil inputs to NewDeployer")
)

// A Deployer is responsible for deploying proxies
type Deployer struct {
	chart *chart.Chart
	cli   client.Client

	inputs *Inputs
}

// Inputs is the set of options used to configure the gateway deployer deployment
type Inputs struct {
	ControllerName string
	Dev            bool
	IstioValues    bootstrap.IstioValues
	ControlPlane   bootstrap.ControlPlane
}

// NewDeployer creates a new gateway deployer
func NewDeployer(cli client.Client, inputs *Inputs) (*Deployer, error) {
	if inputs == nil {
		return nil, NilDeployerInputsErr
	}

	helmChart, err := loadFs(helm.GlooGatewayHelmChart)
	if err != nil {
		return nil, err
	}
	// simulate what `helm package` in the Makefile does
	if version.Version != version.UndefinedVersion {
		helmChart.Metadata.AppVersion = version.Version
		helmChart.Metadata.Version = version.Version
	}

	return &Deployer{
		cli:    cli,
		chart:  helmChart,
		inputs: inputs,
	}, nil
}

// GetGvksToWatch returns the list of GVKs that the deployer will watch for
func (d *Deployer) GetGvksToWatch(ctx context.Context) ([]schema.GroupVersionKind, error) {
	// The deployer watches all resources (Deployment, Service, ServiceAccount, and ConfigMap)
	// that it creates via the deployer helm chart.
	//
	// In order to get the GVKs for the resources to watch, we need:
	// - a placeholder Gateway (only the name and namespace are used, but the actual values don't matter,
	//   as we only care about the GVKs of the rendered resources)
	// - the minimal values that render all the proxy resources (HPA is not included because it's not
	//   fully integrated/working at the moment)
	//
	// Note: another option is to hardcode the GVKs here, but rendering the helm chart is a
	// _slightly_ more dynamic way of getting the GVKs. It isn't a perfect solution since if
	// we add more resources to the helm chart that are gated by a flag, we may forget to
	// update the values here to enable them.
	emptyGw := &api.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "default",
		},
	}
	// TODO(Law): these must be set explicitly as we don't have defaults for them
	// and the internal template isn't robust enough.
	// This should be empty eventually -- the template must be resilient against nil-pointers
	// i.e. don't add stuff here!
	vals := map[string]any{
		"gateway": map[string]any{
			"istio": map[string]any{
				"enabled": false,
			},
			"image": map[string]any{},
		},
	}

	objs, err := d.renderChartToObjects(emptyGw, vals)
	if err != nil {
		return nil, err
	}
	var ret []schema.GroupVersionKind
	for _, obj := range objs {
		gvk := obj.GetObjectKind().GroupVersionKind()
		if !slices.Contains(ret, gvk) {
			ret = append(ret, gvk)
		}
	}

	log.FromContext(ctx).V(1).Info("watching GVKs", "GVKs", ret)
	return ret, nil
}

func jsonConvert(in *helmConfig, out interface{}) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func (d *Deployer) renderChartToObjects(gw *api.Gateway, vals map[string]any) ([]client.Object, error) {
	objs, err := d.Render(gw.Name, gw.Namespace, vals)
	if err != nil {
		return nil, err
	}

	for _, obj := range objs {
		obj.SetNamespace(gw.Namespace)
	}

	return objs, nil
}

// getGatewayParametersForGateway returns the a merged GatewayParameters object resulting from the default GwParams object and
// the GwParam object specifically associated with the given Gateway (if one exists).
func (d *Deployer) getGatewayParametersForGateway(ctx context.Context, gw *api.Gateway) (*v1alpha1.GatewayParameters, error) {
	logger := log.FromContext(ctx)

	// check for a gateway params annotation on the Gateway
	gwpName := gw.GetAnnotations()[wellknown.GatewayParametersAnnotationName]
	if gwpName == "" {
		// there is no custom GatewayParameters; use GatewayParameters attached to GatewayClass
		logger.V(1).Info("no GatewayParameters found for Gateway",
			"gatewayName", gw.GetName(),
			"gatewayNamespace", gw.GetNamespace())
		return d.getDefaultGatewayParameters(ctx, gw)
	}

	// the GatewayParameters must live in the same namespace as the Gateway
	gwpNamespace := gw.GetNamespace()
	gwp := &v1alpha1.GatewayParameters{}
	err := d.cli.Get(ctx, client.ObjectKey{Namespace: gwpNamespace, Name: gwpName}, gwp)
	if err != nil {
		return nil, getGatewayParametersError(err, gwpNamespace, gwpName, gw.GetNamespace(), gw.GetName(), "Gateway")
	}

	defaultGwp, err := d.getDefaultGatewayParameters(ctx, gw)
	if err != nil {
		return nil, err
	}

	mergedGwp := defaultGwp
	deepMergeGatewayParameters(mergedGwp, gwp)
	return mergedGwp, nil
}

// gets the default GatewayParameters associated with the GatewayClass of the provided Gateway
func (d *Deployer) getDefaultGatewayParameters(ctx context.Context, gw *api.Gateway) (*v1alpha1.GatewayParameters, error) {
	gwc, err := d.getGatewayClassFromGateway(ctx, gw)
	if err != nil {
		return nil, err
	}
	return d.getGatewayParametersForGatewayClass(ctx, gwc)
}

// Gets the GatewayParameters object (if any) associated with a given GatewayClass.
func (d *Deployer) getGatewayParametersForGatewayClass(ctx context.Context, gwc *api.GatewayClass) (*v1alpha1.GatewayParameters, error) {
	logger := log.FromContext(ctx)

	paramRef := gwc.Spec.ParametersRef
	if paramRef == nil {
		return nil, eris.Errorf("no default GatewayParameters associated with GatewayClass %s/%s", gwc.GetNamespace(), gwc.GetName())
	}
	gwpName := paramRef.Name
	if gwpName == "" {
		err := eris.New("no GatewayParameters found for GatewayClass")
		logger.Error(err,
			"gatewayClassName", gwc.GetName(),
			"gatewayClassNamespace", gwc.GetNamespace())
		return nil, err
	}

	gwpNamespace := ""
	if paramRef.Namespace != nil {
		gwpNamespace = string(*paramRef.Namespace)
	}

	gwp := &v1alpha1.GatewayParameters{}
	err := d.cli.Get(ctx, client.ObjectKey{Namespace: gwpNamespace, Name: gwpName}, gwp)
	if err != nil {
		return nil, getGatewayParametersError(err, gwpNamespace, gwpName, gwc.GetNamespace(), gwc.GetName(), "GatewayClass")
	}

	return gwp, nil
}

func (d *Deployer) getGatewayClassFromGateway(ctx context.Context, gw *api.Gateway) (*api.GatewayClass, error) {
	if gw == nil {
		return nil, eris.New("nil Gateway")
	}

	if gw.Spec.GatewayClassName == "" {
		return nil, eris.New("GatewayClassName must not be empty")
	}

	gwc := &api.GatewayClass{}
	err := d.cli.Get(ctx, client.ObjectKey{Name: string(gw.Spec.GatewayClassName)}, gwc)
	if err != nil {
		return nil, eris.Errorf("failed to get GatewayClass for Gateway %s/%s", gw.GetName(), gw.GetNamespace())
	}

	return gwc, nil
}

func (d *Deployer) getValues(gw *api.Gateway, gwParam *v1alpha1.GatewayParameters) (*helmConfig, error) {
	// construct the default values
	vals := &helmConfig{
		Gateway: &helmGateway{
			Name:             &gw.Name,
			GatewayName:      &gw.Name,
			GatewayNamespace: &gw.Namespace,
			Ports:            getPortsValues(gw),
			Xds: &helmXds{
				// The xds host/port MUST map to the Service definition for the Control Plane
				// This is the socket address that the Proxy will connect to on startup, to receive xds updates
				Host: &d.inputs.ControlPlane.Kube.XdsHost,
				Port: &d.inputs.ControlPlane.Kube.XdsPort,
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
	istioConfig := kubeProxyConfig.GetIstio()

	sdsContainerConfig := kubeProxyConfig.GetSdsContainer()
	statsConfig := kubeProxyConfig.GetStats()
	istioContainerConfig := istioConfig.GetIstioProxyContainer()
	aiExtensionConfig := kubeProxyConfig.GetAiExtension()

	gateway := vals.Gateway

	// deployment values
	gateway.ReplicaCount = deployConfig.GetReplicas()

	// TODO: The follow stanza has been commented out as autoscaling support has been removed.
	// see https://github.com/solo-io/solo-projects/issues/5948 for more info.
	//
	// autoscalingVals := getAutoscalingValues(kubeProxyConfig.GetAutoscaling())
	// vals.Gateway.Autoscaling = autoscalingVals
	// if autoscalingVals == nil && deployConfig.GetReplicas() != nil {
	// 	replicas := deployConfig.GetReplicas().GetValue()
	// 	vals.Gateway.ReplicaCount = &replicas
	// }

	// service values
	gateway.Service = getServiceValues(svcConfig)
	// pod template values
	gateway.ExtraPodAnnotations = podConfig.GetExtraAnnotations()
	gateway.ExtraPodLabels = podConfig.GetExtraLabels()
	gateway.ImagePullSecrets = podConfig.GetImagePullSecrets()
	gateway.PodSecurityContext = podConfig.GetSecurityContext()
	gateway.NodeSelector = podConfig.GetNodeSelector()
	gateway.Affinity = podConfig.GetAffinity()
	gateway.Tolerations = podConfig.GetTolerations()

	// envoy container values
	logLevel := envoyContainerConfig.GetBootstrap().GetLogLevel()
	compLogLevels := envoyContainerConfig.GetBootstrap().GetComponentLogLevels()
	gateway.LogLevel = logLevel
	compLogLevelStr, err := ComponentLogLevelsToString(compLogLevels)
	if err != nil {
		return nil, err
	}
	gateway.ComponentLogLevel = &compLogLevelStr

	gateway.Resources = envoyContainerConfig.GetResources()
	gateway.SecurityContext = envoyContainerConfig.GetSecurityContext()
	gateway.Image = getImageValues(envoyContainerConfig.GetImage())

	// istio values
	gateway.Istio = getIstioValues(d.inputs.IstioValues, istioConfig)
	gateway.SdsContainer = getSdsContainerValues(sdsContainerConfig)
	gateway.IstioContainer = getIstioContainerValues(istioContainerConfig)
	gateway.AIExtension = getAIExtensionValues(aiExtensionConfig)

	gateway.Stats = getStatsValues(statsConfig)

	return vals, nil
}

// Render relies on a `helm install` to render the Chart with the injected values
// It returns the list of Objects that are rendered, and an optional error if rendering failed,
// or converting the rendered manifests to objects failed.
func (d *Deployer) Render(name, ns string, vals map[string]any) ([]client.Object, error) {
	mem := driver.NewMemory()
	mem.SetNamespace(ns)
	cfg := &action.Configuration{
		Releases: storage.Init(mem),
	}
	install := action.NewInstall(cfg)
	install.Namespace = ns
	install.ReleaseName = name

	// We rely on the Install object in `clientOnly` mode
	// This means that there is no i/o (i.e. no reads/writes to k8s) that would need to be cancelled.
	// This essentially guarantees that this function terminates quickly and doesn't block the rest of the controller.
	install.ClientOnly = true
	installCtx := context.Background()

	release, err := install.RunWithContext(installCtx, d.chart, vals)
	if err != nil {
		return nil, fmt.Errorf("failed to render helm chart for gateway %s.%s: %w", ns, name, err)
	}

	objs, err := ConvertYAMLToObjects(d.cli.Scheme(), []byte(release.Manifest))
	if err != nil {
		return nil, fmt.Errorf("failed to convert helm manifest yaml to objects for gateway %s.%s: %w", ns, name, err)
	}
	return objs, nil
}

// GetObjsToDeploy does the following:
//
// * performs GatewayParameters lookup/merging etc to get a final set of helm values
//
// * use those helm values to render the internal `gloo-gateway` helm chart into k8s objects
//
// * sets ownerRefs on all generated objects
//
// * returns the objects to be deployed by the caller
func (d *Deployer) GetObjsToDeploy(ctx context.Context, gw *api.Gateway) ([]client.Object, error) {
	gwParam, err := d.getGatewayParametersForGateway(ctx, gw)
	if err != nil {
		return nil, err
	}
	// If this is a self-managed Gateway, skip gateway auto provisioning
	if gwParam != nil && gwParam.Spec.SelfManaged != nil {
		return nil, nil
	}

	logger := log.FromContext(ctx)

	vals, err := d.getValues(gw, gwParam)
	if err != nil {
		return nil, fmt.Errorf("failed to get values to render objects for gateway %s.%s: %w", gw.GetNamespace(), gw.GetName(), err)
	}
	logger.V(1).Info("got deployer helm values",
		"gatewayName", gw.GetName(),
		"gatewayNamespace", gw.GetNamespace(),
		"values", vals)

	// convert to json for helm (otherwise go template fails, as the field names are uppercase)
	var convertedVals map[string]any
	err = jsonConvert(vals, &convertedVals)
	if err != nil {
		return nil, fmt.Errorf("failed to convert helm values for gateway %s.%s: %w", gw.GetNamespace(), gw.GetName(), err)
	}
	objs, err := d.renderChartToObjects(gw, convertedVals)
	if err != nil {
		return nil, fmt.Errorf("failed to get objects to deploy for gateway %s.%s: %w", gw.GetNamespace(), gw.GetName(), err)
	}

	// Set owner ref
	for _, obj := range objs {
		obj.SetOwnerReferences([]metav1.OwnerReference{{
			Kind:       gw.Kind,
			APIVersion: gw.APIVersion,
			Controller: ptr.To(true),
			UID:        gw.UID,
			Name:       gw.Name,
		}})
	}

	return objs, nil
}

func (d *Deployer) DeployObjs(ctx context.Context, objs []client.Object) error {
	logger := log.FromContext(ctx)
	for _, obj := range objs {
		logger.V(1).Info("deploying object", "kind", obj.GetObjectKind(), "namespace", obj.GetNamespace(), "name", obj.GetName())
		if err := d.cli.Patch(ctx, obj, client.Apply, client.ForceOwnership, client.FieldOwner(d.inputs.ControllerName)); err != nil {
			return fmt.Errorf("failed to apply object %s %s: %w", obj.GetObjectKind().GroupVersionKind().String(), obj.GetName(), err)
		}
	}
	return nil
}

func loadFs(filesystem fs.FS) (*chart.Chart, error) {
	var bufferedFiles []*loader.BufferedFile
	entries, err := fs.ReadDir(filesystem, ".")
	if err != nil {
		return nil, err
	}
	if len(entries) != 1 {
		return nil, fmt.Errorf("expected exactly one entry in the chart folder, got %v", entries)
	}

	root := entries[0].Name()
	err = fs.WalkDir(filesystem, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		data, readErr := fs.ReadFile(filesystem, path)
		if readErr != nil {
			return readErr
		}

		relativePath, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}

		bufferedFile := &loader.BufferedFile{
			Name: relativePath,
			Data: data,
		}

		bufferedFiles = append(bufferedFiles, bufferedFile)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return loader.LoadFiles(bufferedFiles)
}

func ConvertYAMLToObjects(scheme *runtime.Scheme, yamlData []byte) ([]client.Object, error) {
	var objs []client.Object

	// Split the YAML manifest into separate documents
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlData), 4096)
	for {
		var obj unstructured.Unstructured
		if err := decoder.Decode(&obj); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		// try to translate to real objects, so they are easier to query later
		gvk := obj.GetObjectKind().GroupVersionKind()
		if realObj, err := scheme.New(gvk); err == nil {
			if realObj, ok := realObj.(client.Object); ok {
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, realObj); err == nil {
					objs = append(objs, realObj)
					continue
				}
			}
		} else if len(obj.Object) == 0 {
			// This can happen with an "empty" document
			continue
		}

		objs = append(objs, &obj)
	}

	return objs, nil
}

// applyFloatingUserId will set the RunAsUser field from all security contexts to null if the floatingUserId field is set
func applyFloatingUserId(dstKube *v1alpha1.KubernetesProxyConfig) {
	floatingUserId := dstKube.GetFloatingUserId()
	if floatingUserId == nil || !*floatingUserId {
		return
	}

	// Pod security context
	podSecurityContext := dstKube.GetPodTemplate().GetSecurityContext()
	if podSecurityContext != nil {
		podSecurityContext.RunAsUser = nil
	}

	// Container security contexts
	securityContexts := []*corev1.SecurityContext{
		dstKube.GetEnvoyContainer().GetSecurityContext(),
		dstKube.GetSdsContainer().GetSecurityContext(),
		dstKube.GetIstio().GetIstioProxyContainer().GetSecurityContext(),
		dstKube.GetAiExtension().GetSecurityContext(),
	}

	for _, securityContext := range securityContexts {
		if securityContext != nil {
			securityContext.RunAsUser = nil
		}
	}

}
