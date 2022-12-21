package helpers

import (
	"fmt"
	"runtime"

	"k8s.io/apimachinery/pkg/util/sets"
)

type RequiredConfiguration struct {
	supportedOS sets.String // empty is considered all
}

func (r RequiredConfiguration) Validate() error {
	if len(r.supportedOS) > 0 {
		if !r.supportedOS.Has(runtime.GOOS) {
			return fmt.Errorf("runtime os (%s), is not in supported set (%+v)", runtime.GOOS, r.supportedOS)
		}
	}

	return nil
}

func DoValidate(requirements []Requirement) error {
	// default
	requiredConfiguration := &RequiredConfiguration{
		supportedOS: sets.NewString(),
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
