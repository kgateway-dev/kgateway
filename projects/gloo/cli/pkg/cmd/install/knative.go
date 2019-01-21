package install

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

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

			imageVersion := opts.Install.Version
			if imageVersion == "" {
				imageVersion = version.Version
			}

			if !installed {
				knativeManifestBytes, err := readKnativeManifest(imageVersion)
				if err != nil {
					return errors.Wrapf(err, "reading knative manifest")
				}
				kubectl := exec.Command("kubectl", "apply", "-f", "-")
				kubectl.Stdin = bytes.NewBuffer(knativeManifestBytes)
				kubectl.Stdout = os.Stdout
				kubectl.Stderr = os.Stderr
				if err := kubectl.Run(); err != nil {
					return err
				}
			}

			if err := createImagePullSecretIfNeeded(opts.Install); err != nil {
				return errors.Wrapf(err, "creating image pull secret")
			}

			glooKnativeManifestBytes, err := readGlooKnativeManifest(imageVersion)
			if err != nil {
				return errors.Wrapf(err, "reading gloo knative manifest")
			}

			if opts.Install.DryRun {
				fmt.Printf("%s", glooKnativeManifestBytes)
				return nil
			}
			return applyManifest(glooKnativeManifestBytes, imageVersion)
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddInstallFlags(pflags, &opts.Install)
	return cmd
}

func readKnativeManifest(version string) ([]byte, error) {
	urlTemplate := "https://github.com/solo-io/gloo/releases/download/v%s/knative-no-istio-0.3.0.yaml"
	return ReadManifest(version, urlTemplate)
}

func readGlooKnativeManifest(version string) ([]byte, error) {
	urlTemplate := "https://github.com/solo-io/gloo/releases/download/v%s/gloo-knative.yaml"
	return ReadManifest(version, urlTemplate)
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
