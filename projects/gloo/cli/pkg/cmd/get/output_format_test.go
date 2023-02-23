package get_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Upstream", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		helpers.UseMemoryClients()
		ctx, cancel = context.WithCancel(context.Background())
		_, err := helpers.MustKubeClient().CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: defaults.GlooSystem,
			},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() { cancel() })

	getUpstream := func(name string) *v1.Upstream {
		up, err := helpers.MustUpstreamClient(ctx).Read("gloo-system", name, clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())

		return up
	}

	tableOutput := `+--------------------+--------+---------+---------------------------------+
|      UPSTREAM      |  TYPE  | STATUS  |             DETAILS             |
+--------------------+--------+---------+---------------------------------+
| jsonplaceholder-80 | Static | Pending | hosts:                          |
|                    |        |         | -                               |
|                    |        |         | jsonplaceholder.typicode.com:80 |
|                    |        |         |                                 |
+--------------------+--------+---------+---------------------------------+`

	kubeYamlOutput := `apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  creationTimestamp: null
  name: jsonplaceholder-80
  namespace: gloo-system
  resourceVersion: "2"
spec:
  static:
    hosts:
    - addr: jsonplaceholder.typicode.com
      port: 80
status: {}
`

	yamlOutput := `---
metadata:
  name: jsonplaceholder-80
  namespace: gloo-system
  resourceVersion: "2"
static:
  hosts:
  - addr: jsonplaceholder.typicode.com
    port: 80
`

	Context("default output should be -o table", func() {
		It("should override/allow -o flags as expected", func() {
			output, err := testutils.GlooctlOut("create upstream static jsonplaceholder-80 --static-hosts jsonplaceholder.typicode.com:80")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring(tableOutput))

			// make sure that we created the upstream that we intended
			up := getUpstream("jsonplaceholder-80")
			staticSpec := up.UpstreamType.(*v1.Upstream_Static).Static
			expectedHosts := []*static.Host{{Addr: "jsonplaceholder.typicode.com", Port: 80}}
			Expect(staticSpec.Hosts).To(Equal(expectedHosts))

			By("should default to -o table")
			output, err = testutils.GlooctlOut("get upstreams")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring(tableOutput))

			By("should respect (unnecessary) -o table flag")
			output, err = testutils.GlooctlOut("get upstreams -o table")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring(tableOutput))

			By("should respect -o yaml flag")
			output, err = testutils.GlooctlOut("get upstreams -o yaml")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring(yamlOutput))

			By("should respect -o kube-yaml flag")
			output, err = testutils.GlooctlOut("get upstreams -o kube-yaml")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring(kubeYamlOutput))
		})
	})
})
