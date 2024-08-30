package helper

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"

	"github.com/pkg/errors"
	"github.com/solo-io/go-utils/log"

	"github.com/solo-io/go-utils/testutils"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var _ TestUpstreamServer = &testServer{}
var _ TestContainer = &testServer{}
var _ TestContainer = &testContainer{}

// A TestContainer is a general-purpose abstraction over a container in which we might
// execute cURL or other, arbitrary commands via kubectl.
type TestContainer interface {
	DeployResources(timeout time.Duration) error
	TerminatePod() error
	DeleteService() error
	TerminatePodAndDeleteService() error
	CanCurl() bool
	// CurlEventuallyShouldRespond checks the response of the request eventually meets expectation
	// The response is type interface{}. See the actual implementation for which types are supported
	CurlEventuallyShouldRespond(opts CurlOpts, response interface{}, ginkgoOffset int, timeout ...time.Duration)
	// CurlEventuallyShouldOutput checks all the output of the curl command eventually meets expectation
	// The response is type interface{}. See the actual implementation for which types are supported
	// Deprecated: Prefer CurlEventuallyShouldRespond
	CurlEventuallyShouldOutput(opts CurlOpts, output interface{}, ginkgoOffset int, timeout ...time.Duration)
	Curl(opts CurlOpts) (string, error)
	Exec(command ...string) (string, error)
	ExecAsync(args ...string) (io.Reader, chan struct{}, error)
}

// A TestUpstreamServer is an extension of a TestContainer which is typically run with the defaultTestServerImage.
// It is used to deploy test http/https services
type TestUpstreamServer interface {
	TestContainer
	DeployServer(timeout time.Duration) error
	DeployServerTls(timeout time.Duration, crt, key []byte) error
}

func newTestContainer(namespace, imageTag, echoName string, port int32, createService bool, command []string) (*testContainer, error) {
	cfg, err := kubeutils.GetRestConfigWithKubeContext("")
	if err != nil {
		return nil, err
	}
	kube, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &testContainer{
		namespace: namespace,
		kube:      kube,

		podName:       echoName,
		port:          port,
		imageTag:      imageTag,
		createService: createService,
		command:       command,
	}, nil
}

// This object represents a container that gets deployed to the cluster to support testing.
type testContainer struct {
	containerImageName string
	containerPort      uint
	namespace          string
	kube               kubernetes.Interface

	imageTag      string
	podName       string
	port          int32
	createService bool
	command       []string
}

func (t *testContainer) DeployResources(timeout time.Duration) error {
	return t.deploy(timeout)
}

// Deploys the specified pod to the kubernetes cluster the kubeconfig is pointing to and waits for the given time for the
// pod to be running.
func (t *testContainer) deploy(timeout time.Duration) error {
	zero := int64(0)
	labels := map[string]string{"gloo": t.podName}
	metadata := metav1.ObjectMeta{
		Name:      t.podName,
		Namespace: t.namespace,
		Labels:    labels,
	}

	// Create http echo pod
	if _, err := t.kube.CoreV1().Pods(t.namespace).Create(context.Background(), &corev1.Pod{
		ObjectMeta: metadata,
		Spec: corev1.PodSpec{
			TerminationGracePeriodSeconds: &zero,
			Containers: []corev1.Container{
				{
					Image:           t.imageTag,
					ImagePullPolicy: corev1.PullIfNotPresent,
					Name:            t.podName,
					Command:         t.command,
				},
			},
		},
	}, metav1.CreateOptions{}); err != nil {
		return err
	}

	if t.createService {
		if _, err := t.kube.CoreV1().Services(t.namespace).Create(context.Background(), &corev1.Service{
			ObjectMeta: metadata,
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name:     "http",
						Protocol: corev1.ProtocolTCP,
						Port:     t.port,
					},
				},
				Selector: labels,
			},
		}, metav1.CreateOptions{}); err != nil {
			return err
		}
	}

	// added to check the time it takes to deploy the pods. This will allow us to
	// comment on the caller why we selected the timeout and what to troubleshoot
	// if it is exceeded.
	// Currently this is at ~4 seconds in CI.
	tStart := time.Now()
	// Wait until the http echo pod is running
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := testutils.WaitPodsRunning(ctx, time.Second, t.namespace, "gloo="+t.podName); err != nil {
		return err
	}

	log.Printf("deployed %s in %s", t.podName, time.Now().Sub(tStart)/time.Second)

	return nil
}

func (t *testContainer) TerminatePod() error {
	if err := testutils.Kubectl("delete", "pod", "-n", t.namespace, t.podName, "--grace-period=0"); err != nil {
		return errors.Wrapf(err, "deleting %s pod", t.podName)
	}
	return nil
}

func (t *testContainer) DeleteService() error {
	if err := testutils.Kubectl("delete", "service", "-n", t.namespace, t.podName, "--grace-period=0"); err != nil {
		return errors.Wrapf(err, "deleting %s service", t.podName)
	}
	return nil
}

func (t *testContainer) TerminatePodAndDeleteService() error {
	if err := t.TerminatePod(); err != nil {
		return err
	}
	if err := t.DeleteService(); err != nil {
		return err
	}
	return nil
}

// testContainer executes a command inside the testContainer container
func (t *testContainer) Exec(command ...string) (string, error) {
	args := append([]string{"exec", "-i", t.podName, "-n", t.namespace, "--"}, command...)
	return testutils.KubectlOut(args...)
}

// Cp copies files into the testContainer container
func (t *testContainer) Cp(files map[string]string) error {
	for k, v := range files {
		if err := testutils.Kubectl("cp", k, fmt.Sprintf("%s/%s:%s", t.namespace, t.podName, v)); err != nil {
			return err
		}
	}
	return nil
}

// ExecAsync executes a command inside the testContainer container
// returning a buffer that can be read from as it executes
func (t *testContainer) ExecAsync(args ...string) (io.Reader, chan struct{}, error) {
	args = append([]string{"exec", "-i", t.podName, "-n", t.namespace, "--"}, args...)
	return testutils.KubectlOutAsync(args...)
}

func (t *testContainer) ExecChan(r io.Reader, args ...string) (<-chan io.Reader, chan struct{}, error) {
	args = append([]string{"exec", "-i", t.podName, "-n", t.namespace, "--"}, args...)
	return testutils.KubectlOutChan(r, args...)
}

func (t *testContainer) CanCurl() bool {
	if out, err := t.Exec("curl", "--version"); err != nil || !strings.HasPrefix(out, "curl") {
		return false
	}
	return true
}
