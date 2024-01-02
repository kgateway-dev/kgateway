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
		MLastRenewSuccessView,
		MLastRenewFailureView,
		MRenewSuccessesView,
		MRenewFailuresView,
	)
}

var (
	// Login metrics
	MLastLoginSuccess     = utils.Int64Measure("gloo.solo.io/vault/last_login_success", "Timestamp of last successful authentication of vault")
	MLastLoginSuccessView = utils.ViewForCounter(MLastLoginSuccess, view.LastValue())

	MLastLoginFailure     = utils.Int64Measure("gloo.solo.io/vault/last_login_failure", "Timestamp of last failed authentication of vault")
	MLastLoginFailureView = utils.ViewForCounter(MLastLoginFailure, view.LastValue())

	MLoginSuccesses     = utils.Int64Measure("gloo.solo.io/vault/login_successes", "Number of successful authentications of vault")
	MLoginSuccessesView = utils.ViewForCounter(MLoginSuccesses, view.Sum())

	MLoginFailures     = utils.Int64Measure("gloo.solo.io/vault/login_failures", "Number of failed authentications of vault")
	MLoginFailuresView = utils.ViewForCounter(MLoginFailures, view.Sum())

	// Renew metrics
	MLastRenewSuccess     = utils.Int64Measure("gloo.solo.io/vault/last_renew_success", "Timestamp of last successful renewal of vault secret lease")
	MLastRenewSuccessView = utils.ViewForCounter(MLastRenewSuccess, view.LastValue())

	MLastRenewFailure     = utils.Int64Measure("gloo.solo.io/vault/last_renew_failure", "Timestamp of last failed renewal of vault secret lease")
	MLastRenewFailureView = utils.ViewForCounter(MLastRenewFailure, view.LastValue())

	MRenewSuccesses     = utils.Int64Measure("gloo.solo.io/vault/renew_successes", "Number of successful renewals of vault secret lease")
	MRenewSuccessesView = utils.ViewForCounter(MRenewSuccesses, view.Sum())

	MRenewFailures     = utils.Int64Measure("gloo.solo.io/vault/renew_failures", "Number of failed renewals of vault secret lease")
	MRenewFailuresView = utils.ViewForCounter(MRenewFailures, view.Sum())
)
