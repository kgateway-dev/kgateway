package clients_test

import (
	"context"

	"github.com/solo-io/gloo/test/testutils"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	. "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients"
)

var _ = Describe("consul", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		// Requires Consul instance. The ConsulFactory will first attempt to
		// use a consul binary on the test executor's $PATH, then will fallback
		// to running a consul docker image and copying the binary from that.
		testutils.ValidateRequirementsAndNotifyGinkgo(testutils.Consul())

		ctx, cancel = context.WithCancel(context.Background())

		consulInstance = consulFactory.MustConsulInstanceWithClient()
		err := consulInstance.Run(ctx)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cancel() // this will clean up the consulInstance
	})

	Context("artifacts as consul key value", func() {

		var (
			artifactClient v1.ArtifactClient
		)

		BeforeEach(func() {
			value, err := protoutils.MarshalYAML(&v1.Artifact{
				Data:     map[string]string{"hi": "bye"},
				Metadata: &core.Metadata{Name: "name", Namespace: "namespace"},
			})
			Expect(err).NotTo(HaveOccurred())

			err = consulInstance.Put("gloo/gloo.solo.io/v1/Artifact/namespace/name", value)
			Expect(err).NotTo(HaveOccurred())

			settings := &v1.Settings{
				ArtifactSource: &v1.Settings_ConsulKvArtifactSource{
					ConsulKvArtifactSource: &v1.Settings_ConsulKv{
						RootKey: "gloo",
					},
				},
			}

			factory, err := ArtifactFactoryForSettings(ctx,
				settings,
				nil,
				nil,
				nil,
				nil,
				consulInstance.Client(),
				"artifacts")
			Expect(err).NotTo(HaveOccurred())
			artifactClient, err = v1.NewArtifactClient(ctx, factory)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			cancel()
		})

		It("should work with artifacts", func() {
			artifact, err := artifactClient.Read("namespace", "name", clients.ReadOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())
			Expect(artifact.GetMetadata().Name).To(Equal("name"))
			Expect(artifact.Data["hi"]).To(Equal("bye"))
		})
	})
})
