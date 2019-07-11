package awscache

import "github.com/solo-io/go-utils/errors"

var (
	ListCredentialError = func(err error) error {
		return errors.Wrapf(err, "unable to list credentials")
	}

	TimeoutError = errors.New("timed out while waiting for response from aws")

	ResourceMapInitializationError = errors.New("credential resource map not initialized correctly")
)
