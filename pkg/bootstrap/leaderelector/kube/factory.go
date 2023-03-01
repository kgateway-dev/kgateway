package kube

import (
	"context"
	"os"
	"time"

	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector"
	"github.com/solo-io/go-utils/contextutils"
	"k8s.io/client-go/rest"
	k8sleaderelection "k8s.io/client-go/tools/leaderelection"
	"sigs.k8s.io/controller-runtime/pkg/leaderelection"
)

var _ leaderelector.ElectionFactory = new(kubeElectionFactory)

const (
	// Define the following values according to the defaults:
	// https://github.com/kubernetes/client-go/blob/master/tools/leaderelection/leaderelection.go
	defaultLeaseDuration = 15 * time.Second
	defaultRetryPeriod   = 2 * time.Second
	defaultRenewPeriod   = 10 * time.Second

	leaseDurationEnvName = "LEADER_ELECTION_LEASE_DURATION"
	retryPeriodEnvName   = "LEADER_ELECTION_RETRY_PERIOD"
	renewPeriodEnvName   = "LEADER_ELECTION_RENEW_PERIOD"
)

// kubeElectionFactory is the implementation for coordinating leader election using
// the k8s leader election tool: https://github.com/kubernetes/client-go/tree/master/tools/leaderelection
type kubeElectionFactory struct {
	restCfg *rest.Config
}

func NewElectionFactory(config *rest.Config) *kubeElectionFactory {
	return &kubeElectionFactory{
		restCfg: config,
	}
}

func (f *kubeElectionFactory) StartElection(ctx context.Context, config *leaderelector.ElectionConfig) (leaderelector.Identity, error) {
	elected := make(chan struct{})
	identity := leaderelector.NewIdentity(elected)

	leOpts := leaderelection.Options{
		LeaderElection:          true,
		LeaderElectionID:        config.Id,
		LeaderElectionNamespace: config.Namespace,
	}
	// Create the resource Lock interface necessary for leader election.
	// Controller runtime requires an event handler provider, but that package is
	// internal so for right now we pass a noop handler.
	resourceLock, err := leaderelection.NewResourceLock(f.restCfg, NewNoopProvider(), leOpts)
	if err != nil {
		return identity, err
	}

	l, err := k8sleaderelection.NewLeaderElector(
		k8sleaderelection.LeaderElectionConfig{
			Lock:          resourceLock,
			LeaseDuration: getLeaseDuration(),
			RenewDeadline: getRenewPeriod(),
			RetryPeriod:   getRetryPeriod(),
			Callbacks: k8sleaderelection.LeaderCallbacks{
				OnStartedLeading: func(callbackCtx context.Context) {
					contextutils.LoggerFrom(callbackCtx).Debug("Started Leading")
					close(elected)
					config.OnStartedLeading(callbackCtx)
				},
				OnStoppedLeading: func() {
					contextutils.LoggerFrom(ctx).Error("Stopped Leading")
					config.OnStoppedLeading()
				},
				OnNewLeader: func(identity string) {
					contextutils.LoggerFrom(ctx).Debugf("New Leader Elected with Identity: %s", identity)
					config.OnNewLeader(identity)
				},
			},
			Name:            config.Id,
			ReleaseOnCancel: true,
		},
	)
	if err != nil {
		return identity, err
	}

	// Start the leader elector process in a goroutine
	contextutils.LoggerFrom(ctx).Debug("Starting Kube Leader Election")
	go l.Run(ctx)

	return identity, nil
}

func getLeaseDuration() time.Duration {
	return getDurationFromEnvOrDefault(leaseDurationEnvName, defaultLeaseDuration)
}

func getRenewPeriod() time.Duration {
	return getDurationFromEnvOrDefault(renewPeriodEnvName, defaultRenewPeriod)
}

func getRetryPeriod() time.Duration {
	return getDurationFromEnvOrDefault(retryPeriodEnvName, defaultRetryPeriod)
}

func getDurationFromEnvOrDefault(envName string, defaultDuration time.Duration) time.Duration {
	duration := defaultDuration

	durationStr := os.Getenv(envName)
	if durationStr != "" {
		if dur, err := time.ParseDuration(durationStr); err == nil {
			duration = dur
		}
	}

	return duration
}
