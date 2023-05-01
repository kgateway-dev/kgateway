package testutils

import (
	"os"
	"strconv"
)

const (
	// InvalidTestReqsEnvVar is used to define the behavior for running tests locally when the provided requirements
	// are not met. See ValidateRequirementsAndNotifyGinkgo for a detail of available behaviors
	InvalidTestReqsEnvVar = "INVALID_TEST_REQS"

	// RunKubeTests is used to enable any tests which depend on Kubernetes. NOTE: Kubernetes back tests should
	// be written into the kube2e suites, and those don't require this guard.
	RunKubeTests = "RUN_KUBE_TESTS"
)

// IsEnvTruthy returns true if a given environment variable has a truthy value
// Examples of truthy values are: "1", "t", "T", "true", "TRUE", "True". Anything else is considered false.
func IsEnvTruthy(envVarName string) bool {
	envValue, _ := strconv.ParseBool(os.Getenv(envVarName))
	return envValue
}
