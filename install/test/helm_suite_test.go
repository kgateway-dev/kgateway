package test

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"testing"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install"

	"helm.sh/helm/v3/pkg/release"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	helm2chartutil "k8s.io/helm/pkg/chartutil"
	helm2renderutil "k8s.io/helm/pkg/renderutil"

	"github.com/solo-io/gloo/pkg/cliutil/helm"

	"github.com/ghodss/yaml"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/strvals"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	helm2chartapi "k8s.io/helm/pkg/proto/hapi/chart"
	k8syamlutil "sigs.k8s.io/yaml"

	"github.com/solo-io/go-utils/testutils"
	v1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/go-utils/manifesttestutils"
)

func TestHelm(t *testing.T) {
	version = os.Getenv("TAGGED_VERSION")
	if version == "" {
		version = "dev"
		pullPolicy = v1.PullAlways
	} else {
		version = version[1:]
		pullPolicy = v1.PullIfNotPresent
	}

	RegisterFailHandler(Fail)
	testutils.RegisterCommonFailHandlers()
	RunSpecs(t, "Helm Suite")
}

var _ = BeforeSuite(func() {
	// generate the values.yaml and Chart.yaml files
	MustMake(".", "-C", "../../", "prepare-helm")
})

type renderTestCase struct {
	rendererName string
	renderer     ChartRenderer
}

var renderers = []renderTestCase{
	{"Helm 2", helm2Renderer{chartDir}},
	{"Helm 3", helm3Renderer{chartDir}},
}

func runTests(callback func(testCase renderTestCase)) {
	for _, r := range renderers {
		callback(r)
	}
}

const (
	namespace = "gloo-system"
	chartDir  = "../helm/gloo"
)

var (
	version    string
	pullPolicy v1.PullPolicy
)

func MustMake(dir string, args ...string) {
	makeCmd := exec.Command("make", args...)
	makeCmd.Dir = dir

	makeCmd.Stdout = GinkgoWriter
	makeCmd.Stderr = GinkgoWriter
	err := makeCmd.Run()

	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

type helmValues struct {
	valuesFile string
	valuesArgs []string // each entry should look like `path.to.helm.field=value`
}

type ChartRenderer interface {
	// returns a TestManifest containing all resources NOT marked by our hook-cleanup annotation
	RenderManifest(namespace string, values helmValues) (TestManifest, error)
}

var _ ChartRenderer = &helm3Renderer{}
var _ ChartRenderer = &helm2Renderer{}

type helm3Renderer struct {
	chartDir string
}

func (h3 helm3Renderer) RenderManifest(namespace string, values helmValues) (TestManifest, error) {
	rel, err := BuildHelm3Release(h3.chartDir, namespace, values)
	if err != nil {
		return nil, err
	}

	// the test manifest utils can only read from a file, ugh
	f, err := ioutil.TempFile("", "*.yaml")
	Expect(err).NotTo(HaveOccurred(), "Should be able to write a temp file for the helm unit test manifest")
	defer func() { _ = os.Remove(f.Name()) }()

	_, err = f.Write([]byte(rel.Manifest))
	Expect(err).NotTo(HaveOccurred(), "Should be able to write the release manifest to the temp file for the helm unit tests")

	// also need to add in the hooks, which are not included in the release manifest
	// be sure to skip the resources that we duplicate because of Helm hook weirdness (see the comment on install.GetNonCleanupHooks)
	nonCleanupHooks, err := helm.GetNonCleanupHooks(rel.Hooks)
	Expect(err).NotTo(HaveOccurred(), "Should be able to get the non-cleanup hooks in the helm unit test setup")

	for _, hook := range nonCleanupHooks {
		manifest := hook.Manifest
		_, err = f.Write([]byte("\n---\n" + manifest))
		Expect(err).NotTo(HaveOccurred(), "Should be able to write the hook manifest to the temp file for the helm unit tests")
	}

	return NewTestManifest(f.Name()), nil
}

func BuildHelm3Release(chartDir, namespace string, values helmValues) (*release.Release, error) {
	chartRequested, err := loader.Load(chartDir)
	if err != nil {
		return nil, err
	}

	helmValues, err := buildHelmValues(chartDir, values)
	if err != nil {
		return nil, err
	}

	client, err := buildRenderer(namespace)
	if err != nil {
		return nil, err
	}

	return client.Run(chartRequested, helmValues)
}

type helm2Renderer struct {
	chartDir string
}

func (h2 helm2Renderer) RenderManifest(namespace string, values helmValues) (TestManifest, error) {
	chart, err := helm2chartutil.Load(h2.chartDir)
	if err != nil {
		return nil, err
	}

	helmValues, err := buildHelmValues(h2.chartDir, values)
	if err != nil {
		return nil, err
	}

	helmValuesRaw, err := yaml.Marshal(helmValues)
	if err != nil {
		return nil, err
	}

	templateConfig := &helm2chartapi.Config{Raw: string(helmValuesRaw), Values: map[string]*helm2chartapi.Value{}}

	renderedTemplates, err := helm2renderutil.Render(chart, templateConfig, helm2renderutil.Options{
		ReleaseOptions: helm2chartutil.ReleaseOptions{
			Name:      constants.GlooReleaseName,
			Namespace: namespace,
		},
	})
	if err != nil {
		return nil, err
	}

	// the test manifest utils can only read from a file, ugh
	f, err := ioutil.TempFile("", "*.yaml")
	Expect(err).NotTo(HaveOccurred(), "Should be able to write a temp file for the helm unit test manifest")
	defer func() { _ = os.Remove(f.Name()) }()

	for _, manifest := range renderedTemplates {
		_, err := f.WriteString(manifest + "\n---\n")
		Expect(err).NotTo(HaveOccurred(), "Should be able to write the release manifest to the temp file for the helm unit tests")
	}

	return NewTestManifest(f.Name()), nil
}

// each entry in valuesArgs should look like `path.to.helm.field=value`
func buildHelmValues(chartDir string, values helmValues) (map[string]interface{}, error) {
	// read the chart's base values file first
	finalValues, err := readValuesFile(path.Join(chartDir, "values.yaml"))
	if err != nil {
		return nil, err
	}

	for _, v := range values.valuesArgs {
		err := strvals.ParseInto(v, finalValues)
		if err != nil {
			return nil, err
		}
	}

	if values.valuesFile != "" {
		// these lines ripped out of Helm internals
		// https://github.com/helm/helm/blob/release-3.0/pkg/cli/values/options.go
		mapFromFile, err := readValuesFile(values.valuesFile)
		if err != nil {
			return nil, err
		}

		// Merge with the previous map
		finalValues = mergeMaps(finalValues, mapFromFile)
	}

	return finalValues, nil
}

func readValuesFile(filePath string) (map[string]interface{}, error) {
	mapFromFile := map[string]interface{}{}

	bytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// NOTE: This is not the default golang yaml.Unmarshal, because that implementation
	// does not unmarshal into a map[string]interface{}; it unmarshals the file into a map[interface{}]interface{}
	// https://github.com/go-yaml/yaml/issues/139
	if err := k8syamlutil.Unmarshal(bytes, &mapFromFile); err != nil {
		return nil, err
	}

	return mapFromFile, nil
}

func buildRenderer(namespace string) (*action.Install, error) {
	settings := install.NewCLISettings(namespace)
	actionConfig := new(action.Configuration)
	noOpDebugLog := func(format string, v ...interface{}) {}

	if err := actionConfig.Init(
		settings.RESTClientGetter(),
		defaults.GlooSystem,
		os.Getenv("HELM_DRIVER"),
		noOpDebugLog,
	); err != nil {
		return nil, err
	}

	renderer := action.NewInstall(actionConfig)
	renderer.DryRun = true
	renderer.Namespace = namespace
	renderer.ReleaseName = "gloo"
	renderer.Namespace = "gloo-system"
	renderer.ClientOnly = true

	return renderer, nil
}

// stolen from Helm internals
// https://github.com/helm/helm/blob/release-3.0/pkg/cli/values/options.go#L88
func mergeMaps(a, b map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(a))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if v, ok := v.(map[string]interface{}); ok {
			if bv, ok := out[k]; ok {
				if bv, ok := bv.(map[string]interface{}); ok {
					out[k] = mergeMaps(bv, v)
					continue
				}
			}
		}
		out[k] = v
	}
	return out
}

func makeUnstructured(yam string) *unstructured.Unstructured {
	jsn, err := yaml.YAMLToJSON([]byte(yam))
	Expect(err).NotTo(HaveOccurred())
	runtimeObj, err := runtime.Decode(unstructured.UnstructuredJSONScheme, jsn)
	Expect(err).NotTo(HaveOccurred())
	return runtimeObj.(*unstructured.Unstructured)
}
