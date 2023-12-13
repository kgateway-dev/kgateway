package vault_test

import (
	"context"
	"reflect"
	"time"

	"github.com/avast/retry-go"
	"github.com/golang/mock/gomock"
	"github.com/hashicorp/vault/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	. "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients/vault"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients/vault/mocks"
	"github.com/solo-io/gloo/test/gomega/assertions"
)

var _ = Describe("ClientAuth", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc

		clientAuth ClientAuth
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

	Context("NewStaticTokenAuth", func() {
		// These tests validate the behavior of the StaticTokenAuth implementation of the ClientAuth interface

		When("token is empty", func() {

			BeforeEach(func() {
				clientAuth = NewStaticTokenAuth("")
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
				clientAuth = NewStaticTokenAuth("placeholder")
			})

			It("should return a vault.Secret", func() {
				secret, err := clientAuth.Login(ctx, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(secret).To(Equal(&api.Secret{
					Auth: &api.SecretAuth{
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
				ctrl := gomock.NewController(GinkgoT())
				internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
				internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(nil, eris.New("mocked error message")).AnyTimes()

				clientAuth = NewRemoteTokenAuth(internalAuthMethod, retry.Attempts(3))
			})

			It("should return the error", func() {
				secret, err := clientAuth.Login(ctx, nil)
				Expect(err).To(MatchError("unable to authenticate to vault: mocked error message"))
				Expect(secret).To(BeNil())

				assertions.ExpectStatLastValueMatches(MLastLoginFailure, Not(BeZero()))
				assertions.ExpectStatSumMatches(MLoginFailures, Equal(3))
			})

		})

		When("internal auth method returns an error, and then a success", func() {

			BeforeEach(func() {
				ctrl := gomock.NewController(GinkgoT())
				internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
				internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(nil, eris.New("error")).Times(1)
				internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(&api.Secret{
					Auth: &api.SecretAuth{
						ClientToken: "a-client-token",
					},
				}, nil).Times(1)

				clientAuth = NewRemoteTokenAuth(internalAuthMethod, retry.Attempts(5))
			})

			It("should return a secret", func() {
				secret, err := clientAuth.Login(ctx, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(secret.Auth.ClientToken).To(Equal("a-client-token"))

				assertions.ExpectStatLastValueMatches(MLastLoginFailure, Not(BeZero()))
				assertions.ExpectStatLastValueMatches(MLastLoginSuccess, Not(BeZero()))
				assertions.ExpectStatSumMatches(MLoginFailures, Equal(1))
				assertions.ExpectStatSumMatches(MLoginSuccesses, Equal(1))
			})

		})

		When("context is cancelled before login succeeds", func() {
			BeforeEach(func() {
				ctrl := gomock.NewController(GinkgoT())
				internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
				// The auth method will return an error twice, and then a success
				// but we plan on cancelling the context before the success
				internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(nil, eris.New("error")).Times(2)
				internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(&api.Secret{
					Auth: &api.SecretAuth{
						ClientToken: "a-client-token",
					},
				}, nil).AnyTimes()
				clientAuth = NewRemoteTokenAuth(internalAuthMethod, retry.Attempts(5))
			})

			It("should return a context error", func() {
				go func() {
					time.Sleep(2 * time.Second)
					cancel()
				}()

				secret, err := clientAuth.Login(ctx, nil)
				Expect(err).To(MatchError("Login canceled: context canceled"))
				Expect(secret).To(BeNil())

				assertions.ExpectStatLastValueMatches(MLastLoginFailure, Not(BeZero()))
				assertions.ExpectStatLastValueMatches(MLastLoginSuccess, BeZero())
				assertions.ExpectStatSumMatches(MLoginFailures, Equal(2))

			})

		})

	})

	Context("NewClientAuth", func() {
		// These tests validate that the constructor maps the Gloo Settings into the appropriate ClientAuth interface
		// it does not test the underlying implementations, as those are handled in the above tests

		DescribeTable("should error on invalid inputs",
			func(vaultSettings *v1.Settings_VaultSecrets, expectedError types.GomegaMatcher) {
				clientAuth, err := ClientAuthFactory(vaultSettings)
				Expect(err).To(expectedError)
				Expect(clientAuth).To(BeNil())
			},
			Entry("partial accessKey / secretAccessKey", &v1.Settings_VaultSecrets{
				AuthMethod: &v1.Settings_VaultSecrets_Aws{
					Aws: &v1.Settings_VaultAwsAuth{
						AccessKeyId:     "access-key-id",
						SecretAccessKey: "",
					},
				},
			}, MatchError("only partial credentials were provided for AWS IAM auth: secret access key must be defined for AWS IAM auth")),
		)

		DescribeTable("should return the correct client auth",
			func(vaultSettings *v1.Settings_VaultSecrets, expectedClientAuth ClientAuth) {
				clientAuth, err := ClientAuthFactory(vaultSettings)
				Expect(err).NotTo(HaveOccurred())

				actualClientAuthType := reflect.ValueOf(clientAuth).Type()
				expectedClientAuthType := reflect.ValueOf(expectedClientAuth).Type()
				Expect(expectedClientAuthType).To(Equal(actualClientAuthType))
			},
			Entry("nil", nil, &StaticTokenAuth{}),
			Entry("nil auth method", &v1.Settings_VaultSecrets{
				AuthMethod: nil,
			}, &StaticTokenAuth{}),
			Entry("access token auth method", &v1.Settings_VaultSecrets{
				AuthMethod: &v1.Settings_VaultSecrets_AccessToken{
					AccessToken: "access-token",
				},
			}, NewStaticTokenAuth("access-token")),
			Entry("aws auth method", &v1.Settings_VaultSecrets{
				AuthMethod: &v1.Settings_VaultSecrets_Aws{
					Aws: &v1.Settings_VaultAwsAuth{},
				},
			}, &RemoteTokenAuth{}),
		)

	})

})
