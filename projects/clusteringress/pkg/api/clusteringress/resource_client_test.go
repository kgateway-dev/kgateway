package clusteringress_test

import (
	"os"
	"time"

	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/go-utils/testutils/clusterlock"

	"github.com/solo-io/solo-kit/test/helpers"

	"github.com/knative/serving/pkg/apis/networking/v1alpha1"
	knativeclientset "github.com/knative/serving/pkg/client/clientset/versioned"
	knativev1alpha1 "github.com/knative/serving/pkg/client/clientset/versioned/typed/networking/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/projects/clusteringress/pkg/api/clusteringress"
	v1 "github.com/solo-io/gloo/projects/clusteringress/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/utils/log"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
)

var _ = Describe("ResourceClient", func() {
	if os.Getenv("RUN_KUBE_TESTS") != "1" {
		log.Printf("This test creates kubernetes resources and is disabled by default. To enable, set RUN_KUBE_TESTS=1 in your env.")
		return
	}
	var (
		locker            *clusterlock.TestClusterLocker
		resourceName      string
		cfg               *rest.Config
		knative           knativeclientset.Interface
		kubeIngressClient knativev1alpha1.ClusterIngressInterface
	)

	BeforeEach(func() {
		resourceName = "trusty-" + helpers.RandString(8)
		var err error
		cfg, err = kubeutils.GetConfig("", "")
		Expect(err).NotTo(HaveOccurred())
		// register knative crd just in case
		apiexts, err := clientset.NewForConfig(cfg)

		options := clusterlock.Options{
			IdPrefix: os.ExpandEnv("clusteringress-${BUILD_ID}-"),
		}
		locker, err = clusterlock.NewTestClusterLocker(kube2e.MustKubeClient(), options)
		Expect(err).NotTo(HaveOccurred())
		Expect(locker.AcquireLock()).NotTo(HaveOccurred())

		Expect(err).NotTo(HaveOccurred())
		_, err = apiexts.ApiextensionsV1beta1().CustomResourceDefinitions().Create(&v1beta1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: "clusteringresses.networking.internal.knative.dev",
			},
			Spec: v1beta1.CustomResourceDefinitionSpec{
				Group:   "networking.internal.knative.dev",
				Version: "v1alpha1",
				Names: v1beta1.CustomResourceDefinitionNames{
					Kind:       "ClusterIngress",
					Plural:     "clusteringresses",
					Singular:   "clusteringress",
					Categories: []string{"all", "knative-internal", "networking"},
				},
				Scope: v1beta1.ClusterScoped,
				Subresources: &v1beta1.CustomResourceSubresources{
					Status: &v1beta1.CustomResourceSubresourceStatus{},
				},
				AdditionalPrinterColumns: []v1beta1.CustomResourceColumnDefinition{
					{Name: "Ready", Type: "string", JSONPath: ".status.conditions[?(@.type==\"Ready\")].status"},
					{Name: "Reason", Type: "string", JSONPath: ".status.conditions[?(@.type==\"Ready\")].reason"},
				},
			},
		})
		if err != nil && !errors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
		// wait for it to be accepted
		Eventually(func() (string, error) {
			crd, err := apiexts.ApiextensionsV1beta1().CustomResourceDefinitions().Get("clusteringresses.networking.internal.knative.dev", metav1.GetOptions{})
			if err != nil {
				return "", err
			}
			return crd.Status.AcceptedNames.Kind, nil
		}, "1s", "0.1s").ShouldNot(BeEmpty())

		knative, err = knativeclientset.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
		kubeIngressClient = knative.NetworkingV1alpha1().ClusterIngresses()
	})
	AfterEach(func() {
		defer locker.ReleaseLock()
		// register knative crd just in case
		apiexts, err := clientset.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
		apiexts.ApiextensionsV1beta1().CustomResourceDefinitions().Delete("clusteringresses.networking.internal.knative.dev", nil)
		kubeIngressClient.Delete(resourceName, nil)
	})

	It("can CRUD on v1beta1 ingresses", func() {
		baseClient := NewResourceClient(knative, &v1.ClusterIngress{})
		ingressClient := v1.NewClusterIngressClientWithBase(baseClient)
		kubeIng, err := kubeIngressClient.Create(&v1alpha1.ClusterIngress{
			ObjectMeta: metav1.ObjectMeta{
				Name: resourceName,
			},
			Spec: v1alpha1.IngressSpec{
				Rules: []v1alpha1.ClusterIngressRule{
					{
						Hosts: []string{
							"helloworld-go.default.example.com",
							"helloworld-go.default.svc.cluster.local",
							"helloworld-go.default.svc",
							"helloworld-go.default",
						},
						HTTP: &v1alpha1.HTTPClusterIngressRuleValue{
							Paths: []v1alpha1.HTTPClusterIngressPath{
								{
									AppendHeaders: map[string]string{
										"knative-serving-namespace": "default",
										"knative-serving-revision":  "helloworld-go-00001",
									},
									Retries: &v1alpha1.HTTPRetry{
										Attempts: 3,
										PerTryTimeout: &metav1.Duration{
											Duration: time.Minute,
										},
									},
									Splits: []v1alpha1.ClusterIngressBackendSplit{
										{
											Percent: 100,
											ClusterIngressBackend: v1alpha1.ClusterIngressBackend{
												ServiceName:      "activator-service",
												ServiceNamespace: "knative-serving",
												ServicePort:      intstr.IntOrString{IntVal: 80},
											},
										},
									},
									Timeout: &metav1.Duration{
										Duration: time.Minute,
									},
								},
							},
						},
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		ingressResource, err := ingressClient.Read(kubeIng.Name, clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())
		convertedIng, err := ToKube(ingressResource)
		Expect(err).NotTo(HaveOccurred())
		Expect(convertedIng.Spec).To(Equal(kubeIng.Spec))
	})
})
