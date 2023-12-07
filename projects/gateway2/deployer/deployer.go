package deployer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"

	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gateway2/helm"
	"github.com/solo-io/gloo/projects/gateway2/ports"
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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	api "sigs.k8s.io/gateway-api/apis/v1"
)

type gatewayPort struct {
	Port       uint16 `json:"port"`
	Protocol   string `json:"protocol"`
	Name       string `json:"name"`
	TargetPort uint16 `json:"targetPort"`
}

type Deployer struct {
	dev            bool
	chart          *chart.Chart
	scheme         *runtime.Scheme
	controllerName string
	host           string
	port           uint16
}

func NewDeployer(scheme *runtime.Scheme, dev bool, controllerName, host string, port uint16) (*Deployer, error) {

	chart, err := loadFs(helm.GlooGatewayHelmChart)
	if err != nil {
		// don't retrun an error is requeueing won't help here
		return nil, err
	}
	// simulate what `helm package` in the Makefile does
	if version.Version != version.UndefinedVersion {
		chart.Metadata.AppVersion = version.Version
		chart.Metadata.Version = version.Version
	}

	return &Deployer{
		dev:            dev,
		chart:          chart,
		scheme:         scheme,
		controllerName: controllerName,
		host:           host,
		port:           port,
	}, nil
}

func (d *Deployer) GetGvksToWatch(ctx context.Context) ([]schema.GroupVersionKind, error) {
	fakeGw := &api.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "default",
		},
	}

	objs, err := d.renderChartToObjects(ctx, fakeGw)
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
	return ret, nil
}

func jsonConvert(in []gatewayPort, out interface{}) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func (d *Deployer) renderChartToObjects(ctx context.Context, gw *api.Gateway) ([]client.Object, error) {

	// must not be nil for helm to not fail.
	gwPorts := []gatewayPort{}
	for _, l := range gw.Spec.Listeners {
		listenerPort := uint16(l.Port)
		if slices.IndexFunc(gwPorts, func(p gatewayPort) bool { return p.Port == listenerPort }) != -1 {
			continue
		}
		var port gatewayPort
		port.Port = listenerPort
		port.TargetPort = ports.TranslatePort(listenerPort)
		port.Name = string(l.Name)
		port.Protocol = "TCP"
		gwPorts = append(gwPorts, port)
	}

	// convert to json for helm (otherwise go template fails, as the field names are uppercase)
	var portsAny []any
	err := jsonConvert(gwPorts, &portsAny)
	if err != nil {
		return nil, err
	}

	vals := map[string]any{
		"controlPlane": map[string]any{"enabled": false},
		"gateway": map[string]any{
			"enabled":     true,
			"name":        gw.Name,
			"gatewayName": gw.Name,
			"ports":       portsAny,
			// Default to Load Balancer
			"service": map[string]any{
				"type": "LoadBalancer",
			},
			"xds": map[string]any{
				"host": d.host,
				"port": d.port,
			},
		},
	}
	if d.dev {
		vals["develop"] = true
	}
	log := log.FromContext(ctx)
	log.Info("rendering helm chart", "vals", vals)
	objs, err := d.Render(ctx, gw.Name, gw.Namespace, vals)
	if err != nil {
		return nil, err
	}

	for _, obj := range objs {
		obj.SetNamespace(gw.Namespace)
	}

	return objs, nil
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
		return nil, fmt.Errorf("failed to render helm chart: %w", err)
	}

	objs, err := ConvertYAMLToObjects(d.scheme, []byte(release.Manifest))
	if err != nil {
		return nil, fmt.Errorf("failed to convert yaml to objects: %w", err)
	}
	return objs, nil
}

func (d *Deployer) GetObjsToDeploy(ctx context.Context, gw *api.Gateway) ([]client.Object, error) {
	objs, err := d.renderChartToObjects(ctx, gw)
	if err != nil {
		return nil, fmt.Errorf("failed to get objects to deploy: %w", err)
	}

	// Set owner ref
	trueVal := true
	for _, obj := range objs {
		obj.SetOwnerReferences([]metav1.OwnerReference{{
			Kind:       gw.Kind,
			APIVersion: gw.APIVersion,
			Controller: &trueVal,
			UID:        gw.UID,
			Name:       gw.Name,
		}})
	}

	return objs, nil
}

func (d *Deployer) DeployObjs(ctx context.Context, objs []client.Object, cli client.Client) error {
	for _, obj := range objs {
		if err := cli.Patch(ctx, obj, client.Apply, client.ForceOwnership, client.FieldOwner(d.controllerName)); err != nil {
			return fmt.Errorf("failed to apply object %s %s: %w", obj.GetObjectKind().GroupVersionKind().String(), obj.GetName(), err)
		}
	}
	return nil
}

func (d *Deployer) Deploy(ctx context.Context, gw *api.Gateway, cli client.Client) error {
	objs, err := d.GetObjsToDeploy(ctx, gw)
	if err != nil {
		return err
	}
	return d.DeployObjs(ctx, objs, cli)
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
