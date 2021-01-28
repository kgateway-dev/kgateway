package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/solo-io/go-utils/testutils"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	. "github.com/onsi/gomega"
)

func CurlWithEphemeralPod(ctx context.Context, logger io.Writer, kubecontext, fromns, frompod string, args ...string) string {
	createargs := []string{"alpha", "debug", "--quiet",
		"--image=curlimages/curl@sha256:aa45e9d93122a3cfdf8d7de272e2798ea63733eeee6d06bd2ee4f2f8c4027d7c",
		"--container=curl", frompod, "-n", fromns, "--", "sleep", "10h"}
	// Execute curl commands from the same pod each time to avoid creating a burdensome number of ephemeral pods.
	// create the curl pod; we do this every time and it will only work the first time, so ignore failures
	executeNoFail(ctx, logger, kubecontext, createargs...)
	args = append([]string{"exec",
		"--container=curl", frompod, "-n", fromns, "--", "curl", "--connect-timeout", "1", "--max-time", "5"}, args...)
	return execute(ctx, logger, kubecontext, args...)
}

// labelSelector is a string map e.g. gloo=gateway-proxy
func FindPodNameByLabel(cfg *rest.Config, ctx context.Context, ns, labelSelector string) string {
	clientset, err := kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
	pl, err := clientset.CoreV1().Pods(ns).List(ctx, v1.ListOptions{LabelSelector: labelSelector})
	Expect(err).NotTo(HaveOccurred())
	Expect(pl.Items).NotTo(BeEmpty())
	return pl.Items[0].GetName()
}

func WaitForRollout(ctx context.Context, logger io.Writer, kubecontext, ns, deployment string) {
	args := []string{"-n", ns, "rollout", "status", "deployment", deployment}
	execute(ctx, logger, kubecontext, args...)
}

func Curl(ctx context.Context, logger io.Writer, kubecontext, ns, fromDeployment, fromContainer, url string) string {
	args := []string{
		"-n", ns,
		"exec", fmt.Sprintf("deployment/%s", fromDeployment),
		"-c", fromContainer,
		"--", "curl", url,
	}
	return execute(ctx, logger, kubecontext, args...)
}

func CreateNamespace(ctx context.Context, logger io.Writer, kubeContext, ns string) {
	args := []string{"create", "namespace", ns}
	out := execute(ctx, logger, kubeContext, args...)
	fmt.Fprintln(logger, out)
}

func DeleteNamespace(ctx context.Context, logger io.Writer, kubeContext, ns string) {
	args := []string{"delete", "namespace", ns}
	out := execute(ctx, logger, kubeContext, args...)
	fmt.Fprintln(logger, out)
}

func LabelNamespace(ctx context.Context, logger io.Writer, kubeContext, ns, label string) {
	args := []string{"label", "namespace", ns, label}
	out := execute(ctx, logger, kubeContext, args...)
	fmt.Fprintln(logger, out)
}

func SetDeploymentEnvVars(
	ctx context.Context,
	kubeContext string,
	logger io.Writer,
	ns string,
	deploymentName string,
	containerName string,
	envVars map[string]string,
) {
	var envVarStrings []string
	for name, value := range envVars {
		envVarStrings = append(envVarStrings, fmt.Sprintf("%s=%s", name, value))
	}
	args := append([]string{"set", "env", "-n", ns, fmt.Sprintf("deployment/%s", deploymentName), "-c", containerName}, envVarStrings...)
	out := execute(ctx, logger, kubeContext, args...)
	fmt.Fprintln(logger, out)
}

func DisableContainer(
	ctx context.Context,
	logger io.Writer,
	kubeContext string,
	ns string,
	deploymentName string,
	containerName string,
) {
	args := append([]string{
		"-n", ns,
		"patch", "deployment", deploymentName,
		"--patch",
		fmt.Sprintf("{\"spec\": {\"template\": {\"spec\": {\"containers\": [{\"name\": \"%s\",\"command\": [\"sleep\", \"20h\"]}]}}}}",
			containerName),
	})
	out := execute(ctx, logger, kubeContext, args...)
	fmt.Fprintln(logger, out)
}

func EnableContainer(
	ctx context.Context,
	logger io.Writer,
	kubeContext string,
	ns string,
	deploymentName string,
) {
	args := append([]string{
		"-n", ns,
		"patch", "deployment", deploymentName,
		"--type", "json",
		"-p", "[{\"op\": \"remove\", \"path\": \"/spec/template/spec/containers/0/command\"}]",
	})
	out := execute(ctx, logger, kubeContext, args...)
	fmt.Fprintln(logger, out)
}

func execute(ctx context.Context, logger io.Writer, kubeContext string, args ...string) string {
	data, err := executeNoFail(ctx, logger, kubeContext, args...)
	Expect(err).NotTo(HaveOccurred())
	return data
}

func executeNoFail(ctx context.Context, logger io.Writer, kubeContext string, args ...string) (string, error) {
	args = append([]string{"--context", kubeContext}, args...)
	fmt.Fprintf(logger, "Executing: kubectl %v \n", args)
	readerChan, done, err := testutils.KubectlOutChan(&bytes.Buffer{}, args...)
	if err != nil {
		return "", err
	}
	defer close(done)
	select {
	case <-ctx.Done():
		return "", nil
	case reader := <-readerChan:
		data, err := ioutil.ReadAll(reader)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(logger, "<kubectl %v> output: %v\n", args, string(data))
		return string(data), nil
	}
}
