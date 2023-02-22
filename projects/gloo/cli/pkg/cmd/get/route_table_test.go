package get_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/samples"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Get RouteTable", func() {
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
	AfterEach(func() {
		helpers.UseDefaultClients()
		cancel()
	})
	It("gets the route table list", func() {
		rt := helpers.MustRouteTableClient(ctx)
		_, rts := samples.LinkedRouteTablesWithVirtualService("vs", defaults.GlooSystem)
		_, err := rt.Write(rts[0], clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		out, err := testutils.GlooctlOut("get rt")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring(`+-------------+--------------------------------+---------+
| ROUTE TABLE |             ROUTES             | STATUS  |
+-------------+--------------------------------+---------+
| node-0      | testRouteName: /root/0 ->      | Pending |
|             | gloo-system.node-1 (route      |         |
|             | table)                         |         |
+-------------+--------------------------------+---------+`))
	})
})
