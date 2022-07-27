package bootstrap

import (
	"flag"
	"os"

	"github.com/solo-io/gloo/pkg/defaults"
)

var (
	setupNamespace string
	setupName      string
	setupDir       string
)

// TODO (ilackarms): move to a flags package
func init() {

	// Allow for more dynamic setting of settings namespace
	// Based on article https://kubernetes.io/docs/tasks/inject-data-application/environment-variable-expose-pod-information/#the-downward-api
	defaultNamespace := os.Getenv(defaults.PodNamespace)
	if defaultNamespace == "" {
		defaultNamespace = defaults.GlooSystem
	}
	flag.StringVar(&setupNamespace, "namespace", defaultNamespace, "namespace to watch for settings crd/file")
	flag.StringVar(&setupName, "name", defaults.SettingsName, "name of settings crd/file to use")
	flag.StringVar(&setupDir, "dir", "",
		"directory to find bootstrap settings if not using kubernetes crds")
}
