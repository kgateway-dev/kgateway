package testutils

import (
	"os"
	"strconv"
)

const (
	// TearDown is used to TearDown assets after a test completes. This is used in kube2e tests to uninstall
	// Gloo after a test suite completes
	TearDown = "TEAR_DOWN"

	// SkipInstall can be used when running Kube suites consecutively, and you didnt tear down the Gloo
	// installation from a previous run
	SkipInstall = "SKIP_INSTALL"

	// KubeTestType is used to indicate which kube2e suite should be run while executing regression tests
	KubeTestType = "KUBE2E_TESTS"

	// InvalidTestReqsEnvVar is used to define the behavior for running tests locally when the provided requirements
	// are not met. See ValidateRequirementsAndNotifyGinkgo for a detail of available behaviors
	InvalidTestReqsEnvVar = "INVALID_TEST_REQS"

	// RunVaultTests is used to enable any tests which depend on Vault.
	RunVaultTests = "RUN_VAULT_TESTS"

	// RunConsulTests is used to enable any tests which depend on Consul.
	RunConsulTests = "RUN_CONSUL_TESTS"

	// WaitOnFail is used to halt execution of a failed test to give the developer a chance to inspect
	// any assets before they are cleaned up when the test completes
	// This functionality is defined: https://github.com/solo-io/solo-kit/blob/main/test/helpers/fail_handler.go
	// and for it to be available, a test must have registered the custom fail handler using `RegisterCommonFailHandlers`
	WaitOnFail = "WAIT_ON_FAIL"

	// SkipTempDisabledTests is used to temporarily disable tests in CI
	// This should be used sparingly, and if you disable a test, you should create a Github issue
	// to track re-enabling the test
	SkipTempDisabledTests = "SKIP_TEMP_DISABLED"

	// EnvoyImageTag is used in e2e tests to specify the tag of the docker image to use for the tests
	// If a tag is not provided, the tests dynamically identify the latest released tag to use
	EnvoyImageTag = "ENVOY_IMAGE_TAG"

	// EnvoyBinary is used in e2e tests to specify the path to the envoy binary to use for the tests
	EnvoyBinary = "ENVOY_BINARY"

	// ConsulBinary is used in e2e tests to specify the path to the consul binary to use for the tests
	ConsulBinary = "CONSUL_BINARY"

	// VaultBinary is used in e2e tests to specify the path to the vault binary to use for the tests
	VaultBinary = "VAULT_BINARY"

	// ServiceLogLevel is used to set the log level for the test services. See services/logging.go for more details
	ServiceLogLevel = "SERVICE_LOG_LEVEL"

	// ReleasedVersion can be used when running KubeE2E tests to have the test suite use a previously released version of Gloo Edge
	// If set to 'LATEST', the most recently released version will be used
	// If set to another value, the test suite will use that version (ie '1.15.0-beta1')
	// This is an optional value, so if it is not set, the test suite will use the locally built version of Gloo Edge
	ReleasedVersion = "RELEASED_VERSION"
)

// ShouldTearDown returns true if any assets that were created before a test (for example Gloo being installed)
// should be torn down after the test.
func ShouldTearDown() bool {
	return IsEnvTruthy(TearDown)
}

// ShouldSkipInstall returns true if any assets that need to be created before a test (for example Gloo being installed)
// should be skipped. This is typically used in tandem with ShouldTearDown when running consecutive tests and skipping
// both the tear down and install of Gloo Edge.
func ShouldSkipInstall() bool {
	return IsEnvTruthy(SkipInstall)
}

// ShouldSkipTempDisabledTests returns true if temporarily disabled tests should be skipped
func ShouldSkipTempDisabledTests() bool {
	return IsEnvTruthy(SkipTempDisabledTests)
}

// IsEnvTruthy returns true if a given environment variable has a truthy value
// Examples of truthy values are: "1", "t", "T", "true", "TRUE", "True". Anything else is considered false.
func IsEnvTruthy(envVarName string) bool {
	envValue, _ := strconv.ParseBool(os.Getenv(envVarName))
	return envValue
}
