package secretwatcher

import "github.com/solo-io/gloo/pkg/storage/dependencies"

type SecretMap map[string]*dependencies.Secret

// Interface is responsible for watching secrets referenced by a config
type Interface interface {
	Run(<-chan struct{})

	TrackSecrets(secretRefs []string)

	// secrets are pushed here whenever they are read
	Secrets() <-chan SecretMap

	// should show valid if the most recent update passed, otherwise a useful error
	Error() <-chan error
}
