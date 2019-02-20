package helpers

import (
	"bytes"
	"context"
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

gloo:
  deployment:
    image:
      repository: soloio/gloo
      tag: {{ .Version }}
      pullPolicy: Never
    replicas: 1
    stats: true
    xdsPort: 9977
discovery:
  deployment:
    image:
      repository: soloio/discovery
      tag: {{ .Version }}
      pullPolicy: Never
    replicas: 1
    stats: true
gateway:
  deployment:
    image:
      repository: soloio/gateway
      tag: {{ .Version }}
      pullPolicy: Never
    replicas: 1
    stats: true
gatewayProxy:
  deployment:
    image:
      repository: soloio/gloo-envoy-wrapper
      tag: {{ .Version }}
      pullPolicy: Never
    httpPort: 8080
    httpsPort: 8443
    replicas: 1
ingress:
  deployment:
    image:
      repository: soloio/ingress
      tag: {{ .Version }}
      pullPolicy: Never
    replicas: 1
ingressProxy:
  deployment:
    image:
      repository: soloio/gloo-envoy-wrapper
      tag: {{ .Version }}
      pullPolicy: Never
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
	"gloo=gateway-proxy",
}

func WaitGlooPods(timeout, interval time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := WaitPodsRunning(ctx, interval, glooPodLabels...); err != nil {
		return err
	}
	return nil
}

func WaitPodsRunning(ctx context.Context, interval time.Duration, labels ...string) error {
	finished := func(output string) bool {
		return strings.Contains(output, "Running") || strings.Contains(output, "ContainerCreating")
	}
	for _, label := range labels {
		if err := WaitPodStatus(ctx, interval, label, "Running or ContainerCreating", finished); err != nil {
			return err
		}
	}
	finished = func(output string) bool {
		return strings.Contains(output, "Running")
	}
	for _, label := range labels {
		if err := WaitPodStatus(ctx, interval, label, "Running", finished); err != nil {
			return err
		}
	}
	return nil
}

func WaitPodsTerminated(ctx context.Context, interval time.Duration, labels ...string) error {
	for _, label := range labels {
		finished := func(output string) bool {
			return !strings.Contains(output, label)
		}
		if err := WaitPodStatus(ctx, interval, label, "terminated", finished); err != nil {
			return err
		}
	}
	return nil
}

func WaitPodStatus(ctx context.Context, interval time.Duration, label, status string, finished func(output string) bool) error {
	tick := time.Tick(interval)
	d, _ := ctx.Deadline()
	log.Debugf("waiting till %v for pod %v to be %v...", d, label, status)
	for {
		select {
		case <-ctx.Done():
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
