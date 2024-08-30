package kubectl

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/types"

	"github.com/solo-io/gloo/pkg/utils/cmdutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/k8s-utils/testutils/kube"

	"github.com/avast/retry-go/v4"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/portforward"
)

// Cli is a utility for executing `kubectl` commands
type Cli struct {
	// receiver is the default destination for the kubectl stdout and stderr
	receiver io.Writer

	// kubeContext is the optional value of the context for a given kubernetes cluster
	// If it is not supplied, no context will be included in the command
	kubeContext string
}

// PodExecOptions describes the options used to execute a command in a pod
type PodExecOptions struct {
	Name      string
	Namespace string
	Container string
}

// NewCli returns an implementation of the kubectl.Cli
func NewCli() *Cli {
	return &Cli{
		receiver:    io.Discard,
		kubeContext: "",
	}
}

// CurlResponse stores the output from a curl, with separate fields for StdErr, which typically contains headers and
// connection information, and SteOut, which typically contains the response body
type CurlResponse struct {
	StdErr string
	StdOut string
}

// WithReceiver sets the io.Writer that will be used by default for the stdout and stderr
// of cmdutils.Cmd created by the Cli
func (c *Cli) WithReceiver(receiver io.Writer) *Cli {
	c.receiver = receiver
	return c
}

// WithKubeContext sets the --context for the kubectl command invoked by the Cli
func (c *Cli) WithKubeContext(kubeContext string) *Cli {
	c.kubeContext = kubeContext
	return c
}

// Command returns a Cmd that executes kubectl command, including the --context if it is defined
// The Cmd sets the Stdout and Stderr to the receiver of the Cli
func (c *Cli) Command(ctx context.Context, args ...string) cmdutils.Cmd {
	if c.kubeContext != "" {
		args = append([]string{"--context", c.kubeContext}, args...)
	}

	return cmdutils.Command(ctx, "kubectl", args...).
		// For convenience, we set the stdout and stderr to the receiver
		// This can still be overwritten by consumers who use the commands
		WithStdout(c.receiver).
		WithStderr(c.receiver)
}

// RunCommand creates a Cmd and then runs it
func (c *Cli) RunCommand(ctx context.Context, args ...string) error {
	return c.Command(ctx, args...).Run().Cause()
}

// Namespaces returns a sorted list of namespaces or an error if one occurred
func (c *Cli) Namespaces(ctx context.Context) ([]string, error) {
	stdout, _, err := c.Execute(ctx, "get", "namespaces", "--sort-by", "metadata.name", "-o", "jsonpath={.items[*].metadata.name}")
	if err != nil {
		return nil, err
	}
	return strings.Fields(stdout), nil
}

// Apply applies the resources defined in the bytes, and returns an error if one occurred
func (c *Cli) Apply(ctx context.Context, content []byte, extraArgs ...string) error {
	args := append([]string{"apply", "-f", "-"}, extraArgs...)
	return c.Command(ctx, args...).
		WithStdin(bytes.NewBuffer(content)).
		Run().
		Cause()
}

// ApplyFile applies the resources defined in a file, and returns an error if one occurred
func (c *Cli) ApplyFile(ctx context.Context, fileName string, extraArgs ...string) error {
	_, err := c.ApplyFileWithOutput(ctx, fileName, extraArgs...)
	return err
}

// ApplyFilePath applies the resources defined at the file path, and returns an error if one occurred.
// If filePath is a directory, this will apply all of the files in the directory.
func (c *Cli) ApplyFilePath(ctx context.Context, filePath string, extraArgs ...string) error {
	stat, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	if !stat.IsDir() {
		_, err := c.ApplyFileWithOutput(ctx, filePath, extraArgs...)
		return err
	}

	args := append([]string{"apply", "-f", filePath}, extraArgs...)
	return c.Command(ctx, args...).
		Run().
		Cause()
}

// ApplyFileWithOutput applies the resources defined in a file,
// if an error occurred, it will be returned along with the output of the command
func (c *Cli) ApplyFileWithOutput(ctx context.Context, fileName string, extraArgs ...string) (string, error) {
	applyArgs := append([]string{"apply", "-f", fileName}, extraArgs...)

	fileInput, err := os.Open(fileName)
	if err != nil {
		return "", err
	}
	defer fileInput.Close()

	runErr := c.Command(ctx, applyArgs...).
		WithStdin(fileInput).
		Run()
	return runErr.OutputString(), runErr.Cause()
}

// Delete deletes the resources defined in the bytes, and returns an error if one occurred
func (c *Cli) Delete(ctx context.Context, content []byte, extraArgs ...string) error {
	args := append([]string{"delete", "-f", "-"}, extraArgs...)
	return c.Command(ctx, args...).
		WithStdin(bytes.NewBuffer(content)).
		Run().
		Cause()
}

// DeleteFile deletes the resources defined in a file, and returns an error if one occurred
func (c *Cli) DeleteFile(ctx context.Context, fileName string, extraArgs ...string) error {
	_, err := c.DeleteFileWithOutput(ctx, fileName, extraArgs...)
	return err
}

// DeleteFileWithOutput deletes the resources defined in a file,
// if an error occurred, it will be returned along with the output of the command
func (c *Cli) DeleteFileWithOutput(ctx context.Context, fileName string, extraArgs ...string) (string, error) {
	applyArgs := append([]string{"delete", "-f", fileName}, extraArgs...)

	fileInput, err := os.Open(fileName)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = fileInput.Close()
	}()

	runErr := c.Command(ctx, applyArgs...).
		WithStdin(fileInput).
		Run()
	return runErr.OutputString(), runErr.Cause()
}

// DeleteFileSafe deletes the resources defined in a file, and returns an error if one occurred
// This differs from DeleteFile in that we always append --ignore-not-found
func (c *Cli) DeleteFileSafe(ctx context.Context, fileName string, extraArgs ...string) error {
	safeArgs := append(extraArgs, "--ignore-not-found")
	return c.DeleteFile(ctx, fileName, safeArgs...)
}

// Copy copies a file from one location to another
func (c *Cli) Copy(ctx context.Context, from, to string) error {
	return c.RunCommand(ctx, "cp", from, to)
}

// DeploymentRolloutStatus waits for the deployment to complete rolling out
func (c *Cli) DeploymentRolloutStatus(ctx context.Context, deployment string, extraArgs ...string) error {
	rolloutArgs := append([]string{
		"rollout",
		"status",
		fmt.Sprintf("deployment/%s", deployment),
	}, extraArgs...)
	return c.RunCommand(ctx, rolloutArgs...)
}

// StartPortForward creates a PortForwarder based on the provides options, starts it, and returns the PortForwarder
// If an error was encountered while starting the PortForwarder, it is returned as well
// NOTE: It is the callers responsibility to close this port-forward
func (c *Cli) StartPortForward(ctx context.Context, options ...portforward.Option) (portforward.PortForwarder, error) {
	options = append([]portforward.Option{
		// We define some default values, which users can then override
		portforward.WithWriters(c.receiver, c.receiver),
		portforward.WithKubeContext(c.kubeContext),
	}, options...)

	portForwarder := portforward.NewCliPortForwarder(options...)
	err := portForwarder.Start(
		ctx,
		retry.LastErrorOnly(true),
		retry.Delay(100*time.Millisecond),
		retry.DelayType(retry.BackOffDelay),
		retry.Attempts(5),
	)
	return portForwarder, err
}

// CurlFromEphemeralPod executes a Curl from a pod, using an ephemeral container to execute the Curl command
func (c *Cli) CurlFromEphemeralPod(ctx context.Context, podMeta types.NamespacedName, options ...curl.Option) string {
	appendOption := func(option curl.Option) {
		options = append(options, option)
	}

	// The e2e test assertions rely on the transforms.WithCurlHttpResponse to validate the response is what
	// we would expect
	// For this transform to behave appropriately, we must execute the request with verbose=true
	appendOption(curl.VerboseOutput())

	curlArgs := curl.BuildArgs(options...)

	return kube.CurlWithEphemeralPodStable(
		ctx,
		c.receiver,
		c.kubeContext,
		podMeta.Namespace,
		podMeta.Name,
		curlArgs...)
}

// CurlFromPod executes a Curl request from the given pod for the given options.
// It differs from CurlFromEphemeralPod in that it does not use an ephemeral container to execute the Curl command
func (c *Cli) CurlFromPod(ctx context.Context, podOpts PodExecOptions, options ...curl.Option) (*CurlResponse, error) {
	appendOption := func(option curl.Option) {
		options = append(options, option)
	}

	// The e2e test assertions rely on the transforms.WithCurlHttpResponse to validate the response is what
	// we would expect
	// For this transform to behave appropriately, we must execute the request with verbose=true
	appendOption(curl.VerboseOutput())

	curlArgs := curl.BuildArgs(options...)

	container := podOpts.Container
	if container == "" {
		container = "curl"
	}

	args := append([]string{
		"exec",
		"--container=" + container,
		podOpts.Name,
		"-n",
		podOpts.Namespace,
		"--",
		"curl",
		"--connect-timeout",
		"1",
		"--max-time",
		"5",
	}, curlArgs...)

	stdout, stderr, err := c.ExecuteOn(ctx, c.kubeContext, args...)

	return &CurlResponse{StdOut: stdout, StdErr: stderr}, err
}

func (c *Cli) ExecuteOn(ctx context.Context, kubeContext string, args ...string) (string, string, error) {
	args = append([]string{"--context", kubeContext}, args...)
	return c.Execute(ctx, args...)
}

func (c *Cli) Execute(ctx context.Context, args ...string) (string, string, error) {
	if c.kubeContext != "" {
		if !slices.Contains(args, "--context") {
			args = append([]string{"--context", c.kubeContext}, args...)
		}
	}

	stdout := new(strings.Builder)
	stderr := new(strings.Builder)

	err := cmdutils.Command(ctx, "kubectl", args...).
		// For convenience, we set the stdout and stderr to the receiver
		// This can still be overwritten by consumers who use the commands
		WithStdout(stdout).
		WithStderr(stderr).Run().Cause()

	return stdout.String(), stderr.String(), err
}

func (c *Cli) Scale(ctx context.Context, namespace string, resource string, replicas uint) error {
	err := c.RunCommand(ctx, "scale", "-n", namespace, fmt.Sprintf("--replicas=%d", replicas), resource, "--timeout=300s")
	if err != nil {
		return err
	}
	time.Sleep(2 * time.Second) // Sleep a bit so the container starts
	return c.RunCommand(ctx, "wait", "-n", namespace, "--for=condition=available", resource, "--timeout=300s")
}

// RestartDeployment restarts a deployment. It does not wait for the deployment to be ready.
func (c *Cli) RestartDeployment(ctx context.Context, name string, extraArgs ...string) error {
	args := append([]string{
		"rollout",
		"restart",
		fmt.Sprintf("deployment/%s", name),
	}, extraArgs...)
	return c.RunCommand(ctx, args...)
}

// RestartDeploymentAndWait restarts a deployment and waits for it to become healthy.
func (c *Cli) RestartDeploymentAndWait(ctx context.Context, name string, extraArgs ...string) error {
	if err := c.RestartDeployment(ctx, name, extraArgs...); err != nil {
		return err
	}
	return c.DeploymentRolloutStatus(ctx, name, extraArgs...)
}

// GetContainerLogs retrieves the logs for the specified container
func (c *Cli) GetContainerLogs(ctx context.Context, namespace string, name string) (string, error) {
	stdout, stderr, err := c.Execute(ctx, "-n", namespace, "logs", name)
	return stdout + stderr, err
}
