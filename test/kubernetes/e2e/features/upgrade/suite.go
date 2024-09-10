package upgrade

import (
	"context"
	"path/filepath"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/tests/base"
	"github.com/solo-io/gloo/test/kubernetes/testutils/helper"
	"github.com/solo-io/skv2/codegen/util"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is the entire Suite of tests for the Upgrade Tests
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	// The release version in the test installation gets overwritten by the test helper
	// So we keep it safe and update it
	releaseVersion := testInst.Metadata.ReleasedVersion
	testHelper := e2e.MustTestHelper(ctx, testInst)
	testHelper.ReleasedVersion = releaseVersion
	testInst.Metadata.ReleasedVersion = releaseVersion

	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, testHelper, base.SimpleTestCase{}, nil),
	}
}

func (s *testingSuite) SetupSuite() {
	// Since we do not need any special setup before a suite, overload this method
}

func (s *testingSuite) TearDownSuite() {
	// Since we do not need any special setup before a suite, overload this method
}

func (s *testingSuite) BeforeTest(suiteName, testName string) {
	err := s.TestHelper.InstallGloo(s.Ctx, 600*time.Second, helper.WithExtraArgs([]string{
		"--values", s.TestInstallation.Metadata.ValuesManifestFile,
	}...),
		helper.WithCRDs(filepath.Join(s.TestHelper.RootDir, "install", "helm", "gloo", "crds")))
	s.TestInstallation.Assertions.Require.NoError(err)
}

func (s *testingSuite) AfterTest(suiteName, testName string) {
	s.TestInstallation.UninstallGlooGateway(s.Ctx, func(ctx context.Context) error {
		return s.TestHelper.UninstallGlooAll()
	})
}

func (s *testingSuite) TestUpdateValidationServerGrpcMaxSizeBytes() {
	// Verify that it was installed with the appropriate settings
	settings := s.GetKubectlOutput("-n", s.TestInstallation.Metadata.InstallNamespace, "get", "settings", "default", "-o", "yaml")
	s.TestInstallation.Assertions.Assert.Contains(settings, "invalidRouteResponseCode: 404")

	s.UpgradeWithCustomValuesFile(filepath.Join(util.MustGetThisDir(), "testdata/manifests", "server-grpc-max-size-bytes.yaml"))

	// Verify that the changes in helm reflected in the settings CR
	settings = s.GetKubectlOutput("-n", s.TestInstallation.Metadata.InstallNamespace, "get", "settings", "default", "-o", "yaml")
	s.TestInstallation.Assertions.Assert.Contains(settings, "invalidRouteResponseCode: 404")
	s.TestInstallation.Assertions.Assert.Contains(settings, "validationServerGrpcMaxSizeBytes: 5000000")
}

func (s *testingSuite) TestAddSecondGatewayProxySeparateNamespace() {
	// Create the namespace used by the secondary GW proxy
	externalNamespace := "other-ns"
	s.GetKubectlOutput("create", "ns", externalNamespace)

	s.UpgradeWithCustomValuesFile(filepath.Join(util.MustGetThisDir(), "testdata/manifests", "secondary-gateway-namespace-validation.yaml"))

	// Ensures deployment is created for both default namespace and external one
	// Note - name of external deployments is kebab-case of gatewayProxies NAME helm value
	deployments := s.GetKubectlOutput("-n", s.TestInstallation.Metadata.InstallNamespace, "get", "deployment", "-A")
	s.TestInstallation.Assertions.Assert.Contains(deployments, "gateway-proxy")
	s.TestInstallation.Assertions.Assert.Contains(deployments, "proxy-external")

	// Ensures service account is created for the external namespace
	serviceAccounts := s.GetKubectlOutput("get", "serviceaccount", "-n", externalNamespace)
	s.TestInstallation.Assertions.Assert.Contains(serviceAccounts, "gateway-proxy")

	// Ensures namespace is cleaned up before continuing
	s.GetKubectlOutput("delete", "ns", externalNamespace)
}

func (s *testingSuite) TestValidationWebhookCABundle() {

	ensureWebhookCABundleMatchesSecretsRootCAValue := func() {
		// Ensure the webhook caBundle should be the same as the secret's root ca value
		secretCert := s.GetKubectlOutput("-n", s.TestInstallation.Metadata.InstallNamespace, "get", "secrets", "gateway-validation-certs", "-o", "jsonpath='{.data.ca\\.crt}'")
		webhookCABundle := s.GetKubectlOutput("-n", s.TestInstallation.Metadata.InstallNamespace, "get", "validatingWebhookConfiguration", "gloo-gateway-validation-webhook-"+s.TestInstallation.Metadata.InstallNamespace, "-o", "jsonpath='{.webhooks[0].clientConfig.caBundle}'")
		s.TestInstallation.Assertions.Assert.Equal(webhookCABundle, secretCert)
	}

	ensureWebhookCABundleMatchesSecretsRootCAValue()

	s.UpgradeWithCustomValuesFile(filepath.Join(util.MustGetThisDir(), "testdata/manifests", "strict-validation.yaml"))

	// Ensure the webhook caBundle should be the same as the secret's root ca value post upgrade
	ensureWebhookCABundleMatchesSecretsRootCAValue()
}

func (s *testingSuite) UpgradeWithCustomValuesFile(valuesFile string) {
	_, err := s.TestHelper.UpgradeGloo(s.Ctx, 600*time.Second, helper.WithExtraArgs([]string{
		// Do not reuse the existing values as we need to install the new chart with the new version of the images
		"--values", valuesFile,
	}...))
	s.TestInstallation.Assertions.Require.NoError(err)
}
