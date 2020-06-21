package kube2e

import (
	"context"
	"io/ioutil"
	"os"

	. "github.com/onsi/gomega"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/check"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/go-utils/testutils/helper"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/solo-io/go-utils/kubeutils"
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
func GlooctlCheckEventuallyHealthy(testHelper *helper.SoloTestHelper, timeoutInterval string) {
	Eventually(func() error {
		opts := &options.Options{
			Metadata: core.Metadata{
				Namespace: testHelper.InstallNamespace,
			},
			Top: options.Top{
				Ctx: context.Background(),
			},
		}
		ok, err := check.CheckResources(opts)
		if err != nil {
			return errors.Wrap(err, "unable to run glooctl check")
		}
		if ok {
			return nil
		}
		return errors.New("glooctl check detected a problem with the installation")
	}, timeoutInterval, "5s").Should(BeNil())
}

func GetHelmValuesOverrideFile(values string) (filename string, cleanup func()) {
	valuesFile, err := ioutil.TempFile("", "values-*.yaml")
	Expect(err).NotTo(HaveOccurred())

	// disabling usage statistics is not important to the functionality of the tests,
	// but we don't want to report usage in CI since we only care about how our users are actually using Gloo.
	// install to a single namespace so we can run multiple invocations of the regression tests against the
	// same cluster in CI.
	_, err = valuesFile.Write([]byte(values))
	Expect(err).NotTo(HaveOccurred())

	err = valuesFile.Close()
	Expect(err).NotTo(HaveOccurred())

	return valuesFile.Name(), func() { _ = os.Remove(valuesFile.Name()) }
}
