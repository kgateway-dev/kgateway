package spec

type Option func(properties *specProperties)

type specProperties struct {
	name          string
	manifestFiles []string
}

func WithManifestFile(manifestFile string) Option {
	return func(properties *specProperties) {
		properties.manifestFiles = append(properties.manifestFiles, manifestFile)
	}
}

func NewSpecOrError(options ...Option) (Scenario, error) {

	properties := &specProperties{
		name:          "",
		manifestFiles: nil,
	}

	for _, opt := range options {
		opt(properties)
	}

	return nil, nil
}
