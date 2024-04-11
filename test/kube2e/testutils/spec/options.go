package spec

type Option func(properties *specProperties)

type specProperties struct {
	name     string
	manifest string
}

func WithName(name string) Option {
	return func(properties *specProperties) {
		properties.name = name
	}
}

func WithManifestFile(manifestFile string) Option {
	return func(properties *specProperties) {
		properties.manifestFiles = append(properties.manifestFiles, manifestFile)
	}
}

func WithInitializedAssertion(assertion ScenarioAssertion) Option {
	return func(properties *specProperties) {
		properties.manifestFiles = append(properties.manifestFiles, manifestFile)
	}
}

func WithAssertion(assertion ScenarioAssertion) Option {
	return func(properties *specProperties) {
		properties.manifestFiles = append(properties.manifestFiles, manifestFile)
	}
}

func WithFinalizedAssertion(assertion ScenarioAssertion) Option {
	return func(properties *specProperties) {
		properties.manifestFiles = append(properties.manifestFiles, manifestFile)
	}
}

func NewScenarioOrError(options ...Option) (Scenario, error) {

	properties := &specProperties{
		name:          "unnamed-test-scenario",
		manifestFiles: nil,
	}

	for _, opt := range options {
		opt(properties)
	}

	return nil, nil
}
