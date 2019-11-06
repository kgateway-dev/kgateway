package install

import "github.com/solo-io/go-utils/errors"

var (
	FailedToFindLabel = func(err error) error {
		return errors.Wrapf(err, "kubectl failed to pull %s label from gloo pod", installationIdLabel)
	}
	LabelNotSet                   = errors.Errorf("%s label has no value on gloo pod", installationIdLabel)
	CantUninstallWithoutInstallId = func(err error) error {
		return errors.Wrapf(err, `Could not find installation ID in 'gloo' pod labels. Use --force to uninstall anyway.
Note that using --force may delete cluster-scoped resources belonging to some other installation of Gloo...
This error may mean that you are trying to use glooctl >=0.20.14 to uninstall a version of Gloo <0.20.13 (or Enterprise Gloo <0.20.9).
Make sure you are on open source Gloo >=0.20.14 or Enterprise Gloo >=0.20.9.
`)
	}
)
