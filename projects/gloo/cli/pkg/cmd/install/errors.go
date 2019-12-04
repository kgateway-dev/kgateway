package install

import "github.com/solo-io/go-utils/errors"

var (
	GlooAlreadyInstalled = func(namespace string) error {
		return errors.Errorf("Gloo has already been installed to namespace %s", namespace)
	}
)
