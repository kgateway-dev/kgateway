package kube

import (
	"context"
	"os"
	"sync/atomic"
	"time"

	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector"
	"github.com/solo-io/gloo/pkg/utils/envutils"
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

	defaultRecoveryTimeout = 60 * time.Second

	leaseDurationEnvName                    = "LEADER_ELECTION_LEASE_DURATION"
	retryPeriodEnvName                      = "LEADER_ELECTION_RETRY_PERIOD"
	renewPeriodEnvName                      = "LEADER_ELECTION_RENEW_PERIOD"
	MaxRecoveryDurationWithoutKubeAPIServer = "MAX_RECOVERY_DURATION_WITHOUT_KUBE_API_SERVER"
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
	var recoveryTimeoutIfKubeAPIServerIsUnreachable time.Duration
	var recoverIfKubeAPIServerIsUnreachable bool
	var err error
	if envutils.IsEnvDefined(MaxRecoveryDurationWithoutKubeAPIServer) {
		recoveryTimeoutIfKubeAPIServerIsUnreachable, err = time.ParseDuration(os.Getenv(MaxRecoveryDurationWithoutKubeAPIServer))
		if err != nil {
			contextutils.LoggerFrom(ctx).Errorf("%s is not a valid duration. Defaulting to 60s", MaxRecoveryDurationWithoutKubeAPIServer)
			recoveryTimeoutIfKubeAPIServerIsUnreachable = defaultRecoveryTimeout
		} else {
			contextutils.LoggerFrom(ctx).Infof("max recovery from kube apiserver unavailability set to %s", recoveryTimeoutIfKubeAPIServerIsUnreachable)
		}
		recoverIfKubeAPIServerIsUnreachable = true
	}

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

	var justFailed = false
	var dontDie func()

	// dieIfUnrecoverable causes gloo to exit after the recoveryTimeout (default 60s) if the context is not cancelled.
	// This function is called when this container is a leader but unable to renew the leader lease (caused by an unreachable kube api server).
	// The context is cancelled if it is able to participate in leader election again, irrespective if it becomes a leader or follower.
	dieIfUnrecoverable := func(ctx context.Context) {
		timer := time.NewTimer(recoveryTimeoutIfKubeAPIServerIsUnreachable)
		select {
		case <-timer.C:
			contextutils.LoggerFrom(ctx).Fatalf("unable to recover from failed leader election, quitting app")
		case <-ctx.Done():
			contextutils.LoggerFrom(ctx).Infof("recovered from lease renewal failure")
		}
	}

	newLeaderElector := func() (*k8sleaderelection.LeaderElector, error) {
		recoveryCtx, cancel := context.WithCancel(ctx)

		return k8sleaderelection.NewLeaderElector(
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
						if recoverIfKubeAPIServerIsUnreachable {
							// Recreate the elected channel and reset the identity to a follower
							// Ref: https://github.com/solo-io/gloo/issues/7346
							elected = make(chan struct{})
							identity.Reset(elected)
							// Die if we are unable to recover from this within the recoveryTimeout
							go dieIfUnrecoverable(recoveryCtx)
							// Set recover to cancel the context to be used the next time `OnNewLeader` is called
							dontDie = cancel
							justFailed = true
						}
					},
					OnNewLeader: func(identity string) {
						contextutils.LoggerFrom(ctx).Debugf("New Leader Elected with Identity: %s", identity)
						config.OnNewLeader(identity)
						// Recover since we were able to re-negotiate leader election
						// Do this only when we just failed and not when someone becomes a leader
						if recoverIfKubeAPIServerIsUnreachable && justFailed {
							dontDie()
							justFailed = false
						}
					},
				},
				Name:            config.Id,
				ReleaseOnCancel: true,
			},
		)
	}

	// The error returned is just validating the config passed. If it passes validation once, it will again
	_, err = newLeaderElector()
	if err != nil {
		return identity, err
	}

	// leaderElector.Run() is a blocking method but we need to return the identity of this container to sub-components so they can
	// perform their respective tasks, hence it runs within a go routine.
	// It runs within an infinite loop so that we can recover if this container is a leader but fails to renew the lease and renegotiate leader election if possible.
	// This can be caused when there is a failure to connect to the kube api server
	go func() {
		var counter atomic.Uint32

		for {
			l, _ := newLeaderElector()
			// Start the leader elector process
			contextutils.LoggerFrom(ctx).Debug("Starting Kube Leader Election")
			l.Run(ctx)

			if !recoverIfKubeAPIServerIsUnreachable {
				contextutils.LoggerFrom(ctx).Fatalf("lost leadership, quitting app")
			}

			contextutils.LoggerFrom(ctx).Errorf("Leader election cycle %v lost. Trying again", counter.Load())
			counter.Add(1)
			// Sleep for the lease duration so another container has a chance to become the leader rather than try to renew
			// in when the kube api server is unreachable by this container
			time.Sleep(getLeaseDuration())
		}
	}()
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
