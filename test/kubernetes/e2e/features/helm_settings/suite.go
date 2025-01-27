package helm_settings

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

	"github.com/ghodss/yaml"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"text/template"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/skv2/codegen/util"
	"github.com/stretchr/testify/suite"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a Suite of tests that is used to validate that the manifests
// used in our Helm unit tests are valid
//
// These tests are defined separately from our `features/helm` tests, to ensure they are run against
// a standalone installation, and do not impact those other tests.

// The helm unit tests involve templating settings with various values set
// and then validating that the templated data matches fixture data.
// The tests assume that the fixture data we have defined is valid yaml that
// will be accepted by a cluster. However, this has not always been the case,
// and it's important that we validate the settings end to end
//
// This solution may not be the best way to validate settings, but it
// attempts to avoid re-running all the helm template tests against a live cluster
// Reference PRs:
//   - https://github.com/solo-io/gloo/pull/5957 (introduced)
//   - https://github.com/solo-io/gloo/pull/9732 (migrated)
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) TestApplySettingsManifestsFromUnitTests() {
	settingsFixturesFolder := filepath.Join(util.GetModuleRoot(), "install", "test", "fixtures", "settings")

	err := filepath.Walk(settingsFixturesFolder, func(settingsFixtureFile string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		templatedSettings, err := makeUnstructuredFromTemplateFile(settingsFixtureFile, s.testInstallation.Metadata.InstallNamespace)
		if err != nil {
			// stop traversing, error
			return err
		}
		settingsBytes, err := templatedSettings.MarshalJSON()
		if err != nil {
			// stop traversing, error
			return err
		}

		// Apply the fixture
		err = s.testInstallation.Actions.Kubectl().Apply(s.ctx, settingsBytes)
		if err != nil {
			// stop traversing, error
			return err
		}

		// continue traversing
		return nil
	})
	s.Assert().NoError(err, "can apply all settings manifests without an error")
}

func makeUnstructuredFromTemplateFile(fixtureName string, values interface{}) (*unstructured.Unstructured, error) {
	tmpl, err := template.ParseFiles(fixtureName)
	if err != nil {
		return nil, err
	}

	var b bytes.Buffer
	err = tmpl.Execute(&b, values)
	if err != nil {
		return nil, err
	}

	jsn, err := yaml.YAMLToJSON(b.Bytes())
	if err != nil {
		return nil, err
	}

	runtimeObj, err := runtime.Decode(unstructured.UnstructuredJSONScheme, jsn)
	if err != nil {
		return nil, err
	}

	return runtimeObj.(*unstructured.Unstructured), nil
}
