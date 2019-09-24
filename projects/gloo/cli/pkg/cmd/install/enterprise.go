package install

import (
	"fmt"
	"os"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/pkg/cliutil/install"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/go-utils/errors"
	"github.com/spf13/cobra"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

const (
	GlooEHelmRepoTemplate = "https://storage.googleapis.com/gloo-ee-helm/charts/gloo-ee-%s.tgz"
)

func enterpriseCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "enterprise",
		Short:  "install the Gloo Enterprise Gateway on kubernetes",
		Long:   "requires kubectl to be installed",
		PreRun: setVerboseMode(opts),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := installGlooE(opts); err != nil {
				return errors.Wrapf(err, "installing gloo enterprise in gateway mode")
			}
			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddEnterpriseInstallFlags(pflags, &opts.Install)
	return cmd
}

func installGlooE(opts *options.Options) error {
	if !opts.Install.DryRun {
		fmt.Printf("Starting Gloo Enterprise installation...\n")
	}
	spec, err := GetEnterpriseInstallSpec(opts)
	if err != nil {
		return err
	}

	kubeInstallClient := NamespacedGlooKubeInstallClient{
		Namespace: opts.Install.Namespace,
		Delegate:  &DefaultGlooKubeInstallClient{},
		Executor:  install.Kubectl,
	}
	if err := InstallGloo(opts, *spec, &kubeInstallClient); err != nil {
		fmt.Fprintf(os.Stderr, "\nGloo failed to install! Detailed logs available at %s.\n", cliutil.GetLogsPath())
		return err
	}
	if !opts.Install.DryRun {
		fmt.Printf("\nGloo Enterprise was successfully installed!\n")
	}
	return nil
}

// enterprise
func GetEnterpriseInstallSpec(opts *options.Options) (*GlooInstallSpec, error) {
	glooEVersion := version.EnterpriseTag

	// Get location of Gloo helm chart
	helmChartArchiveUri := fmt.Sprintf(GlooEHelmRepoTemplate, glooEVersion)
	if helmChartOverride := opts.Install.HelmChartOverride; helmChartOverride != "" {
		helmChartArchiveUri = helmChartOverride
	}

	extraValues := map[string]interface{}{
		"license_key": opts.Install.LicenseKey,
	}

	if opts.Install.Upgrade {
		extraValues["gloo"] = map[string]interface{}{
			"gateway": map[string]interface{}{
				"upgrade": "true",
			},
		}
	} else {
		extraValues["gloo"] = map[string]interface{}{
			"namespace": map[string]interface{}{
				"create": "true",
			},
		}
	}

	return &GlooInstallSpec{
		HelmArchiveUri:    helmChartArchiveUri,
		ProductName:       "glooe",
		ValueFileName:     "",
		ExtraValues:       extraValues,
		ExcludeResources:  pvcExists(opts.Install.Namespace),
		UserValueFileName: opts.Install.HelmChartValues,
	}, nil
}

const PersistentVolumeClaim = "PersistentVolumeClaim"

func pvcExists(namespace string) install.ResourceMatcherFunc {
	return func(resource install.ResourceType) (bool, error) {
		kubeClient, err := helpers.KubeClient()
		if err != nil {
			return false, err
		}

		// If this is a PVC, check if it already exists. If so, exclude this resource from the manifest.
		// We don't want to overwrite existing PVCs.
		if resource.TypeMeta.Kind == PersistentVolumeClaim {

			_, err := kubeClient.CoreV1().PersistentVolumeClaims(namespace).Get(resource.Metadata.Name, v1.GetOptions{})
			if err != nil {
				if !kubeerrors.IsNotFound(err) {
					return false, errors.Wrapf(err, "retrieving %s: %s.%s", PersistentVolumeClaim, namespace, resource.Metadata.Name)
				}
			} else {
				// The PVC exists, exclude it from manifest
				return true, nil
			}
		}
		return false, nil
	}
}
