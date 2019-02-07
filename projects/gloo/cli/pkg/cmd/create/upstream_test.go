package create_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/static"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("Upstream", func() {

	BeforeEach(func() {
		helpers.UseMemoryClients()
	})

	It("should create static upstream", func() {
		err := testutils.Glooctl("create upstream static jsonplaceholder-80 --static-hosts jsonplaceholder.typicode.com:80")
		Expect(err).NotTo(HaveOccurred())

		up, err := helpers.MustUpstreamClient().Read("gloo-system", "jsonplaceholder-80", clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())

		staticSpec := up.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_Static).Static
		expectedHosts := []*static.Host{{Addr: "jsonplaceholder.typicode.com", Port: 80}}
		Expect(staticSpec.Hosts).To(Equal(expectedHosts))
	})

	It("should create aws upstream", func() {
		err := testutils.Glooctl("create upstream aws --aws-region us-east-1 --aws-secret-name aws-lambda-access --name aws-us-east-1")
		Expect(err).NotTo(HaveOccurred())

		up, err := helpers.MustUpstreamClient().Read("gloo-system", "aws-us-east-1", clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())

		awsSpec := up.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_Aws).Aws
		Expect(awsSpec.Region).To(Equal("us-east-1"))
		Expect(awsSpec.SecretRef.Name).To(Equal("aws-lambda-access"))

	})

	Context("kube upstream", func() {

		It("kube service name not provided", func() {
			err := testutils.Glooctl("create upstream kube --name kube-upstream")
			Expect(err).To(HaveOccurred())
		})

		expectKubeUpstream := func(name, namespace string, port uint32, selector map[string]string) {
			up, err := helpers.MustUpstreamClient().Read("gloo-system", "kube-upstream", clients.ReadOpts{})
			Expect(err).NotTo(HaveOccurred())

			kubeSpec := up.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_Kube).Kube
			Expect(kubeSpec.ServiceName).To(Equal(name))
			Expect(kubeSpec.ServiceNamespace).To(Equal(namespace))
			Expect(kubeSpec.ServicePort).To(Equal(port))
			Expect(kubeSpec.Selector).To(BeEquivalentTo(selector))
		}

		It("should create kube upstream with default namespace and port", func() {
			err := testutils.Glooctl("create upstream kube --name kube-upstream --kube-service kube-service")
			Expect(err).NotTo(HaveOccurred())
			expectKubeUpstream("kube-service", "default", uint32(80), nil)
		})

		It("should create kube upstream with custom namespace and port", func() {
			err := testutils.Glooctl("create upstream kube --name kube-upstream --kube-service kube-service --kube-service-namespace custom --kube-service-port 100")
			Expect(err).NotTo(HaveOccurred())
			expectKubeUpstream("kube-service", "custom", uint32(100), nil)
		})

		It("should create kube upstream with labels selector", func() {
			err := testutils.Glooctl("create upstream kube --name kube-upstream --kube-service kube-service --kube-service-labels foo=bar,gloo=baz")
			Expect(err).NotTo(HaveOccurred())
			expectKubeUpstream("kube-service", "default", uint32(80), map[string]string{"foo": "bar", "gloo": "baz"})
		})
	})

})
