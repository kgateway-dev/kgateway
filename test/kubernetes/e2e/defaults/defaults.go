package defaults

import (
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/kubectl"
)

var (
	CurlPodExecOpt = kubectl.PodExecOptions{
		Name:      "curl",
		Namespace: "curl",
		Container: "curl",
	}

	CurlPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "curl",
			Namespace: "curl",
		},
	}

	CurlPodManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "curl_pod.yaml")

	CurlPodLabelSelector = "app.kubernetes.io/name=curl"

	HttpEchoPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "http-echo",
			Namespace: "http-echo",
		},
	}

	HttpEchoPodManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "http_echo.yaml")

	TcpEchoPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tcp-echo",
			Namespace: "tcp-echo",
		},
	}

	TcpEchoPodManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "tcp_echo.yaml")

	NginxPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: "nginx",
		},
	}

	NginxSvc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: "nginx",
		},
	}

	NginxPodManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "nginx_pod.yaml")

	NginxResponse = `<!DOCTYPE html>
<html>
<head>
<title>Welcome to nginx!</title>
<style>
html { color-scheme: light dark; }
body { width: 35em; margin: 0 auto;
font-family: Tahoma, Verdana, Arial, sans-serif; }
</style>
</head>
<body>
<h1>Welcome to nginx!</h1>
<p>If you see this page, the nginx web server is successfully installed and
working. Further configuration is required.</p>

<p>For online documentation and support please refer to
<a href="http://nginx.org/">nginx.org</a>.<br/>
Commercial support is available at
<a href="http://nginx.com/">nginx.com</a>.</p>

<p><em>Thank you for using nginx.</em></p>
</body>
</html>`

	HttpbinManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httpbin.yaml")

	HttpbinLabelSelector = "app=httpbin"

	HttpbinPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httpbin", // FIXME: this is incorrect, pod name is ~generated
			Namespace: "default",
		},
	}

	WellKnownAppLabel = "app.kubernetes.io/name"
)
