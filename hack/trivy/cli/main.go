package main

import (
	"context"
	"log"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/hack/trivy"
	"github.com/spf13/cobra"
)

// NOTE: This utility really belongs in go-utils, as it is a convenient way to run scans locally

func main() {
	ctx := context.Background()
	app := RootApp(ctx)
	if err := app.Execute(); err != nil {
		log.Fatalf("unable to run: %v\n", err)
	}
}

type options struct {
	ctx     context.Context
	version string
}

func scanVersionCommand(opts *options) *cobra.Command {
	app := &cobra.Command{
		Use:   "scan",
		Short: "runs trivy scans on images for version specified",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.version == "" {
				return eris.New("version required but not provided")
			}
			err := trivy.ScanVersion(opts.version)
			return err
		},
	}
	return app
}

// RootApp configures the CLI, including possible commands and input args.
func RootApp(ctx context.Context) *cobra.Command {
	opts := &options{
		ctx: ctx,
	}
	app := &cobra.Command{}

	// add commands
	app.AddCommand(scanVersionCommand(opts))

	// add args/flags
	app.PersistentFlags().StringVarP(&opts.version, "version", "v", "", "The version to scan")

	// mark required args
	_ = app.MarkFlagRequired("version")

	return app
}
