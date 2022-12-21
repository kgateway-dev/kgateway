package helpers

import (
	"fmt"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"runtime"
	"strconv"

	"github.com/hashicorp/go-multierror"
	"github.com/onsi/ginkgo"

	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// SkipInvalidTestsEnvVar can be set to true to skip tests which don't meet certain local requirements (like OS)
	// If this value is not set, tests which don't meet requirements will fail
	SkipInvalidTestsEnvVar = "SKIP_INVALID_TESTS"
)

type RequiredConfiguration struct {
	supportedOS   sets.String
	supportedArch sets.String

	// Set of env variables which must be defined
	definedEnvVar []string

	// Set of env variables which must have a truthy (1, true, T) value
	truthyEnvVar []string
}

// Validate returns an error is the RequiredConfiguration is not met
func (r RequiredConfiguration) Validate() error {
	var errs *multierror.Error

	errs = multierror.Append(
		errs,
		r.validateOS(),
		r.validateArch(),
		r.validateDefinedEnv(),
		r.validateTruthyEnv())

	return errs.ErrorOrNil()
}

func (r RequiredConfiguration) validateOS() error {
	if r.supportedOS.Len() == 0 {
		// An empty set is considered to support all
		return nil
	}
	if r.supportedOS.Has(runtime.GOOS) {
		return nil
	}

	return fmt.Errorf("runtime os (%s), is not in supported set (%v)", runtime.GOOS, r.supportedOS.UnsortedList())
}

func (r RequiredConfiguration) validateArch() error {
	if r.supportedArch.Len() == 0 {
		// An empty set is considered to support all
		return nil
	}
	if r.supportedArch.Has(runtime.GOARCH) {
		return nil
	}

	return fmt.Errorf("runtime arch (%s), is not in supported set (%v)", runtime.GOARCH, r.supportedArch.UnsortedList())
}

func (r RequiredConfiguration) validateDefinedEnv() error {
	for _, env := range r.definedEnvVar {
		if _, found := os.LookupEnv(env); !found {
			return fmt.Errorf("env (%s) is not defined", env)
		}
	}
	return nil
}

func (r RequiredConfiguration) validateTruthyEnv() error {
	for _, env := range r.truthyEnvVar {
		envValue := os.Getenv(env)
		envBoolValue, _ := strconv.ParseBool(envValue)
		if !envBoolValue {
			return fmt.Errorf("env (%s) needs to be truthy, but is (%s)", env, envValue)
		}
	}
	return nil
}

// ValidateRequirementsAndNotifyGinkgo validates that the provided Requirements are met, and if they are not, either:
// 	A. Notifies Ginkgo that the current spec was skipped if SKIP_INVALID_TESTS=1
// 	B. Notifies Ginkgo that the current spec has failed otherwise
func ValidateRequirementsAndNotifyGinkgo(requirements ...Requirement) {
	err := ValidateRequirements(requirements)
	if err == nil {
		return
	}

	skipInvalidTests := os.Getenv(SkipInvalidTestsEnvVar)
	boolValue, _ := strconv.ParseBool(skipInvalidTests)

	message := fmt.Sprintf("Test requirements not met: %v", err)
	if boolValue {
		ginkgo.Skip(message)
	} else {
		ginkgo.Fail(message)
	}
}

// ValidateRequirements returns an error if any of the Requirements are not met
func ValidateRequirements(requirements []Requirement) error {
	// default
	requiredConfiguration := &RequiredConfiguration{
		supportedOS:   sets.NewString(),
		supportedArch: sets.NewString(),
	}

	// apply requirements
	for _, requirement := range requirements {
		requirement(requiredConfiguration)
	}

	// perform validation
	return requiredConfiguration.Validate()
}

// Requirement represents an required property for tests.
type Requirement func(configuration *RequiredConfiguration)

func LinuxOnly() Requirement {
	return func(configuration *RequiredConfiguration) {
		configuration.supportedOS = sets.NewString("linux")
	}
}

func DefinedEnv(env string) Requirement {
	return func(configuration *RequiredConfiguration) {
		configuration.definedEnvVar = append(configuration.definedEnvVar, env)
	}
}

func TruthyEnv(env string) Requirement {
	return func(configuration *RequiredConfiguration) {
		configuration.truthyEnvVar = append(configuration.truthyEnvVar, env)
	}
}

func Kubernetes() Requirement {
	return func(configuration *RequiredConfiguration) {
		DefinedEnv(clientcmd.RecommendedConfigPathEnvVar)(configuration)
		TruthyEnv("RUN_KUBE_TESTS")(configuration)
	}
}
