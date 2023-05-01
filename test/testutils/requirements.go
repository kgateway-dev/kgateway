package testutils

import (
	"fmt"
	"os"
	"runtime"

	"github.com/onsi/ginkgo"

	"github.com/hashicorp/go-multierror"
	"k8s.io/apimachinery/pkg/util/sets"
)

// ValidateRequirementsAndNotifyGinkgo validates that the provided Requirements are met, and if they are not, uses
// the InvalidTestReqsEnvVar to determine how to proceed:
// Options are:
//   - `run`: Ignore any invalid requirements and execute the tests
//   - `skip`: Notify Ginkgo that the current spec was skipped
//   - `fail`: Notify Ginkgo that the current spec has failed [DEFAULT]
func ValidateRequirementsAndNotifyGinkgo(requirements ...Requirement) {
	err := ValidateRequirements(requirements)
	if err == nil {
		return
	}
	message := fmt.Sprintf("Test requirements not met: %v \n\n Consider using %s=skip to skip these tests", err, InvalidTestReqsEnvVar)
	switch os.Getenv(InvalidTestReqsEnvVar) {
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
		reasons:       map[string]string{},
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

	// User defined reasons for why particular environmental conditions are required
	reasons map[string]string
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

	// If there are no errors, return
	if errs.ErrorOrNil() == nil {
		return nil
	}

	// If there are reasons defined, include them in the error message
	if len(r.reasons) > 0 {
		errs = multierror.Append(
			errs,
			fmt.Errorf("user defined reasons: %+v", r.reasons))
	}

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
		if !IsEnvTruthy(env) {
			return fmt.Errorf("env (%s) needs to be truthy", env)
		}
	}
	return nil
}

// Requirement represents a required property for tests.
type Requirement func(configuration *RequiredConfiguration)

// TruthyEnv returns a Requirement that expects tests to have the injected environment variable set to a truthy value
func TruthyEnv(env string) Requirement {
	return func(configuration *RequiredConfiguration) {
		configuration.truthyEnvVar = append(configuration.truthyEnvVar, env)
	}
}

// Kubernetes returns a Requirement that expects tests to require Kubernetes configuration
func Kubernetes(reason string) Requirement {
	return func(configuration *RequiredConfiguration) {
		configuration.reasons["kubernetes"] = reason
		TruthyEnv(RunKubeTests)(configuration)
	}
}
