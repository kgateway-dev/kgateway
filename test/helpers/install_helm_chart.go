package helpers

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/log"
	"github.com/solo-io/solo-kit/test/setup"
)

func DeployGlooWithHelm(namespace, imageVersion string, enableKnative, verbose bool) error {
	log.Printf("deploying gloo with version %v", imageVersion)
	values, err := ioutil.TempFile("", "gloo-test-")
	if err != nil {
		return err
	}
	defer os.Remove(values.Name())
	if _, err := io.Copy(values, GlooHelmValues(namespace, imageVersion, enableKnative)); err != nil {
		return err
	}
	err = values.Close()
	if err != nil {
		return err
	}

	// make the manifest
	manifestContents, err := RunCommandOutput(verbose,
		"helm", "template", GlooHelmChartDir(),
		"--namespace", namespace,
		"-f", values.Name(),
	)
	if err != nil {
		return err
	}

	if err := RunCommandInput(manifestContents, verbose, "kubectl", "apply", "-f", "-"); err != nil {
		return err
	}

	return nil
}

func GlooHelmValues(namespace, version string, enableKnative bool) io.Reader {
	b := &bytes.Buffer{}

	err := template.Must(template.New("gloo-helm-values").Parse(`
# note: these values must remain consistent with 
# install/helm/gloo/values.yaml
namespace:
  create: false
rbac:
  create: true

settings:
  integrations:
    knative:
      enabled: {{ .EnableKnative }}
      proxy:
        image: soloio/gloo-envoy-wrapper:{{ .Version }}
        httpPort: 80
        httpsPort: 443
        replicas: 1

  # namespaces that Gloo should watch. this includes watches set for pods, services, as well as CRD configuration objects
  watchNamespaces: []
  # the namespace that Gloo should write discovery data (Upstreams)
  writeNamespace: {{ .Namespace }}

deployment:
  imagePullPolicy: IfNotPresent
  gloo:
    xdsPort: 9977
    image: soloio/gloo:{{ .Version }}
    replicas: 1
  discovery:
    image: soloio/discovery:{{ .Version }}
    replicas: 1
  gateway:
    image: soloio/gateway:{{ .Version }}
    replicas: 1
  gatewayProxy:
    image: soloio/gloo-envoy-wrapper:{{ .Version }}
    httpPort: 8080
    replicas: 1
  ingress:
    image: soloio/ingress:{{ .Version }}
    replicas: 1
  ingressProxy:
    image: soloio/gloo-envoy-wrapper:{{ .Version }}
    httpPort: 80
    httpsPort: 443
    replicas: 1
`)).Execute(b, struct {
		Version       string
		Namespace     string
		EnableKnative bool
	}{
		Version:       version,
		Namespace:     namespace,
		EnableKnative: enableKnative,
	})
	if err != nil {
		panic(err)
	}

	return b
}

var glooPodLabels = []string{
	"gloo=gloo",
	"gloo=discovery",
	"gloo=gateway",
	"gloo=ingress",
}

func WaitGlooPods(timeout, interval time.Duration) error {
	if err := WaitPodsRunning(timeout, interval, glooPodLabels...); err != nil {
		return err
	}
	return nil
}

func WaitPodsRunning(timeout, interval time.Duration, labels ...string) error {
	finished := func(output string) bool {
		return strings.Contains(output, "Running") || strings.Contains(output, "ContainerCreating")
	}
	for _, label := range labels {
		if err := WaitPodStatus(timeout, interval, label, "Running", finished); err != nil {
			return err
		}
	}
	finished = func(output string) bool {
		return strings.Contains(output, "Running")
	}
	for _, label := range labels {
		if err := WaitPodStatus(timeout, interval, label, "Running", finished); err != nil {
			return err
		}
	}
	return nil
}

func WaitPodsTerminated(timeout, interval time.Duration, labels ...string) error {
	for _, label := range labels {
		finished := func(output string) bool {
			return !strings.Contains(output, label)
		}
		if err := WaitPodStatus(timeout, interval, label, "terminated", finished); err != nil {
			return err
		}
	}
	return nil
}

func WaitPodStatus(timeout, interval time.Duration, label, status string, finished func(output string) bool) error {
	tick := time.Tick(interval)

	log.Debugf("waiting %v for pod %v to be %v...", timeout, label, status)
	for {
		select {
		case <-time.After(timeout):
			return fmt.Errorf("timed out waiting for %v to be %v", label, status)
		case <-tick:
			out, err := setup.KubectlOut("get", "pod", "-l", label)
			if err != nil {
				return fmt.Errorf("failed getting pod: %v", err)
			}
			if strings.Contains(out, "CrashLoopBackOff") {
				out = KubeLogs(label)
				return errors.Errorf("%v in crash loop with logs %v", label, out)
			}
			if strings.Contains(out, "ErrImagePull") || strings.Contains(out, "ImagePullBackOff") {
				out, _ = setup.KubectlOut("describe", "pod", "-l", label)
				return errors.Errorf("%v in ErrImagePull with description %v", label, out)
			}
			if finished(out) {
				return nil
			}
		}
	}
}

func KubeLogs(label string) string {
	out, err := setup.KubectlOut("logs", "-l", label)
	if err != nil {
		out = err.Error()
	}
	return out
}
