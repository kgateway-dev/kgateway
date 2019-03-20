package install

import (
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/cliutil/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
)

func UninstallGloo(opts *options.Options, cli install.KubeCli) error {
	if opts.Uninstall.DeleteNamespace || opts.Uninstall.DeleteAll {
		if err := deleteNamespace(cli, opts.Uninstall.Namespace); err != nil {
			return err
		}
	} else {
		if err := deleteGlooSystem(cli, opts.Uninstall.Namespace); err != nil {
			return err
		}
	}

	if opts.Uninstall.DeleteCrds || opts.Uninstall.DeleteAll {
		if err := deleteGlooCrds(cli); err != nil {
			return err
		}
	}

	if opts.Uninstall.DeleteAll {
		if err := deleteRbac(cli); err != nil {
			return err
		}
	}

	// TODO: remove knative crds
	return uninstallKnativeIfNecessary(cli)
}

func deleteRbac(cli install.KubeCli) error {
	for _, rbacKind := range GlooRbacKinds {
		if err := cli.Kubectl(nil, "delete", rbacKind, "-l", "gloo"); err != nil {
			return errors.Wrapf(err, "delete rbac failed")
		}
	}
	return nil
}

func deleteGlooSystem(cli install.KubeCli, namespace string) error {
	for _, kind := range GlooSystemKinds {
		if err := cli.Kubectl(nil, "delete", kind, "-l", "gloo", "-n", namespace); err != nil {
			return errors.Wrapf(err, "delete gloo system failed")
		}
	}
	return nil
}

func deleteGlooCrds(cli install.KubeCli) error {
	args := []string { "delete", "crd" }
	for _, crd := range GlooCrdNames {
		args = append(args, crd)
	}
	if err := cli.Kubectl(nil, args...); err != nil {
		return errors.Wrapf(err, "deleting crds failed")
	}
	return nil
}

func deleteNamespace(cli install.KubeCli, namespace string) error {
	if err := cli.Kubectl(nil, "delete", "namespace", namespace); err != nil {
		return errors.Wrapf(err, "delete gloo failed")
	}
	return nil
}

func uninstallKnativeIfNecessary(cli install.KubeCli) error {
	knativeExists, isOurInstall, err := install.CheckKnativeInstallation()
	if err != nil {
		return errors.Wrapf(err, "finding knative installation")
	}
	if knativeExists && isOurInstall {
		if err := cli.Kubectl(nil, "delete", "namespace", constants.KnativeServingNamespace); err != nil {
			return errors.Wrapf(err, "delete knative failed")
		}
	}
	return nil
}
