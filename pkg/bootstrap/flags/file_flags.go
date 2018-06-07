package flags

import (
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/spf13/cobra"
)

func AddFileFlags(cmd *cobra.Command, opts *bootstrap.Options) {
	cmd.PersistentFlags().StringVar(&opts.FileOptions.ConfigDir, "file.config.dir", "_gloo_config", "root directory to use for storing gloo config files")
	cmd.PersistentFlags().StringVar(&opts.FileOptions.SecretDir, "file.secret.dir", "_gloo_config/secrets", "root directory to use for storing gloo secret files")
	cmd.PersistentFlags().StringVar(&opts.FileOptions.FilesDir, "file.files.dir", "_gloo_config/files", "root directory to use for storing gloo input files")
}
