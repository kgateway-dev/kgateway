package vault_test

import (
	"testing"

	"github.com/solo-io/gloo/test/gomega/assertions"
	"github.com/solo-io/gloo/test/helpers"
	"go.opencensus.io/stats/view"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients/vault"
)

func TestVaultClient(t *testing.T) {
	leakDetector := helpers.DeferredGoroutineLeakDetector(t)
	defer leakDetector()

	RegisterFailHandler(Fail)
	RunSpecs(t, "Vault Client Suite")
}

var _ = BeforeSuite(func() {
	resetViews()
})

var _ = AfterSuite(func() {
	resetViews()
})

// resetViews resets the views used in this package
// this is useful for ensuring that we are not leaking metrics between tests within this package
// or between this package and other packages
func resetViews() {
	views := []*view.View{
		MLastLoginSuccessView,
		MLoginFailuresView,
		MLoginSuccessesView,
		MLastLoginFailureView,
		MLastRenewFailureView,
		MLastRenewSuccessView,
		MRenewFailuresView,
		MRenewSuccessesView,
	}
	view.Unregister(views...)
	_ = view.Register(views...)
	assertions.ExpectStatLastValueMatches(MLastLoginSuccess, BeZero())
	assertions.ExpectStatLastValueMatches(MLastLoginFailure, BeZero())
	assertions.ExpectStatSumMatches(MLoginSuccesses, BeZero())
	assertions.ExpectStatSumMatches(MLoginFailures, BeZero())
	assertions.ExpectStatLastValueMatches(MLastRenewFailure, BeZero())
	assertions.ExpectStatLastValueMatches(MLastRenewSuccess, BeZero())
	assertions.ExpectStatSumMatches(MRenewFailures, BeZero())
	assertions.ExpectStatSumMatches(MRenewSuccesses, BeZero())
}
