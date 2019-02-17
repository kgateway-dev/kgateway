package setuputils

import (
	"flag"
	"os"
	"strings"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

// TODO (ilackarms): move to a flags package
type arrayFlags []string

func (i *arrayFlags) String() string {
	return strings.Join(*i, ",")
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

const (
	MY_POD_NAMESPACE = "MY_POD_NAMESPACE"
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
	defaultNamespace := os.Getenv(MY_POD_NAMESPACE)
	if defaultNamespace == "" {
		defaultNamespace = defaults.GlooSystem
	}
	flag.StringVar(&setupNamespace, "namespace", defaultNamespace, "namespace to watch for settings crd/file")
	flag.StringVar(&setupName, "name", defaults.SettingsName, "name of settings crd/file to use")
	flag.StringVar(&setupDir, "dir", "", "directory to find bootstrap settings if not using "+
		"kubernetes crds")
}
