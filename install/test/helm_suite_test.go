package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	errors "github.com/rotisserie/eris"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"testing"
	"text/template"

	"github.com/solo-io/k8s-utils/installutils/kuberesource"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/solo-io/gloo/install/helm/gloo/generate"

	"github.com/onsi/ginkgo/reporters"

	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/cliutil/helm"
	glooVersion "github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install"
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
	k8syamlutil "sigs.k8s.io/yaml"
)

const (
	namespace = defaults.GlooSystem
	releaseName = "gloo"
	chartDir  = "../helm/gloo"
	outputDir = "../../_output/helm/charts"
)

var (
	version    string
	pullPolicy v1.PullPolicy
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
	{"Helm 3", helm3Renderer{chartDir: chartDir}},
}

func runTests(callback func(testCase renderTestCase)) {
	for _, r := range renderers {
		callback(r)
	}
}



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

	// Validate that the provided values match the Go types used to construct out docs
	err = validateHelmValues(helmValues)
	if err != nil {
		return nil, err
	}

	// Install the chart, outputting the templates into the _output/helm directory for easier debugging
	installAction, err := createInstallAction(namespace)
	if err != nil {
		return nil, errors.Wrap(err, "`Helm Install` failed")
	}
	helmRelease, err :=  installAction.Run(chartRequested, helmValues)
	if err != nil {
		return nil, err
	}

	// Lint the generated templates
	lintAction := createLintAction(namespace)
	lintResult := lintAction.Run([]string{chartDir}, helmValues)
	if len(lintResult.Errors) > 0 {
		return nil, errors.Errorf("`Helm Lint` failed. Messages: %+v", lintResult.Messages)
	}

	return helmRelease, nil
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

// validateHelmValues ensures that the unstructured helm values that are provided
// to a chart match the Go type used to generate the Helm documentation
// Returns nil if all the provided values are all included in the Go struct
// Returns an error if a provided value is not included in the Go struct.
//
// Example:
//  Failed to render manifest
//      Unexpected error:
//          <*errors.errorString | 0xc000fedf40>: {
//              s: "error unmarshaling JSON: while decoding JSON: json: unknown field \"useTlsTagging\"",
//          }
//          error unmarshaling JSON: while decoding JSON: json: unknown field "useTlsTagging"
//      occurred
//
// This means that the unstructured values provided to the Helm chart contain a field `useTlsTagging`
// but the Go struct does not contain that field.
func validateHelmValues(unstructuredHelmValues map[string]interface{}) error {
	// This Go type is the source of truth for the Helm docs
	var structuredHelmValues generate.HelmConfig

	unstructuredHelmValueBytes, err := json.Marshal(unstructuredHelmValues)
	if err != nil {
		return err
	}

	// This ensures that an error will be raised if there is an unstructured helm value
	// defined but there is not the equivalent type defined in our Go struct
	//
	// When an error occurs, this means the Go type needs to be amended
	// to include the new field (which is the source of truth for our docs)
	return k8syamlutil.UnmarshalStrict(unstructuredHelmValueBytes, &structuredHelmValues)
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

func createInstallAction(namespace string) (*action.Install, error) {
	settings := install.NewCLISettings(namespace, "")
	actionConfig := new(action.Configuration)
	noOpDebugLog := func(format string, v ...interface{}) {}

	if err := actionConfig.Init(
		settings.RESTClientGetter(),
		namespace,
		os.Getenv("HELM_DRIVER"),
		noOpDebugLog,
	); err != nil {
		return nil, err
	}

	renderer := action.NewInstall(actionConfig)
	renderer.DryRun = true
	renderer.OutputDir = outputDir
	renderer.Namespace = namespace
	renderer.ReleaseName = releaseName
	renderer.ClientOnly = true

	return renderer, nil
}

func createLintAction(namespace string) *action.Lint {
	lintAction := action.NewLint()
	lintAction.Strict = false
	lintAction.Namespace = namespace
	lintAction.WithSubcharts = false

	return lintAction
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

func makeRoleBindingFromUnstructured(resource *unstructured.Unstructured) *rbacv1.RoleBinding {
	bindingObject, err := kuberesource.ConvertUnstructured(resource)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("RoleBinding %+v should be able to convert from unstructured", resource))
	structuredRoleBinding, ok := bindingObject.(*rbacv1.RoleBinding)
	Expect(ok).To(BeTrue(), fmt.Sprintf("RoleBinding %+v should be able to cast to a structured role binding", resource))
	return structuredRoleBinding
}

func makeClusterRoleBindingFromUnstructured(resource *unstructured.Unstructured) *rbacv1.ClusterRoleBinding {
	bindingObject, err := kuberesource.ConvertUnstructured(resource)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("ClusterRoleBinding %+v should be able to convert from unstructured", resource))
	structuredClusterRoleBinding, ok := bindingObject.(*rbacv1.ClusterRoleBinding)
	Expect(ok).To(BeTrue(), fmt.Sprintf("ClusterRoleBinding %+v should be able to cast to a structured cluster role binding", resource))
	return structuredClusterRoleBinding
}
