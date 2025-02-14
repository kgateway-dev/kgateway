package helmutils

import (
	"context"
	"fmt"
	"io"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmdutils"
)

// Client is a utility for executing `helm` commands
type Client struct {
	// receiver is the default destination for the helm stdout and stderr
	receiver io.Writer

	namespace string
}

// InstallOpts is a set of typical options for a helm install which can be passed in
// instead of requiring the caller to remember the helm cli flags.
type InstallOpts struct {
	// KubeContext is the kubernetes context to use.
	KubeContext string

	// Namespace is the namespace to which the release will be installed.
	Namespace string

	// CreateNamespace controls whether to create the namespace or error if it doesn't exist.
	CreateNamespace bool

	// ValuesFiles is a list of absolute paths to YAML values for the installation.
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

// NewClient returns an implementation of the helmutils.Client
func NewClient() *Client {
	return &Client{
		receiver: io.Discard,
	}
}

// WithReceiver sets the io.Writer that will be used by default for the stdout and stderr
// of cmdutils.Cmd created by the Client
func (c *Client) WithReceiver(receiver io.Writer) *Client {
	c.receiver = receiver
	return c
}

// WithNamespace sets the namespace that all commands will be invoked against
func (c *Client) WithNamespace(ns string) *Client {
	c.namespace = ns
	return c
}

// Command returns a Cmd that executes kubectl command, including the --context if it is defined
// The Cmd sets the Stdout and Stderr to the receiver of the Cli
func (c *Client) Command(ctx context.Context, args ...string) cmdutils.Cmd {
	if c.namespace != "" {
		args = append([]string{"--namespace", c.namespace}, args...)
	}

	return cmdutils.Command(ctx, "helm", args...).
		// For convenience, we set the stdout and stderr to the receiver
		// This can still be overwritten by consumers who use the commands
		WithStdout(c.receiver).
		WithStderr(c.receiver)
}

// RunCommand creates a Cmd and then runs it
func (c *Client) RunCommand(ctx context.Context, args ...string) error {
	return c.Command(ctx, args...).Run().Cause()
}

func (c *Client) Install(ctx context.Context, installOpts InstallOpts) error {
	args := append([]string{"install"}, installOpts.all()...)
	return c.RunCommand(ctx, args...)
}

func (c *Client) Delete(ctx context.Context, extraArgs ...string) error {
	args := append([]string{
		"delete",
	}, extraArgs...)

	return c.RunCommand(ctx, args...)
}

func (c *Client) AddRepository(ctx context.Context, chartName string, chartUrl string, extraArgs ...string) error {
	args := append([]string{
		"repo",
		"add",
		chartName,
		chartUrl,
	}, extraArgs...)
	return c.RunCommand(ctx, args...)
}
