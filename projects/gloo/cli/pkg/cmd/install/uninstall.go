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

var subchartAppNames = []string{"glooe-grafana", "glooe-prometheus"}

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
This error may mean that you are trying to use glooctl >=0.20.14 to uninstall a version of Gloo <0.20.13 (or Enterprise Gloo <0.20.9).
Make sure you are on open source Gloo >=0.20.14 or Enterprise Gloo >=0.20.9.
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

	fmt.Printf("Removing gloo, installation ID %s...\n", installationId)

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
			// if we don't have an installation ID, attempt to delete everything with app=gloo and app=$subChartName
			for _, appName := range append(subchartAppNames, "gloo") {
				err = cli.Kubectl(nil, "delete", kind, "-l", fmt.Sprintf("app=%s", appName), "-n", namespace)
				if err != nil {
					break
				}
			}
		} else {
			// otherwise, delete everything with both the label app=gloo and installationId=$installationId (as well as subchart resources)
			glooComponentLabelValue := fmt.Sprintf("app=gloo,installationId=%s", installationId)
			err = cli.Kubectl(nil, "delete", kind, "-l", glooComponentLabelValue, "-n", namespace)
			if err != nil {
				failedComponents += kind + " "
				continue
			}

			for _, appName := range subchartAppNames {
				err = cli.Kubectl(nil, "delete", kind, "-l", fmt.Sprintf("app=%s", appName), "-n", namespace)
				if err != nil {
					break
				}
			}
		}

		if err != nil {
			failedComponents += kind + " "
			continue
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
