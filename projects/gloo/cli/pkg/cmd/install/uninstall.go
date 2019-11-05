package install

import (
	"fmt"
	"os"
	"strings"

	"github.com/solo-io/go-utils/errors"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/pkg/cliutil/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
)

const installationIdLabel = "installationId"

func UninstallGloo(opts *options.Options, cli install.KubeCli) error {
	if err := uninstallGloo(opts, cli); err != nil {
		fmt.Fprintf(os.Stderr, "Uninstall failed. Detailed logs available at %s.\n", cliutil.GetLogsPath())
		return err
	}
	return nil
}

func uninstallGloo(opts *options.Options, cli install.KubeCli) error {
	// attempt to uninstall by deleting resources with the label containing this installation ID
	installationId := findInstallationId(opts, cli)
	if installationId == "" && !opts.Uninstall.Force {
		return errors.New(`Could not find installation ID in 'gloo' pod labels. Use --force to uninstall anyway.
Note that using --force may delete cluster-scoped resources belonging to some other installation of Gloo...
This error may mean that the version of glooctl you are using is newer than the version of Gloo running in-cluster.
`)
	}

	if opts.Uninstall.DeleteNamespace || opts.Uninstall.DeleteAll {
		deleteNamespace(cli, opts.Uninstall.Namespace)
	} else {
		deleteGlooSystem(cli, opts.Uninstall.Namespace, installationId)
	}

	if opts.Uninstall.DeleteCrds || opts.Uninstall.DeleteAll {
		deleteGlooCrds(cli)
	}

	if opts.Uninstall.DeleteAll {
		deleteRbac(cli, installationId)
	}

	uninstallKnativeIfNecessary()

	return nil
}

// attempt to read the installation id off of the gloo pod labels
func findInstallationId(opts *options.Options, cli install.KubeCli) string {
	jsonPath := fmt.Sprintf("-ojsonpath='{.items[0].metadata.labels.%s}'", installationIdLabel)
	installationId, err := cli.KubectlOut(nil, "-n", opts.Uninstall.Namespace, "get", "pod", "-l", "gloo=gloo", jsonPath)
	if err != nil {
		return ""
	}

	fmt.Printf("Removing gloo installation ID %s...\n", installationId)

	// the jsonpath formatting will leave single-quotes at the beginning and end of the installation ID. Strip them out before returning the value
	return strings.Replace(string(installationId), "'", "", -1)
}

func deleteRbac(cli install.KubeCli, installationId string) {
	fmt.Printf("Removing Gloo RBAC configuration...\n")
	failedRbacs := ""
	for _, rbacKind := range GlooRbacKinds {
		var err error
		if installationId == "" {
			err = cli.Kubectl(nil, "delete", rbacKind, "-l", "app=gloo")
		} else {
			labelValue := fmt.Sprintf("installationId=%s", installationId)
			err = cli.Kubectl(nil, "delete", rbacKind, "-l", labelValue)
		}

		if err != nil {
			failedRbacs += rbacKind + " "
		}
	}
	if len(failedRbacs) > 0 {
		fmt.Printf("Unable to delete Gloo RBACs: %s. Continuing...\n", failedRbacs)
	}
}

func deleteGlooSystem(cli install.KubeCli, namespace, installationId string) {
	fmt.Printf("Removing Gloo system components from namespace %s...\n", namespace)
	failedComponents := ""
	for _, kind := range GlooSystemKinds {
		var err error
		if installationId == "" {
			for _, appName := range []string{"gloo", "glooe-grafana", "glooe-prometheus"} {
				err = cli.Kubectl(nil, "delete", kind, "-l", fmt.Sprintf("app=%s", appName), "-n", namespace)
			}
		} else {
			labelValue := fmt.Sprintf("installationId=%s", installationId)
			err = cli.Kubectl(nil, "delete", kind, "-l", labelValue, "-n", namespace)
		}

		if err != nil {
			failedComponents += kind + " "
		}
	}
	if len(failedComponents) > 0 {
		fmt.Printf("Unable to delete gloo system components: %s. Continuing...\n", failedComponents)
	}
}

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
