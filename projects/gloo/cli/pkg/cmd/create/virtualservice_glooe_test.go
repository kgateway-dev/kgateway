package create_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"

	"github.com/solo-io/gloo/pkg/cliutil/testutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("VirtualService", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		helpers.UseMemoryClients()
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() { cancel() })

	Context("Interactive tests", func() {

		It("should create vs with no rate limits and auth", func() {
			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString("Add a domain for this virtual service (empty defaults to all domains)?")
				c.SendLine("")
				c.ExpectString("do you wish to add rate limiting to the virtual service")
				c.SendLine("n")
				c.ExpectString("do you wish to add an auth config reference to the virtual host")
				c.SendLine("n")

				c.ExpectString("Use default namespace (gloo-system)?")
				c.SendLine("")
				c.ExpectString("name of the resource:")
				c.SendLine("default")

				c.ExpectEOF()
			}, func() {
				err := testutils.Glooctl("create vs -i")
				Expect(err).NotTo(HaveOccurred())
				_, err = helpers.MustVirtualServiceClient(ctx).Read("gloo-system", "default", clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())
			})
		})

		It("should create vs with auth config", func() {
			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString("Add a domain for this virtual service (empty defaults to all domains)?")
				c.SendLine("")
				c.ExpectString("do you wish to add rate limiting to the virtual service")
				c.SendLine("n")
				c.ExpectString("do you wish to add an auth config reference to the virtual host")
				c.SendLine("y")
				c.ExpectString("auth config namespace?")
				c.SendLine("ns1")
				c.ExpectString("auth config name?")
				c.SendLine("ac1")

				c.ExpectString("Use default namespace (gloo-system)?")
				c.SendLine("")
				c.ExpectString("name of the resource:")
				c.SendLine("vs1")

				c.ExpectEOF()
			}, func() {
				err := testutils.Glooctl("create vs -i")
				Expect(err).NotTo(HaveOccurred())
				vs, err := helpers.MustVirtualServiceClient(ctx).Read("gloo-system", "vs1", clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())
				acRef := vs.VirtualHost.Options.Extauth.Spec.(*v1.ExtAuthExtension_ConfigRef).ConfigRef
				Expect(acRef.Name).To(Equal("ac1"))
				Expect(acRef.Namespace).To(Equal("ns1"))
			})
		})
	})

	var _ = Describe("dry-run", func() {
		It("can print as kube yaml in dry run", func() {
			out, err := testutils.GlooctlOut("create virtualservice kube --dry-run --name vs --domains foo.bar,baz.qux")
			Expect(err).NotTo(HaveOccurred())
			fmt.Print(out)
			Expect(out).To(ContainSubstring(`apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  creationTimestamp: null
  name: vs
  namespace: gloo-system
spec:
  displayName: vs
  virtualHost:
    domains:
    - foo.bar
    - baz.qux
status: {}
`))
		})

		It("can print as solo-kit yaml in dry run", func() {
			out, err := testutils.GlooctlOut("create virtualservice kube --dry-run -oyaml --name vs --domains foo.bar,baz.qux")
			Expect(err).NotTo(HaveOccurred())
			fmt.Print(out)
			Expect(out).To(ContainSubstring(`---
displayName: vs
metadata:
  name: vs
  namespace: gloo-system
virtualHost:
  domains:
  - foo.bar
  - baz.qux
`))
		})
	})

})
