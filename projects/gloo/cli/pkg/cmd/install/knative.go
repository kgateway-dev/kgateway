package install

import (
	"time"

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
	const (
		knativeUrlTemplate     = "https://github.com/solo-io/gloo/releases/download/v%s/knative-no-istio-0.3.0.yaml"
		knativeCrdsUrlTemplate = "https://github.com/solo-io/gloo/releases/download/v%s/knative-crds-0.3.0.yaml"
		glooKnativeUrlTemplate = "https://github.com/solo-io/gloo/releases/download/v%s/gloo-knative.yaml"
	)
	cmd := &cobra.Command{
		Use:   "knative",
		Short: "install Knative with Gloo on kubernetes",
		Long:  "requires kubectl to be installed",
		RunE: func(cmd *cobra.Command, args []string) error {
			installed, ours, err := knativeInstalled()
			if err != nil {
				return err
			}

			// it's okay to update the installation if we own it
			if !installed || ours {
				if err := installFromUri(opts, opts.Install.Knative.CrdManifestOverride, knativeCrdsUrlTemplate, nil); err != nil {
					return errors.Wrapf(err, "installing knative crds from manifest")
				}
				if err := waitForKnativeCrdsRegistered(time.Second*5, time.Millisecond*500); err != nil {
					return errors.Wrapf(err, "waiting for knative crds to become registered")
				}
				if err := installFromUri(opts, opts.Install.Knative.InstallManifestOverride, knativeUrlTemplate, nil); err != nil {
					return errors.Wrapf(err, "installing knative-serving from manifest")
				}
			}

			if err := preInstall(opts.Install.Namespace); err != nil {
				return errors.Wrapf(err, "pre-install failed")
			}
			if err := installFromUri(opts, opts.Install.GlooManifestOverride, glooKnativeUrlTemplate, nil); err != nil {
				return errors.Wrapf(err, "installing ingress from manifest")
			}
			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddInstallFlags(pflags, &opts.Install)
	flagutils.AddKnativeInstallFlags(pflags, &opts.Install.Knative)
	return cmd
}

const knativeServingNamespace = "knative-serving"

func knativeInstalled() (bool, bool, error) {
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
		if ns.Name == knativeServingNamespace {
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
