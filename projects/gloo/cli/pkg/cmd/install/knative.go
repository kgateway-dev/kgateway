package install

import (
	"fmt"

	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/go-utils/errors"
	"github.com/solo-io/go-utils/kubeutils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
)

const (
	knativeUrlTemplate     = "https://github.com/solo-io/gloo/releases/download/v%s/knative-no-istio-0.3.0.yaml"
	glooKnativeUrlTemplate = "https://github.com/solo-io/gloo/releases/download/v%s/gloo-knative.yaml"
)

func KnativeCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "knative",
		Short: "install Knative with Gloo on kubernetes",
		Long:  "requires kubectl to be installed",
		RunE: func(cmd *cobra.Command, args []string) error {
			installed, err := knativeInstalled()
			if err != nil {
				return err
			}

			if !installed {
				knativeManifestBytes, err := readKnativeManifest(opts, knativeUrlTemplate)
				if err != nil {
					return errors.Wrapf(err, "reading knative manifest")
				}
				if opts.Install.DryRun {
					fmt.Printf("%s", knativeManifestBytes)
				} else {
					if err := applyManifest(knativeManifestBytes); err != nil {
						return err
					}
				}
			}

			if err := createImagePullSecretIfNeeded(opts.Install); err != nil {
				return errors.Wrapf(err, "creating image pull secret")
			}

			glooKnativeManifestBytes, err := readGlooManifest(opts, glooKnativeUrlTemplate)
			if err != nil {
				return errors.Wrapf(err, "reading gloo knative manifest")
			}

			if opts.Install.DryRun {
				fmt.Printf("%s", glooKnativeManifestBytes)
				return nil
			}
			return applyManifest(glooKnativeManifestBytes)
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddInstallFlags(pflags, &opts.Install)
	return cmd
}

func readKnativeManifest(opts *options.Options, urlTemplate string) ([]byte, error) {
	if opts.Install.KnativeManifest != "" {
		return readManifestFromFile(opts.Install.KnativeManifest)
	}
	if version.Version == version.UndefinedVersion || version.Version == version.DevVersion {
		return nil, errors.Errorf("You must provide a file containing the knative manifest when running an unreleased version of glooctl.")
	}
	return readManifest(version.Version, urlTemplate)
}

const knativeServingNamespace = "knative-serving"

func knativeInstalled() (bool, error) {
	restCfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return false, err
	}
	kube, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return false, err
	}
	namespaces, err := kube.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	for _, ns := range namespaces.Items {
		if ns.Name == knativeServingNamespace {
			return true, nil
		}
	}
	return false, nil
}
