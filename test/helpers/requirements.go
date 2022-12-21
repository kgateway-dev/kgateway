package helpers

import (
	"fmt"
	"os"
	"runtime"
	"strconv"

	"github.com/hashicorp/go-multierror"
	"github.com/onsi/ginkgo"

	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// InvalidTestsEnvVar is used to define the behavior for running tests locally when the provided requirements
	// are not met. See ValidateRequirementsAndNotifyGinkgo for a detail of available behaviors
	InvalidTestsEnvVar = "INVALID_TESTS"
)

// ValidateRequirementsAndNotifyGinkgo validates that the provided Requirements are met, and if they are not, uses
// the InvalidTestsEnvVar to determine how to proceed:
// Options are:
//	- `run`: Ignore any invalid requirements and execute the tests
//	- `skip`: Notify Ginkgo that the current spec was skipped
//	- `fail`: Notify Ginkgo that the current spec has failed
func ValidateRequirementsAndNotifyGinkgo(requirements ...Requirement) {
	err := ValidateRequirements(requirements)
	if err == nil {
		return
	}
	message := fmt.Sprintf("Test requirements not met: %v", err)
	switch os.Getenv(InvalidTestsEnvVar) {
	case "run":
		// ignore the error from validating requirements and let the tests proceed
		return

	case "skip":
		ginkgo.Skip(message)

	case "fail":
		fallthrough
	default:
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

type RequiredConfiguration struct {
	supportedOS   sets.String
	supportedArch sets.String

	// Set of env variables which must be defined
	definedEnvVar []string

	// Set of env variables which must have a truthy value
	// Examples: "1", "t", "T", "true", "TRUE", "True"
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

// Requirement represents a required property for tests.
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
		TruthyEnv("RUN_KUBE_TESTS")(configuration)
	}
}
