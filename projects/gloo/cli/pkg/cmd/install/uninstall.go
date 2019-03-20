package install

import (
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/cliutil/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
)

func uninstallGloo(opts *options.Options) error {
	if err := deleteNamespace(opts.Uninstall.Namespace); err != nil {
		return err
	}
	return uninstallKnativeIfNecessary()
}

func delete

func deleteNamespace(namespace string) error {
	if err := install.Kubectl(nil, "delete", "namespace", namespace); err != nil {
		return errors.Wrapf(err, "delete gloo failed")
	}
	return nil
}

func uninstallKnativeIfNecessary() error {
	knativeExists, isOurInstall, err := install.CheckKnativeInstallation()
	if err != nil {
		return errors.Wrapf(err, "finding knative installation")
	}
	if knativeExists && isOurInstall {
		if err := install.Kubectl(nil, "delete", "namespace", constants.KnativeServingNamespace); err != nil {
			return errors.Wrapf(err, "delete knative failed")
		}
	}
	return nil
}
