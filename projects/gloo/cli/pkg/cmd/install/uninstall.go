package install

import (
	"fmt"
	"io"
	"os"

	"github.com/solo-io/gloo/pkg/cliutil"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	"github.com/solo-io/gloo/pkg/cliutil/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
)

func UninstallGloo(opts *options.Options, cli install.KubeCli) error {
	uninstaller := NewUninstaller(DefaultHelmClient(), cli)
	if err := uninstaller.Uninstall(opts); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Uninstall failed. Detailed logs available at %s.\n", cliutil.GetLogsPath())
		return err
	}
	return nil
}

type Uninstaller interface {
	Uninstall(cliArgs *options.Options) error
}

type uninstaller struct {
	helmClient HelmClient
	kubeCli    install.KubeCli
	output     io.Writer
}

func NewUninstaller(helmClient HelmClient, kubeCli install.KubeCli) Uninstaller {
	return NewUninstallerWithOutput(helmClient, kubeCli, os.Stdout)
}

// visible for testing
func NewUninstallerWithOutput(helmClient HelmClient, kubeCli install.KubeCli, output io.Writer) Uninstaller {
	return &uninstaller{
		helmClient: helmClient,
		kubeCli:    kubeCli,
		output:     output,
	}
}

func (u *uninstaller) Uninstall(cliArgs *options.Options) error {
	namespace := cliArgs.Uninstall.Namespace
	releaseName := cliArgs.Uninstall.HelmReleaseName

	// Check whether Helm release object exists
	releaseExists, err := u.helmClient.ReleaseExists(namespace, releaseName)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(u.output, "Removing Gloo system components from namespace %s...\n", namespace)
	if releaseExists {

		// If the release object exists, then we want to delegate the uninstall to the Helm libraries.
		uninstallAction, err := u.helmClient.NewUninstall(namespace)
		if err != nil {
			return err
		}

		if cliArgs.Uninstall.DeleteCrds || cliArgs.Uninstall.DeleteAll {
			// Helm never deletes CRDs, so we collect the CRD names to delete them ourselves if need be.
			// We need to run this first, as it depends on the release still being present.
			crdNames, err := u.findCrdNamesForRelease(namespace)
			if err != nil {
				return err
			}

			u.deleteGlooCrds(crdNames)
		}

		if _, err = uninstallAction.Run(releaseName); err != nil {
			return err
		}

	} else {

		// The release object does not exist, so it is not possible to exactly tell which resources are part of
		// the originals installation. We take a best effort approach.

		glooLabels := LabelsToFlagString(GlooComponentLabels)
		for _, kind := range GlooNamespacedKinds {
			if err := u.kubeCli.Kubectl(nil, "delete", kind, "-n", namespace, "-l", glooLabels); err != nil {
				return err
			}
		}

		// If the `--all` flag was provided, also delete the cluster-scoped resources.
		if cliArgs.Uninstall.DeleteAll {
			for _, kind := range GlooClusterScopedKinds {
				if err := u.kubeCli.Kubectl(nil, "delete", kind, "-l", glooLabels); err != nil {
					return err
				}
			}
		}
	}

	u.uninstallKnativeIfNecessary()

	// delete our hard-coded crd names even if releaseExists because helm chart for glooe doesn't show gloo dependency (https://github.com/helm/helm/issues/7847)
	if cliArgs.Uninstall.DeleteCrds || cliArgs.Uninstall.DeleteAll {
		u.deleteGlooCrds(GlooCrdNames)
	}

	if cliArgs.Uninstall.DeleteNamespace || cliArgs.Uninstall.DeleteAll {
		u.deleteNamespace(cliArgs.Uninstall.Namespace)
	}

	return nil
}

// Note: will not find CRDs of dependencies due to confusing but intended helm behavior (https://github.com/helm/helm/issues/7847)
func (u *uninstaller) findCrdNamesForRelease(namespace string) (crdNames []string, err error) {
	lister, err := u.helmClient.ReleaseList(namespace)
	if err != nil {
		return nil, err
	}
	releases, err := lister.Run()
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, NoReleaseForCRDs
	} else if len(releases) > 1 {
		return nil, MultipleReleasesForCRDs
	}

	rel := releases[0]
	for _, crd := range rel.Chart.CRDs() {
		resource, err := makeUnstructured(string(crd.Data))
		if err != nil {
			return nil, err
		}

		crdNames = append(crdNames, resource.GetName())
	}

	return crdNames, nil
}

// expects the Helm release to still be present
func (u *uninstaller) deleteGlooCrds(crdNames []string) {
	if len(crdNames) == 0 {
		return
	}

	_, _ = fmt.Fprintf(u.output, "Removing Gloo CRDs...\n")
	args := []string{"delete", "crd"}
	for _, crdName := range crdNames {
		args = append(args, crdName)
	}
	if err := u.kubeCli.Kubectl(nil, args...); err != nil {
		_, _ = fmt.Fprintf(u.output, "Unable to delete Gloo CRDs. Continuing...\n")
	}
}

func (u *uninstaller) deleteNamespace(namespace string) {
	_, _ = fmt.Fprintf(u.output, "Removing namespace %s... ", namespace)
	if err := u.kubeCli.Kubectl(nil, "delete", "namespace", namespace); err != nil {
		_, _ = fmt.Fprintf(u.output, "\nUnable to delete namespace %s. Continuing...\n", namespace)
	} else {
		_, _ = fmt.Fprintf(u.output, "Done.\n")
	}
}

func makeUnstructured(manifest string) (*unstructured.Unstructured, error) {
	jsn, err := yaml.YAMLToJSON([]byte(manifest))
	if err != nil {
		return nil, err
	}
	runtimeObj, err := runtime.Decode(unstructured.UnstructuredJSONScheme, jsn)
	if err != nil {
		return nil, err
	}
	return runtimeObj.(*unstructured.Unstructured), nil
}

func (u *uninstaller) uninstallKnativeIfNecessary() {
	_, installOpts, err := checkKnativeInstallation()
	if err != nil {
		_, _ = fmt.Fprintf(u.output, "Finding knative installation\n")
		return
	}
	if installOpts != nil {
		_, _ = fmt.Fprintf(u.output, "Removing knative components installed by Gloo %#v...\n", installOpts)
		manifests, err := RenderKnativeManifests(*installOpts)
		if err != nil {
			_, _ = fmt.Fprintf(u.output, "Could not determine which knative components to remove. Continuing...\n")
			return
		}
		if err := install.KubectlDelete([]byte(manifests), "--ignore-not-found"); err != nil {
			_, _ = fmt.Fprintf(u.output, "Unable to delete knative. Continuing...\n")
		}
	}
}
