package install

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/errors"
	"github.com/spf13/cobra"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

const (
	GlooEHelmRepoTemplate = "https://storage.googleapis.com/gloo-ee-helm/charts/gloo-ee-%s.tgz"
)

func enterpriseCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "enterprise",
		Short:  "install the Gloo Enterprise Gateway on Kubernetes",
		Long:   "requires kubectl to be installed",
		PreRun: setVerboseMode(opts),
		RunE: func(cmd *cobra.Command, args []string) error {

			extraValues := map[string]interface{}{
				"license_key": opts.Install.LicenseKey,
				"gloo": map[string]interface{}{
					"namespace": map[string]interface{}{
						"create": "true",
					},
				},
			}

			if err := Install(&opts.Install, extraValues, true); err != nil {
				return errors.Wrapf(err, "installing Gloo Enterprise in gateway mode")
			}

			return nil
		},
	}

	pFlags := cmd.PersistentFlags()
	flagutils.AddEnterpriseInstallFlags(pFlags, &opts.Install)
	return cmd
}

//const PersistentVolumeClaim = "PersistentVolumeClaim"

// TODO(helm3): Since we rely on helm and not on a simple `kubectl apply` this check should be redundant. Still worth verifying.
//func pvcExists(namespace string) install.ResourceMatcherFunc {
//	return func(resource install.ResourceType) (bool, error) {
//		kubeClient, err := helpers.KubeClient()
//		if err != nil {
//			return false, err
//		}
//
//		// If this is a PVC, check if it already exists. If so, exclude this resource from the manifest.
//		// We don't want to overwrite existing PVCs.
//		if resource.TypeMeta.Kind == PersistentVolumeClaim {
//
//			_, err := kubeClient.CoreV1().PersistentVolumeClaims(namespace).Get(resource.Metadata.Name, v1.GetOptions{})
//			if err != nil {
//				if !kubeerrors.IsNotFound(err) {
//					return false, errors.Wrapf(err, "retrieving %s: %s.%s", PersistentVolumeClaim, namespace, resource.Metadata.Name)
//				}
//			} else {
//				// The PVC exists, exclude it from manifest
//				return true, nil
//			}
//		}
//		return false, nil
//	}
//}
