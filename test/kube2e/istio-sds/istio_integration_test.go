package istio_test

import (
	"fmt"
	"path/filepath"
	"time"

	kubernetesplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"
	skerrors "github.com/solo-io/solo-kit/pkg/errors"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/go-utils/testutils/exec"
	"github.com/solo-io/skv2/codegen/util"
	"k8s.io/apimachinery/pkg/util/intstr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/k8s-utils/testutils/helper"
	kubeService "github.com/solo-io/solo-kit/api/external/kubernetes/service"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	httpbinName = "httpbin"
	httpbinPort = 8000
)

var _ = Describe("Gloo + Istio SDS integration tests", func() {
	var (
		upstreamRef, serviceRef, virtualServiceRef core.ResourceRef
	)

	BeforeEach(func() {
		serviceRef = core.ResourceRef{Name: httpbinName, Namespace: namespace}
		virtualServiceRef = core.ResourceRef{Name: httpbinName, Namespace: namespace}

		service := corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceRef.Name,
				Namespace: serviceRef.Namespace,
				Labels:    map[string]string{"gloo": httpbinName},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name:       "http",
						Port:       httpbinPort,
						TargetPort: intstr.FromInt(httpbinPort),
						Protocol:   corev1.ProtocolTCP,
					},
				},
				Selector: map[string]string{"gloo": httpbinName},
			},
		}
		var err error
		_, err = resourceClientSet.ServiceClient().Write(
			&kubernetes.Service{Service: kubeService.Service{Service: service}},
			clients.WriteOpts{},
		)
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() error {
			_, err := resourceClientSet.ServiceClient().Read(serviceRef.Namespace, service.Name, clients.ReadOpts{})
			return err
		}, "5s", "1s").Should(BeNil())

		// the upstream should be created by discovery service
		upstreamRef = core.ResourceRef{
			Name:      kubernetesplugin.UpstreamName(namespace, httpbinName, httpbinPort),
			Namespace: namespace,
		}
		helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
			return resourceClientSet.UpstreamClient().Read(upstreamRef.Namespace, upstreamRef.Name, clients.ReadOpts{})
		})

		virtualService := &v1.VirtualService{
			Metadata: &core.Metadata{
				Name:      virtualServiceRef.Name,
				Namespace: virtualServiceRef.Namespace,
			},
			VirtualHost: &v1.VirtualHost{
				Domains: []string{"*"},
				Routes: []*v1.Route{{
					Action: &v1.Route_RouteAction{
						RouteAction: &gloov1.RouteAction{
							Destination: &gloov1.RouteAction_Single{
								Single: &gloov1.Destination{
									DestinationType: &gloov1.Destination_Upstream{
										Upstream: &upstreamRef,
									},
								},
							},
						},
					},
					Matchers: []*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/",
							},
						},
					},
				}},
			},
		}
		_, err = resourceClientSet.VirtualServiceClient().Write(virtualService, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
		helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
			return resourceClientSet.VirtualServiceClient().Read(virtualServiceRef.Namespace, virtualServiceRef.Name, clients.ReadOpts{})
		})
	})

	AfterEach(func() {
		var err error
		err = resourceClientSet.VirtualServiceClient().Delete(virtualServiceRef.Namespace, virtualServiceRef.Name, clients.DeleteOpts{
			IgnoreNotExist: true,
		})
		Expect(err).NotTo(HaveOccurred())
		helpers.EventuallyResourceDeleted(func() (resources.InputResource, error) {
			return resourceClientSet.VirtualServiceClient().Read(virtualServiceRef.Namespace, virtualServiceRef.Name, clients.ReadOpts{})
		})

		err = resourceClientSet.ServiceClient().Delete(serviceRef.Namespace, serviceRef.Name, clients.DeleteOpts{
			IgnoreNotExist: true,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() bool {
			_, err := resourceClientSet.ServiceClient().Read(serviceRef.Namespace, serviceRef.Name, clients.ReadOpts{})
			// we should receive a DNE error, meaning it's now deleted
			return err != nil && skerrors.IsNotExist(err)
		}, "5s", "1s").Should(BeTrue())

		err = resourceClientSet.UpstreamClient().Delete(upstreamRef.Namespace, upstreamRef.Name, clients.DeleteOpts{
			IgnoreNotExist: true,
		})
		helpers.EventuallyResourceDeleted(func() (resources.InputResource, error) {
			return resourceClientSet.UpstreamClient().Read(upstreamRef.Namespace, upstreamRef.Name, clients.ReadOpts{})
		})
	})

	Context("strict peer auth", func() {
		BeforeEach(func() {
			err := exec.RunCommand(testHelper.RootDir, false, "kubectl", "apply", "-f", filepath.Join(util.GetModuleRoot(), "test", "kube2e", "istio-sds", "peerauth_strict.yaml"))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should make a request with the expected cert header", func() {
			// the /headers endpoint will respond with the headers the request to the client contains
			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/headers",
				Method:            "GET",
				Host:              helper.TestrunnerName,
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 10,
				Verbose:           false,
				WithoutStats:      true,
				ReturnHeaders:     false,
			}, fmt.Sprintf("\"X-Forwarded-Client-Cert\""), 1, time.Minute*1)
		})
	})
})
