package create_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/argsutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/create"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("Upstream", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		helpers.UseMemoryClients()
	})

	AfterEach(func() {
		cancel()
	})

	getUpstream := func(name string) *v1.Upstream {
		up, err := helpers.MustUpstreamClient(ctx).Read("gloo-system", name, clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())
		return up
	}

	Context("Empty args and flags", func() {
		It("should give clear error message", func() {
			err := testutils.Glooctl("create upstream")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(create.EmptyUpstreamCreateError))
		})
	})

	Context("static", func() {
		It("should error when no name provided", func() {
			err := testutils.Glooctl("create upstream static --static-hosts jsonplaceholder.typicode.com:80")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(argsutils.NameError))
		})

		It("should error when host has invalid format", func() {
			err := testutils.Glooctl(`create upstream static netlify --static-hosts "https://netlify.com:443"`)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid host format"))
		})

		It("should error when hosts not provided", func() {
			err := testutils.Glooctl("create upstream static jsonplaceholder-80")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("must provide at least 1 host for static upstream"))
		})

		It("should work", func() {
			err := testutils.Glooctl("create upstream static jsonplaceholder-80 --static-hosts jsonplaceholder.typicode.com:80")
			Expect(err).NotTo(HaveOccurred())

			up := getUpstream("jsonplaceholder-80")

			staticSpec := up.UpstreamType.(*v1.Upstream_Static).Static
			expectedHosts := []*static.Host{{Addr: "jsonplaceholder.typicode.com", Port: 80}}
			Expect(staticSpec.Hosts).To(Equal(expectedHosts))
		})
	})

	Context("AWS", func() {
		It("should error out when no name provided", func() {
			err := testutils.Glooctl("create upstream aws --aws-secret-name aws-lambda-access")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(argsutils.NameError))
		})

		It("should error out when no secret name provided", func() {
			err := testutils.Glooctl("create upstream aws --name aws-us-east-1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("aws secret name must not be empty"))
		})

		expectAwsUpstream := func(name, region, secretName, secretNamespace string) {
			up := getUpstream(name)
			awsSpec := up.UpstreamType.(*v1.Upstream_Aws).Aws
			Expect(awsSpec.Region).To(Equal(region))
			Expect(awsSpec.SecretRef.Name).To(Equal(secretName))
			Expect(awsSpec.SecretRef.Namespace).To(Equal(secretNamespace))
		}

		It("should work with default region when no region provided", func() {
			err := testutils.Glooctl("create upstream aws --aws-secret-name aws-lambda-access --name aws-us-east-1")
			Expect(err).NotTo(HaveOccurred())
			expectAwsUpstream("aws-us-east-1", "us-east-1", "aws-lambda-access", "gloo-system")
		})

		It("should work", func() {
			err := testutils.Glooctl("create upstream aws --aws-region us-west-1 --aws-secret-name aws-lambda-access --aws-secret-namespace custom-namespace --name aws-us-west-1")
			Expect(err).NotTo(HaveOccurred())
			expectAwsUpstream("aws-us-west-1", "us-west-1", "aws-lambda-access", "custom-namespace")
		})
	})

	Context("Azure", func() {
		It("should error out when no name provided", func() {
			err := testutils.Glooctl("create upstream azure --azure-secret-name azure-secret")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(argsutils.NameError))
		})

		It("should error out when no secret name provided", func() {
			err := testutils.Glooctl("create upstream azure --name azure-upstream")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("azure secret name must not be empty"))
		})

		expectAzureUpstream := func(name, functionAppName, secretName, secretNamespace string) {
			up := getUpstream(name)

			azureSpec := up.UpstreamType.(*v1.Upstream_Azure).Azure
			Expect(azureSpec.FunctionAppName).To(Equal(functionAppName))
			Expect(azureSpec.SecretRef.Name).To(Equal(secretName))
			Expect(azureSpec.SecretRef.Namespace).To(Equal(secretNamespace))
		}

		It("should work when no app name provided", func() {
			err := testutils.Glooctl("create upstream azure --azure-secret-name azure-secret --name azure-upstream")
			Expect(err).NotTo(HaveOccurred())
			expectAzureUpstream("azure-upstream", "", "azure-secret", "gloo-system")
		})

		It("should work", func() {
			err := testutils.Glooctl("create upstream azure --azure-app-name azure-app --azure-secret-name azure-secret --azure-secret-namespace custom-namespace --name azure-upstream")
			Expect(err).NotTo(HaveOccurred())
			expectAzureUpstream("azure-upstream", "azure-app", "azure-secret", "custom-namespace")
		})
	})

	Context("Kube", func() {

		It("should error when service name not provided", func() {
			err := testutils.Glooctl("create upstream kube --name kube-upstream")
			Expect(err).To(HaveOccurred())
		})

		expectKubeUpstream := func(name, namespace string, port uint32, selector map[string]string) {
			up := getUpstream("kube-upstream")
			kubeSpec := up.UpstreamType.(*v1.Upstream_Kube).Kube
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

		It("can print as kube yaml in dry-run", func() {
			out, err := testutils.GlooctlOut("create upstream kube --dry-run --name kube-upstream --kube-service kube-service --kube-service-labels foo=bar,gloo=baz")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(ContainSubstring(`apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  creationTimestamp: null
  name: kube-upstream
  namespace: gloo-system
spec:
  kube:
    selector:
      foo: bar
      gloo: baz
    serviceName: kube-service
    serviceNamespace: default
    servicePort: 80
status: {}
`))
		})

		It("can print as solo-kit yaml in dry-run", func() {
			out, err := testutils.GlooctlOut("create upstream kube --dry-run -oyaml --name kube-upstream --kube-service kube-service --kube-service-labels foo=bar,gloo=baz")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(ContainSubstring(`---
kube:
  selector:
    foo: bar
    gloo: baz
  serviceName: kube-service
  serviceNamespace: default
  servicePort: 80
metadata:
  name: kube-upstream
  namespace: gloo-system
`))
		})

	})

	Context("Consul", func() {
		It("should error out when no name provided", func() {
			err := testutils.Glooctl("create upstream consul")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(argsutils.NameError))
		})

		It("should error out when no consul service name provided", func() {
			err := testutils.Glooctl("create upstream consul --name consul-upstream")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("must provide consul service name"))
		})

		expectConsulUpstream := func(name, consulService string, tags []string) {
			up := getUpstream(name)
			consulSpec := up.UpstreamType.(*v1.Upstream_Consul).Consul
			Expect(consulSpec.ServiceName).To(Equal(consulService))
			Expect(consulSpec.ServiceTags).To(ContainElements(tags))
		}

		It("should work with consul service name only", func() {
			err := testutils.Glooctl("create upstream consul --name consul-upstream --consul-service consul-service")
			Expect(err).NotTo(HaveOccurred())
			expectConsulUpstream("consul-upstream", "consul-service", []string{})
		})

		It("should work with consul service name and tags", func() {
			err := testutils.Glooctl("create upstream consul --name consul-upstream --consul-service consul-service --consul-service-tags foo,bar")
			Expect(err).NotTo(HaveOccurred())
			expectConsulUpstream("consul-upstream", "consul-service", []string{"foo", "bar"})
		})
	})

})
