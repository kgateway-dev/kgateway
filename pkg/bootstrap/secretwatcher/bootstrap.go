package secretwatcher

import (
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/bootstrap/secretstorage"
	"github.com/solo-io/gloo/pkg/secretwatcher"
)

func Bootstrap(opts bootstrap.Options) (secretwatcher.Interface, error) {
	client, err := secretstorage.Bootstrap(opts)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create secret storage client")
	}
	return secretwatcher.NewSecretWatcher(client)
}
