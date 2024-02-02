package vault_test

import (
	"context"
	"time"

	"github.com/avast/retry-go"
	"github.com/golang/mock/gomock"
	vault "github.com/hashicorp/vault/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	errors "github.com/rotisserie/eris"

	. "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients/vault"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients/vault/mocks"
	"github.com/solo-io/gloo/test/gomega/assertions"
)

type testWatcher struct {
	DoneChannel  chan error
	RenewChannel chan *vault.RenewOutput
}

func (t *testWatcher) DoneCh() <-chan error {
	return t.DoneChannel
}

func (t *testWatcher) RenewCh() <-chan *vault.RenewOutput {
	return t.RenewChannel
}

func (*testWatcher) Start() {}
func (*testWatcher) Stop()  {}

var _ = Describe("Vault Token Renewal Logic", func() {
	var (
		client  *vault.Client
		renewer *VaultTokenRenewer
		secret  *vault.Secret

		clientAuth ClientAuth
		ctrl       *gomock.Controller
		errMock    = errors.New("mocked error message")
		tw         TokenWatcher

		doneCh  chan error
		renewCh chan *vault.RenewOutput

		sleepTime = 100 * time.Millisecond

		renewableSecret = func() *vault.Secret {
			return &vault.Secret{
				Auth: &vault.SecretAuth{
					Renewable:   true,
					ClientToken: "test-token-renewable",
				},
				LeaseDuration: 100,
			}
		}

		nonRenewableSecret = func() *vault.Secret {
			return &vault.Secret{
				Auth: &vault.SecretAuth{
					Renewable:   false,
					ClientToken: "test-token-nonrenewable",
				},
				LeaseDuration: 100,
			}
		}
	)

	//nolint:unparam // required for signature
	var getTestWatcher = func(_ *vault.Client, _ *vault.Secret, _ int) (TokenWatcher, error) {
		return tw, nil
	}

	//nolint:unparam // required for signature
	var getErrorWatcher = func(_ *vault.Client, _ *vault.Secret, _ int) (TokenWatcher, error) {
		return nil, errMock
	}

	BeforeEach(func() {
		secret = renewableSecret()
		resetViews()

		doneCh = make(chan error, 1)
		renewCh = make(chan *vault.RenewOutput, 1)

		tw = &testWatcher{
			DoneChannel:  doneCh,
			RenewChannel: renewCh,
		}

	})

	When("Login always succeeds", func() {
		var (
			ctx    context.Context
			cancel context.CancelFunc
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			ctrl = gomock.NewController(GinkgoT())
			internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
			internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(secret, nil).AnyTimes()
			client = &vault.Client{}

			clientAuth = NewRemoteTokenAuth(internalAuthMethod, nil, retry.Attempts(3))

			renewer = NewVaultTokenRenewer(&NewVaultTokenRenewerParams{
				LeaseIncrement: 1,
				GetWatcher:     getTestWatcher,
			})

		})
		It("Renewal should work", func() {
			// Run through the basic channel output and look at the metrics
			go func() {
				time.Sleep(sleepTime)
				renewCh <- &vault.RenewOutput{}
				time.Sleep(sleepTime)
				doneCh <- errors.Errorf("Renewal error")
				time.Sleep(sleepTime)
				renewCh <- &vault.RenewOutput{}
				time.Sleep(sleepTime)
				cancel()
			}()

			renewer.ManageTokenRenewal(ctx, client, clientAuth, secret)
			// This kicks off the go rountine so we need to sleep to give it time to run
			time.Sleep(5 * sleepTime)

			assertions.ExpectStatLastValueMatches(MLastLoginFailure, BeZero())
			assertions.ExpectStatLastValueMatches(MLastLoginSuccess, Not(BeZero()))
			assertions.ExpectStatSumMatches(MLoginFailures, BeZero())
			assertions.ExpectStatSumMatches(MLoginSuccesses, Equal(1))
			assertions.ExpectStatLastValueMatches(MLastRenewFailure, Not(BeZero()))
			assertions.ExpectStatLastValueMatches(MLastRenewSuccess, Not(BeZero()))
			assertions.ExpectStatSumMatches(MRenewFailures, Equal(1))
			assertions.ExpectStatSumMatches(MRenewSuccesses, Equal(2))
		})
	})

	When("Login fails sometimes", func() {
		var (
			ctx    context.Context
			cancel context.CancelFunc
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			ctrl = gomock.NewController(GinkgoT())
			internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
			client = &vault.Client{}

			loginCount := 0
			// Fail every other login
			internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, _ *vault.Client) (*vault.Secret, error) {
				loginCount += 1
				if loginCount%2 == 0 {
					return secret, nil
				}

				return nil, errMock

			})

			clientAuth = NewRemoteTokenAuth(internalAuthMethod, nil, retry.Attempts(3))

			renewer = NewVaultTokenRenewer(&NewVaultTokenRenewerParams{
				LeaseIncrement: 1,
				GetWatcher:     getTestWatcher,
			})

		})

		It("should work with failures captured in metrics", func() {
			// Run through the basic channel output and look at the metrics
			go func() {
				time.Sleep(sleepTime)
				renewCh <- &vault.RenewOutput{}
				time.Sleep(sleepTime)
				doneCh <- errors.Errorf("Renewal error")
				time.Sleep(sleepTime)
				renewCh <- &vault.RenewOutput{}
				time.Sleep(1 * time.Second) // A little extra sleep to let logins retry
				cancel()
			}()

			renewer.ManageTokenRenewal(ctx, client, clientAuth, secret)
			// This kicks off the go rountine so we need to sleep to give it time to run
			time.Sleep(4*sleepTime + 1*time.Second)

			assertions.ExpectStatLastValueMatches(MLastLoginFailure, Not(BeZero()))
			assertions.ExpectStatLastValueMatches(MLastLoginSuccess, Not(BeZero()))
			assertions.ExpectStatSumMatches(MLoginFailures, Equal(1))
			assertions.ExpectStatSumMatches(MLoginSuccesses, Equal(1))
			assertions.ExpectStatLastValueMatches(MLastRenewFailure, Not(BeZero()))
			assertions.ExpectStatLastValueMatches(MLastRenewSuccess, Not(BeZero()))
			assertions.ExpectStatSumMatches(MRenewFailures, Equal(1))
			assertions.ExpectStatSumMatches(MRenewSuccesses, Equal(2))
		})
	})

	When("Login always fails", func() {
		var (
			ctx    context.Context
			cancel context.CancelFunc
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			ctrl = gomock.NewController(GinkgoT())
			internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
			client = &vault.Client{}

			internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(nil, errMock).AnyTimes()

			clientAuth = NewRemoteTokenAuth(internalAuthMethod, nil, retry.Attempts(3))

			renewer = NewVaultTokenRenewer(&NewVaultTokenRenewerParams{
				LeaseIncrement: 1,
				GetWatcher:     getTestWatcher,
			})

		})

		It("Should renew once then get stuck on the login failure", func() {
			// Run through the basic channel output and look at the metrics
			go func() {
				time.Sleep(sleepTime)
				renewCh <- &vault.RenewOutput{}
				time.Sleep(sleepTime)
				doneCh <- errors.Errorf("Renewal error")
				time.Sleep(sleepTime)
				renewCh <- &vault.RenewOutput{}
				time.Sleep(sleepTime)
				cancel()
			}()

			// Use the blocking version here so we can check the error
			err := renewer.RenewToken(ctx, client, clientAuth, secret)
			Expect(err).To(MatchError("unable to log in to auth method: login canceled: context canceled"))

			assertions.ExpectStatLastValueMatches(MLastLoginFailure, Not(BeZero()))
			assertions.ExpectStatLastValueMatches(MLastLoginSuccess, BeZero())
			assertions.ExpectStatSumMatches(MLoginFailures, Not(BeZero()))
			assertions.ExpectStatSumMatches(MLoginSuccesses, BeZero())
			assertions.ExpectStatLastValueMatches(MLastRenewFailure, Not(BeZero()))
			assertions.ExpectStatLastValueMatches(MLastRenewSuccess, Not(BeZero()))
			assertions.ExpectStatSumMatches(MRenewFailures, Equal(1))
			// We only get one success because we're blocked after the first failure
			assertions.ExpectStatSumMatches(MRenewSuccesses, Equal(1))
		})
	})

	When("There is a non-renewable token then the token is updated", func() {
		var (
			internalAuthMethod *mocks.MockAuthMethod
			ctx                context.Context
			cancel             context.CancelFunc
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			ctrl = gomock.NewController(GinkgoT())
			internalAuthMethod = mocks.NewMockAuthMethod(ctrl)
			client = &vault.Client{}

		})

		When("We leave time to wait for the token to be updated", func() {
			BeforeEach(func() {
				gomock.InOrder(
					internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Times(1).Return(nonRenewableSecret(), nil),
					internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).AnyTimes().Return(renewableSecret(), nil),
				)
				renewer = NewVaultTokenRenewer(&NewVaultTokenRenewerParams{
					LeaseIncrement:           1,
					GetWatcher:               getTestWatcher,
					RetryOnNonRenewableSleep: 1, // Pass this in so we don't have to wait for the default
				})

				clientAuth = NewRemoteTokenAuth(internalAuthMethod, nil, retry.Attempts(3))
			})

			It("should work when the secret is updated to be renewable", func() {

				// Run through the basic channel output and look at the metrics
				go func() {
					time.Sleep(sleepTime)
					doneCh <- errors.Errorf("Renewal error") // Force renewal
					time.Sleep(2 * time.Second)              // Give it time to retry the login
					renewCh <- &vault.RenewOutput{}
					time.Sleep(sleepTime)
					cancel()
				}()

				// Use the blocking version here so we can mirror the test below where an error does occur
				err := renewer.RenewToken(ctx, client, clientAuth, nonRenewableSecret())
				Expect(err).NotTo(HaveOccurred())

				// The login never fails, it just returns an non-renewable secret
				assertions.ExpectStatLastValueMatches(MLastLoginFailure, BeZero())
				assertions.ExpectStatLastValueMatches(MLastLoginSuccess, Not(BeZero()))
				assertions.ExpectStatSumMatches(MLoginFailures, BeZero())
				// Log in once for the first, passed in unrenewable secret,
				// then again for the unrenewable from the mocked response and then again for the success
				assertions.ExpectStatSumMatches(MLoginSuccesses, Equal(3))
				assertions.ExpectStatLastValueMatches(MLastRenewFailure, Not(BeZero()))
				assertions.ExpectStatLastValueMatches(MLastRenewSuccess, Not(BeZero()))
				assertions.ExpectStatSumMatches(MRenewFailures, Equal(1))
				assertions.ExpectStatSumMatches(MRenewSuccesses, Equal(1))
			})

		})

		// This is the same as the above test, but we set RetryOnNonRenewableSleep to a higher value
		// to validate that it is applied. In this case it should sleep for the default 60 seconds
		// which means that we will not have time to retry the login before the context is cancelled
		// and it will return faster than that
		When("We don't leave time for the sleep loop to finish", func() {
			BeforeEach(func() {
				ctx, cancel = context.WithCancel(context.Background())
				ctrl = gomock.NewController(GinkgoT())
				internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
				client = &vault.Client{}

				gomock.InOrder(
					internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Times(0).Return(nonRenewableSecret(), nil),
					internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).AnyTimes().Return(renewableSecret(), nil),
				)
				renewer = NewVaultTokenRenewer(&NewVaultTokenRenewerParams{
					LeaseIncrement: 1,
					GetWatcher:     getTestWatcher,
					// Default RetryOnNonRenewableSleep is fine because we are going to cancel the context before it finishes
				})

				clientAuth = NewRemoteTokenAuth(internalAuthMethod, nil, retry.Attempts(3))
			})

			It("should fail when RetryOnNonRenewableSleep is less than our sleep time ", func() {

				// Don't Give time for enough retries to get a renewable token
				// Keep the extra messages on the channels to make sure we don't get any extra metrics
				go func() {
					time.Sleep(sleepTime)
					doneCh <- errors.Errorf("Renewal error") // Force renewal
					time.Sleep(2 * time.Second)              // Not enough time to re-check the token
					renewCh <- &vault.RenewOutput{}
					time.Sleep(sleepTime)
					cancel()
				}()

				// Use the blocking version here so we can capture the error
				start := time.Now()
				err := renewer.RenewToken(ctx, client, clientAuth, nonRenewableSecret())
				elapsed := time.Since(start)
				Expect(elapsed).To(BeNumerically("<", 2500*time.Millisecond)) // 2.2 seconds plus a little extra. Much less than the default 60 seconds
				Expect(err).NotTo(HaveOccurred())

				// The login never fails, it just returns an non-renewable secret
				assertions.ExpectStatLastValueMatches(MLastLoginFailure, BeZero())
				assertions.ExpectStatLastValueMatches(MLastLoginSuccess, BeZero())
				assertions.ExpectStatSumMatches(MLoginFailures, BeZero())
				// We never get past the 'sleep' in the check for renewability so don't trigger any metrics
				assertions.ExpectStatSumMatches(MLoginSuccesses, BeZero())
				assertions.ExpectStatLastValueMatches(MLastRenewFailure, BeZero())
				assertions.ExpectStatLastValueMatches(MLastRenewSuccess, BeZero())
				assertions.ExpectStatSumMatches(MRenewFailures, BeZero())
				assertions.ExpectStatSumMatches(MRenewSuccesses, BeZero())
			})

		})

	})

	// This test is very similar to the previous test, but we are using a much longer sleep interval and we are calling
	// the non-blocking version of RenewToken. The sleep is long enough to ensure that the test has completed before the
	// timer expires, ensuring that if a go routine is leaked it won't closer before the test is done.
	When("There is a non-renewable token with a long RetryOnNonRenewableSleep", func() {
		var (
			ctx    context.Context
			cancel context.CancelFunc
		)
		When("We have a very large RetryOnNonRenewableSleep", func() {
			BeforeEach(func() {
				ctx, cancel = context.WithCancel(context.Background())
				ctrl = gomock.NewController(GinkgoT())
				internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
				client = &vault.Client{}
				gomock.InOrder(
					internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Times(0).Return(nonRenewableSecret(), nil),
				)
				renewer = NewVaultTokenRenewer(&NewVaultTokenRenewerParams{
					LeaseIncrement:           1,
					GetWatcher:               getTestWatcher,
					RetryOnNonRenewableSleep: 5000, //Really long
				})
				clientAuth = NewRemoteTokenAuth(internalAuthMethod, nil, retry.Attempts(3))
			})
			It("should not hang", func() {
				// Sleep for a second then cancel
				go func() {
					time.Sleep(1 * time.Second)
					cancel()
				}()
				// Use non-blocking version here so we can let the leak detector catch any leaks
				renewer.ManageTokenRenewal(ctx, client, clientAuth, nonRenewableSecret())
				time.Sleep(3*sleepTime + 2*time.Second)
				// The login never fails, it just returns an non-renewable secret
				assertions.ExpectStatLastValueMatches(MLastLoginFailure, BeZero())
				assertions.ExpectStatLastValueMatches(MLastLoginSuccess, BeZero())
				assertions.ExpectStatSumMatches(MLoginFailures, BeZero())
				// We never get past the 'sleep' in the check for renewability so don't trigger any metrics
				assertions.ExpectStatSumMatches(MLoginSuccesses, BeZero())
				assertions.ExpectStatLastValueMatches(MLastRenewFailure, BeZero())
				assertions.ExpectStatLastValueMatches(MLastRenewSuccess, BeZero())
				assertions.ExpectStatSumMatches(MRenewFailures, BeZero())
				assertions.ExpectStatSumMatches(MRenewSuccesses, BeZero())
			})
		})
	})

	When("Initializing the watcher returns an error", func() {
		var (
			ctx context.Context
		)
		BeforeEach(func() {
			ctx, _ = context.WithCancel(context.Background())
			ctrl = gomock.NewController(GinkgoT())
			internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
			internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(secret, nil).AnyTimes()
			client = &vault.Client{}

			clientAuth = NewRemoteTokenAuth(internalAuthMethod, nil, retry.Attempts(3))

			renewer = NewVaultTokenRenewer(&NewVaultTokenRenewerParams{
				LeaseIncrement: 1,
				GetWatcher:     getErrorWatcher,
			})

		})

		It("Renewal should return an error", func() {
			err := renewer.RenewToken(ctx, client, clientAuth, renewableSecret())
			Expect(err).To(MatchError(ErrInitializeWatcher(errMock)))

			// We didnt get a change to do anything, so all the metrics should be zero
			assertions.ExpectStatLastValueMatches(MLastLoginFailure, BeZero())
			assertions.ExpectStatLastValueMatches(MLastLoginSuccess, BeZero())
			assertions.ExpectStatSumMatches(MLoginFailures, BeZero())
			assertions.ExpectStatSumMatches(MLoginSuccesses, BeZero())
			assertions.ExpectStatLastValueMatches(MLastRenewFailure, BeZero())
			assertions.ExpectStatLastValueMatches(MLastRenewSuccess, BeZero())
			assertions.ExpectStatSumMatches(MRenewFailures, BeZero())
			assertions.ExpectStatSumMatches(MRenewSuccesses, BeZero())
		})
	})

})
