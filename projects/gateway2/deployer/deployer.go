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
	"github.com/solo-io/gloo/projects/gateway2/extensions"
	"github.com/solo-io/gloo/projects/gateway2/helm"
	"github.com/solo-io/gloo/projects/gateway2/pkg/api/gateway.gloo.solo.io/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"golang.org/x/exp/slices"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
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
	NilK8sExtensionsErr  = eris.New("nil K8sGatewayExtensions to NewDeployer")
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
	ControlPlane   bootstrap.ControlPlane
	Extensions     extensions.K8sGatewayExtensions
}

// NewDeployer creates a new gateway deployer
func NewDeployer(cli client.Client, inputs *Inputs) (*Deployer, error) {
	if inputs == nil {
		return nil, NilDeployerInputsErr
	}
	if inputs.Extensions == nil {
		return nil, NilK8sExtensionsErr
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
	vals := map[string]any{
		"gateway": map[string]any{
			"serviceAccount": map[string]any{
				"create": true,
			},
			"istio": map[string]any{
				"enabled": false,
			},
			"image": map[string]any{},
		},
	}

	objs, err := d.renderChartToObjects(ctx, emptyGw, vals)
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

func (d *Deployer) renderChartToObjects(ctx context.Context, gw *api.Gateway, vals map[string]any) ([]client.Object, error) {
	objs, err := d.Render(ctx, gw.Name, gw.Namespace, vals)
	if err != nil {
		return nil, err
	}

	for _, obj := range objs {
		obj.SetNamespace(gw.Namespace)
	}

	return objs, nil
}

// Gets the GatewayParameters object (if any) associated with a given Gateway.
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
	kubeProxyConfig := gwParam.Spec.GetKube()
	deployConfig := kubeProxyConfig.GetDeployment()
	podConfig := kubeProxyConfig.GetPodTemplate()
	envoyContainerConfig := kubeProxyConfig.GetEnvoyContainer()
	svcConfig := kubeProxyConfig.GetService()
	istioConfig := kubeProxyConfig.GetIstio()
	sdsContainerConfig := kubeProxyConfig.GetSdsContainer()
	istioContainerConfig := istioConfig.GetIstioContainer()

	// deployment values
	autoscalingVals := getAutoscalingValues(kubeProxyConfig.GetAutoscaling())
	vals.Gateway.Autoscaling = autoscalingVals
	if autoscalingVals == nil && deployConfig.GetReplicas() != nil {
		replicas := deployConfig.GetReplicas().GetValue()
		vals.Gateway.ReplicaCount = &replicas
	}

	// service values
	vals.Gateway.Service = getServiceValues(svcConfig)

	// pod template values
	vals.Gateway.ExtraPodAnnotations = podConfig.GetExtraAnnotations()
	vals.Gateway.ExtraPodLabels = podConfig.GetExtraLabels()
	vals.Gateway.ImagePullSecrets = podConfig.GetImagePullSecrets()
	vals.Gateway.PodSecurityContext = podConfig.GetSecurityContext()
	vals.Gateway.NodeSelector = podConfig.GetNodeSelector()
	vals.Gateway.Affinity = podConfig.GetAffinity()
	vals.Gateway.Tolerations = podConfig.GetTolerations()

	// envoy container values
	logLevel := envoyContainerConfig.GetBootstrap().GetLogLevel().GetValue()
	compLogLevels := envoyContainerConfig.GetBootstrap().GetComponentLogLevels()
	vals.Gateway.LogLevel = &logLevel
	compLogLevelStr, err := ComponentLogLevelsToString(compLogLevels)
	if err != nil {
		return nil, err
	}
	vals.Gateway.ComponentLogLevel = &compLogLevelStr

	// istio values
	vals.Gateway.Istio = getIstioValues(istioConfig)
	vals.Gateway.SdsContainer = getSdsContainerValues(sdsContainerConfig)
	vals.Gateway.IstioContainer = getIstioContainerValues(istioContainerConfig)

	vals.Gateway.Resources = envoyContainerConfig.GetResources()
	vals.Gateway.SecurityContext = envoyContainerConfig.GetSecurityContext()
	vals.Gateway.Image = getEnvoyImageValues(envoyContainerConfig.GetImage())

	return vals, nil
}

func (d *Deployer) Render(ctx context.Context, name, ns string, vals map[string]any) ([]client.Object, error) {
	mem := driver.NewMemory()
	mem.SetNamespace(ns)
	cfg := &action.Configuration{
		Releases: storage.Init(mem),
	}
	client := action.NewInstall(cfg)
	client.Namespace = ns
	client.ReleaseName = name
	client.ClientOnly = true
	release, err := client.RunWithContext(ctx, d.chart, vals)
	if err != nil {
		return nil, fmt.Errorf("failed to render helm chart for gateway %s.%s: %w", ns, name, err)
	}

	objs, err := ConvertYAMLToObjects(d.cli.Scheme(), []byte(release.Manifest))
	if err != nil {
		return nil, fmt.Errorf("failed to convert helm manifest yaml to objects for gateway %s.%s: %w", ns, name, err)
	}
	return objs, nil
}

func (d *Deployer) GetObjsToDeploy(ctx context.Context, gw *api.Gateway) ([]client.Object, error) {
	gwParam, err := d.getGatewayParametersForGateway(ctx, gw)
	if err != nil {
		return nil, err
	}
	// If this is a self-managed Gateway, skip gateway auto provisioning
	if gwParam != nil && gwParam.Spec.GetSelfManaged() != nil {
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
	objs, err := d.renderChartToObjects(ctx, gw, convertedVals)
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
