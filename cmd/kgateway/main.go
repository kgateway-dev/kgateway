package main

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/setup"
	"github.com/kgateway-dev/kgateway/v2/internal/version"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/probes"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"github.com/go-logr/zapr"
	ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
	var kgatewayVersion bool
	var devMode bool

	cmd := &cobra.Command{
		Use:   "kgateway",
		Short: "Runs the kgateway controller",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Handle version flag
			if kgatewayVersion {
				fmt.Println(version.String())
				return nil
			}

			// Check if env var enables dev mode
			if os.Getenv("KGTW_DEV_MODE") == "true" {
				devMode = true
			}

			// Initialize logger
			var zapLogger *zap.Logger
			var err error
			if devMode {
				cfg := zap.NewDevelopmentConfig()
				cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel) // zap has no TraceLevel
				cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
				zapLogger, err = cfg.Build()
				fmt.Println("[Logger] Running in dev mode: text output, debug level")
			} else {
				cfg := zap.NewProductionConfig()
				cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
				zapLogger, err = cfg.Build()
			}
			if err != nil {
				return fmt.Errorf("failed to initialize logger: %w", err)
			}
			ctrl.SetLogger(zapr.NewLogger(zapLogger))

			// Start probes and controller
			probes.StartLivenessProbeServer(cmd.Context())
			s := setup.New()
			if err := s.Start(cmd.Context()); err != nil {
				return fmt.Errorf("err in main: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&kgatewayVersion, "version", "v", false, "Print the version of kgateway")
	cmd.Flags().BoolVar(&devMode, "dev", false, "Enable development logging (text output + trace level)")

	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
