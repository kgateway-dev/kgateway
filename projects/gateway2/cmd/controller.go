package cmd

import (
	"github.com/solo-io/gloo/projects/gateway2/controller"
	"github.com/spf13/cobra"
)

var cfg controller.ControllerConfig

// controllerCmd represents the controller command
var controllerCmd = &cobra.Command{
	Use:   "controller",
	Short: "Start the gateway api controller",
	Long: `Starts the gateway api controller. reads in Gateway API resources and translates them to
    Envoy xDS configuration.`,
	Run: func(cmd *cobra.Command, args []string) {
		controller.Start(cfg)
	},
}

func init() {
	rootCmd.AddCommand(controllerCmd)
	controllerCmd.Flags().StringVar(&cfg.GatewayClassName, "class-name", "gloo-gateway", "The gateway class name that we own")
	controllerCmd.Flags().StringVar(&cfg.GatewayControllerName, "controller-name", "solo.io/gloo-gateway", "The gateway class controller name")
	controllerCmd.Flags().StringVar(&cfg.Release, "release-name", "", "The release name for gateway resources")
	controllerCmd.Flags().BoolVar(&cfg.Dev, "dev-mode", false, "Turn on dev mode (more verbose logging, etc.)")
	controllerCmd.Flags().BoolVar(&cfg.AutoProvision, "auto-provision", false, "Auto provision gateway resources")
	controllerCmd.Flags().StringVar(&cfg.XdsServer, "xds-server", "", "Xds host to set on auto provisioned gateway resources")
	controllerCmd.Flags().Uint16Var(&cfg.XdsPort, "xds-port", 8080, "Xds port to set on auto provisioned gateway resources")
}
