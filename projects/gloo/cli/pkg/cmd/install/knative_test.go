package install

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/go-utils/testutils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

var _ = Describe("Knative", func() {
	knativeInstallOpts := options.Knative{
		InstallKnativeVersion:         "0.10.0",
		InstallKnativeEventingVersion: "0.10.0",
		InstallKnativeEventing:        true,
		InstallKnativeMonitoring:      true,
	}
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		// Knative support has been deprecated in Gloo Edge 1.10 (https://github.com/solo-io/gloo/issues/5707)
		// and will be removed in Gloo Edge 1.11.
		// These tests are not run during CI.
		Skip("This test is for a deprecated feature.")

		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() { cancel() })

	Context("RenderKnativeManifests", func() {
		It("renders manifests for each knative component", func() {
			manifests, err := RenderKnativeManifests(knativeInstallOpts)
			Expect(err).NotTo(HaveOccurred())
			Expect(manifests).To(Equal(expected0100Manifests))
		})
	})
	Context("checkKnativeInstallation", func() {
		It("returns true, opts if knative was installed by us", func() {
			optsJson, err := json.Marshal(knativeInstallOpts)
			Expect(err).NotTo(HaveOccurred())
			kc := fake.NewSimpleClientset()
			_, err = kc.CoreV1().Namespaces().Create(ctx, &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "knative-serving",
					Annotations: map[string]string{installedByUsAnnotationKey: string(optsJson)},
				},
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			installed, opts, err := checkKnativeInstallation(ctx, kc)
			Expect(err).NotTo(HaveOccurred())
			Expect(installed).To(BeTrue())
			Expect(opts).To(Equal(&knativeInstallOpts))
		})
		It("returns true, nil if knative was installed but not by us", func() {
			kc := fake.NewSimpleClientset()
			_, err := kc.CoreV1().Namespaces().Create(ctx, &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "knative-serving",
				},
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			installed, opts, err := checkKnativeInstallation(ctx, kc)
			Expect(err).NotTo(HaveOccurred())
			Expect(installed).To(BeTrue())
			Expect(opts).To(BeNil())
		})
		It("returns false, nil if knative was not installed", func() {
			kc := fake.NewSimpleClientset()

			installed, opts, err := checkKnativeInstallation(ctx, kc)
			Expect(err).NotTo(HaveOccurred())
			Expect(installed).To(BeFalse())
			Expect(opts).To(BeNil())
		})
	})
})

// does not contain any networking.knative.dev/ingress-provider=istio resources
var expected0100Manifests = func() string {
	currentFile, err := testutils.GetCurrentFile()
	if err != nil {
		panic(err)
	}
	knativeTestYaml := filepath.Join(filepath.Dir(currentFile), "knative_test_yaml.yaml")
	b, err := os.ReadFile(knativeTestYaml)
	if err != nil {
		panic(err)
	}
	return string(b)

}()
