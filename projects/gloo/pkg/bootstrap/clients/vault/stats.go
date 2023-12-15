package vault

import (
	"github.com/solo-io/gloo/pkg/utils"
	"go.opencensus.io/stats/view"
)

func init() {
	_ = view.Register(
		MLastLoginSuccessView,
		MLastLoginFailureView,
		MLoginSuccessesView,
		MLoginFailuresView,
	)
}

var (
	MLastLoginSuccess     = utils.Int64Measure("gloo.solo.io/vault/last_login_success", "Timestamp of last successful authentication of vault")
	MLastLoginSuccessView = utils.ViewForCounter(MLastLoginSuccess, view.LastValue())

	MLastLoginFailure     = utils.Int64Measure("gloo.solo.io/vault/last_login_failure", "Timestamp of last failed authentication of vault")
	MLastLoginFailureView = utils.ViewForCounter(MLastLoginFailure, view.LastValue())

	MLoginSuccesses     = utils.Int64Measure("gloo.solo.io/vault/login_successes", "Number of successful authentications of vault")
	MLoginSuccessesView = utils.ViewForCounter(MLoginSuccesses, view.Sum())

	MLoginFailures     = utils.Int64Measure("gloo.solo.io/vault/login_failures", "Number of failed authentications of vault")
	MLoginFailuresView = utils.ViewForCounter(MLoginFailures, view.Sum())
)
