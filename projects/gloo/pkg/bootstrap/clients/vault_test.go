package clients_test

import (
	"context"

	"github.com/avast/retry-go"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients/vault"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients/vault/mocks"
	"github.com/solo-io/gloo/test/gomega/assertions"
	"go.opencensus.io/stats/view"
)

var _ = Describe("ClientAuth", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc

		clientAuth vault.ClientAuth
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

	AfterEach(func() {
		cancel()
	})

	// Context("newStaticTokenAuth", func() {
	// 	// These tests validate the behavior of the staticTokenAuth implementation of the ClientAuth interface

	// 	When("token is empty", func() {

	// 		BeforeEach(func() {
	// 			clientAuth = newStaticTokenAuth("")
	// 		})

	// 		It("login should return an error", func() {
	// 			secret, err := clientAuth.Login(ctx, nil)
	// 			Expect(err).To(MatchError(ErrEmptyToken))
	// 			Expect(secret).To(BeNil())

	// 			assertions.ExpectStatLastValueMatches(MLastLoginFailure, Not(BeZero()))
	// 			assertions.ExpectStatSumMatches(MLoginFailures, Equal(1))
	// 		})

	// 		It("startRenewal should return nil", func() {
	// 			err := clientAuth.StartRenewal(ctx, nil)
	// 			Expect(err).NotTo(HaveOccurred())
	// 		})

	// 	})

	// 	When("token is not empty", func() {

	// 		BeforeEach(func() {
	// 			clientAuth = newStaticTokenAuth("placeholder")
	// 		})

	// 		It("should return a vault.Secret", func() {
	// 			secret, err := clientAuth.Login(ctx, nil)
	// 			Expect(err).NotTo(HaveOccurred())
	// 			Expect(secret).To(Equal(&api.Secret{
	// 				Auth: &api.SecretAuth{
	// 					ClientToken: "placeholder",
	// 				},
	// 			}))
	// 			assertions.ExpectStatLastValueMatches(MLastLoginSuccess, Not(BeZero()))
	// 			assertions.ExpectStatSumMatches(MLoginSuccesses, Equal(1))
	// 		})

	// 		It("startRenewal should return nil", func() {
	// 			err := clientAuth.StartRenewal(ctx, nil)
	// 			Expect(err).NotTo(HaveOccurred())
	// 		})

	// 	})

	// })

	Context("newRemoteTokenAuth", func() {
		// These tests validate the behavior of the remoteTokenAuth implementation of the ClientAuth interface

		When("internal auth method always returns an error", func() {

			BeforeEach(func() {
				ctrl := gomock.NewController(GinkgoT())
				internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
				internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(nil, eris.New("mocked error message")).AnyTimes()

				clientAuth = newRemoteTokenAuth(internalAuthMethod, retry.Attempts(3))
			})

			It("should return the error", func() {
				secret, err := clientAuth.Login(ctx, nil)
				Expect(err).To(MatchError("unable to authenticate to vault: mocked error message"))
				Expect(secret).To(BeNil())
			})

		})

		// When("internal auth method returns an error, and then a success", func() {

		// 	BeforeEach(func() {
		// 		ctrl := gomock.NewController(GinkgoT())
		// 		internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
		// 		internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(nil, eris.New("error")).Times(1)
		// 		internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(&api.Secret{
		// 			Auth: &api.SecretAuth{
		// 				ClientToken: "a-client-token",
		// 			},
		// 		}, nil).Times(1)

		// 		clientAuth = newRemoteTokenAuth(internalAuthMethod, retry.Attempts(5))
		// 	})

		// It("should return a secret", func() {
		// 	secret, err := clientAuth.Login(ctx, nil)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	Expect(secret.Auth.ClientToken).To(Equal("a-client-token"))

		// 	assertions.ExpectStatLastValueMatches(vault.MLastLoginFailure, Not(BeZero()))
		// 	assertions.ExpectStatLastValueMatches(vault.MLastLoginSuccess, Not(BeZero()))
		// 	assertions.ExpectStatSumMatches(vault.MLoginFailures, Equal(1))
		// 	assertions.ExpectStatSumMatches(vault.MLoginSuccesses, Equal(1))
		// })

	})

	// When("context is cancelled before login succeeds", func() {
	// 	BeforeEach(func() {
	// 		ctrl := gomock.NewController(GinkgoT())
	// 		internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
	// 		// The auth method will return an error twice, and then a success
	// 		// but we plan on cancelling the context before the success
	// 		internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(nil, eris.New("error")).Times(2)
	// 		internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(&api.Secret{
	// 			Auth: &api.SecretAuth{
	// 				ClientToken: "a-client-token",
	// 			},
	// 		}, nil).AnyTimes()
	// 		clientAuth = newRemoteTokenAuth(internalAuthMethod, retry.Attempts(5))
	// 	})

	// 	It("should return a context error", func() {
	// 		go func() {
	// 			time.Sleep(2 * time.Second)
	// 			cancel()
	// 		}()

	// 		secret, err := clientAuth.Login(ctx, nil)
	// 		Expect(err).To(MatchError("Login canceled: context canceled"))
	// 		Expect(secret).To(BeNil())

	// 		assertions.ExpectStatLastValueMatches(MLastLoginFailure, Not(BeZero()))
	// 		assertions.ExpectStatLastValueMatches(MLastLoginSuccess, BeZero())
	// 		assertions.ExpectStatSumMatches(MLoginFailures, Equal(2))

	// 	})

	// })

	// })

})

func resetViews() {
	views := []*view.View{
		vault.MLastLoginFailureView,
		vault.MLastLoginSuccessView,
		vault.MLoginFailuresView,
		vault.MLoginSuccessesView,
		vault.MLastLoginFailureView,
	}
	view.Unregister(views...)
	_ = view.Register(views...)
	assertions.ExpectStatLastValueMatches(vault.MLastLoginSuccess, BeZero())
	assertions.ExpectStatLastValueMatches(vault.MLastLoginFailure, BeZero())
	assertions.ExpectStatSumMatches(vault.MLoginSuccesses, BeZero())
	assertions.ExpectStatSumMatches(vault.MLoginFailures, BeZero())
}
