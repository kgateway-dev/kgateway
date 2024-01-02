package vault

// White box tests to avoid having to expose internal states only for testing

import (
	"reflect"

	vault "github.com/hashicorp/vault/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Constructor tests", func() {
	When("We call NewVaultTokenRenewer with default parameters", func() {

		It("Populates the expected default values", func() {
			renewer := NewVaultTokenRenewer(&NewVaultTokenRenewerParams{})

			expectedGetWatcher := reflect.ValueOf(vaultGetWatcher)
			instantiatedGetWatcher := reflect.ValueOf(renewer.getWatcher)

			Expect(expectedGetWatcher).To(Equal(instantiatedGetWatcher))
			Expect(renewer.leaseIncrement).To(Equal(0))
			Expect(renewer.retryOnNonRenewableSleep).To(Equal(60))
		})

	})

	When("We call NewVaultTokenRenewer with passed parameters", func() {

		It("Populates the passed values", func() {
			testGetWatcher := getWatcherFunc(func(_ *vault.Client, _ *vault.Secret, _ int) (TokenWatcher, error) {
				return nil, nil
			})

			renewer := NewVaultTokenRenewer(&NewVaultTokenRenewerParams{
				GetWatcher:               testGetWatcher,
				LeaseIncrement:           1,
				RetryOnNonRenewableSleep: 2,
			})

			expectedGetWatcher := reflect.ValueOf(testGetWatcher)
			instantiatedGetWatcher := reflect.ValueOf(renewer.getWatcher)

			Expect(expectedGetWatcher).To(Equal(instantiatedGetWatcher))
			Expect(renewer.leaseIncrement).To(Equal(1))
			Expect(renewer.retryOnNonRenewableSleep).To(Equal(2))
		})

	})

})
