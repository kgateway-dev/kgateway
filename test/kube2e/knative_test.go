package kube2e_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/go-utils/kubeinstallutils"
	"github.com/solo-io/go-utils/kubeutils"
	"github.com/solo-io/solo-kit/test/setup"
	"github.com/solo-io/solo-kit/test/testutils"
	"io/ioutil"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"path/filepath"
)

var _ = FDescribe("Kube2e: Knative-Ingress", func() {
	BeforeEach(func() {
		deployKnative("a")
	})
	AfterEach(func() {
		deleteKnative("a")
	})
	It("works", func() {
		return

		cfg, err := kubeutils.GetConfig("", "")
		Expect(err).NotTo(HaveOccurred())

		kube, err := kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
		kubeIngressClient := kube.ExtensionsV1beta1().Ingresses(namespace)
		backend := &v1beta1.IngressBackend{
			ServiceName: "testrunner",
			ServicePort: intstr.IntOrString{
				IntVal: testRunnerPort,
			},
		}
		kubeIng, err := kubeIngressClient.Create(&v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "simple-ingress-route",
				Namespace:   namespace,
				Annotations: map[string]string{"kubernetes.io/ingress.class": "gloo"},
			},
			Spec: v1beta1.IngressSpec{
				Backend: backend,
				//TLS: []v1beta1.IngressTLS{
				//	{
				//		Hosts:      []string{"some.host"},
				//		SecretName: "doesntexistanyway",
				//	},
				//},
				Rules: []v1beta1.IngressRule{
					{
						//Host: "some.host",
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{
									{
										Backend: *backend,
									},
								},
							},
						},
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(kubeIng).NotTo(BeNil())

		ingressProxy := "ingress-proxy"
		ingressPort := 80
		setup.CurlEventuallyShouldRespond(setup.CurlOpts{
			Protocol: "http",
			Path:     "/",
			Method:   "GET",
			Host:     ingressProxy,
			Service:  ingressProxy,
			Port:     ingressPort,
		}, helpers.SimpleHttpResponse)
	})
})

func deployKnative(namespace string) {
	cfg, err := kubeutils.GetConfig("", "")
	Expect(err).NotTo(HaveOccurred())

	b, err := ioutil.ReadFile(filepath.Join(helpers.GlooTestArtifactsDir(), "knative-no-istio.yaml"))
	Expect(err).NotTo(HaveOccurred())

	err = testutils.DeployFromYaml(cfg, namespace, string(b))
	Expect(err).NotTo(HaveOccurred())
}

func deleteKnative(namespace string) {
	cfg, err := kubeutils.GetConfig("", "")
	Expect(err).NotTo(HaveOccurred())

	b, err := ioutil.ReadFile(filepath.Join(helpers.GlooTestArtifactsDir(), "knative-no-istio.yaml"))
	Expect(err).NotTo(HaveOccurred())

	err = DeleteFromYaml(cfg, namespace, string(b))
	Expect(err).NotTo(HaveOccurred())
}

func DeleteFromYaml(cfg *rest.Config, namespace, yamlManifest string) error {
	kube, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}

	apiext, err := clientset.NewForConfig(cfg)
	if err != nil {
		return err
	}

	installer := kubeinstallutils.NewKubeInstaller(kube, apiext, namespace)

	kubeObjs, err := kubeinstallutils.ParseKubeManifest(yamlManifest)
	if err != nil {
		return err
	}

	for _, kubeOjb := range kubeObjs {
		if err := installer.Delete(kubeOjb); err != nil {
			return err
		}
	}
	return nil
}
