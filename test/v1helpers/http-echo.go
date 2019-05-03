package v1helpers

import (
	"bytes"
	"context"
	"time"

	"github.com/solo-io/go-utils/log"

	"github.com/solo-io/go-utils/errors"
	"github.com/solo-io/go-utils/kubeutils"
	"github.com/solo-io/go-utils/testutils"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	defaultHttpEchoImage = "kennship/http-echo:latest"
	HttpEchoName         = "http-echo"
	HttpEchoPort         = 3000

	defaultTestRunnerImage = "soloio/testrunner:latest"
	TestrunnerName         = "testrunner"
	TestRunnerPort         = 1234

	// This response is given by the testrunner when the SimpleServer is started
	SimpleHttpResponse = `<!DOCTYPE html PUBLIC "-//W3C//DTD HTML 3.2 Final//EN"><html>
<title>Directory listing for /</title>
<body>
<h2>Directory listing for /</h2>
<hr>
<ul>
<li><a href="bin/">bin/</a>
<li><a href="pkg/">pkg/</a>
<li><a href="protoc-3.3.0-linux-x86_64.zip">protoc-3.3.0-linux-x86_64.zip</a>
<li><a href="protoc3/">protoc3/</a>
<li><a href="src/">src/</a>
</ul>
<hr>
</body>
</html>`
)

func NewTestRunner(namespace string) (*TestContainer, error) {
	return newTestContainer(namespace, defaultTestRunnerImage, TestrunnerName, TestRunnerPort)
}

func NewEchoHttp(namespace string) (*TestContainer, error) {
	return newTestContainer(namespace, defaultHttpEchoImage, HttpEchoName, HttpEchoPort)
}

func newTestContainer(namespace, imageTag, echoName string, port int32) (*TestContainer, error) {
	cfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return nil, err
	}
	kube, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &TestContainer{
		namespace: namespace,
		kube:      kube,

		echoName: echoName,
		port:     port,
		imageTag: imageTag,
	}, nil
}

// This object represents a container that gets deployed to the cluster to support testing.
type TestContainer struct {
	containerImageName string
	containerPort      uint
	namespace          string
	kube               *kubernetes.Clientset

	imageTag string
	echoName string
	port     int32
}

// Deploys the http echo to the kubernetes cluster the kubeconfig is pointing to and waits for the given time for the
// http-echo pod to be running.
func (t *TestContainer) Deploy(timeout time.Duration) error {
	zero := int64(0)
	labels := map[string]string{"gloo": t.echoName}
	metadata := metav1.ObjectMeta{
		Name:      t.echoName,
		Namespace: t.namespace,
		Labels:    labels,
	}

	// Create http echo pod
	if _, err := t.kube.CoreV1().Pods(t.namespace).Create(&corev1.Pod{
		ObjectMeta: metadata,
		Spec: corev1.PodSpec{
			TerminationGracePeriodSeconds: &zero,
			Containers: []corev1.Container{
				{
					Image:           t.imageTag,
					ImagePullPolicy: corev1.PullIfNotPresent,
					Name:            t.echoName,
				},
			},
		},
	}); err != nil {
		return err
	}

	// Create http echo service
	if _, err := t.kube.CoreV1().Services(t.namespace).Create(&corev1.Service{
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
	}); err != nil {
		return err
	}

	// Wait until the http echo pod is running
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := testutils.WaitPodsRunning(ctx, time.Second, t.namespace, "gloo="+t.echoName); err != nil {
		return err
	}
	log.Printf("deployed %s", t.echoName)
	return nil
}

func (t *TestContainer) Terminate() error {
	if err := testutils.Kubectl("delete", "pod", "-n", t.namespace, t.echoName, "--grace-period=0"); err != nil {
		return errors.Wrapf(err, "deleting %s pod", t.echoName)
	}
	return nil

}

// TestContainer executes a command inside the TestContainer container
func (t *TestContainer) Exec(command ...string) (string, error) {
	args := append([]string{"exec", "-i", t.echoName, "-n", t.namespace, "--"}, command...)
	return testutils.KubectlOut(args...)
}

// TestRunnerAsync executes a command inside the TestContainer container
// returning a buffer that can be read from as it executes
func (t *TestContainer) TestRunnerAsync(args ...string) (*bytes.Buffer, chan struct{}, error) {
	args = append([]string{"exec", "-i", t.echoName, "-n", t.namespace, "--"}, args...)
	return testutils.KubectlOutAsync(args...)
}
