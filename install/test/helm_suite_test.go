package test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"text/template"

	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/cliutil/helm"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/makefile"
	glootestutils "github.com/solo-io/gloo/test/testutils"
	soloHelm "github.com/solo-io/go-utils/helmutils"
	"github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/k8s-utils/installutils/kuberesource"
	. "github.com/solo-io/k8s-utils/manifesttestutils"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
	pullPolicy corev1.PullPolicy
)

func TestHelm(t *testing.T) {
	RegisterFailHandler(Fail)
	testutils.RegisterCommonFailHandlers()
	RunSpecs(t, "Helm Suite")
}

var _ = BeforeSuite(func() {
	version = makefile.MustGetVersion(".", "-C", "../../")
	pullPolicy = corev1.PullIfNotPresent
	// generate the values.yaml and Chart.yaml files
	makefile.MustMake(".", "-C", "../../", "generate-helm-files", "-B")
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

type ChartRenderer interface {
	// returns a TestManifest containing all resources
	RenderManifest(namespace string, values glootestutils.HelmValues) (TestManifest, error)
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

func (h3 helm3Renderer) RenderManifest(namespace string, values glootestutils.HelmValues) (TestManifest, error) {
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

func buildHelm3Release(chartDir, namespace string, values glootestutils.HelmValues) (*release.Release, error) {
	chartRequested, err := loader.Load(chartDir)
	if err != nil {
		return nil, errors.Errorf("failed to load chart directory: %s", err.Error())
	}

	helmValues, err := glootestutils.BuildHelmValues(values)
	if err != nil {
		return nil, errors.Errorf("failure in buildHelmValues: %s", err.Error())
	}

	// Validate that the provided values match the Go types used to construct out docs
	err = glootestutils.ValidateHelmValues(helmValues)
	if err != nil {
		return nil, errors.Errorf("failure in ValidateHelmValues: %s", err.Error())
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

func makeUnstructured(yam string) *unstructured.Unstructured {
	jsn, err := yaml.YAMLToJSON([]byte(yam))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	runtimeObj, err := runtime.Decode(unstructured.UnstructuredJSONScheme, jsn)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return runtimeObj.(*unstructured.Unstructured)
}

//nolint:unparam // values always receives "gloo-system"
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
