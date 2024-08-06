package helm

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/stretchr/testify/suite"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/gateway"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/tests/base"
	"github.com/solo-io/skv2/codegen/util"
	"github.com/solo-io/solo-kit/pkg/code-generator/schemagen"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is the entire Suite of tests for the Upgrade Tests
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, e2e.MustTestHelper(ctx, testInst), base.SimpleTestCase{}, helmTestCases),
	}
}

func (s *testingSuite) TestProductionRecommendations() {
	envoyDeployment := s.GetKubectlOutput("-n", s.TestInstallation.Metadata.InstallNamespace, "get", "deployment", "gateway-proxy", "-o", "yaml")
	s.Contains(envoyDeployment, "readinessProbe:")
	s.Contains(envoyDeployment, "/envoy-hc")
	s.Contains(envoyDeployment, "readyReplicas: 1")
}

func (s *testingSuite) TestChangedConfigMapTriggersRollout() {
	expectConfigDumpToContain := func(str string) {
		dump, err := gateway.GetEnvoyAdminData(context.TODO(), "gateway-proxy", s.TestHelper.InstallNamespace, "/config_dump", 5*time.Second)
		s.NoError(err)
		s.Contains(dump, str)
	}

	getChecksum := func() string {
		return s.GetKubectlOutput("-n", s.TestInstallation.Metadata.InstallNamespace, "get", "deployment", "gateway-proxy", "-o", "jsonpath='{.spec.template.metadata.annotations.checksum/gateway-proxy-envoy-config}'")
	}

	// The default value is 250000
	expectConfigDumpToContain(`"global_downstream_max_connections": 250000`)
	oldChecksum := getChecksum()

	// A change in the config map should trigger a new deployment anyway
	s.UpgradeWithCustomValuesFile(configMapChangeSetup)

	// We upgrade Gloo with a new value of `globalDownstreamMaxConnections` on envoy
	// This should cause the checkup annotation on the deployment to change and therefore
	// the deployment should be updated with the new value
	expectConfigDumpToContain(`"global_downstream_max_connections": 12345`)
	newChecksum := getChecksum()
	s.NotEqual(oldChecksum, newChecksum)
}

func (s *testingSuite) TestApplyCRDs() {
	var crdsByFileName = map[string]v1.CustomResourceDefinition{}
	crdDir := filepath.Join(util.GetModuleRoot(), "install", "helm", "gloo", "crds")

	err := filepath.Walk(crdDir, func(crdFile string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Parse the file, and extract the CRD
		crd, err := schemagen.GetCRDFromFile(crdFile)
		if err != nil {
			return err
		}
		crdsByFileName[crdFile] = crd

		// continue traversing
		return nil
	})
	s.NoError(err)

	for crdFile, crd := range crdsByFileName {
		// Apply the CRD
		err := s.TestHelper.ApplyFile(s.Ctx, crdFile)
		s.NoError(err)

		// Ensure the CRD is eventually accepted
		out, _, err := s.TestHelper.Execute(s.Ctx, "get", "crd", crd.GetName())
		s.NoError(err)
		s.Contains(out, crd.GetName())
	}
}

// The local helm tests involve templating settings with various values set
// and then validating that the templated data matches fixture data.
// The tests assume that the fixture data we have defined is valid yaml that
// will be accepted by a cluster. However, this has not always been the case
// and it's important that we validate the settings end to end
//
// This solution may not be the best way to validate settings, but it
// attempts to avoid re-running all the helm template tests against a live cluster
// func (s *testingSuite) TestApplySettings() {
// 	settingsFixturesFolder := filepath.Join(util.GetModuleRoot(), "install", "test", "fixtures", "settings")

// 	err := filepath.Walk(settingsFixturesFolder, func(settingsFixtureFile string, info os.FileInfo, err error) error {
// 		if err != nil {
// 			return err
// 		}
// 		if info.IsDir() {
// 			return nil
// 		}

// 		fmt.Println(settingsFixtureFile)
// 		// Apply the fixture
// 		out, e, err := s.TestHelper.Execute(s.Ctx, "apply", "-f", settingsFixtureFile)
// 		fmt.Println(out, e, err)
// 		s.NoError(err)

// 		// continue traversing
// 		return nil
// 	})
// 	s.NoError(err)
// }

// func (s *testingSuite) TestProtoDescriptorBin() {
// 	protoDescriptor := getExampleProtoDescriptor()
// 	fmt.Println(protoDescriptor)
// 	gateway := s.GetKubectlOutput("-n", s.TestInstallation.Metadata.InstallNamespace, "get", "gateways.gateway.solo.io", "gateway-proxy", "-o", "yaml")
// 	fmt.Println(gateway)
// }

// // return a base64-encoded proto descriptor to use for testing
// func getExampleProtoDescriptor() string {
// 	pathToDescriptors := filepath.Join(util.MustGetThisDir(), "../../../../v1helpers/test_grpc_service/descriptors/proto.pb")
// 	bytes, err := os.ReadFile(pathToDescriptors)
// 	fmt.Println(err)
// 	return base64.StdEncoding.EncodeToString(bytes)
// }
