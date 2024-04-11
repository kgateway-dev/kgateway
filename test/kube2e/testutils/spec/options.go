package spec

type Option func(properties *specProperties)

type specProperties struct {
	name                 string
	manifest             string
	initializedAssertion ScenarioAssertion
	assertion            ScenarioAssertion
	finalizedAssertion   ScenarioAssertion
}

func WithName(name string) Option {
	return func(properties *specProperties) {
		properties.name = name
	}
}

func WithManifestFile(manifestFile string) Option {
	return func(properties *specProperties) {
		properties.manifest = manifestFile
	}
}

func WithInitializedAssertion(assertion ScenarioAssertion) Option {
	return func(properties *specProperties) {
		properties.initializedAssertion = assertion
	}
}

func WithAssertion(assertion ScenarioAssertion) Option {
	return func(properties *specProperties) {
		properties.assertion = assertion
	}
}

func WithFinalizedAssertion(assertion ScenarioAssertion) Option {
	return func(properties *specProperties) {
		properties.finalizedAssertion = assertion
	}
}
