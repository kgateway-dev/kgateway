package clients_test

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	. "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/file"
)

var _ = Describe("secrets", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc

		settings *v1.Settings

		tmpDir string
		err    error

		resourceClientParams = factory.NewResourceClientParams{
			ResourceType: &v1.Secret{},
			Token:        "",
		}
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		tmpDir, err = os.MkdirTemp("", "")
		Expect(err).NotTo(HaveOccurred())
	})
	AfterEach(func() {
		err = os.RemoveAll(tmpDir)
		Expect(err).NotTo(HaveOccurred())

		cancel()
	})
	getDirectorySource := func(dir string) *v1.Settings_Directory {
		return &v1.Settings_Directory{
			Directory: dir,
		}
	}

	getOptionsDirectorySource := func(dir string) *v1.Settings_SecretOptions_Source {
		return &v1.Settings_SecretOptions_Source{
			Source: &v1.Settings_SecretOptions_Source_Directory{
				Directory: getDirectorySource(dir),
			},
		}
	}
	When("called with SecretSource API", func() {
		BeforeEach(func() {
			settings = &v1.Settings{SecretSource: &v1.Settings_DirectorySecretSource{
				DirectorySecretSource: getDirectorySource(tmpDir),
			}}
		})
		It("does not return a multi client factory", func() {
			var f factory.ResourceClientFactory
			f, err = SecretFactoryForSettings(ctx, SecretFactoryParams{
				Settings: settings,
			})
			Expect(f).NotTo(BeAssignableToTypeOf(&MultiSecretResourceClientFactory{}))
		})
	})
	When("called with secretOptions API", func() {
		// we expect kube source to fail since we have nil settings that
		// are required for the kube client.
		getOptionsKubeSource := func() *v1.Settings_SecretOptions_Source {
			return &v1.Settings_SecretOptions_Source{
				Source: &v1.Settings_SecretOptions_Source_Kubernetes{
					Kubernetes: &v1.Settings_KubernetesSecrets{},
				},
			}
		}

		BeforeEach(func() {
			settings = &v1.Settings{SecretOptions: &v1.Settings_SecretOptions{
				Sources: []*v1.Settings_SecretOptions_Source{
					getOptionsDirectorySource(tmpDir),
				},
			}}
		})

		When("multiple sources are provided", func() {
			var (
				tmpDir2 string
			)
			BeforeEach(func() {
				tmpDir2, err = os.MkdirTemp("", "")
				Expect(err).NotTo(HaveOccurred())

				Expect(settings).NotTo(BeNil())
				secretOpts := settings.GetSecretOptions()
				secretOpts.Sources = append(secretOpts.Sources, getOptionsDirectorySource(tmpDir2))
				settings.SecretOptions = secretOpts
			})

			AfterEach(func() {
				err = os.RemoveAll(tmpDir2)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns a multi client", func() {
				var f factory.ResourceClientFactory
				f, err = SecretFactoryForSettings(ctx, SecretFactoryParams{
					Settings: settings,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(f).To(BeAssignableToTypeOf(&MultiSecretResourceClientFactory{}))

				c, err := f.NewResourceClient(ctx, resourceClientParams)
				Expect(err).NotTo(HaveOccurred())
				Expect(c).To(BeAssignableToTypeOf(&MultiSecretResourceClient{}))
			})
			When("a client is failing", func() {
				BeforeEach(func() {
					Expect(settings).NotTo(BeNil())
					secretOpts := settings.GetSecretOptions()
					secretOpts.Sources = append(secretOpts.Sources, getOptionsKubeSource())
					settings.SecretOptions = secretOpts
				})
				It("returns error", func() {
					f, err := SecretFactoryForSettings(ctx, SecretFactoryParams{
						Settings:   settings,
						PluralName: v1.SecretCrd.Plural,
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(f).To(BeAssignableToTypeOf(&MultiSecretResourceClientFactory{}))

					_, err = f.NewResourceClient(ctx, resourceClientParams)
					Expect(err).To(HaveOccurred())
				})
			})
		})
		When("a single source is provided", func() {
			It("returns a single client", func() {
				var f factory.ResourceClientFactory
				f, err = SecretFactoryForSettings(ctx, SecretFactoryParams{
					Settings: settings,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(f).To(BeAssignableToTypeOf(&MultiSecretResourceClientFactory{}))

				c, err := f.NewResourceClient(ctx, resourceClientParams)
				Expect(err).NotTo(HaveOccurred())
				Expect(c).To(BeAssignableToTypeOf(&file.ResourceClient{}))
			})
			When("the client is failing", func() {
				BeforeEach(func() {
					Expect(settings).NotTo(BeNil())
					secretOpts := settings.GetSecretOptions()
					secretOpts.Sources = []*v1.Settings_SecretOptions_Source{getOptionsKubeSource()}
					settings.SecretOptions = secretOpts
				})
				It("returns error", func() {
					f, err := SecretFactoryForSettings(ctx, SecretFactoryParams{
						Settings:   settings,
						PluralName: v1.SecretCrd.Plural,
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(f).To(BeAssignableToTypeOf(&MultiSecretResourceClientFactory{}))

					_, err = f.NewResourceClient(ctx, resourceClientParams)
					Expect(err).To(HaveOccurred())
				})
			})
		})

		It("returns an error when a nil source map is provided", func() {
			_, err := NewMultiSecretResourceClientFactory(MultiSecretFactoryParams{})
			Expect(err).To(MatchError(ErrNilSourceSlice))
		})
	})
})
