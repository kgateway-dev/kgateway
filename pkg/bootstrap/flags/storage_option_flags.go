package flags

import (
	"fmt"
	"strings"
	"time"

	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/spf13/cobra"
)

func AddConfigStorageOptionFlags(cmd *cobra.Command, opts *bootstrap.Options) {
	cmd.PersistentFlags().StringVar(&opts.ConfigStorageOptions.Type, "storage.type", bootstrap.WatcherTypeFile, fmt.Sprintf("storage backend for config objects. supported: [%s]", strings.Join(bootstrap.SupportedCwTypes, " | ")))
	cmd.PersistentFlags().DurationVar(&opts.ConfigStorageOptions.SyncFrequency, "storage.refreshrate", time.Second, "refresh rate for polling config")
}

func AddSecretStorageOptionFlags(cmd *cobra.Command, opts *bootstrap.Options) {
	cmd.PersistentFlags().StringVar(&opts.SecretStorageOptions.Type, "secrets.type", bootstrap.WatcherTypeFile, fmt.Sprintf("storage backend for secrets. supported: [%s]", strings.Join(bootstrap.SupportedSwTypes, " | ")))
	cmd.PersistentFlags().DurationVar(&opts.SecretStorageOptions.SyncFrequency, "secrets.refreshrate", time.Second, "refresh rate for polling secrets")
}

func AddFileStorageOptionFlags(cmd *cobra.Command, opts *bootstrap.Options) {
	cmd.PersistentFlags().StringVar(&opts.FileStorageOptions.Type, "filewatcher.type", bootstrap.WatcherTypeFile, fmt.Sprintf("storage backend for raw files. supported: [%s]", strings.Join(bootstrap.SupportedFwTypes, " | ")))
	cmd.PersistentFlags().DurationVar(&opts.FileStorageOptions.SyncFrequency, "filewatcher.refreshrate", time.Second, "refresh rate for polling config")
}
