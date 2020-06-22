package helpers

import (
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	. "github.com/onsi/gomega"
	"github.com/solo-io/go-utils/testutils/helper"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func EventuallyReachesConsistentState(testHelper *helper.SoloTestHelper) {
	metricsPort := strconv.Itoa(9091)
	portFwd := exec.Command("kubectl", "port-forward", "-n", testHelper.InstallNamespace,
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
	}, "3s", "0.5s").ShouldNot(BeEmpty())

	Expect(bodyResp).To(ContainSubstring("api_gloo_solo_io_emitter_snap_out"))
	findSnapOut := regexp.MustCompile("api_gloo_solo_io_emitter_snap_out ([\\d]+)")
	matches := findSnapOut.FindAllStringSubmatch(bodyResp, -1)
	Expect(matches).To(HaveLen(1))
	snapOut := matches[0][1]
	return snapOut
}

func TearDownTestHelper(testHelper *helper.SoloTestHelper) {
	Expect(testHelper).ToNot(BeNil())
	err := testHelper.UninstallGloo()
	Expect(err).NotTo(HaveOccurred())
	_, err = MustKubeClient().CoreV1().Namespaces().Get(testHelper.InstallNamespace, metav1.GetOptions{})
	Expect(apierrors.IsNotFound(err)).To(BeTrue())
}
