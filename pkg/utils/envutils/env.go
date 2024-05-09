package envutils

import (
	"os"
	"strconv"
)

// IsEnvTruthy returns true if a given environment variable has a truthy value
// Examples of truthy values are: "1", "t", "T", "true", "TRUE", "True". Anything else is considered false.
func IsEnvTruthy(envVarName string) bool {
	envValue, _ := strconv.ParseBool(os.Getenv(envVarName))
	return envValue
}

// IsEnvDefined returns true if a given environment variable has any value
func IsEnvDefined(envVarName string) bool {
	envValue := os.Getenv(envVarName)
	return len(envValue) > 0
}
