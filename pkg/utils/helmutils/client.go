package helmutils

import (
	"context"
	"fmt"
	"io"

	"github.com/solo-io/gloo/pkg/utils/cmdutils"
)

// Client is a utility for executing `helm` commands
type Client struct {
	// receiver is the default destination for the helm stdout and stderr
	receiver io.Writer

	namespace string
}

// InstallOpts is a set of typical options for a helm install which can be passed in
// instead of requiring the caller to remember the helm cli flags. extraArgs should
// always be accepted and respected when using InstallOpts.
type InstallOpts struct {
	// KubeContext is the kubernetes context to use.
	KubeContext string

	// Namespace is the namespace to which the release will be installed.
	Namespace string
	// CreateNamespace controls whether to create the namespace or error if it doesn't exist.
	CreateNamespace bool

	// ValuesFile is the path to the YAML values for the installation.
	ValuesFile string

	// ReleaseName is the name of the release to install. Usually will be "gloo".
	ReleaseName string

	// Repository is the remote repo to use. Usually will be one of the constants exported
	// from this package. Ignored if LocalChartPath is set.
	Repository string

	// ChartName is the name of the chart to use. Usually will be "gloo". Ignored if LocalChartPath is set.
	ChartName string

	// LocalChartPath is the path to a locally built tarballed chart to install
	LocalChartPath string
}

func (o InstallOpts) all() []string {
	return append([]string{o.chart(), o.release()}, o.flags()...)
}

func (o InstallOpts) flags() []string {
	args := []string{}
	appendIfNonEmpty := func(fld, flag string) {
		if fld != "" {
			args = append(args, flag, fld)
		}
	}

	appendIfNonEmpty(o.KubeContext, "--kube-context")
	appendIfNonEmpty(o.Namespace, "--namespace")
	if o.CreateNamespace {
		args = append(args, "--create-namespace")
	}
	appendIfNonEmpty(o.ValuesFile, "--values")

	return args
}

func (o InstallOpts) chart() string {
	if o.LocalChartPath != "" {
		return o.LocalChartPath
	}

	if o.Repository == "" || o.ChartName == "" {
		return RemoteChartName
	}

	return fmt.Sprintf("%s/%s", o.Repository, o.ChartName)
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

func (c *Client) Install(ctx context.Context, extraArgs ...string) error {
	args := append([]string{
		"install",
	}, extraArgs...)

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

func (c *Client) AddGlooRepository(ctx context.Context, extraArgs ...string) error {
	return c.AddRepository(ctx, ChartName, ChartRepositoryUrl, extraArgs...)
}

func (c *Client) AddPrGlooRepository(ctx context.Context, extraArgs ...string) error {
	return c.AddRepository(ctx, ChartName, PrChartRepositoryUrl, extraArgs...)
}

func (c *Client) InstallGloo(ctx context.Context, installOpts InstallOpts, extraArgs ...string) error {
	args := append(installOpts.all(), extraArgs...)
	return c.Install(ctx, args...)
}
