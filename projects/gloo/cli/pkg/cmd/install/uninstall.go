package install

import (
	"fmt"
	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/pkg/cliutil/helm"
	"github.com/solo-io/gloo/pkg/cliutil/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"os"
)

func UninstallGloo(opts *options.Options, cli install.KubeCli) error {
	if err := uninstallGloo(opts, cli); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Uninstall failed. Detailed logs available at %s.\n", cliutil.GetLogsPath())
		return err
	}
	return nil
}

func uninstallGloo(opts *options.Options, cli install.KubeCli) error {

	if releaseExists, err := ReleaseExists(opts.Uninstall.Namespace); err != nil {
		return err
	} else if !releaseExists {
		fmt.Printf("No Gloo installation found in namespace %s\n", opts.Uninstall.Namespace)
		return nil
	}

	uninstallAction, err := helm.NewUninstall(opts.Uninstall.Namespace)
	if err != nil {
		return err
	}

	fmt.Printf("Removing Gloo system components from namespace %s...\n", opts.Uninstall.Namespace)
	if _, err = uninstallAction.Run(constants.GlooReleaseName); err != nil {
		return err
	}

	if opts.Uninstall.DeleteNamespace || opts.Uninstall.DeleteAll {
		deleteNamespace(cli, opts.Uninstall.Namespace)
	}

	if opts.Uninstall.DeleteCrds || opts.Uninstall.DeleteAll {
		deleteGlooCrds(cli)
	}

	uninstallKnativeIfNecessary()

	return nil
}

// TODO(helm3): would be better to get the CRDs from the release object that we get back from uninstall
func deleteGlooCrds(cli install.KubeCli) {
	fmt.Printf("Removing Gloo CRDs...\n")
	args := []string{"delete", "crd"}
	for _, crd := range GlooCrdNames {
		args = append(args, crd)
	}
	if err := cli.Kubectl(nil, args...); err != nil {
		fmt.Printf("Unable to delete Gloo CRDs. Continuing...\n")
	}
}

func deleteNamespace(cli install.KubeCli, namespace string) {
	fmt.Printf("Removing namespace %s...\n", namespace)
	if err := cli.Kubectl(nil, "delete", "namespace", namespace); err != nil {
		fmt.Printf("Unable to delete namespace %s. Continuing...\n", namespace)
	}
}

func uninstallKnativeIfNecessary() {
	_, installOpts, err := checkKnativeInstallation()
	if err != nil {
		fmt.Printf("Finding knative installation\n")
		return
	}
	if installOpts != nil {
		fmt.Printf("Removing knative components installed by Gloo %#v...\n", installOpts)
		manifests, err := RenderKnativeManifests(*installOpts)
		if err != nil {
			fmt.Printf("Could not determine which knative components to remove. Continuing...\n")
			return
		}
		if err := install.KubectlDelete([]byte(manifests), "--ignore-not-found"); err != nil {
			fmt.Printf("Unable to delete knative. Continuing...\n")
		}
	}
}
