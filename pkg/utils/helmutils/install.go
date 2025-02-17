package helmutils

import (
	"fmt"
)

// InstallOpts is a set of typical options for a helm install which can be passed in
// instead of requiring the caller to remember the helm cli flags.
type InstallOpts struct {
	// KubeContext is the kubernetes context to use.
	KubeContext string

	// Namespace is the namespace to which the release will be installed.
	Namespace string

	// CreateNamespace controls whether to create the namespace or error if it doesn't exist.
	CreateNamespace bool

	// ValuesFiles is a list of absolute paths to YAML values for the installation. They will be
	// applied in the order they are specified.
	ValuesFiles []string

	// ExtraArgs allows passing in arbitrary extra arguments to the install.
	ExtraArgs []string

	// ReleaseName is the name of the release to install.
	ReleaseName string

	// Repository is the remote repo to use. Ignored if ChartUri is set.
	Repository string

	// ChartName is the name of the chart to use. Ignored if ChartUri is set.
	ChartName string

	// ChartUri may refer to a local chart path (e.g. to a tgz file) or a remote chart uri (e.g. oci://...) to install.
	// If provided, then Repository and ChartName are ignored.
	ChartUri string

	// Version can be used to install a specific release version (e.g. v2.0.0)
	Version string
}

func (o InstallOpts) all() []string {
	return append([]string{o.release(), o.chart()}, o.flags()...)
}

func (o InstallOpts) flags() []string {
	args := []string{}
	appendIfNonEmpty := func(flagVal, flagName string) {
		if flagVal != "" {
			args = append(args, flagName, flagVal)
		}
	}

	appendIfNonEmpty(o.KubeContext, "--kube-context")
	appendIfNonEmpty(o.Namespace, "--namespace")
	if o.CreateNamespace {
		args = append(args, "--create-namespace")
	}
	appendIfNonEmpty(o.Version, "--version")
	for _, valsFile := range o.ValuesFiles {
		appendIfNonEmpty(valsFile, "--values")
	}
	for _, extraArg := range o.ExtraArgs {
		args = append(args, extraArg)
	}

	return args
}

func (o InstallOpts) chart() string {
	if o.ChartUri != "" {
		return o.ChartUri
	}

	if o.Repository != "" && o.ChartName != "" {
		return fmt.Sprintf("%s/%s", o.Repository, o.ChartName)
	}

	return DefaultChartUri
}

func (o InstallOpts) release() string {
	if o.ReleaseName != "" {
		return o.ReleaseName
	}

	return ChartName
}
