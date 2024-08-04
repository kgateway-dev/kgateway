package envutils

import (
	"os"
	"strconv"
)

// IsEnvTruthy returns true if a given environment variable has a truthy value
// Examples of truthy values are: "1", "t", "T", "true", "TRUE", "True". Anything else is considered false.
func IsEnvTruthy(envVarName string) bool {
	return IsTruthyValue(os.Getenv(envVarName))
}

// IsEnvDefined returns true if a given environment variable has any value
func IsEnvDefined(envVarName string) bool {
	envValue := os.Getenv(envVarName)
	return len(envValue) > 0
}

// IsTruthyValue returns true if a given value is "truthy".
// Examples of truthy values are: "1", "t", "T", "true", "TRUE", "True". Anything else is considered false.
func IsTruthyValue(value string) bool {
	envValue, _ := strconv.ParseBool(value)
	return envValue
}
