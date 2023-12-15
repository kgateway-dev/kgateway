package clients_test

import (
	"context"
	"time"

	"github.com/avast/retry-go"
	"github.com/golang/mock/gomock"
	"github.com/hashicorp/vault/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients/vault"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients/vault/mocks"
	"github.com/solo-io/gloo/test/gomega/assertions"
	"go.opencensus.io/stats/view"

	. "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients"
)

var _ = Describe("ClientAuth", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc

		clientAuth vault.ClientAuth
		ctrl       *gomock.Controller
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		// The tests below will be responsible for assigning this variable
		// We re-set it here, just to be safe
		clientAuth = nil

		// We should not have any metrics set before running the tests
		// This ensures that we are no leaking metrics between tests
		resetViews()
	})

	JustBeforeEach(func() {
		Expect(clientAuth).NotTo(BeNil())
	})

	AfterEach(func() {
		cancel()
	})

	Context("Access Token Auth", func() {
		// These tests validate the behavior of the StaticTokenAuth implementation of the ClientAuth interface

		When("token is empty", func() {

			BeforeEach(func() {
				var err error
				vaultSettings := &v1.Settings_VaultSecrets{
					AuthMethod: &v1.Settings_VaultSecrets_AccessToken{
						AccessToken: "",
					},
				}
				clientAuth, err = vault.ClientAuthFactory(vaultSettings)
				Expect(err).NotTo(HaveOccurred())
			})

			It("login should return an error", func() {
				secret, err := clientAuth.Login(ctx, nil)
				Expect(err).To(MatchError(vault.ErrEmptyToken))
				Expect(secret).To(BeNil())

				assertions.ExpectStatLastValueMatches(vault.MLastLoginFailure, Not(BeZero()))
				assertions.ExpectStatSumMatches(vault.MLoginFailures, Equal(1))
			})

			// It("startRenewal should return nil", func() {
			// 	err := clientAuth.StartRenewal(ctx, nil)
			// 	Expect(err).NotTo(HaveOccurred())
			// })

		})

		When("token is not empty", func() {

			BeforeEach(func() {
				vaultSettings := &v1.Settings_VaultSecrets{
					AuthMethod: &v1.Settings_VaultSecrets_AccessToken{
						AccessToken: "placeholder",
					},
				}

				clientAuth, _ = vault.ClientAuthFactory(vaultSettings)
			})

			It("should return a vault.Secret", func() {
				secret, err := clientAuth.Login(ctx, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(secret).To(Equal(&api.Secret{
					Auth: &api.SecretAuth{
						ClientToken: "placeholder",
					},
				}))
				assertions.ExpectStatLastValueMatches(vault.MLastLoginSuccess, Not(BeZero()))
				assertions.ExpectStatSumMatches(vault.MLoginSuccesses, Equal(1))
			})

			// It("startRenewal should return nil", func() {
			// 	err := clientAuth.StartRenewal(ctx, nil)
			// 	Expect(err).NotTo(HaveOccurred())
			// })

		})

	})

	Context("NewRemoteTokenAuth", func() {
		// These tests validate the behavior of the RemoteTokenAuth implementation of the ClientAuth interface

		When("internal auth method always returns an error", func() {

			BeforeEach(func() {
				ctrl = gomock.NewController(GinkgoT())
				internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
				internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(nil, eris.New("mocked error message")).AnyTimes()

				clientAuth = vault.NewRemoteTokenAuth(internalAuthMethod, retry.Attempts(3))
			})

			It("should return the error", func() {
				secret, err := VaultClientForSettings(ctx, nil, clientAuth)
				Expect(err).To(MatchError("unable to log in to auth method: unable to authenticate to vault: mocked error message"))
				Expect(secret).To(BeNil())
			})

		})

		When("internal auth method returns an error, and then a success", func() {

			BeforeEach(func() {
				ctrl = gomock.NewController(GinkgoT())
				internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
				internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(nil, eris.New("error")).Times(1)
				internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(&api.Secret{
					Auth: &api.SecretAuth{
						ClientToken: "a-client-token",
					},
				}, nil).Times(1)

				clientAuth = vault.NewRemoteTokenAuth(internalAuthMethod, retry.Attempts(5))
			})

			It("should return a client", func() {
				client, err := VaultClientForSettings(ctx, nil, clientAuth)
				Expect(err).NotTo(HaveOccurred())
				Expect(client).ToNot((BeNil()))

				assertions.ExpectStatLastValueMatches(vault.MLastLoginFailure, Not(BeZero()))
				assertions.ExpectStatLastValueMatches(vault.MLastLoginSuccess, Not(BeZero()))
				assertions.ExpectStatSumMatches(vault.MLoginFailures, Equal(1))
				assertions.ExpectStatSumMatches(vault.MLoginSuccesses, Equal(1))
			})
		})

	})

	When("context is cancelled before login succeeds", func() {

		retryAttempts := uint(5)
		BeforeEach(func() {
			ctrl = gomock.NewController(GinkgoT())
			internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
			// The auth method will return an error twice, and then a success
			// but we plan on cancelling the context before the success
			internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(nil, eris.New("error")).AnyTimes()
			clientAuth = vault.NewRemoteTokenAuth(internalAuthMethod, retry.Attempts(retryAttempts))
		})

		It("should return a context error", func() {
			go func() {
				time.Sleep(2 * time.Second)
				cancel()
			}()

			client, err := VaultClientForSettings(ctx, nil, clientAuth)
			Expect(err).To(MatchError("unable to log in to auth method: Login canceled: context canceled"))
			Expect(client).To(BeNil())

			assertions.ExpectStatLastValueMatches(vault.MLastLoginFailure, Not(BeZero()))
			assertions.ExpectStatLastValueMatches(vault.MLastLoginSuccess, BeZero())
			// Validate that the number of login failures is less than the number of retry attempts
			// This means we stopped the login attempts before they were exhausted
			assertions.ExpectStatSumMatches(vault.MLoginFailures, BeNumerically("<", retryAttempts))
			assertions.ExpectStatSumMatches(vault.MLoginSuccesses, BeZero())

		})

	})

})

func resetViews() {
	views := []*view.View{
		vault.MLastLoginFailureView,
		vault.MLastLoginSuccessView,
		vault.MLoginFailuresView,
		vault.MLoginSuccessesView,
	}
	view.Unregister(views...)
	_ = view.Register(views...)
	assertions.ExpectStatLastValueMatches(vault.MLastLoginSuccess, BeZero())
	assertions.ExpectStatLastValueMatches(vault.MLastLoginFailure, BeZero())
	assertions.ExpectStatSumMatches(vault.MLoginSuccesses, BeZero())
	assertions.ExpectStatSumMatches(vault.MLoginFailures, BeZero())
}
