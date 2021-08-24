package postrun

import (
	"os"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
)

func UnsetPodNamespaceEnv(opts *options.Options, cmd *cobra.Command) error {
	return os.Unsetenv("POD_NAMESPACE")
}
