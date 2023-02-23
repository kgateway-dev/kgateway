package kubeutils

import (
	"os"
	"strconv"
)

const (
	TearDown     = "TEAR_DOWN"
	SkipInstall  = "SKIP_INSTALL"
	KubeTestType = "KUBE2E_TESTS"
)

func ShouldTearDown() bool {
	return IsEnvTruthy(TearDown)
}

func ShouldSkipInstall() bool {
	return IsEnvTruthy(SkipInstall)
}

func IsKubeTestType(expectedType string) bool {
	return expectedType == os.Getenv(KubeTestType)
}

func IsEnvTruthy(envVarName string) bool {
	envValue, _ := strconv.ParseBool(os.Getenv(envVarName))
	return envValue
}
