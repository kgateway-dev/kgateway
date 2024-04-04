package setup_test

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	skerrors "github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/statusutils"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("ControlPlane", func() {

	Context("xds host", func() {

		AfterEach(func() {
			err := os.Unsetenv(statusutils.PodNamespaceEnvName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("respects POD_NAMESPACE", func() {
			err := os.Setenv(statusutils.PodNamespaceEnvName, "other-ns")
			Expect(err).NotTo(HaveOccurred())
			xdsHost := setup.GetControlPlaneXdsHost()
			Expect(xdsHost).To(Equal("gloo.other-ns.svc.cluster.local"))
		})

		It("uses default value when POD_NAMESPACE not set", func() {
			xdsHost := setup.GetControlPlaneXdsHost()
			Expect(xdsHost).To(Equal("gloo.gloo-system.svc.cluster.local"))
		})
	})

	Context("xds port", func() {

		var (
			ctx       context.Context
			svcClient skkube.ServiceClient
			err       error

			svc1 *skkube.Service
			svc2 *skkube.Service
		)

		BeforeEach(func() {
			ctx = context.Background()
			inMemoryFactory := &factory.MemoryResourceClientFactory{
				Cache: memory.NewInMemoryResourceCache(),
			}
			svcClient, err = skkube.NewServiceClient(ctx, inMemoryFactory)
			Expect(err).NotTo(HaveOccurred())

			svc1 = skkube.NewService("gloo-system", kubeutils.GlooServiceName)
			svc1.Spec = corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name: kubeutils.GlooXdsPortName,
						Port: 1111,
					},
				},
			}

			svc2 = skkube.NewService("other-ns", kubeutils.GlooServiceName)
			svc2.Spec = corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name: kubeutils.GlooXdsPortName,
						Port: 2222,
					},
				},
			}
		})

		AfterEach(func() {
			err := os.Unsetenv(statusutils.PodNamespaceEnvName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns xds port from gloo service in default namespace", func() {
			// write both services
			_, err = svcClient.Write(svc1, clients.WriteOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())
			_, err = svcClient.Write(svc2, clients.WriteOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())

			// should return the port from gloo service in gloo-system
			port, err := setup.GetControlPlaneXdsPort(ctx, svcClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(port).To(Equal(int32(1111)))
		})

		It("returns xds port from gloo service in POD_NAMESPACE namespace", func() {
			err := os.Setenv(statusutils.PodNamespaceEnvName, "other-ns")
			Expect(err).NotTo(HaveOccurred())

			// write both services
			_, err = svcClient.Write(svc1, clients.WriteOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())
			_, err = svcClient.Write(svc2, clients.WriteOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())

			// should return the port from gloo service in other-ns
			port, err := setup.GetControlPlaneXdsPort(ctx, svcClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(port).To(Equal(int32(2222)))
		})

		It("returns error when no gloo service exists in the POD_NAMESPACE namespace", func() {
			err := os.Setenv(statusutils.PodNamespaceEnvName, "other-ns")
			Expect(err).NotTo(HaveOccurred())

			// only write svc1
			_, err = svcClient.Write(svc1, clients.WriteOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())

			_, err = setup.GetControlPlaneXdsPort(ctx, svcClient)
			Expect(err).To(HaveOccurred())
			Expect(skerrors.IsNotExist(err)).To(BeTrue())
		})

		It("returns error when the expected port name is not found", func() {
			err := os.Setenv(statusutils.PodNamespaceEnvName, "other-ns")
			Expect(err).NotTo(HaveOccurred())

			// modify the port name
			svc2.Spec.Ports[0].Name = "test-name"
			_, err = svcClient.Write(svc2, clients.WriteOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())

			_, err = setup.GetControlPlaneXdsPort(ctx, svcClient)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(setup.NoXdsPortFoundError))
		})
	})

})
