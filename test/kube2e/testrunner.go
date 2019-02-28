package kube2e

import (
	"context"
	"fmt"
	"github.com/solo-io/gloo/test/helpers"
	"time"

	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/utils/log"
	"github.com/solo-io/solo-kit/test/setup"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// TODO(ilackarms): tie testrunner to solo CI test containers and then handle image tagging
	defaultTestRunnerImage = "soloio/testrunner:latest"
	TestRunnerPort         = 1234
)

func deployTestRunner(namespace, image string, port int32) error {
	cfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return err
	}
	kube, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	labels := map[string]string{"gloo": "testrunner"}
	if _, err := kube.CoreV1().Pods(namespace).Create(&v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testrunner",
			Namespace: namespace,
			// needed for WaitPodsRunning
			Labels: labels,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Image:           image,
					ImagePullPolicy: v1.PullIfNotPresent,
					Name:            "testrunner",
				},
			},
		},
	}); err != nil {
		return err
	}
	if _, err := kube.CoreV1().Services(namespace).Create(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testrunner",
			Namespace: namespace,
			// needed for WaitPodsRunning
			Labels: labels,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     "http",
					Protocol: v1.ProtocolTCP,
					Port:     port,
				},
			},
			Selector: labels,
		},
	}); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := helpers.WaitPodsRunning(ctx, time.Second, namespace, "gloo=testrunner"); err != nil {
		return err
	}
	go func() {
		if err := StartSimpleHttpServer(namespace, port); err != nil {
			log.Warnf("failed to start HTTP Server in Test Runner: %v", err)
		}
	}()
	return nil
}

// this response is given by the testrunner when the SimpleServer is started
const SimpleHttpResponse = `<!DOCTYPE html PUBLIC "-//W3C//DTD HTML 3.2 Final//EN"><html>
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

func StartSimpleHttpServer(namespace string, port int32) error {
	_, err := setup.TestRunner(namespace, "python", "-m", "SimpleHTTPServer", fmt.Sprintf("%v", port))
	return err
}
