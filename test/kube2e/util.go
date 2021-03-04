package kube2e

import (
	"context"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/check"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	clienthelpers "github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/k8s-utils/testutils/helper"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/gomega"
	errors "github.com/rotisserie/eris"
	"k8s.io/client-go/kubernetes"
)

func MustKubeClient() kubernetes.Interface {
	restConfig, err := kubeutils.GetConfig("", "")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	kubeClient, err := kubernetes.NewForConfig(restConfig)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return kubeClient
}

// Check that everything is OK by running `glooctl check`
func GlooctlCheckEventuallyHealthy(offset int, testHelper *helper.SoloTestHelper, timeoutInterval string) {
	EventuallyWithOffset(offset, func() error {
		opts := &options.Options{
			Metadata: core.Metadata{
				Namespace: testHelper.InstallNamespace,
			},
			Top: options.Top{
				Ctx: context.Background(),
			},
		}
		err := check.CheckResources(opts)
		if err != nil {
			return errors.New("glooctl check detected a problem with the installation")
		}
		return nil
	}, timeoutInterval, "5s").Should(BeNil())
}

func GetHelmValuesOverrideFile() (filename string, cleanup func()) {
	values, err := ioutil.TempFile("", "values-*.yaml")
	Expect(err).NotTo(HaveOccurred())

	// disabling usage statistics is not important to the functionality of the tests,
	// but we don't want to report usage in CI since we only care about how our users are actually using Gloo.
	// install to a single namespace so we can run multiple invocations of the regression tests against the
	// same cluster in CI.
	_, err = values.Write([]byte(`
global:
  image:
    pullPolicy: IfNotPresent
  glooRbac:
    namespaced: true
    nameSuffix: e2e-test-rbac-suffix
settings:
  singleNamespace: true
  create: true
  replaceInvalidRoutes: true
gloo:
  deployment:
    disableUsageStatistics: true
gatewayProxies:
  gatewayProxy:
    healthyPanicThreshold: 0
`))
	Expect(err).NotTo(HaveOccurred())

	err = values.Close()
	Expect(err).NotTo(HaveOccurred())

	return values.Name(), func() { _ = os.Remove(values.Name()) }
}

func EventuallyReachesConsistentState(installNamespace string) {
	metricsPort := strconv.Itoa(9091)
	portFwd := exec.Command("kubectl", "port-forward", "-n", installNamespace,
		"deployment/gloo", metricsPort)
	portFwd.Stdout = os.Stderr
	portFwd.Stderr = os.Stderr
	err := portFwd.Start()
	Expect(err).ToNot(HaveOccurred())

	defer func() {
		if portFwd.Process != nil {
			portFwd.Process.Kill()
		}
	}()

	// make sure we eventually reach an eventually consistent state
	lastSnapOut := getSnapOut(metricsPort)

	eventuallyConsistentPollingInterval := 7 * time.Second // >= 5s for metrics reporting, which happens every 5s
	time.Sleep(eventuallyConsistentPollingInterval)

	Eventually(func() bool {
		currentSnapOut := getSnapOut(metricsPort)
		consistent := lastSnapOut == currentSnapOut
		lastSnapOut = currentSnapOut
		return consistent
	}, "30s", eventuallyConsistentPollingInterval).Should(Equal(true))

	Consistently(func() string {
		currentSnapOut := getSnapOut(metricsPort)
		return currentSnapOut
	}, "30s", eventuallyConsistentPollingInterval).Should(Equal(lastSnapOut))
}

// needs a port-forward of the metrics port before a call to this will work
func getSnapOut(metricsPort string) string {
	var bodyResp string
	Eventually(func() string {
		res, err := http.Post("http://localhost:"+metricsPort+"/metrics", "", nil)
		if err != nil || res.StatusCode != 200 {
			return ""
		}
		defer res.Body.Close()
		body, err := ioutil.ReadAll(res.Body)
		Expect(err).ToNot(HaveOccurred())
		bodyResp = string(body)
		return bodyResp
	}, "5s", "1s").ShouldNot(BeEmpty())

	Expect(bodyResp).To(ContainSubstring("api_gloo_solo_io_emitter_snap_out"))
	findSnapOut := regexp.MustCompile("api_gloo_solo_io_emitter_snap_out ([\\d]+)")
	matches := findSnapOut.FindAllStringSubmatch(bodyResp, -1)
	Expect(matches).To(HaveLen(1))
	snapOut := matches[0][1]
	return snapOut
}

// enable/disable strict validation
func UpdateAlwaysAcceptSetting(ctx context.Context, alwaysAccept bool, installNamespace string) {
	UpdateSettings(func(settings *v1.Settings) {
		Expect(settings.Gateway).NotTo(BeNil())
		Expect(settings.Gateway.Validation).NotTo(BeNil())
		settings.Gateway.Validation.AlwaysAccept = &wrappers.BoolValue{Value: alwaysAccept}
	}, ctx, installNamespace)
}

func UpdateRestEdsSetting(ctx context.Context, enableRestEds bool, installNamespace string) {
	UpdateSettings(func(settings *v1.Settings) {
		Expect(settings.Gloo).NotTo(BeNil())
		settings.Gloo.EnableRestEds = &wrappers.BoolValue{Value: enableRestEds}
	}, ctx, installNamespace)
}

func UpdateSettings(f func(settings *v1.Settings), ctx context.Context, installNamespace string) {
	settingsClient := clienthelpers.MustSettingsClient(ctx)
	settings, err := settingsClient.Read(installNamespace, "default", clients.ReadOpts{})
	Expect(err).NotTo(HaveOccurred())

	f(settings)

	_, err = settingsClient.Write(settings, clients.WriteOpts{OverwriteExisting: true})
	Expect(err).NotTo(HaveOccurred())

	// when validation config changes, the validation server restarts -- give time for it to come up again.
	// without the wait, the validation webhook may temporarily fallback to it's failurePolicy, which is not
	// what we want to test.
	time.Sleep(3 * time.Second)
}
