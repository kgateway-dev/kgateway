package prerun

import (
	"os"

	"github.com/solo-io/gloo/v2/pkg/defaults"

	"github.com/solo-io/solo-kit/pkg/utils/statusutils"

	"github.com/solo-io/gloo/v2/pkg/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
)

func SetKubeConfigEnv(opts *options.Options, cmd *cobra.Command) error {
	// If kubeconfig is set, and not equal to "", set the ENV
	if opts.Top.KubeConfig != "" {
		return os.Setenv("KUBECONFIG", opts.Top.KubeConfig)
	}
	return nil
}

func SetPodNamespaceEnv(opts *options.Options, cmd *cobra.Command) error {
	// Gloo supports having resources with statuses set by multiple controllers
	// This feature was made possible by: https://github.com/solo-io/solo-kit/pull/447
	//
	// Statuses are written by controllers responsible for keeping track of the status
	// of the resource. However, if a Kube client unmarshals a resource, and it
	// contains the deprecated format (ie a single status), it needs to know which key
	// to set in the map of statuses. This is determined by the POD_NAMESPACE env variable.
	// https://github.com/solo-io/solo-kit/blob/33fda1f5c53cd3c91298760d2f275f6b834a424d/pkg/api/v1/clients/factory/resource_client_factory.go#L90

	// This case, of a resource containing a single status, is not one we need to protect
	// against in the CLI. Therefore, we just need to ensure that the POD_NAMESPACE
	// variable is set so that the resource client can be created.
	return os.Setenv(statusutils.PodNamespaceEnvName, defaults.GlooSystem)
}
