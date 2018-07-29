package kube_e2e

import (
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/knative"
	"github.com/solo-io/gloo/test/helpers"
	kubev1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Knative", func() {

	var ns *kubev1.Namespace

	BeforeEach(func() {
		var options meta_v1.GetOptions
		svc, err := kube.CoreV1().Services("istio-system").Get("knative-ingressgateway", options)
		if err != nil || svc == nil {
			Skip("this test must be run with knative pre-installed")
		}

		// add istio label on namespace
		ns = &kubev1.Namespace{}
		ns.Name = namespace + "-knative"
		ns.Labels = map[string]string{
			"istio-injection": "enabled",
		}

		_, err = kube.CoreV1().Namespaces().Create(ns)
		Expect(err).NotTo(HaveOccurred())

		// apply the knative service..
		err = helpers.Kubectl("apply", "-n", ns.Name, "-f", filepath.Join(helpers.KubeResourcesDirectory(), "knative.yaml"))
		Expect(err).NotTo(HaveOccurred())
		time.Sleep(time.Second * 10)

	})

	AfterEach(func() {
		var options meta_v1.DeleteOptions
		// remove istio label
		if ns != nil {
			err := kube.CoreV1().Namespaces().Delete(ns.Name, &options)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	FIt("should detect the upstream service info", func() {
		var upstreamToTest *v1.Upstream

		Eventually(func() *v1.Upstream {
			list, err := gloo.V1().Upstreams().List()
			if err != nil {
				return nil
			}
			for _, us := range list {
				if us.Type != knative.UpstreamTypeKnative {
					continue
				}

				upstreamToTest = us
				break
			}
			return upstreamToTest
		}, "5m", "5s").ShouldNot(BeNil())

		spec, err := knative.DecodeUpstreamSpec(upstreamToTest.Spec)
		Expect(err).NotTo(HaveOccurred())

		Expect(spec.Hostname).To(Equal("helloworld-go." + ns.Name + ".example.com"))

		_, err = gloo.V1().VirtualServices().Create(&v1.VirtualService{
			Name: "knative-service",
			Routes: []*v1.Route{{
				Matcher: &v1.Route_RequestMatcher{
					RequestMatcher: &v1.RequestMatcher{
						Path: &v1.RequestMatcher_PathPrefix{
							PathPrefix: "/",
						},
					},
				},
				SingleDestination: &v1.Destination{
					DestinationType: &v1.Destination_Upstream{
						Upstream: &v1.UpstreamDestination{
							Name: upstreamToTest.Name,
						},
					},
				},
			}},
		})
		Expect(err).NotTo(HaveOccurred())
		// make sure that curl returns something we expect
		curlEventuallyShouldRespond(curlOpts{
			path: "/",
		}, "Hello", time.Minute)
	})

})
