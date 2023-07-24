package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/pkg/errors"
	"github.com/solo-io/k8s-utils/installutils/kuberesource"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/solo-io/gloo/install/helm/gloo/generate"

	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/cliutil/helm"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	soloHelm "github.com/solo-io/go-utils/helmutils"
	"github.com/solo-io/go-utils/testutils"
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
	namespace      = defaults.GlooSystem
	releaseName    = "gloo"
	chartDir       = "../helm/gloo"
	debugOutputDir = "../../_output/helm/charts"

	// the Gateway CR helm templates are stored as yaml in a configmap with this name
	customResourceConfigMapName = "gloo-custom-resource-config"
)

var (
	version    string
	pullPolicy v1.PullPolicy
)

func TestHelm(t *testing.T) {
	RegisterFailHandler(Fail)
	testutils.RegisterCommonFailHandlers()
	RunSpecs(t, "Helm Suite")
}

var _ = BeforeSuite(func() {
	version = MustGetVersion()
	pullPolicy = v1.PullIfNotPresent
	// generate the values.yaml and Chart.yaml files
	MustMake(".", "-C", "../../", "generate-helm-files", "-B")
})

type renderTestCase struct {
	rendererName string
	renderer     ChartRenderer
}

var renderers = []renderTestCase{
	{"Helm 3", helm3Renderer{
		chartDir:          chartDir,
		manifestOutputDir: "", // set to the value of the const debugOutputDir when debugging locally
	}},
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

func MustMakeReturnStdout(dir string, args ...string) string {
	makeCmd := exec.Command("make", args...)
	makeCmd.Dir = dir

	var stdout bytes.Buffer
	makeCmd.Stdout = &stdout

	makeCmd.Stderr = GinkgoWriter
	err := makeCmd.Run()

	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	return stdout.String()
}

// MustGetVersion returns the VERSION that will be used to build the chart
func MustGetVersion() string {
	output := MustMakeReturnStdout(".", "-C", "../../", "print-VERSION") // use print-VERSION so version matches on forks
	lines := strings.Split(output, "\n")

	// output from a fork:
	// <[]string | len:4, cap:4>: [
	//	"make[1]: Entering directory '/workspace/gloo'",
	//	"<VERSION>",
	//	"make[1]: Leaving directory '/workspace/gloo'",
	//	"",
	// ]

	// output from the gloo repo:
	// <[]string | len:2, cap:2>: [
	//	"<VERSION>",
	//	"",
	// ]

	if len(lines) == 4 {
		// This is being executed from a fork
		return lines[1]
	}

	if len(lines) == 2 {
		// This is being executed from the Gloo repo
		return lines[0]
	}

	// Error loudly to prevent subtle failures
	Fail(fmt.Sprintf("print-VERSION output returned unknown format. %v", lines))
	return "version-not-found"
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
	// manifestOutputDir is a useful field to set when running tests locally
	// it will output the generated manifest to a directory that you can easily
	// inspect. If this value is an empty string, it will print the manifest to a temporary
	// file and automatically clean it up.
	manifestOutputDir string
}

func (h3 helm3Renderer) RenderManifest(namespace string, values helmValues) (TestManifest, error) {
	rel, err := buildHelm3Release(h3.chartDir, namespace, values)
	if err != nil {
		return nil, errors.Errorf("failure in buildHelm3Release: %s", err.Error())
	}

	// the test manifest utils can only read from a file
	var testManifestFile *os.File

	if h3.manifestOutputDir == "" {
		testManifestFile, err = os.CreateTemp("", "*.yaml")
		Expect(err).NotTo(HaveOccurred(), "Should be able to write a temp file for the helm unit test manifest")
		defer func() {
			_ = os.Remove(testManifestFile.Name())

		}()
	} else {
		// Create a new file, with the version name, or truncate the file if one already exists
		testManifestFile, err = os.Create(fmt.Sprintf("%s.yaml", filepath.Join(h3.manifestOutputDir, version)))
		Expect(err).NotTo(HaveOccurred(), "Should be able to write a file to the manifestOutputDir for the helm unit test manifest")
	}

	_, err = testManifestFile.Write([]byte(rel.Manifest))
	Expect(err).NotTo(HaveOccurred(), "Should be able to write the release manifest to the manifest file for the helm unit tests")

	hooks, err := helm.GetHooks(rel.Hooks)
	Expect(err).NotTo(HaveOccurred(), "Should be able to get the hooks in the helm unit test setup")

	for _, hook := range hooks {
		manifest := hook.Manifest
		_, err = testManifestFile.Write([]byte("\n---\n" + manifest))
		Expect(err).NotTo(HaveOccurred(), "Should be able to write the hook manifest to the manifest file for the helm unit tests")
	}

	err = testManifestFile.Close()
	Expect(err).NotTo(HaveOccurred(), "Should be able to close the manifest file")

	// check the manifest for lines that are not correctly parsed
	manifestData, err := os.ReadFile(testManifestFile.Name())
	Expect(err).ToNot(HaveOccurred())
	windowsFound := soloHelm.FindHelmChartWhiteSpaces(string(manifestData), soloHelm.HelmDetectOptions{})
	Expect(windowsFound).To(BeEmpty(), "Helm chart has parsing, white spacing, or formatting issues present")

	return NewTestManifest(testManifestFile.Name()), nil
}

func buildHelm3Release(chartDir, namespace string, values helmValues) (*release.Release, error) {
	chartRequested, err := loader.Load(chartDir)
	if err != nil {
		return nil, errors.Errorf("failed to load chart directory: %s", err.Error())
	}

	helmValues, err := buildHelmValues(chartDir, values)
	if err != nil {
		return nil, errors.Errorf("failure in buildHelmValues: %s", err.Error())
	}

	// Validate that the provided values match the Go types used to construct out docs
	err = validateHelmValues(helmValues)
	if err != nil {
		return nil, errors.Errorf("failure in validateHelmValues: %s", err.Error())
	}

	// Install the chart
	installAction, err := createInstallAction(namespace)
	if err != nil {
		return nil, errors.Errorf("failure in createInstallAction: %s", err.Error())
	}
	release, err := installAction.Run(chartRequested, helmValues)
	if err != nil {
		return nil, errors.Errorf("failure in installAction.run: %s", err.Error())
	}
	return release, err
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
//
//	Failed to render manifest
//	    Unexpected error:
//	        <*errors.errorString | 0xc000fedf40>: {
//	            s: "error unmarshaling JSON: while decoding JSON: json: unknown field \"useTlsTagging\"",
//	        }
//	        error unmarshaling JSON: while decoding JSON: json: unknown field "useTlsTagging"
//	    occurred
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

	bytes, err := os.ReadFile(filePath)
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
	renderer.Namespace = namespace
	renderer.ReleaseName = releaseName
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
