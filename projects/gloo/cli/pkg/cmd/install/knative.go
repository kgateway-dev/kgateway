package install

import (
	"fmt"
	"time"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"

	"github.com/solo-io/go-utils/errors"
	"github.com/solo-io/go-utils/kubeutils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
)

func knativeCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "knative",
		Short: "install Knative with Gloo on kubernetes",
		Long:  "requires kubectl to be installed",
		RunE: func(cmd *cobra.Command, args []string) error {

			// Get Gloo release version
			version, err := getGlooVersion(opts)
			if err != nil {
				return err
			}

			// Get location of Gloo install manifest
			manifestUri := fmt.Sprintf(constants.GlooHelmRepoTemplate, version)
			if manifestOverride := opts.Install.GlooManifestOverride; manifestOverride != "" {
				manifestUri = manifestOverride
			}

			// Pre-install step writes to k8s. Run only if this is not a dry run.
			if !opts.Install.DryRun {
				if err := preInstall(opts.Install.Namespace); err != nil {
					return errors.Wrapf(err, "pre-install failed")
				}
			}

			// Internally checks for dry run
			if err := installAdditionalKnativeResources(version, opts); err != nil {
				return err
			}

			if err := installFromUri(manifestUri, opts, constants.KnativeValuesFileName); err != nil {
				return errors.Wrapf(err, "installing knative from manifest")
			}
			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddInstallFlags(pflags, &opts.Install)
	return cmd
}

// Checks whether Knative needs to be installed or upgraded
func installAdditionalKnativeResources(glooReleaseVersion string, options *options.Options) error {
	installed, ours, err := knativeInstalled()
	if err != nil {
		return err
	}

	// It's okay to update the installation if we own it
	if !installed || ours {
		if err := downloadAndInstall(fmt.Sprintf(constants.KnativeCrdsUrlTemplate, glooReleaseVersion), options); err != nil {
			return errors.Wrapf(err, "installing knative crds from manifest")
		}
		if err := waitForKnativeCrdsRegistered(time.Second*5, time.Millisecond*500); err != nil {
			return errors.Wrapf(err, "waiting for knative crds to become registered")
		}
		if err := downloadAndInstall(fmt.Sprintf(constants.KnativeUrlTemplate, glooReleaseVersion), options); err != nil {
			return errors.Wrapf(err, "installing knative-serving from manifest")
		}
	}
	return nil
}

func knativeInstalled() (isInstalled bool, isOurs bool, err error) {
	restCfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return false, false, err
	}
	kube, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return false, false, err
	}
	namespaces, err := kube.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		return false, false, err
	}
	for _, ns := range namespaces.Items {
		if ns.Name == constants.KnativeServingNamespace {
			ours := ns.Labels != nil && ns.Labels["app"] == "gloo"
			return true, ours, nil
		}
	}
	return false, false, nil
}

// register knative crds first
func waitForKnativeCrdsRegistered(timeout, interval time.Duration) error {
	elapsed := time.Duration(0)
	for {
		select {
		case <-time.After(interval):
			if err := kubectl(nil, "get", "images.caching.internal.knative.dev"); err == nil {
				return nil
			}
			elapsed += interval
			if elapsed > timeout {
				return errors.Errorf("failed to confirm knative crd registration after %v", timeout)
			}
		}
	}
}

func downloadAndInstall(manifestUri string, opts *options.Options) error {
	manifest, err := getFileManifestBytes(manifestUri)
	if err != nil {
		return errors.Wrapf(err, "failed to download: %s", manifestUri)
	}
	return installManifest(manifest, opts)
}
