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
	controllerCmd.Flags().StringVarP(&cfg.GatewayClassName, "class-name", "c", "solo-gateway", "The gateway class name that we own")
}
