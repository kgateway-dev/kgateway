package vault

import (
	"github.com/solo-io/gloo/pkg/utils/statsutils"
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
	MLastLoginSuccess     = statsutils.Int64Measure("gloo.solo.io/vault/last_login_success", "Timestamp of last successful authentication of vault")
	MLastLoginSuccessView = statsutils.ViewForCounter(MLastLoginSuccess, view.LastValue())

	MLastLoginFailure     = statsutils.Int64Measure("gloo.solo.io/vault/last_login_failure", "Timestamp of last failed authentication of vault")
	MLastLoginFailureView = statsutils.ViewForCounter(MLastLoginFailure, view.LastValue())

	MLoginSuccesses     = statsutils.Int64Measure("gloo.solo.io/vault/login_successes", "Number of successful authentications of vault")
	MLoginSuccessesView = statsutils.ViewForCounter(MLoginSuccesses, view.Sum())

	MLoginFailures     = statsutils.Int64Measure("gloo.solo.io/vault/login_failures", "Number of failed authentications of vault")
	MLoginFailuresView = statsutils.ViewForCounter(MLoginFailures, view.Sum())

	// Renew metrics
	MLastRenewSuccess     = statsutils.Int64Measure("gloo.solo.io/vault/last_renew_success", "Timestamp of last successful renewal of vault secret lease")
	MLastRenewSuccessView = statsutils.ViewForCounter(MLastRenewSuccess, view.LastValue())

	MLastRenewFailure     = statsutils.Int64Measure("gloo.solo.io/vault/last_renew_failure", "Timestamp of last failed renewal of vault secret lease")
	MLastRenewFailureView = statsutils.ViewForCounter(MLastRenewFailure, view.LastValue())

	MRenewSuccesses     = statsutils.Int64Measure("gloo.solo.io/vault/renew_successes", "Number of successful renewals of vault secret lease")
	MRenewSuccessesView = statsutils.ViewForCounter(MRenewSuccesses, view.Sum())

	MRenewFailures     = statsutils.Int64Measure("gloo.solo.io/vault/renew_failures", "Number of failed renewals of vault secret lease")
	MRenewFailuresView = statsutils.ViewForCounter(MRenewFailures, view.Sum())
)
