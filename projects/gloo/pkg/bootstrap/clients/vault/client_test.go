package vault_test

import (
	"context"
	"time"

	"github.com/avast/retry-go"
	"github.com/golang/mock/gomock"
	vaultapi "github.com/hashicorp/vault/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	. "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients/vault"

	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients/vault/mocks"
	"github.com/solo-io/gloo/test/gomega/assertions"
)

// testRenewal is a mock implementation of the Renewer interface.
// It has the 'calledTimes' field to track whether or not it has been called
type testRenewal struct {
	calledTimes int
}

func (t *testRenewal) ManageTokenRenewal(ctx context.Context, client *vaultapi.Client, clientAuth ClientAuth, secret *vaultapi.Secret) {
	t.calledTimes += 1
}

func (t *testRenewal) TimesCalled() int {
	return t.calledTimes
}

var _ = Describe("ClientAuth", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc

		clientAuth   ClientAuth
		ctrl         *gomock.Controller
		tokenRenewer *testRenewal
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
				clientAuth, err = ClientAuthFactory(vaultSettings)
				Expect(err).NotTo(HaveOccurred())
			})

			It("login should return an error", func() {
				secret, err := clientAuth.Login(ctx, nil)
				Expect(err).To(MatchError(ErrEmptyToken))
				Expect(secret).To(BeNil())

				assertions.ExpectStatLastValueMatches(MLastLoginFailure, Not(BeZero()))
				assertions.ExpectStatSumMatches(MLoginFailures, Equal(1))
			})

		})

		When("token is not empty", func() {

			BeforeEach(func() {
				vaultSettings := &v1.Settings_VaultSecrets{
					AuthMethod: &v1.Settings_VaultSecrets_AccessToken{
						AccessToken: "placeholder",
					},
				}
				var err error
				clientAuth, err = ClientAuthFactory(vaultSettings)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return a Secret", func() {
				secret, err := clientAuth.Login(ctx, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(secret).To(Equal(&vaultapi.Secret{
					Auth: &vaultapi.SecretAuth{
						ClientToken: "placeholder",
					},
				}))
				assertions.ExpectStatLastValueMatches(MLastLoginSuccess, Not(BeZero()))
				assertions.ExpectStatSumMatches(MLoginSuccesses, Equal(1))
			})

		})

	})

	Context("NewRemoteTokenAuth", func() {
		// These tests validate the behavior of the RemoteTokenAuth implementation of the ClientAuth interface

		When("internal auth method always returns an error", func() {

			BeforeEach(func() {
				ctrl = gomock.NewController(GinkgoT())
				internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
				internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(nil, eris.New("mocked error message")).AnyTimes()

				tokenRenewer = &testRenewal{}
				clientAuth = NewRemoteTokenAuth(internalAuthMethod, &testRenewal{}, retry.Attempts(3))
			})

			It("should return the error", func() {
				secret, err := NewAuthenticatedClient(ctx, nil, clientAuth)
				Expect(err).To(MatchError("unable to log in to auth method: unable to authenticate to vault: mocked error message"))
				Expect(secret).To(BeNil())

				// We had an error, so don't expect renewal to be started
				Expect(tokenRenewer.TimesCalled()).To(Equal(0))

				assertions.ExpectStatLastValueMatches(MLastLoginFailure, Not(BeZero()))
				assertions.ExpectStatLastValueMatches(MLastLoginSuccess, BeZero())
				assertions.ExpectStatSumMatches(MLoginFailures, Equal(3))
				assertions.ExpectStatSumMatches(MLoginSuccesses, Equal(0))
			})

		})

		When("internal auth method returns an error, and then a success", func() {
			var (
				client *vaultapi.Client
				err    error
			)

			BeforeEach(func() {
				secret := &vaultapi.Secret{
					Auth: &vaultapi.SecretAuth{
						ClientToken: "a-client-token",
					},
				}

				ctrl = gomock.NewController(GinkgoT())
				internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
				internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(nil, eris.New("error")).Times(1)
				internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(secret, nil).Times(1)

				tokenRenewer = &testRenewal{}
				clientAuth = NewRemoteTokenAuth(internalAuthMethod, tokenRenewer, retry.Attempts(5))
			})

			It("should return a client", func() {
				client, err = NewAuthenticatedClient(ctx, nil, clientAuth)
				Expect(err).NotTo(HaveOccurred())
				Expect(client).ToNot((BeNil()))
				Expect(tokenRenewer.TimesCalled()).To(Equal(1))

				assertions.ExpectStatLastValueMatches(MLastLoginFailure, Not(BeZero()))
				assertions.ExpectStatLastValueMatches(MLastLoginSuccess, Not(BeZero()))
				assertions.ExpectStatSumMatches(MLoginFailures, Equal(1))
				assertions.ExpectStatSumMatches(MLoginSuccesses, Equal(1))
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
			tokenRenewer = &testRenewal{}
			clientAuth = NewRemoteTokenAuth(internalAuthMethod, tokenRenewer, retry.Attempts(retryAttempts))
		})

		It("should return a context error", func() {
			go func() {
				time.Sleep(2 * time.Second)
				cancel()
			}()

			client, err := NewAuthenticatedClient(ctx, nil, clientAuth)
			Expect(err).To(MatchError("unable to log in to auth method: login canceled: context canceled"))
			Expect(client).To(BeNil())
			Expect(tokenRenewer.TimesCalled()).To(Equal(0))

			assertions.ExpectStatLastValueMatches(MLastLoginFailure, Not(BeZero()))
			assertions.ExpectStatLastValueMatches(MLastLoginSuccess, BeZero())
			// Validate that the number of login failures is less than the number of retry attempts
			// This means we stopped the login attempts before they were exhausted
			assertions.ExpectStatSumMatches(MLoginFailures, BeNumerically("<", retryAttempts))
			assertions.ExpectStatSumMatches(MLoginSuccesses, BeZero())

		})

	})

})
