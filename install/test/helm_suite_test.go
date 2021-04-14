package test

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"testing"
	"text/template"

	"github.com/onsi/ginkgo/reporters"

	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/cliutil/helm"
	glooVersion "github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/go-utils/versionutils/git"
	. "github.com/solo-io/k8s-utils/manifesttestutils"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/strvals"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	helm2chartutil "k8s.io/helm/pkg/chartutil"
	helm2chartapi "k8s.io/helm/pkg/proto/hapi/chart"
	helm2renderutil "k8s.io/helm/pkg/renderutil"
	k8syamlutil "sigs.k8s.io/yaml"
)

func TestHelm(t *testing.T) {
	RegisterFailHandler(Fail)
	testutils.RegisterCommonFailHandlers()
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Helm Suite", []Reporter{junitReporter})
}

var _ = BeforeSuite(func() {
	version = os.Getenv("TAGGED_VERSION")
	if !glooVersion.IsReleaseVersion() {
		gitInfo, err := git.GetGitRefInfo("./")
		Expect(err).NotTo(HaveOccurred())
		// remove the "v" prefix
		version = gitInfo.Tag[1:]
	} else {
		version = version[1:]
	}
	pullPolicy = v1.PullIfNotPresent
	// generate the values.yaml and Chart.yaml files
	MustMake(".", "-C", "../../", "generate-helm-files", "-B")
})

type renderTestCase struct {
	rendererName string
	renderer     ChartRenderer
}

var renderers = []renderTestCase{
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
	// returns a TestManifest containing all resources
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

	hooks, err := helm.GetHooks(rel.Hooks)

	Expect(err).NotTo(HaveOccurred(), "Should be able to get the hooks in the helm unit test setup")

	for _, hook := range hooks {
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
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	runtimeObj, err := runtime.Decode(unstructured.UnstructuredJSONScheme, jsn)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return runtimeObj.(*unstructured.Unstructured)
}

func makeUnstructureFromTemplateFile(fixtureName string, values interface{}) *unstructured.Unstructured {
	tmpl, err := template.ParseFiles(fixtureName)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	var b bytes.Buffer
	err = tmpl.Execute(&b, values)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return makeUnstructured(b.String())
}
