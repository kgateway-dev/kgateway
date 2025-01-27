package get_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/get"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Root", func() {

	emptyFlagsMsg := fmt.Sprintf("Error: %s", get.EmptyGetError.Error())

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

	Context("Empty args and flags", func() {
		It("should give clear error message", func() {
			msg, err := testutils.GlooctlOut("get")
			Expect(msg).To(ContainSubstring(emptyFlagsMsg))
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(get.EmptyGetError))
		})
	})

	Context("Unset Gloo namespace", func() {
		It("should give clear error message", func() {
			err := helpers.MustKubeClient().CoreV1().Namespaces().Delete(ctx, defaults.GlooSystem, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())
			_, err = testutils.GlooctlOut("get upstreams")
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(get.UnsetNamespaceError))
		})
	})
})
