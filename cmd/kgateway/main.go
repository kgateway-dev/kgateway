package main

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/setup"
	"github.com/kgateway-dev/kgateway/v2/internal/version"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/probes"
)

func main() {
	var (
		kgatewayVersion bool
		devMode         bool
	)

	cmd := &cobra.Command{
		Use:   "kgateway",
		Short: "Runs the kgateway controller",
		RunE: func(cmd *cobra.Command, args []string) error {
			if kgatewayVersion {
				fmt.Println(version.String())
				return nil
			}

			// Initialize logging based on --dev flag
			logging.InitLogger(devMode)

			probes.StartLivenessProbeServer(cmd.Context())
			s := setup.New()
			if err := s.Start(cmd.Context()); err != nil {
				return fmt.Errorf("err in main: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&kgatewayVersion, "version", "v", false, "Print the version of kgateway")
	cmd.Flags().BoolVar(&devMode, "dev", false, "Enable developer mode logging (text format + debug level)")

	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
