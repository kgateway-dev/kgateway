package manifest

import (
	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
)

type Option func(properties *properties)

type properties struct {
	name     string
	manifest string

	initializedObjectsAssertion assertions.DiscreteAssertion
	finalizedObjectsAssertion   assertions.DiscreteAssertion
}

func WithName(name string) Option {
	return func(properties *properties) {
		properties.name = name
	}
}

func WithManifestFile(manifestFile string) Option {
	return func(properties *properties) {
		properties.manifest = manifestFile
	}
}

func WithInitializedObjectsAssertion(assertion assertions.DiscreteAssertion) Option {
	return func(properties *properties) {
		properties.initializedObjectsAssertion = assertion
	}
}

func WithFinalizedObjectsAssertion(assertion assertions.DiscreteAssertion) Option {
	return func(properties *properties) {
		properties.finalizedObjectsAssertion = assertion
	}
}
