package install

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"

	. "github.com/onsi/ginkgo"
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
			_, err = kc.CoreV1().Namespaces().Create(&v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "knative-serving",
					Annotations: map[string]string{installedByUsAnnotationKey: string(optsJson)},
				},
			})
			Expect(err).NotTo(HaveOccurred())

			installed, opts, err := checkKnativeInstallation(kc)
			Expect(err).NotTo(HaveOccurred())
			Expect(installed).To(BeTrue())
			Expect(opts).To(Equal(&knativeInstallOpts))
		})
		It("returns true, nil if knative was installed but not by us", func() {
			kc := fake.NewSimpleClientset()
			_, err := kc.CoreV1().Namespaces().Create(&v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "knative-serving",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			installed, opts, err := checkKnativeInstallation(kc)
			Expect(err).NotTo(HaveOccurred())
			Expect(installed).To(BeTrue())
			Expect(opts).To(BeNil())
		})
		It("returns false, nil if knative was not installed", func() {
			kc := fake.NewSimpleClientset()

			installed, opts, err := checkKnativeInstallation(kc)
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
	b, err := ioutil.ReadFile(knativeTestYaml)
	if err != nil {
		panic(err)
	}
	return string(b)

}()
